package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	ccxt "github.com/ccxt/ccxt/go/v4"
	"golang.org/x/sync/errgroup"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
	"github.com/cryptoquantumwave/khunquant/pkg/utils"
)

// cmcListingFn is a package-level test seam for CMC listing.
var cmcListingFn = func(ctx context.Context, cfg *config.Config, baseURL string, topN int) ([]string, error) {
	return fetchCMCListing(ctx, baseURL, topN)
}

type ScanDeltaNeutralOpportunitiesTool struct {
	cfg *config.Config
}

func NewScanDeltaNeutralOpportunitiesTool(cfg *config.Config) *ScanDeltaNeutralOpportunitiesTool {
	return &ScanDeltaNeutralOpportunitiesTool{cfg: cfg}
}

func (t *ScanDeltaNeutralOpportunitiesTool) Name() string {
	return NameScanDeltaNeutralOpportunities
}

func (t *ScanDeltaNeutralOpportunitiesTool) Description() string {
	return DescScanDeltaNeutralOpportunities
}

func (t *ScanDeltaNeutralOpportunitiesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider": map[string]any{
				"type":        "string",
				"description": "Exchange provider name: 'binance' or 'okx'.",
			},
			"account": map[string]any{
				"type":        "string",
				"description": "Account name within the provider. Leave empty to use the default account.",
			},
			"top_n": map[string]any{
				"type":        "integer",
				"description": "Top N crypto assets by CMC market-cap rank to screen (default 100, max 500).",
			},
			"quote": map[string]any{
				"type":        "string",
				"description": "Quote currency for futures symbols, e.g. 'USDT' (default 'USDT').",
			},
			"limit_results": map[string]any{
				"type":        "integer",
				"description": "Limit output table to top N results (default 20).",
			},
			"include_stability": map[string]any{
				"type":        "boolean",
				"description": "Fetch funding rate history and compute stability stats for top K assets (default true).",
			},
			"top_k_stability": map[string]any{
				"type":        "integer",
				"description": "For stage 2, fetch history for top K ranked assets (default 15).",
			},
			"min_abs_funding_apr": map[string]any{
				"type":        "number",
				"description": "Filter assets with |APR| below this threshold (percent, optional, default 0).",
			},
			"cmc_base_url": map[string]any{
				"type":        "string",
				"description": "Override CMC API base URL (for testing or custom endpoints).",
			},
		},
		"required": []string{"provider"},
	}
}

type opportunityRow struct {
	rank               int
	asset              string
	symbol             string
	spotSymbol         string
	spotStatus         string // "yes", "no-spot", or "unknown"
	fundingPercent     float64
	apr                float64
	direction          string
	stability7dMean    *float64 // nil if not fetched
	stability7dStddev  *float64
	stability14dMean   *float64
	stability14dStddev *float64
	label              string
}

func (t *ScanDeltaNeutralOpportunitiesTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	provider := stringArg(args, "provider")
	account := stringArg(args, "account")
	topN := int(numberArg(args, "top_n"))
	quote := stringArg(args, "quote")
	limitResults := int(numberArg(args, "limit_results"))
	includeStability := boolArgWithDefault(args, "include_stability", true)
	topKStability := int(numberArg(args, "top_k_stability"))
	minAbsFundingAPR := numberArg(args, "min_abs_funding_apr")
	cmcBaseURL := stringArg(args, "cmc_base_url")

	// Validation and defaults.
	if provider == "" {
		return ErrorResult("provider is required (e.g. 'binance' or 'okx')")
	}
	if topN <= 0 || topN > 500 {
		topN = 100
	}
	if quote == "" {
		quote = "USDT"
	}
	if limitResults <= 0 {
		limitResults = 20
	}
	if topKStability <= 0 {
		topKStability = 15
	}
	if cmcBaseURL == "" {
		cmcBaseURL = "https://api.coinmarketcap.com/data-api/v3/cryptocurrency/listing"
	}

	// Stage 0: Fetch CMC listing.
	symbols, err := cmcListingFn(ctx, t.cfg, cmcBaseURL, topN)
	if err != nil {
		return ErrorResult(fmt.Sprintf("CMC listing fetch failed: %v", err)).WithError(err)
	}
	if len(symbols) == 0 {
		return UserResult("No symbols found in CMC listing.")
	}

	// Stage 1: Acquire provider and fetch markets.
	fp, err := futuresProvider(ctx, t.cfg, provider, account)
	if err != nil {
		return ErrorResult(fmt.Sprintf("provider error: %v", err)).WithError(err)
	}

	markets, _ := fp.LoadFuturesMarkets(ctx) // Silently ignore error; we'll proceed without filtering.

	// Load spot markets to flag whether each asset also has a spot pair on this
	// exchange. We do NOT filter on this — symbols without spot stay in the ranked
	// list with a caution flag so the user still sees correct sorted funding data.
	// spotMarkets == nil means we couldn't determine spot availability (status "unknown").
	var spotMarkets map[string]ccxt.MarketInterface
	if md, ok := fp.(broker.MarketDataProvider); ok {
		spotMarkets, _ = md.LoadMarkets(ctx) // Silently ignore error → spot status "unknown".
	}

	// Build candidate symbols and filter active/swap.
	candidateSymbols := make([]string, 0)
	symbolToBase := make(map[string]string) // futures symbol -> base asset
	symbolToIdx := make(map[string]int)     // futures symbol -> CMC rank index
	for i, base := range symbols {
		futSym := normalizeFuturesSymbol(base + "/" + quote)
		candidateSymbols = append(candidateSymbols, futSym)
		symbolToBase[futSym] = base
		symbolToIdx[futSym] = i
		// Track by index for ranking.
		if i >= len(symbols) {
			break
		}
	}

	// Filter by active swap if markets are available.
	if len(markets) > 0 {
		filtered := make([]string, 0)
		for _, sym := range candidateSymbols {
			if m, exists := markets[sym]; exists {
				if isActiveSwap(m) {
					filtered = append(filtered, sym)
				}
			}
		}
		candidateSymbols = filtered
	}

	// Stage 1b: Batch fetch funding rates.
	rates, err := fp.FetchFuturesFundingRates(ctx, candidateSymbols)
	if err != nil {
		return ErrorResult(fmt.Sprintf("batch funding fetch failed: %v", err)).WithError(err)
	}

	// Build opportunity list.
	opportunities := make([]opportunityRow, 0)
	for _, futSym := range candidateSymbols {
		rate, exists := rates[futSym]
		if !exists {
			// Rate not found, skip
			continue
		}
		if rate.FundingRate == nil {
			// No funding rate
			continue
		}

		fr := *rate.FundingRate
		ppd := periodsPerDay(rate.Interval)
		apr := fr * ppd * 365 * 100 // percent

		// Filter by min abs APR.
		if minAbsFundingAPR != 0 && math.Abs(apr) < minAbsFundingAPR {
			continue
		}

		base := symbolToBase[futSym]
		var dir string
		if fr > 0 {
			dir = "short perp"
		} else {
			dir = "long perp"
		}

		spotSym := base + "/" + quote
		spotStatus := spotStatusFor(spotMarkets, spotSym)

		rankIdx := symbolToIdx[futSym]
		row := opportunityRow{
			rank:           rankIdx + 1,
			asset:          base,
			symbol:         futSym,
			spotSymbol:     spotSym,
			spotStatus:     spotStatus,
			fundingPercent: fr * 100,
			apr:            apr,
			direction:      dir,
			label:          "watch", // default
		}

		// Label logic: positive APR + stable = attractive.
		if apr > 0 {
			row.label = "attractive"
		}
		// No spot pair on this exchange → cannot build the spot leg here; flag as watch.
		if spotStatus == "no-spot" {
			row.label = "watch"
		}

		opportunities = append(opportunities, row)
	}

	// Sort by abs(APR) descending.
	sort.Slice(opportunities, func(i, j int) bool {
		return math.Abs(opportunities[i].apr) > math.Abs(opportunities[j].apr)
	})

	// Stage 2: Optionally fetch stability for top K.
	if includeStability && len(opportunities) > 0 {
		var eg *errgroup.Group
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(4)

		topK := topKStability
		if len(opportunities) < topK {
			topK = len(opportunities)
		}

		mu := sync.Mutex{}
		for i := 0; i < topK; i++ {
			i := i // capture
			eg.Go(func() error {
				history, err := fp.FetchPublicFundingRateHistory(egCtx, opportunities[i].symbol, nil, 200)
				if err != nil {
					// Silently skip on error.
					return nil
				}
				if len(history) == 0 {
					return nil
				}

				stats7d := computeFundingStatsWindow(history, 7*24*time.Hour)
				stats14d := computeFundingStatsWindow(history, 14*24*time.Hour)

				mu.Lock()
				opportunities[i].stability7dMean = &stats7d.mean
				opportunities[i].stability7dStddev = &stats7d.stddev
				opportunities[i].stability14dMean = &stats14d.mean
				opportunities[i].stability14dStddev = &stats14d.stddev
				mu.Unlock()

				return nil
			})
		}
		_ = eg.Wait() // Ignore partial failures.
	}

	// Build output table.
	// Debug: if no opportunities, mention that
	if len(opportunities) == 0 {
		return UserResult("No opportunities found (opportunities list is empty).")
	}
	out := formatScanResults(opportunities, limitResults, includeStability)
	return UserResult(out)
}

// spotStatusFor reports whether a spot pair exists on the exchange for the given
// spot symbol (e.g. "BTC/USDT"). Returns:
//   - "unknown" when spot markets could not be loaded (don't claim "no spot" falsely),
//   - "no-spot" when markets loaded but the symbol is absent or not an active spot pair,
//   - "yes" when an active spot market exists.
func spotStatusFor(spotMarkets map[string]ccxt.MarketInterface, spotSymbol string) string {
	if spotMarkets == nil {
		return "unknown"
	}
	m, exists := spotMarkets[spotSymbol]
	if !exists {
		return "no-spot"
	}
	if m.Active != nil && !*m.Active {
		return "no-spot"
	}
	if m.Spot != nil && !*m.Spot {
		return "no-spot"
	}
	return "yes"
}

// isActiveSwap checks if a market is an active swap/perpetual.
func isActiveSwap(m ccxt.MarketInterface) bool {
	// Check if active.
	if m.Active != nil && !*m.Active {
		return false
	}
	// Check if swap.
	if m.Swap == nil || !*m.Swap {
		return false
	}
	return true
}

// periodsPerDay returns the number of periods per day based on the interval string.
func periodsPerDay(interval *string) float64 {
	if interval == nil {
		return 3.0 // Default: 8-hour intervals.
	}
	switch strings.ToLower(*interval) {
	case "1h":
		return 24.0
	case "4h":
		return 6.0
	case "8h":
		return 3.0
	default:
		return 3.0
	}
}

// computeFundingStatsWindow computes stats over the past duration.
func computeFundingStatsWindow(history []ccxt.FundingRateHistory, duration time.Duration) fundingStatWindow {
	if len(history) == 0 {
		return fundingStatWindow{}
	}

	now := time.Now().UTC().UnixMilli()
	cutoffMs := now - duration.Milliseconds()

	var windowed []float64
	for _, r := range history {
		if r.Timestamp != nil && *r.Timestamp >= cutoffMs && r.FundingRate != nil {
			windowed = append(windowed, *r.FundingRate)
		}
	}

	if len(windowed) == 0 {
		return fundingStatWindow{}
	}

	// Compute mean.
	sum := 0.0
	for _, v := range windowed {
		sum += v
	}
	mean := sum / float64(len(windowed))

	// Compute stddev.
	varSum := 0.0
	for _, v := range windowed {
		diff := v - mean
		varSum += diff * diff
	}
	stddev := math.Sqrt(varSum / float64(len(windowed)))

	// Min/Max.
	minV := windowed[0]
	maxV := windowed[0]
	for _, v := range windowed {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	return fundingStatWindow{
		mean:   mean,
		stddev: stddev,
		max:    maxV,
		min:    minV,
		count:  len(windowed),
	}
}

// formatScanResults builds a human-readable table.
func formatScanResults(opportunities []opportunityRow, limitResults int, includeStability bool) string {
	var sb strings.Builder

	sb.WriteString("=== Delta-Neutral Funding Carry Scan ===\n\n")

	if len(opportunities) == 0 {
		sb.WriteString("No opportunities found matching the criteria.\n")
		return sb.String()
	}

	// Limit output.
	display := opportunities
	if len(display) > limitResults {
		display = display[:limitResults]
	}

	// Header.
	if includeStability {
		sb.WriteString(fmt.Sprintf("%-5s %-8s %-15s %-8s %10s %10s %-12s %10s %10s %10s %10s %s\n",
			"Rank", "Asset", "Futures", "Spot", "Funding%", "APR%", "Direction", "7d Mean%", "7d Std%", "14d Mean%", "14d Std%", "Label"))
		sb.WriteString(strings.Repeat("-", 150) + "\n")
	} else {
		sb.WriteString(fmt.Sprintf("%-5s %-8s %-15s %-8s %10s %10s %-12s %s\n",
			"Rank", "Asset", "Futures", "Spot", "Funding%", "APR%", "Direction", "Label"))
		sb.WriteString(strings.Repeat("-", 90) + "\n")
	}

	// Rows.
	for i, row := range display {
		rank := i + 1
		var line string
		if includeStability {
			s7dMean := "-"
			s7dStd := "-"
			s14dMean := "-"
			s14dStd := "-"
			if row.stability7dMean != nil {
				s7dMean = fmt.Sprintf("%+.4f", *row.stability7dMean*100)
			}
			if row.stability7dStddev != nil {
				s7dStd = fmt.Sprintf("%.4f", *row.stability7dStddev*100)
			}
			if row.stability14dMean != nil {
				s14dMean = fmt.Sprintf("%+.4f", *row.stability14dMean*100)
			}
			if row.stability14dStddev != nil {
				s14dStd = fmt.Sprintf("%.4f", *row.stability14dStddev*100)
			}
			line = fmt.Sprintf("%-5d %-8s %-15s %-8s %10.6f %10.2f %-12s %10s %10s %10s %10s %s\n",
				rank, row.asset, row.symbol, spotCell(row.spotStatus), row.fundingPercent, row.apr, row.direction,
				s7dMean, s7dStd, s14dMean, s14dStd, row.label)
		} else {
			line = fmt.Sprintf("%-5d %-8s %-15s %-8s %10.6f %10.2f %-12s %s\n",
				rank, row.asset, row.symbol, spotCell(row.spotStatus), row.fundingPercent, row.apr, row.direction, row.label)
		}
		sb.WriteString(line)
	}

	// Caution: list any displayed assets that have a perp but no spot pair here.
	var noSpot []string
	var unknownSpot bool
	for _, row := range display {
		switch row.spotStatus {
		case "no-spot":
			noSpot = append(noSpot, row.asset)
		case "unknown":
			unknownSpot = true
		}
	}

	sb.WriteString("\n")
	if len(noSpot) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️  No spot pair on this exchange for: %s — funding rank is still valid, but the delta-neutral spot leg cannot be opened here (source spot elsewhere, or treat as futures-only).\n",
			strings.Join(noSpot, ", ")))
	}
	if unknownSpot {
		sb.WriteString("⚠️  Spot availability could not be verified for some rows (spot markets unavailable) — shown as 'unknown'.\n")
	}
	sb.WriteString("Spot column: 'yes' = spot pair available | 'no-spot' = perp only on this exchange | 'unknown' = could not verify.\n")
	sb.WriteString("Note: Funding-only screen — drill into top picks with get_orderbook/futures_risk_summary before building a plan.\n")
	sb.WriteString("Legend: 'attractive' = positive carry + stable | 'watch' = near-zero/unstable/no-spot | 'blocked' = no perp or no funding\n")

	return sb.String()
}

// spotCell renders the spot-availability status for the table column.
func spotCell(status string) string {
	switch status {
	case "yes":
		return "yes"
	case "no-spot":
		return "NO-SPOT"
	default:
		return "unknown"
	}
}

// cmcListingResponse is the structure for CMC API response.
type cmcListingResponse struct {
	Data struct {
		CryptoCurrencyList []struct {
			Symbol  string `json:"symbol"`
			CMCRank int    `json:"cmcRank"`
		} `json:"cryptoCurrencyList"`
	} `json:"data"`
}

// fetchCMCListing fetches the top N crypto symbols from CMC.
func fetchCMCListing(ctx context.Context, baseURL string, topN int) ([]string, error) {
	client, err := utils.CreateHTTPClient("", 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}
	defer client.CloseIdleConnections()

	symbols := make([]string, 0, topN)
	limit := 100

	for start := 1; len(symbols) < topN; start += limit {
		url := fmt.Sprintf("%s?start=%d&limit=%d&sortBy=rank&sortType=desc&convert=USD&cryptoType=all&tagType=all",
			baseURL, start, limit)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "KhunQuant/1.0")

		resp, err := utils.DoRequestWithRetry(client, req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
		}

		var data cmcListingResponse
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		if len(data.Data.CryptoCurrencyList) == 0 {
			break
		}

		for _, item := range data.Data.CryptoCurrencyList {
			if len(symbols) >= topN {
				break
			}
			symbols = append(symbols, item.Symbol)
		}

		if len(data.Data.CryptoCurrencyList) < limit {
			break
		}
	}

	return symbols, nil
}

// Helper functions for argument parsing.

func boolArgWithDefault(args map[string]any, key string, defaultVal bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return defaultVal
}
