package tools

import (
	"context"
	"fmt"
	"time"

	ccxt "github.com/ccxt/ccxt/go/v4"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// GetDeltaNeutralSpreadTool fetches live entry/exit spread and 3d/7d/14d/30d funding APR
// for a given spot + futures symbol pair.
type GetDeltaNeutralSpreadTool struct {
	cfg *config.Config
}

// NewGetDeltaNeutralSpreadTool constructs a GetDeltaNeutralSpreadTool.
func NewGetDeltaNeutralSpreadTool(cfg *config.Config) *GetDeltaNeutralSpreadTool {
	return &GetDeltaNeutralSpreadTool{cfg: cfg}
}

func (t *GetDeltaNeutralSpreadTool) Name() string { return NameGetDeltaNeutralSpread }

func (t *GetDeltaNeutralSpreadTool) Description() string { return DescGetDeltaNeutralSpread }

func (t *GetDeltaNeutralSpreadTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"spot_provider": map[string]any{
				"type":        "string",
				"description": "Exchange provider for spot (e.g. 'binance').",
			},
			"spot_account": map[string]any{
				"type":        "string",
				"description": "Account name (empty = default).",
			},
			"spot_symbol": map[string]any{
				"type":        "string",
				"description": "CCXT spot symbol (e.g. 'VIRTUAL/USDT').",
			},
			"futures_provider": map[string]any{
				"type":        "string",
				"description": "Exchange provider for futures (e.g. 'binance').",
			},
			"futures_account": map[string]any{
				"type":        "string",
				"description": "Account name (empty = default).",
			},
			"futures_symbol": map[string]any{
				"type":        "string",
				"description": "CCXT perp symbol (e.g. 'VIRTUAL/USDT:USDT').",
			},
		},
		"required": []string{"spot_provider", "spot_symbol", "futures_provider", "futures_symbol"},
	}
}

func (t *GetDeltaNeutralSpreadTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	spotProviderID := stringArg(args, "spot_provider")
	spotAccount := stringArg(args, "spot_account")
	spotSymbol := stringArg(args, "spot_symbol")
	futuresProviderID := stringArg(args, "futures_provider")
	futuresAccount := stringArg(args, "futures_account")
	futuresSymbol := stringArg(args, "futures_symbol")

	if spotProviderID == "" || spotSymbol == "" || futuresProviderID == "" || futuresSymbol == "" {
		return ErrorResult("spot_provider, spot_symbol, futures_provider, and futures_symbol are required")
	}

	// Resolve spot provider.
	sp, err := broker.CreateProviderForAccount(spotProviderID, spotAccount, t.cfg)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create spot provider: %v", err))
	}
	md, ok := sp.(broker.MarketDataProvider)
	if !ok {
		return ErrorResult("spot provider does not support market data")
	}

	// Resolve futures provider.
	fp, err := futuresProvider(ctx, t.cfg, futuresProviderID, futuresAccount)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create futures provider: %v", err))
	}

	// Fetch spot price.
	spotTicker, err := md.FetchTicker(ctx, spotSymbol)
	if err != nil || spotTicker.Last == nil || *spotTicker.Last <= 0 {
		return ErrorResult(fmt.Sprintf("failed to fetch spot price for %s: %v", spotSymbol, err))
	}
	spotPrice := *spotTicker.Last

	// Fetch futures mark price.
	markPrice, err := fp.FetchFuturesMarkPrice(ctx, futuresSymbol)
	if err != nil || markPrice <= 0 {
		return ErrorResult(fmt.Sprintf("failed to fetch futures mark price for %s: %v", futuresSymbol, err))
	}

	// Fetch current funding rate.
	fundingRate, err := fp.FetchFuturesFundingRate(ctx, futuresSymbol)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to fetch funding rate for %s: %v", futuresSymbol, err))
	}

	// Compute spreads.
	entrySpread := deltaneutral.EntrySpreadPct(markPrice, spotPrice)
	exitSpread := deltaneutral.ExitSpreadPct(spotPrice, markPrice)

	fundingPeriodsPerDay := deltaneutral.FundingPeriodsPerDay(fundingRate.Interval)

	// Build basic output header.
	fundingRateVal := 0.0
	if fundingRate.FundingRate != nil {
		fundingRateVal = *fundingRate.FundingRate * 100
	}
	intervalStr := "8h"
	if fundingRate.Interval != nil {
		intervalStr = *fundingRate.Interval
	}

	out := fmt.Sprintf(
		"%s spread (%s) — %s\n\n"+
			"Spot price:    %g\n"+
			"Futures mark:  %g\n"+
			"Entry spread:  %+.4f%%   (futures - spot) / spot\n"+
			"Exit spread:   %+.4f%%   (spot - futures) / futures\n\n"+
			"Funding rate:  %.6f%% (per %s)\n",
		spotSymbol, futuresProviderID, time.Now().UTC().Format("2006-01-02 15:04:05Z"),
		spotPrice, markPrice,
		entrySpread, exitSpread,
		fundingRateVal, intervalStr,
	)

	// Fetch funding history for window APR. 800 records cover ≥30d even for 1h-funding
	// perps (24 periods/day → ~33d), and far more for 4h/8h cadences.
	history, histErr := fp.FetchPublicFundingRateHistory(ctx, futuresSymbol, nil, 800)
	if histErr != nil || len(history) == 0 {
		out += "Funding history unavailable — could not compute 3d/7d/14d/30d APR."
		return UserResult(out)
	}

	apr3d := computeWindowAPR(history, 3*24*time.Hour, fundingPeriodsPerDay)
	apr7d := computeWindowAPR(history, 7*24*time.Hour, fundingPeriodsPerDay)
	apr14d := computeWindowAPR(history, 14*24*time.Hour, fundingPeriodsPerDay)
	apr30d := computeWindowAPR(history, 30*24*time.Hour, fundingPeriodsPerDay)

	out += "Funding APR (annualised from history):\n"
	if apr3d != nil {
		out += fmt.Sprintf("  3d  APR:  %+.2f%%\n", *apr3d)
	} else {
		out += "  3d  APR:  (insufficient data)\n"
	}
	if apr7d != nil {
		out += fmt.Sprintf("  7d  APR:  %+.2f%%\n", *apr7d)
	} else {
		out += "  7d  APR:  (insufficient data)\n"
	}
	if apr14d != nil {
		out += fmt.Sprintf("  14d APR:  %+.2f%%\n", *apr14d)
	} else {
		out += "  14d APR:  (insufficient data)\n"
	}
	if apr30d != nil {
		out += fmt.Sprintf("  30d APR:  %+.2f%%\n", *apr30d)
	} else {
		out += "  30d APR:  (insufficient data)\n"
	}

	return UserResult(out)
}

// computeWindowAPR computes the mean funding rate over a time window and annualises it.
// Returns nil when no data points fall within the window.
func computeWindowAPR(history []ccxt.FundingRateHistory, window time.Duration, fundingPeriodsPerDay float64) *float64 {
	cutoff := time.Now().Add(-window)

	var sum float64
	var count int
	for _, pt := range history {
		if pt.Timestamp != nil && pt.FundingRate != nil {
			if time.UnixMilli(*pt.Timestamp).After(cutoff) {
				sum += *pt.FundingRate
				count++
			}
		}
	}

	if count == 0 {
		return nil
	}
	mean := sum / float64(count)
	apr := mean * fundingPeriodsPerDay * 365 * 100
	return &apr
}
