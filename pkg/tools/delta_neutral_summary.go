package tools

import (
	"context"
	"fmt"

	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
)

// GetDeltaNeutralSummaryTool returns the economic summary for a delta-neutral plan.
type GetDeltaNeutralSummaryTool struct {
	store *deltaneutral.Store
}

func NewGetDeltaNeutralSummaryTool(store *deltaneutral.Store) *GetDeltaNeutralSummaryTool {
	return &GetDeltaNeutralSummaryTool{store: store}
}

func (t *GetDeltaNeutralSummaryTool) Name() string { return NameGetDeltaNeutralSummary }

func (t *GetDeltaNeutralSummaryTool) Description() string {
	return "Get the economic summary for a delta-neutral plan derived from its latest monitor snapshot: health score, delta drift, current funding rate, liquidation distance, margin ratio, and estimated PnL."
}

func (t *GetDeltaNeutralSummaryTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan_id": map[string]any{
				"type":        "integer",
				"description": "ID of the delta-neutral plan.",
			},
		},
		"required": []string{"plan_id"},
	}
}

func (t *GetDeltaNeutralSummaryTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	planIDf, _ := args["plan_id"].(float64)
	planID := int64(planIDf)
	if planID <= 0 {
		return ErrorResult("plan_id is required")
	}

	plan, err := t.store.GetPlan(ctx, planID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("plan not found: %v", err))
	}

	// Get latest snapshot
	snapshot, _ := t.store.LatestSnapshot(ctx, planID)

	out := fmt.Sprintf("Delta-Neutral Summary — Plan %d: %s\n\n", planID, plan.Name)
	out += fmt.Sprintf("  Asset:              %s\n", plan.Asset)
	out += fmt.Sprintf("  Status:             %s\n", plan.Status)
	out += fmt.Sprintf("  Enabled:            %v\n", plan.Enabled)
	out += "\n"
	out += fmt.Sprintf("Positions:\n")
	out += fmt.Sprintf("  Spot:               %s on %s\n", plan.SpotSymbol, plan.SpotProvider)
	out += fmt.Sprintf("  Futures:            %s on %s (leverage %d)\n", plan.FuturesSymbol, plan.FuturesProvider, plan.FuturesLeverage)
	out += fmt.Sprintf("  Capital:            %.2f USDT\n", plan.CapitalUSDT)
	out += "\n"
	out += fmt.Sprintf("Estimated Costs:\n")
	out += fmt.Sprintf("  Entry cost:         %.4f USDT\n", plan.EstimatedEntryCostUSDT)
	out += fmt.Sprintf("  Exit cost:          %.4f USDT\n", plan.EstimatedExitCostUSDT)
	out += fmt.Sprintf("  Expected daily funding: %.4f USDT\n", plan.ExpectedDailyFundingUSDT)
	out += fmt.Sprintf("  Breakeven days:     %.2f days\n", plan.BreakevenDays)
	out += "\n"

	if snapshot != nil {
		out += fmt.Sprintf("Latest Monitor (as of %s):\n", snapshot.CheckedAt.Format("2006-01-02 15:04:05 UTC"))
		out += fmt.Sprintf("  Health:             %s (score: %d/100)\n", snapshot.HealthLabel, snapshot.HealthScore)
		out += fmt.Sprintf("  Delta drift:        %.2f%% (threshold: %.2f%%)\n", snapshot.DeltaDriftPct, plan.RiskPolicy.MaxDeltaDriftPct)
		out += fmt.Sprintf("  Current funding:    %.4f%%\n", snapshot.CurrentFundingRate*100)
		out += fmt.Sprintf("  Est next funding:   %.4f USDT\n", snapshot.EstimatedNextFundingUSDT)
		out += fmt.Sprintf("  Liquidation price:  %.4f\n", snapshot.LiquidationPrice)
		out += fmt.Sprintf("  Liquidation dist:   %.2f%% (threshold: %.2f%%)\n", snapshot.LiquidationDistancePct, plan.RiskPolicy.MinLiquidationDistancePct)
		out += fmt.Sprintf("  Margin ratio:       %.2f%% (%s)\n", snapshot.MarginRatioPct, snapshot.MarginState)
		out += fmt.Sprintf("  Unrealized PnL:     %.4f USDT\n", snapshot.FuturesUnrealizedPnLUSDT)
		out += fmt.Sprintf("  Data status:        %s\n", snapshot.DataStatus)

		if snapshot.ThresholdBreached {
			out += fmt.Sprintf("  ⚠️  ALERT: Threshold breached!\n")
			if len(snapshot.BreachCodes) > 0 {
				out += fmt.Sprintf("     Codes: %v\n", snapshot.BreachCodes)
			}
		} else {
			out += fmt.Sprintf("  ✓ All thresholds OK\n")
		}
		if snapshot.ErrorMsg != "" {
			out += fmt.Sprintf("  Error: %s\n", snapshot.ErrorMsg)
		}
	} else {
		out += fmt.Sprintf("Latest Monitor: None (monitoring has not yet executed)\n")
	}

	return UserResult(out)
}
