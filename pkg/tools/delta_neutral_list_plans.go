package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
)

// ListDeltaNeutralPlansTool lists all configured delta-neutral plans.
type ListDeltaNeutralPlansTool struct {
	store *deltaneutral.Store
}

func NewListDeltaNeutralPlansTool(store *deltaneutral.Store) *ListDeltaNeutralPlansTool {
	return &ListDeltaNeutralPlansTool{store: store}
}

func (t *ListDeltaNeutralPlansTool) Name() string { return NameListDeltaNeutralPlans }

func (t *ListDeltaNeutralPlansTool) Description() string {
	return "List all configured delta-neutral funding strategy plans, including their status, asset, providers, and monitor interval."
}

func (t *ListDeltaNeutralPlansTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"filter_enabled": map[string]any{
				"type":        "boolean",
				"description": "If true, return only enabled plans. If false, return only disabled plans. Omit to return all.",
			},
			"filter_status": map[string]any{
				"type":        "string",
				"enum":        []string{"draft", "ready", "active", "paused", "closed"},
				"description": "Filter by plan status. Omit to return all statuses.",
			},
			"filter_asset": map[string]any{
				"type":        "string",
				"description": "Filter by asset (e.g. 'BTC'). Omit to return all assets.",
			},
		},
	}
}

func (t *ListDeltaNeutralPlansTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	f := deltaneutral.QueryFilter{Limit: 100}

	if v, ok := args["filter_enabled"].(bool); ok {
		f.Enabled = &v
	}

	if v, ok := args["filter_status"].(string); ok && v != "" {
		f.Status = &v
	}

	if v, ok := args["filter_asset"].(string); ok && v != "" {
		f.Asset = v
	}

	plans, err := t.store.ListPlans(ctx, f)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to list delta-neutral plans: %v", err))
	}

	if len(plans) == 0 {
		return UserResult("No delta-neutral plans found. Use create_delta_neutral_plan to set one up.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Delta-Neutral Plans (%d total):\n\n", len(plans)))
	for _, p := range plans {
		status := "✓ enabled"
		if !p.Enabled {
			status = "⊘ disabled"
		}
		sb.WriteString(fmt.Sprintf("  [%d] %s — %s (%s)\n", p.ID, p.Name, p.Asset, status))
		sb.WriteString(fmt.Sprintf("      Status:           %s\n", p.Status))
		sb.WriteString(fmt.Sprintf("      Spot:             %s on %s\n", p.SpotSymbol, p.SpotProvider))
		sb.WriteString(fmt.Sprintf("      Futures:          %s on %s (leverage %d)\n", p.FuturesSymbol, p.FuturesProvider, p.FuturesLeverage))
		sb.WriteString(fmt.Sprintf("      Capital:          %.2f USDT\n", p.CapitalUSDT))
		sb.WriteString(fmt.Sprintf("      Monitor interval: %s\n", p.MonitorInterval))
		if p.CrossExchange {
			sb.WriteString(fmt.Sprintf("      ⚠️  Cross-exchange (spot and futures on different exchanges)\n"))
		}
		sb.WriteString("\n")
	}
	return UserResult(sb.String())
}
