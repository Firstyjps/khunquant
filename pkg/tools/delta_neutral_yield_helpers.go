package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// earnWindowMeanPct delegates to deltaneutral.EarnWindowMeanPct (the canonical
// trailing-window earn average shared with the gateway monitor).
func earnWindowMeanPct(points []broker.EarnRatePoint, window time.Duration, now time.Time) (float64, int) {
	return deltaneutral.EarnWindowMeanPct(points, window, now)
}

// yieldDigest fetches all yield series points (time-ASC) for a plan and returns
// a deduplicated [first, middle, latest] slice for a compact trend digest.
// Returns nil on error or when no data exists.
func yieldDigest(ctx context.Context, store *deltaneutral.Store, planID int64) []deltaneutral.SnapshotSeriesPoint {
	pts, err := store.ListSnapshotSeries(ctx, planID, time.Time{}, 0)
	if err != nil || len(pts) == 0 {
		return nil
	}
	n := len(pts)
	seen := make(map[int]bool)
	var result []deltaneutral.SnapshotSeriesPoint
	for _, idx := range []int{0, n / 2, n - 1} {
		if !seen[idx] {
			seen[idx] = true
			result = append(result, pts[idx])
		}
	}
	return result
}

// formatYieldDigest renders a compact multi-line text block showing first/mid/latest
// yield records. Returns an empty string when points is nil/empty.
func formatYieldDigest(points []deltaneutral.SnapshotSeriesPoint) string {
	if len(points) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Yield History (first / mid / latest):\n")
	labels := []string{"first", "mid", "latest"}
	for i, p := range points {
		lbl := "latest"
		if i < len(labels) {
			lbl = labels[i]
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s | funding %.6f | fundAPY %.4f%% | earn %.4f%% | combined %.4f%%\n",
			lbl,
			p.CheckedAt.Format("2006-01-02 15:04 UTC"),
			p.CurrentFundingRate,
			p.FundingAPYPct,
			p.EarnAPYPct,
			p.CombinedAPYPct,
		))
	}
	return sb.String()
}

// CostEstimates holds live-computed cost and yield estimates for a DN plan.
type CostEstimates struct {
	EntryCostUSDT         float64
	ExitCostUSDT          float64
	DailyFundingUSDT      float64 // funding leg only
	DailyEarnUSDT         float64 // earn leg only
	DailyCombinedUSDT     float64 // funding + earn
	BreakevenFundingDays  float64 // breakeven using funding only
	BreakevenCombinedDays float64 // breakeven using funding + earn
}

// estimateCosts computes entry/exit fees and daily funding from the plan
// and the latest snapshot. Uses OKX standard taker fee rates (spot 0.10%,
// futures 0.05%) and the snapshot's current funding rate with a default
// 8-hour (3×/day) funding interval.
func estimateCosts(plan deltaneutral.Plan, snapshot *deltaneutral.MonitorSnapshot) CostEstimates {
	const spotTakerFee = 0.0010    // 0.10% — OKX spot market order
	const futuresTakerFee = 0.0005 // 0.05% — OKX swap market order

	entry := plan.SpotNotionalUSDT*spotTakerFee + plan.FuturesNotionalUSDT*futuresTakerFee
	exit := entry // closing both legs at market is symmetric
	roundTrip := entry + exit

	var dailyFunding, dailyEarn float64
	if snapshot != nil {
		if snapshot.FundingAPYPct != 0 && snapshot.FuturesNotionalUSDT > 0 {
			// Use the stored annualised APY (computed with the actual interval at
			// snapshot time) to get the correct daily yield regardless of whether
			// the perp settles every 1h, 4h, or 8h.
			dailyFunding = (snapshot.FundingAPYPct / 100.0 / 365.0) * snapshot.FuturesNotionalUSDT
		}
		if snapshot.EarnAPYPct > 0 && snapshot.SpotValueUSDT > 0 {
			dailyEarn = (snapshot.EarnAPYPct / 100.0 / 365.0) * snapshot.SpotValueUSDT
		}
	}

	dailyCombined := dailyFunding + dailyEarn

	var breakevenFunding, breakevenCombined float64
	if dailyFunding > 0 {
		breakevenFunding = roundTrip / dailyFunding
	}
	if dailyCombined > 0 {
		breakevenCombined = roundTrip / dailyCombined
	}

	return CostEstimates{
		EntryCostUSDT:         entry,
		ExitCostUSDT:          exit,
		DailyFundingUSDT:      dailyFunding,
		DailyEarnUSDT:         dailyEarn,
		DailyCombinedUSDT:     dailyCombined,
		BreakevenFundingDays:  breakevenFunding,
		BreakevenCombinedDays: breakevenCombined,
	}
}

// LiveProjection holds projected yield metrics fetched from live market data
// for a plan that has not yet been opened (no monitor snapshots exist).
type LiveProjection struct {
	FundingAPYPct         float64
	EarnAPYPct            float64
	CombinedAPYPct        float64
	DailyFundingUSDT      float64
	DailyEarnUSDT         float64
	DailyCombinedUSDT     float64
	AnnualFundingUSDT     float64
	AnnualEarnUSDT        float64
	AnnualCombinedUSDT    float64
	BreakevenFundingDays  float64
	BreakevenCombinedDays float64
	FundingRateRaw        float64 // per-period rate for display
	FundingInterval       string  // e.g. "8h"
	NotionalUSDT          float64 // actual notional used for yield math
	// Historical funding averages over 3M/6M/12M windows (zero when history fetch fails)
	Funding90dAPYPct   float64
	Funding90dRateRaw  float64
	Funding90dCount    int
	Funding180dAPYPct  float64
	Funding180dRateRaw float64
	Funding180dCount   int
	Funding365dAPYPct  float64
	Funding365dRateRaw float64
	Funding365dCount   int
	// Historical earn (flexible-savings) averages over 3M/6M/12M windows, as percent.
	// Each value falls back to the current EarnAPYPct when the window has no rate-history
	// points, so combined math is always valid. The matching *Count is 0 when the value
	// is the current-rate fallback (no real history), > 0 when it is a true avg.
	Earn90dAPYPct  float64
	Earn90dCount   int
	Earn180dAPYPct float64
	Earn180dCount  int
	Earn365dAPYPct float64
	Earn365dCount  int
	Warnings       []string
}

// FetchLiveProjection fetches the current funding rate and earn APY for a plan's
// symbols and computes projected annual yield and breakeven using the plan notionals.
// Returns a non-nil projection with partial data even when some fetches fail
// (failures are recorded in Warnings).
func FetchLiveProjection(ctx context.Context, cfg *config.Config, plan deltaneutral.Plan) *LiveProjection {
	proj := &LiveProjection{}

	// --- Funding rate (current + 7d/14d history) ---
	fp, err := futuresProvider(ctx, cfg, plan.FuturesProvider, plan.FuturesAccount)
	if err != nil {
		proj.Warnings = append(proj.Warnings, fmt.Sprintf("futures provider: %v", err))
	} else {
		fr, err := fp.FetchFuturesFundingRate(ctx, plan.FuturesSymbol)
		if err != nil {
			proj.Warnings = append(proj.Warnings, fmt.Sprintf("funding rate: %v", err))
		} else if fr.FundingRate != nil {
			proj.FundingRateRaw = *fr.FundingRate
			if fr.Interval != nil {
				proj.FundingInterval = *fr.Interval
			} else {
				proj.FundingInterval = "8h"
			}
			proj.FundingAPYPct = deltaneutral.AnnualizeFundingRatePct(proj.FundingRateRaw, fr.Interval)
		}

		// Fetch historical funding rates for 3M/6M/12M context.
		// limit>100 enables ccxt pagination; ~1100 records cover 365d at 8h cadence.
		history, hErr := fp.FetchPublicFundingRateHistory(ctx, plan.FuturesSymbol, nil, 1100)
		if hErr != nil {
			proj.Warnings = append(proj.Warnings, fmt.Sprintf("funding history: %v", hErr))
		} else {
			w90 := computeFundingStatsWindow(history, 90*24*time.Hour)
			w180 := computeFundingStatsWindow(history, 180*24*time.Hour)
			w365 := computeFundingStatsWindow(history, 365*24*time.Hour)
			if w90.count > 0 {
				proj.Funding90dRateRaw = w90.mean
				proj.Funding90dAPYPct = deltaneutral.AnnualizeFundingRatePct(w90.mean, &proj.FundingInterval)
				proj.Funding90dCount = w90.count
			}
			if w180.count > 0 {
				proj.Funding180dRateRaw = w180.mean
				proj.Funding180dAPYPct = deltaneutral.AnnualizeFundingRatePct(w180.mean, &proj.FundingInterval)
				proj.Funding180dCount = w180.count
			}
			if w365.count > 0 {
				proj.Funding365dRateRaw = w365.mean
				proj.Funding365dAPYPct = deltaneutral.AnnualizeFundingRatePct(w365.mean, &proj.FundingInterval)
				proj.Funding365dCount = w365.count
			}
		}
	}

	// --- Earn APY ---
	baseCur := plan.SpotSymbol
	if idx := strings.Index(plan.SpotSymbol, "/"); idx > 0 {
		baseCur = strings.ToUpper(plan.SpotSymbol[:idx])
	}
	var bestProductID, bestProductType string
	ep, err := earnProvider(ctx, cfg, plan.SpotProvider, plan.SpotAccount)
	if err != nil {
		proj.Warnings = append(proj.Warnings, fmt.Sprintf("earn provider: %v", err))
	} else {
		products, err := ep.FetchFlexibleEarnProducts(ctx, baseCur)
		if err != nil {
			proj.Warnings = append(proj.Warnings, fmt.Sprintf("earn products: %v", err))
		} else {
			for _, p := range products {
				if strings.ToUpper(p.Asset) == baseCur {
					apy := p.APY * 100
					if apy > proj.EarnAPYPct {
						proj.EarnAPYPct = apy
						bestProductID = p.ProductID
						bestProductType = p.Type
					}
				}
			}
		}
	}

	// --- Historical earn (flexible-savings) averages ---
	// Default every window to the current earn rate so combined math is always
	// valid; override with a true trailing average when rate history is available.
	proj.Earn90dAPYPct = proj.EarnAPYPct
	proj.Earn180dAPYPct = proj.EarnAPYPct
	proj.Earn365dAPYPct = proj.EarnAPYPct
	// staking-defi products have a flat APY with no public rate-history endpoint;
	// the current rate already represents every window, so skip the history fetch.
	if ep != nil && proj.EarnAPYPct > 0 && bestProductType != "staking-defi" {
		now := time.Now()
		// since=now-364d with a generous limit triggers adapter pagination + caching.
		earnSince := now.Add(-364 * 24 * time.Hour).UnixMilli()
		points, hErr := ep.FetchFlexibleEarnRateHistory(ctx, bestProductID, baseCur, &earnSince, 9000)
		if hErr != nil {
			proj.Warnings = append(proj.Warnings, fmt.Sprintf("earn history: %v", hErr))
		} else {
			if m, n := earnWindowMeanPct(points, 90*24*time.Hour, now); n > 0 {
				proj.Earn90dAPYPct, proj.Earn90dCount = m, n
			}
			if m, n := earnWindowMeanPct(points, 180*24*time.Hour, now); n > 0 {
				proj.Earn180dAPYPct, proj.Earn180dCount = m, n
			}
			if m, n := earnWindowMeanPct(points, 365*24*time.Hour, now); n > 0 {
				proj.Earn365dAPYPct, proj.Earn365dCount = m, n
			}
		}
	}

	proj.CombinedAPYPct = proj.FundingAPYPct + proj.EarnAPYPct

	// --- Daily and annual projections ---
	notional := plan.SpotNotionalUSDT
	if notional <= 0 {
		// No notional set yet (draft plan or dry-run): estimate as half of capital.
		// For a 1x delta-neutral position: ~50% spot, ~50% futures margin.
		notional = plan.CapitalUSDT * 0.5
	}
	proj.NotionalUSDT = notional
	proj.DailyFundingUSDT = (proj.FundingAPYPct / 100 / 365) * notional
	proj.DailyEarnUSDT = (proj.EarnAPYPct / 100 / 365) * notional
	proj.DailyCombinedUSDT = proj.DailyFundingUSDT + proj.DailyEarnUSDT
	proj.AnnualFundingUSDT = (proj.FundingAPYPct / 100) * notional
	proj.AnnualEarnUSDT = (proj.EarnAPYPct / 100) * notional
	proj.AnnualCombinedUSDT = (proj.CombinedAPYPct / 100) * notional

	// --- Breakeven ---
	const spotFee, futFee = 0.001, 0.0005
	futNotional := plan.FuturesNotionalUSDT
	if futNotional <= 0 {
		futNotional = notional
	}
	roundTrip := 2 * (notional*spotFee + futNotional*futFee)
	if proj.DailyFundingUSDT > 0 {
		proj.BreakevenFundingDays = roundTrip / proj.DailyFundingUSDT
	}
	if proj.DailyCombinedUSDT > 0 {
		proj.BreakevenCombinedDays = roundTrip / proj.DailyCombinedUSDT
	}

	return proj
}

// FormatLiveProjection renders the projection as a text block for tool responses.
func FormatLiveProjection(p *LiveProjection) string {
	var sb strings.Builder
	sb.WriteString("⚡ PROJECTED YIELD (live market rates)\n\n")

	// Funding APY — current + historical averages
	sb.WriteString(fmt.Sprintf("  Funding APY (current): %+.4f%%  (rate %.6f, interval %s)\n",
		p.FundingAPYPct, p.FundingRateRaw, p.FundingInterval))
	if p.Funding90dCount > 0 {
		sb.WriteString(fmt.Sprintf("  Funding APY (3M avg):  %+.4f%%  (avg rate %.6f, %d periods)\n",
			p.Funding90dAPYPct, p.Funding90dRateRaw, p.Funding90dCount))
	}
	if p.Funding180dCount > 0 {
		sb.WriteString(fmt.Sprintf("  Funding APY (6M avg):  %+.4f%%  (avg rate %.6f, %d periods)\n",
			p.Funding180dAPYPct, p.Funding180dRateRaw, p.Funding180dCount))
	}
	if p.Funding365dCount > 0 {
		sb.WriteString(fmt.Sprintf("  Funding APY (12M avg): %+.4f%%  (avg rate %.6f, %d periods)\n",
			p.Funding365dAPYPct, p.Funding365dRateRaw, p.Funding365dCount))
	}
	// Earn APY — current + historical averages (windowed only when real history exists)
	sb.WriteString(fmt.Sprintf("  Earn APY (current):    %+.4f%%\n", p.EarnAPYPct))
	if p.Earn90dCount > 0 {
		sb.WriteString(fmt.Sprintf("  Earn APY (3M avg):     %+.4f%%  (%d points)\n", p.Earn90dAPYPct, p.Earn90dCount))
	}
	if p.Earn180dCount > 0 {
		sb.WriteString(fmt.Sprintf("  Earn APY (6M avg):     %+.4f%%  (%d points)\n", p.Earn180dAPYPct, p.Earn180dCount))
	}
	if p.Earn365dCount > 0 {
		sb.WriteString(fmt.Sprintf("  Earn APY (12M avg):    %+.4f%%  (%d points)\n", p.Earn365dAPYPct, p.Earn365dCount))
	}

	// Combined APY lines — each window pairs the matched funding + earn average
	sb.WriteString(fmt.Sprintf("  Combined APY (current): %+.4f%%\n", p.CombinedAPYPct))
	if p.Funding90dCount > 0 {
		sb.WriteString(fmt.Sprintf("  Combined APY (3M avg):  %+.4f%%\n", p.Funding90dAPYPct+p.Earn90dAPYPct))
	}
	if p.Funding180dCount > 0 {
		sb.WriteString(fmt.Sprintf("  Combined APY (6M avg):  %+.4f%%\n", p.Funding180dAPYPct+p.Earn180dAPYPct))
	}
	if p.Funding365dCount > 0 {
		sb.WriteString(fmt.Sprintf("  Combined APY (12M avg): %+.4f%%\n", p.Funding365dAPYPct+p.Earn365dAPYPct))
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("  Daily funding (current): %+.4f USDT\n", p.DailyFundingUSDT))
	sb.WriteString(fmt.Sprintf("  Daily earn:              %+.4f USDT\n", p.DailyEarnUSDT))
	sb.WriteString(fmt.Sprintf("  Daily combined (current):%+.4f USDT\n\n", p.DailyCombinedUSDT))

	sb.WriteString(fmt.Sprintf("  Annual funding (current): %+.2f USDT  (%.4f%% × %.2f USDT spot notional)\n",
		p.AnnualFundingUSDT, p.FundingAPYPct, p.NotionalUSDT))
	if p.Funding90dCount > 0 {
		annual90d := (p.Funding90dAPYPct / 100) * p.NotionalUSDT
		sb.WriteString(fmt.Sprintf("  Annual funding (3M avg):  %+.2f USDT\n", annual90d))
	}
	if p.Funding180dCount > 0 {
		annual180d := (p.Funding180dAPYPct / 100) * p.NotionalUSDT
		sb.WriteString(fmt.Sprintf("  Annual funding (6M avg):  %+.2f USDT\n", annual180d))
	}
	if p.Funding365dCount > 0 {
		annual365d := (p.Funding365dAPYPct / 100) * p.NotionalUSDT
		sb.WriteString(fmt.Sprintf("  Annual funding (12M avg): %+.2f USDT\n", annual365d))
	}
	sb.WriteString(fmt.Sprintf("  Annual earn:              %+.2f USDT\n", p.AnnualEarnUSDT))
	sb.WriteString(fmt.Sprintf("  Annual combined (current):%+.2f USDT\n", p.AnnualCombinedUSDT))
	if p.Funding90dCount > 0 {
		comb90d := ((p.Funding90dAPYPct + p.Earn90dAPYPct) / 100) * p.NotionalUSDT
		sb.WriteString(fmt.Sprintf("  Annual combined (3M avg): %+.2f USDT\n", comb90d))
	}
	if p.Funding180dCount > 0 {
		comb180d := ((p.Funding180dAPYPct + p.Earn180dAPYPct) / 100) * p.NotionalUSDT
		sb.WriteString(fmt.Sprintf("  Annual combined (6M avg): %+.2f USDT\n", comb180d))
	}
	if p.Funding365dCount > 0 {
		comb365d := ((p.Funding365dAPYPct + p.Earn365dAPYPct) / 100) * p.NotionalUSDT
		sb.WriteString(fmt.Sprintf("  Annual combined (12M avg):%+.2f USDT\n", comb365d))
	}
	sb.WriteString("\n")

	// Breakeven at current rate
	if p.BreakevenFundingDays > 0 {
		sb.WriteString(fmt.Sprintf("  Breakeven (current, funding only):   %.2f days\n", p.BreakevenFundingDays))
	}
	if p.BreakevenCombinedDays > 0 {
		sb.WriteString(fmt.Sprintf("  Breakeven (current, funding + earn): %.2f days\n", p.BreakevenCombinedDays))
	}
	// Breakeven at 3M/6M average rates (more stable estimate)
	const spotFeeDisp, futFeeDisp = 0.001, 0.0005
	roundTripDisp := 2 * (p.NotionalUSDT*spotFeeDisp + p.NotionalUSDT*futFeeDisp)
	if p.Funding90dCount > 0 {
		daily90d := (p.Funding90dAPYPct / 100 / 365) * p.NotionalUSDT
		dailyComb90d := daily90d + (p.Earn90dAPYPct/100/365)*p.NotionalUSDT
		if dailyComb90d > 0 {
			sb.WriteString(fmt.Sprintf("  Breakeven (3M avg, funding + earn):  %.2f days\n", roundTripDisp/dailyComb90d))
		}
	}
	if p.Funding180dCount > 0 {
		daily180d := (p.Funding180dAPYPct / 100 / 365) * p.NotionalUSDT
		dailyComb180d := daily180d + (p.Earn180dAPYPct/100/365)*p.NotionalUSDT
		if dailyComb180d > 0 {
			sb.WriteString(fmt.Sprintf("  Breakeven (6M avg, funding + earn):  %.2f days\n", roundTripDisp/dailyComb180d))
		}
	}
	if p.Funding365dCount > 0 {
		daily365d := (p.Funding365dAPYPct / 100 / 365) * p.NotionalUSDT
		dailyComb365d := daily365d + (p.Earn365dAPYPct/100/365)*p.NotionalUSDT
		if dailyComb365d > 0 {
			sb.WriteString(fmt.Sprintf("  Breakeven (12M avg, funding + earn): %.2f days\n", roundTripDisp/dailyComb365d))
		}
	}

	if len(p.Warnings) > 0 {
		sb.WriteString("\n  ⚠ Partial data (some fetches failed):\n")
		for _, w := range p.Warnings {
			sb.WriteString(fmt.Sprintf("    - %s\n", w))
		}
	}
	return sb.String()
}

// parsePeriodSince maps a period string (7d/14d/30d/3m/6m/all) to a since time.
// Defaults to 7d when the input is empty or unrecognised.
func parsePeriodSince(period string) (time.Time, string) {
	now := time.Now().UTC()
	switch period {
	case "14d":
		return now.AddDate(0, 0, -14), "14d"
	case "30d":
		return now.AddDate(0, 0, -30), "30d"
	case "3m":
		return now.AddDate(0, -3, 0), "3m"
	case "6m":
		return now.AddDate(0, -6, 0), "6m"
	case "all":
		return time.Time{}, "all"
	default:
		return now.AddDate(0, 0, -7), "7d"
	}
}
