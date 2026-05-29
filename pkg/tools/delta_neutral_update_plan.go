package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/cron"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
)

// UpdateDeltaNeutralPlanTool updates an existing delta-neutral plan's configuration.
type UpdateDeltaNeutralPlanTool struct {
	store       *deltaneutral.Store
	cronService *cron.CronService
}

func NewUpdateDeltaNeutralPlanTool(store *deltaneutral.Store, cronService *cron.CronService) *UpdateDeltaNeutralPlanTool {
	return &UpdateDeltaNeutralPlanTool{store: store, cronService: cronService}
}

func (t *UpdateDeltaNeutralPlanTool) Name() string { return NameUpdateDeltaNeutralPlan }

func (t *UpdateDeltaNeutralPlanTool) Description() string {
	return "Update an existing delta-neutral plan. Editable fields: name, enabled state, monitor_interval (recreates cron job when changed), " +
		"risk thresholds (funding rate, liquidation distance, delta drift, slippage, capital limits, leverage, reserve margin), " +
		"and notification routing. Provider/account bindings cannot be changed after draft status — pause/close the plan first to re-configure the legs."
}

func (t *UpdateDeltaNeutralPlanTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan_id": map[string]any{
				"type":        "integer",
				"description": "ID of the plan to update.",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Rename the plan. Also updates the cron job label.",
			},
			"enabled": map[string]any{
				"type":        "boolean",
				"description": "Enable or disable the plan.",
			},
			"monitor_interval": map[string]any{
				"type":        "string",
				"enum":        []string{"30s", "1m", "3m", "5m", "10m", "15m", "30m", "1h", "2h", "3h", "4h", "8h", "1d"},
				"description": "Change the monitor interval. Recreates the cron job when changed.",
			},
			"risk_policy": map[string]any{
				"type":        "object",
				"description": "Update specific risk thresholds (partial update).",
				"properties": map[string]any{
					"min_funding_rate": map[string]any{
						"type":        "number",
						"description": "Minimum acceptable funding rate.",
					},
					"max_breakeven_days": map[string]any{
						"type":        "number",
						"description": "Max days to breakeven.",
					},
					"min_liquidation_distance_pct": map[string]any{
						"type":        "number",
						"description": "Minimum liquidation distance in percent.",
					},
					"max_delta_drift_pct": map[string]any{
						"type":        "number",
						"description": "Maximum allowed delta drift.",
					},
					"max_slippage_bps": map[string]any{
						"type":        "number",
						"description": "Maximum slippage in basis points.",
					},
					"max_capital_usdt": map[string]any{
						"type":        "number",
						"description": "Maximum capital limit.",
					},
					"max_leverage": map[string]any{
						"type":        "integer",
						"description": "Maximum leverage allowed.",
					},
					"reserve_margin_usdt": map[string]any{
						"type":        "number",
						"description": "Margin buffer to maintain.",
					},
				},
			},
			"notify": map[string]any{
				"type":        "object",
				"description": "Update notification routing.",
				"properties": map[string]any{
					"channel": map[string]any{
						"type":        "string",
						"description": "Channel for alerts.",
					},
					"chat_id": map[string]any{
						"type":        "string",
						"description": "ChatID for alert delivery.",
					},
				},
			},
		},
		"required": []string{"plan_id"},
	}
}

func (t *UpdateDeltaNeutralPlanTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	planIDf, _ := args["plan_id"].(float64)
	planID := int64(planIDf)
	if planID <= 0 {
		return ErrorResult("plan_id is required")
	}

	plan, err := t.store.GetPlan(ctx, planID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("plan not found: %v", err))
	}

	changed := false

	// Update name
	if v, ok := args["name"].(string); ok && v != "" {
		plan.Name = v
		changed = true
	}

	// Update enabled state
	if v, ok := args["enabled"].(bool); ok {
		plan.Enabled = v
		t.cronService.EnableJob(plan.CronJobID, v)
		changed = true
	}

	// Update monitor_interval and recreate cron job if changed
	if newInterval, ok := args["monitor_interval"].(string); ok && newInterval != "" {
		if !deltaneutral.ValidInterval(newInterval) {
			return ErrorResult(fmt.Sprintf("monitor_interval %q is not supported", newInterval))
		}
		if newInterval != plan.MonitorInterval {
			ms, err := deltaneutral.IntervalToMS(newInterval)
			if err != nil {
				return ErrorResult(fmt.Sprintf("invalid monitor_interval: %v", err))
			}
			// Remove old cron job and create new one
			if plan.CronJobID != "" {
				t.cronService.RemoveJob(plan.CronJobID)
			}
			cronMsg := fmt.Sprintf("[DN-MONITOR] plan_id=%d", planID)
			job, err := t.cronService.AddJob(
				fmt.Sprintf("dn:%d:%s", planID, plan.Name),
				cron.CronSchedule{Kind: "every", EveryMS: &ms},
				cronMsg,
				false,
				plan.NotifyChannel,
				plan.NotifyChatID,
			)
			if err != nil {
				return ErrorResult(fmt.Sprintf("failed to recreate cron job: %v", err))
			}
			job.Payload.NoHistory = true
			t.cronService.UpdateJob(job)
			plan.CronJobID = job.ID
			plan.MonitorInterval = newInterval
			changed = true
		}
	}

	// Update risk policy (partial)
	if riskMap, ok := args["risk_policy"].(map[string]any); ok {
		if v, ok := riskMap["min_funding_rate"].(float64); ok {
			plan.RiskPolicy.MinFundingRate = v
			changed = true
		}
		if v, ok := riskMap["max_breakeven_days"].(float64); ok {
			plan.RiskPolicy.MaxBreakevenDays = v
			changed = true
		}
		if v, ok := riskMap["min_liquidation_distance_pct"].(float64); ok {
			plan.RiskPolicy.MinLiquidationDistancePct = v
			changed = true
		}
		if v, ok := riskMap["max_delta_drift_pct"].(float64); ok {
			plan.RiskPolicy.MaxDeltaDriftPct = v
			changed = true
		}
		if v, ok := riskMap["max_slippage_bps"].(float64); ok {
			plan.RiskPolicy.MaxSlippageBps = v
			changed = true
		}
		if v, ok := riskMap["max_capital_usdt"].(float64); ok {
			plan.RiskPolicy.MaxCapitalUSDT = v
			changed = true
		}
		if v, ok := riskMap["max_leverage"].(float64); ok {
			plan.RiskPolicy.MaxLeverage = int(v)
			changed = true
		}
		if v, ok := riskMap["reserve_margin_usdt"].(float64); ok {
			plan.RiskPolicy.ReserveMarginUSDT = v
			changed = true
		}
	}

	// Update notification routing
	if notif, ok := args["notify"].(map[string]any); ok {
		if v, _ := notif["channel"].(string); v != "" {
			plan.NotifyChannel = v
			changed = true
		}
		if v, _ := notif["chat_id"].(string); v != "" {
			plan.NotifyChatID = v
			changed = true
		}
	}

	if !changed {
		return UserResult("No changes specified.")
	}

	plan.UpdatedAt = time.Now().UTC()
	if err := t.store.UpdatePlan(ctx, plan); err != nil {
		return ErrorResult(fmt.Sprintf("failed to update plan: %v", err))
	}

	// Sync cron job name if plan name changed
	if job := t.cronService.GetJob(plan.CronJobID); job != nil {
		job.Name = fmt.Sprintf("dn:%d:%s", plan.ID, plan.Name)
		_ = t.cronService.UpdateJob(job)
	}

	status := "enabled"
	if !plan.Enabled {
		status = "disabled"
	}
	out := fmt.Sprintf("Plan %d (%s) updated: %s, monitor_interval=%s\n",
		plan.ID, plan.Name, status, plan.MonitorInterval)
	return UserResult(out)
}
