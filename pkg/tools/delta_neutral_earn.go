package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// GetDeltaNeutralEarnTool reports the flexible-savings (earn) APY for a spot asset:
// the current best rate plus trailing 3M/6M/12M averages from rate history.
type GetDeltaNeutralEarnTool struct{ cfg *config.Config }

func NewGetDeltaNeutralEarnTool(cfg *config.Config) *GetDeltaNeutralEarnTool {
	return &GetDeltaNeutralEarnTool{cfg: cfg}
}

func (t *GetDeltaNeutralEarnTool) Name() string { return NameGetDeltaNeutralEarn }

func (t *GetDeltaNeutralEarnTool) Description() string { return DescGetDeltaNeutralEarn }

func (t *GetDeltaNeutralEarnTool) Parameters() map[string]any {
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
			"asset": map[string]any{
				"type":        "string",
				"description": "Base asset / currency, e.g. 'ZEC'. A spot symbol like 'ZEC/USDT' is also accepted (the base is used).",
			},
		},
		"required": []string{"provider", "asset"},
	}
}

func (t *GetDeltaNeutralEarnTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	providerID := stringArg(args, "provider")
	asset := strings.ToUpper(stringArg(args, "asset"))
	if idx := strings.Index(asset, "/"); idx > 0 {
		asset = asset[:idx]
	}
	account := stringArg(args, "account")

	if providerID == "" || asset == "" {
		return ErrorResult("provider and asset are required")
	}

	ep, err := earnProvider(ctx, t.cfg, providerID, account)
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}

	// Current best APY + product id/type for the asset.
	var currentAPY float64
	var productID, productType string
	products, err := ep.FetchFlexibleEarnProducts(ctx, asset)
	if err != nil {
		return ErrorResult(fmt.Sprintf("get_delta_neutral_earn: products: %v", err)).WithError(err)
	}
	for _, p := range products {
		if strings.ToUpper(p.Asset) == asset {
			apy := p.APY * 100
			if apy > currentAPY {
				currentAPY = apy
				productID = p.ProductID
				productType = p.Type
			}
		}
	}
	if currentAPY == 0 && productID == "" {
		return UserResult(fmt.Sprintf("No flexible-earn product found for %s on %s.", asset, providerID))
	}

	var history []broker.EarnRatePoint
	if productType != "staking-defi" {
		// since=now-364d with a generous limit triggers adapter pagination + caching,
		// so the 3M/6M/12M windows are backed by real history.
		earnSince := time.Now().Add(-364 * 24 * time.Hour).UnixMilli()
		history, err = ep.FetchFlexibleEarnRateHistory(ctx, productID, asset, &earnSince, 9000)
		if err != nil {
			return ErrorResult(fmt.Sprintf("get_delta_neutral_earn: rate history: %v", err)).WithError(err)
		}
	}

	return UserResult(formatEarnStats(providerID, asset, currentAPY, productType, history))
}

// formatEarnStats renders current + trailing-window earn APY stats as text.
func formatEarnStats(providerID, asset string, currentAPY float64, productType string, history []broker.EarnRatePoint) string {
	now := time.Now()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Flexible Earn APY: %s on %s\n\n", asset, providerID))
	sb.WriteString(fmt.Sprintf("  Current (best): %+.4f%%\n", currentAPY))

	if productType == "staking-defi" {
		sb.WriteString("  (staking-defi product — flat APY, no rate history)\n")
		return sb.String()
	}
	if len(history) == 0 {
		sb.WriteString("  (no rate history available — current rate only)\n")
		return sb.String()
	}

	windows := []struct {
		label string
		dur   time.Duration
	}{
		{"3M avg", 90 * 24 * time.Hour},
		{"6M avg", 180 * 24 * time.Hour},
		{"12M avg", 365 * 24 * time.Hour},
	}
	for _, w := range windows {
		mean, n := deltaneutral.EarnWindowMeanPct(history, w.dur, now)
		if n > 0 {
			lo, hi := earnMinMaxPct(history, w.dur, now)
			sb.WriteString(fmt.Sprintf("  %-8s %+.4f%%  (min %+.4f%%, max %+.4f%%, %d points)\n",
				w.label, mean, lo, hi, n))
		} else {
			sb.WriteString(fmt.Sprintf("  %-8s (no data)\n", w.label))
		}
	}

	// Recent records (latest 10).
	sorted := make([]broker.EarnRatePoint, len(history))
	copy(sorted, history)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })
	recent := sorted
	if len(recent) > 10 {
		recent = recent[len(recent)-10:]
	}
	sb.WriteString("\n=== Recent Records (latest 10) ===\n")
	sb.WriteString(fmt.Sprintf("%-25s %12s\n", "Time (UTC)", "APY"))
	sb.WriteString(strings.Repeat("-", 38) + "\n")
	for i := len(recent) - 1; i >= 0; i-- {
		r := recent[i]
		ts := time.UnixMilli(r.Timestamp).UTC().Format("2006-01-02 15:04:05")
		sb.WriteString(fmt.Sprintf("%-25s %+11.4f%%\n", ts, r.Rate*100))
	}

	sb.WriteString("\nNote: Earn APY is variable and tiered by amount — verify on the exchange before committing.\n")
	return sb.String()
}

// earnMinMaxPct returns the min and max rate (as percent) within the trailing window.
func earnMinMaxPct(points []broker.EarnRatePoint, window time.Duration, now time.Time) (float64, float64) {
	cutoffMs := now.Add(-window).UnixMilli()
	var lo, hi float64
	first := true
	for _, pt := range points {
		if pt.Timestamp < cutoffMs {
			continue
		}
		r := pt.Rate * 100
		if first {
			lo, hi = r, r
			first = false
			continue
		}
		if r < lo {
			lo = r
		}
		if r > hi {
			hi = r
		}
	}
	return lo, hi
}
