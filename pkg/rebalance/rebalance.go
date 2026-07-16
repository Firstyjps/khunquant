// Package rebalance implements deterministic portfolio rebalancing:
// a pure evaluator computes allocation drift and a concrete trade proposal,
// so the agent orchestrates but never invents numbers (numeric-lockdown).
package rebalance

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Plan is a persisted rebalancing plan for one exchange account.
type Plan struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"` // "binance", "okx", ...
	Account  string `json:"account"`  // "" = default account
	Quote    string `json:"quote"`    // trade quote currency, e.g. "USDT"
	// Targets maps asset → target weight in percent. Must include the quote
	// asset if a stable buffer is wanted. Weights should sum to ~100.
	Targets map[string]float64 `json:"targets"`
	// TolerancePct is the per-asset drift threshold in percentage points;
	// no trade is proposed while every asset is within tolerance.
	TolerancePct float64 `json:"tolerance_pct"`
	// MinTradeUSD suppresses dust trades below this notional.
	MinTradeUSD float64 `json:"min_trade_usd"`
	// Mode: "alert" (propose + notify, default) or "auto" (execute + report).
	Mode          string `json:"mode"`
	Schedule      string `json:"schedule"` // cron expression
	Timezone      string `json:"timezone,omitempty"`
	Enabled       bool   `json:"enabled"`
	CronJobID     string `json:"cron_job_id,omitempty"`
	NotifyChannel string `json:"notify_channel,omitempty"`
	NotifyChatID  string `json:"notify_chat_id,omitempty"`
	CreatedAtMS   int64  `json:"created_at_ms"`
	UpdatedAtMS   int64  `json:"updated_at_ms"`
}

// ModeAlert and ModeAuto are the two plan execution modes.
const (
	ModeAlert = "alert"
	ModeAuto  = "auto"
)

// NormalizeMode maps user input to a valid mode ("" → alert).
func NormalizeMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ModeAlert:
		return ModeAlert, nil
	case ModeAuto:
		return ModeAuto, nil
	default:
		return "", fmt.Errorf("mode must be %q or %q", ModeAlert, ModeAuto)
	}
}

// ValidateTargets checks weights are positive and sum to ~100 (±1 for rounding).
func ValidateTargets(targets map[string]float64) error {
	if len(targets) < 2 {
		return fmt.Errorf("targets need at least 2 assets (got %d)", len(targets))
	}
	var sum float64
	for asset, w := range targets {
		if w <= 0 {
			return fmt.Errorf("target weight for %s must be > 0 (got %g)", asset, w)
		}
		sum += w
	}
	if math.Abs(sum-100) > 1 {
		return fmt.Errorf("target weights must sum to 100%% (got %.2f%%)", sum)
	}
	return nil
}

// AssetState is one asset's live balance and price (in quote terms).
type AssetState struct {
	Asset      string
	Amount     float64
	PriceQuote float64 // price of 1 unit in the plan's quote currency (quote asset itself = 1)
}

// Drift is the allocation gap for one target asset.
type Drift struct {
	Asset      string
	Amount     float64
	ValueQuote float64
	CurrentPct float64
	TargetPct  float64
	DriftPts   float64 // CurrentPct - TargetPct (positive = overweight)
	DeltaQuote float64 // notional to trade back to target (positive = sell that much, negative = buy)
}

// Trade is one proposed order (always ASSET/QUOTE spot pair).
type Trade struct {
	Asset         string
	Symbol        string
	Side          string // "buy" | "sell"
	AmountAsset   float64
	NotionalQuote float64
}

// Proposal is the deterministic rebalancing verdict.
type Proposal struct {
	TotalValueQuote float64
	Drifts          []Drift
	Trades          []Trade
	MaxDriftPts     float64
	NeedsRebalance  bool
	// Skipped lists sub-minimum trades that were suppressed (asset: notional).
	Skipped []string
}

// Evaluate computes drift and a trade proposal. Pure function — no I/O.
// Only assets present in targets participate; other holdings are ignored.
// The quote asset rebalances implicitly (it is the counterparty of every trade),
// so no explicit trade is emitted for it.
func Evaluate(states []AssetState, plan Plan) (Proposal, error) {
	if err := ValidateTargets(plan.Targets); err != nil {
		return Proposal{}, err
	}
	quote := strings.ToUpper(plan.Quote)
	if quote == "" {
		return Proposal{}, fmt.Errorf("plan quote currency is required")
	}
	if _, ok := plan.Targets[quote]; !ok {
		return Proposal{}, fmt.Errorf("targets must include the quote asset %s (its weight is the stable buffer)", quote)
	}

	byAsset := map[string]AssetState{}
	for _, s := range states {
		byAsset[strings.ToUpper(s.Asset)] = s
	}

	var total float64
	values := map[string]float64{}
	for asset := range plan.Targets {
		s, ok := byAsset[asset]
		if !ok {
			values[asset] = 0
			continue
		}
		price := s.PriceQuote
		if asset == quote {
			price = 1
		}
		if price <= 0 {
			return Proposal{}, fmt.Errorf("no price for %s in %s — cannot evaluate", asset, quote)
		}
		values[asset] = s.Amount * price
		total += values[asset]
	}
	if total <= 0 {
		return Proposal{}, fmt.Errorf("portfolio value is zero for the target assets")
	}

	prop := Proposal{TotalValueQuote: total}
	assets := make([]string, 0, len(plan.Targets))
	for a := range plan.Targets {
		assets = append(assets, a)
	}
	sort.Strings(assets)

	for _, asset := range assets {
		currentPct := values[asset] / total * 100
		targetPct := plan.Targets[asset]
		driftPts := currentPct - targetPct
		delta := (currentPct - targetPct) / 100 * total
		d := Drift{
			Asset:      asset,
			Amount:     byAsset[asset].Amount,
			ValueQuote: values[asset],
			CurrentPct: currentPct,
			TargetPct:  targetPct,
			DriftPts:   driftPts,
			DeltaQuote: delta,
		}
		prop.Drifts = append(prop.Drifts, d)
		if math.Abs(driftPts) > prop.MaxDriftPts {
			prop.MaxDriftPts = math.Abs(driftPts)
		}

		if asset == quote {
			continue // quote rebalances implicitly via the other legs
		}
		if math.Abs(driftPts) <= plan.TolerancePct {
			continue
		}
		notional := math.Abs(delta)
		if notional < plan.MinTradeUSD {
			prop.Skipped = append(prop.Skipped, fmt.Sprintf("%s (%.2f %s below min %.2f)", asset, notional, quote, plan.MinTradeUSD))
			continue
		}
		side := "sell"
		if delta < 0 {
			side = "buy"
		}
		price := byAsset[asset].PriceQuote
		prop.Trades = append(prop.Trades, Trade{
			Asset:         asset,
			Symbol:        asset + "/" + quote,
			Side:          side,
			AmountAsset:   notional / price,
			NotionalQuote: notional,
		})
	}

	// Sells first: free up quote balance before buys consume it.
	sort.SliceStable(prop.Trades, func(i, j int) bool {
		if prop.Trades[i].Side != prop.Trades[j].Side {
			return prop.Trades[i].Side == "sell"
		}
		return prop.Trades[i].NotionalQuote > prop.Trades[j].NotionalQuote
	})
	prop.NeedsRebalance = len(prop.Trades) > 0
	return prop, nil
}

// FormatProposal renders the proposal as a fixed-width report (used by both
// the check tool and the cron alert so numbers always come from Evaluate).
func FormatProposal(plan Plan, prop Proposal) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Rebalance check: %s (%s/%s, quote %s) — total %.2f %s\n\n",
		plan.Name, plan.Provider, accountOrDefault(plan.Account), plan.Quote, prop.TotalValueQuote, plan.Quote))
	sb.WriteString(fmt.Sprintf("%-8s %14s %10s %10s %10s %14s\n", "Asset", "Amount", "Now %", "Target %", "Drift", "Value"))
	sb.WriteString(strings.Repeat("-", 72) + "\n")
	for _, d := range prop.Drifts {
		sb.WriteString(fmt.Sprintf("%-8s %14.6f %9.2f%% %9.2f%% %+9.2f %14.2f\n",
			d.Asset, d.Amount, d.CurrentPct, d.TargetPct, d.DriftPts, d.ValueQuote))
	}
	sb.WriteString(fmt.Sprintf("\nMax drift: %.2f pts (tolerance %.2f pts)\n", prop.MaxDriftPts, plan.TolerancePct))
	if !prop.NeedsRebalance {
		sb.WriteString("✅ Within tolerance — no trades needed.\n")
	} else {
		sb.WriteString(fmt.Sprintf("\nProposed trades (%d, sells first):\n", len(prop.Trades)))
		for i, t := range prop.Trades {
			sb.WriteString(fmt.Sprintf("  %d. %-4s %.6f %s (≈ %.2f %s)\n",
				i+1, strings.ToUpper(t.Side), t.AmountAsset, t.Symbol, t.NotionalQuote, plan.Quote))
		}
	}
	if len(prop.Skipped) > 0 {
		sb.WriteString("\nSkipped (below min trade size): " + strings.Join(prop.Skipped, ", ") + "\n")
	}
	return sb.String()
}

func accountOrDefault(account string) string {
	if account == "" {
		return "default"
	}
	return account
}
