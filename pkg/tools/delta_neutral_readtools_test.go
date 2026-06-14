package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
)

// seedDNPlan creates a delta-neutral plan in the store and returns its ID.
func seedDNPlan(t *testing.T, store *deltaneutral.Store, name, status string) int64 {
	t.Helper()
	now := time.Now().UTC()
	plan := &deltaneutral.Plan{
		Name:                name,
		Asset:               "BTC",
		Status:              status,
		Mode:                deltaneutral.ExecutionModeApproval,
		SpotProvider:        "binance",
		SpotAccount:         "main",
		SpotSymbol:          "BTC/USDT",
		SpotSide:            "buy",
		FuturesProvider:     "okx",
		FuturesAccount:      "futures",
		FuturesSymbol:       "BTC/USDT:USDT",
		FuturesSide:         "short",
		FuturesMarginMode:   "cross",
		FuturesLeverage:     3,
		CapitalUSDT:         10000,
		SpotNotionalUSDT:    5000,
		FuturesNotionalUSDT: 5000,
		ReserveMarginUSDT:   500,
		MonitorInterval:     "5m",
		CronJobID:           "dn:1:test",
		Enabled:             true,
		CrossExchange:       true,
		NotifyChannel:       "telegram",
		NotifyChatID:        "123",
		CreatedAt:           now,
		UpdatedAt:           now,
		RiskPolicy:          deltaneutral.DefaultRiskPolicy(),
	}
	id, err := store.SavePlan(context.Background(), plan)
	if err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	return id
}

func seedDNSnapshot(t *testing.T, store *deltaneutral.Store, planID int64, breached bool) {
	t.Helper()
	snap := &deltaneutral.MonitorSnapshot{
		PlanID:                 planID,
		CheckedAt:              time.Now().UTC(),
		SpotValueUSDT:          5000,
		FuturesNotionalUSDT:    5010,
		CurrentFundingRate:     0.0001,
		DeltaDriftPct:          0.2,
		LiquidationDistancePct: 30,
		LiquidationPrice:       40000,
		MarginRatioPct:         12,
		MarginState:            "safe",
		HealthScore:            82,
		HealthLabel:            "healthy",
		ThresholdBreached:      breached,
		DataStatus:             "ok",
		CreatedAt:              time.Now().UTC(),
	}
	if breached {
		snap.BreachCodes = []string{"delta_drift_high"}
	}
	if _, err := store.SaveSnapshot(context.Background(), snap); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
}

func seedDNAlert(t *testing.T, store *deltaneutral.Store, planID int64) {
	t.Helper()
	alert := &deltaneutral.Alert{
		PlanID:            planID,
		TriggeredAt:       time.Now().UTC(),
		Severity:          "danger",
		Code:              "delta_drift_high",
		Message:           "delta drift exceeded threshold",
		RecommendedAction: "rebalance the legs",
		DeliveredChannel:  "telegram",
		DeliveredChatID:   "123",
		CreatedAt:         time.Now().UTC(),
	}
	if _, err := store.SaveAlert(context.Background(), alert); err != nil {
		t.Fatalf("seed alert: %v", err)
	}
}

func TestGetDeltaNeutralPlanTool(t *testing.T) {
	store := setupTempDeltaNeutralStore(t)
	defer store.Close()
	id := seedDNPlan(t, store, "get-plan-test", "active")
	seedDNSnapshot(t, store, id, true)
	seedDNAlert(t, store, id)

	tool := NewGetDeltaNeutralPlanTool(nil, store)
	if tool.Name() != NameGetDeltaNeutralPlan {
		t.Fatalf("unexpected name %q", tool.Name())
	}
	if tool.Description() == "" || tool.Parameters() == nil {
		t.Fatal("description/parameters must be populated")
	}

	if res := tool.Execute(context.Background(), map[string]any{}); !res.IsError {
		t.Fatal("expected error for missing plan_id")
	}
	if res := tool.Execute(context.Background(), map[string]any{"plan_id": 9999.0}); !res.IsError {
		t.Fatal("expected error for missing plan")
	}
	res := tool.Execute(context.Background(), map[string]any{"plan_id": float64(id)})
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.ForLLM)
	}
	for _, want := range []string{"get-plan-test", "BTC/USDT:USDT", "Latest Snapshot", "healthy", "Latest Alert", "delta_drift_high"} {
		if !strings.Contains(res.ForUser, want) {
			t.Fatalf("output missing %q:\n%s", want, res.ForUser)
		}
	}
}

func TestGetDeltaNeutralPlanTool_NoSnapshot(t *testing.T) {
	store := setupTempDeltaNeutralStore(t)
	defer store.Close()
	id := seedDNPlan(t, store, "no-snap", "ready")

	res := NewGetDeltaNeutralPlanTool(nil, store).Execute(context.Background(), map[string]any{"plan_id": float64(id)})
	if res.IsError {
		t.Fatalf("unexpected error: %v", res.ForLLM)
	}
	if !strings.Contains(res.ForUser, "Latest Snapshot: None") {
		t.Fatalf("expected no-snapshot note:\n%s", res.ForUser)
	}
}

func TestListDeltaNeutralPlansTool(t *testing.T) {
	store := setupTempDeltaNeutralStore(t)
	defer store.Close()

	tool := NewListDeltaNeutralPlansTool(store)
	if tool.Name() != NameListDeltaNeutralPlans {
		t.Fatalf("unexpected name %q", tool.Name())
	}
	_ = tool.Description()
	_ = tool.Parameters()

	if res := tool.Execute(context.Background(), map[string]any{}); res.IsError || !strings.Contains(res.ForUser, "No delta-neutral plans") {
		t.Fatalf("expected empty-list message, got: %v / %s", res.IsError, res.ForUser)
	}

	seedDNPlan(t, store, "plan-a", "active")
	seedDNPlan(t, store, "plan-b", "draft")

	res := tool.Execute(context.Background(), map[string]any{})
	if res.IsError || !strings.Contains(res.ForUser, "plan-a") || !strings.Contains(res.ForUser, "plan-b") {
		t.Fatalf("expected both plans listed:\n%s", res.ForUser)
	}
	res2 := tool.Execute(context.Background(), map[string]any{
		"filter_enabled": true,
		"filter_status":  "active",
		"filter_asset":   "BTC",
	})
	if res2.IsError {
		t.Fatalf("unexpected error: %v", res2.ForLLM)
	}
}

func TestGetDeltaNeutralSummaryTool_Errors(t *testing.T) {
	store := setupTempDeltaNeutralStore(t)
	defer store.Close()
	id := seedDNPlan(t, store, "summary-test", "active")
	seedDNSnapshot(t, store, id, false)

	tool := NewGetDeltaNeutralSummaryTool(nil, store)
	if tool.Name() != NameGetDeltaNeutralSummary {
		t.Fatalf("unexpected name %q", tool.Name())
	}
	_ = tool.Description()
	_ = tool.Parameters()

	if res := tool.Execute(context.Background(), map[string]any{}); !res.IsError {
		t.Fatal("expected error for missing plan_id")
	}
	if res := tool.Execute(context.Background(), map[string]any{"plan_id": 9999.0}); !res.IsError {
		t.Fatal("expected error for unknown plan")
	}
	res := tool.Execute(context.Background(), map[string]any{"plan_id": float64(id)})
	if res.IsError || !strings.Contains(res.ForUser, "summary-test") {
		t.Fatalf("expected summary for plan:\n%s", res.ForUser)
	}
}

func TestGetDeltaNeutralHistoryTool_WithData(t *testing.T) {
	store := setupTempDeltaNeutralStore(t)
	defer store.Close()
	id := seedDNPlan(t, store, "history-test", "active")

	tool := NewGetDeltaNeutralHistoryTool(store)
	if tool.Name() != NameGetDeltaNeutralHistory {
		t.Fatalf("unexpected name %q", tool.Name())
	}
	_ = tool.Description()
	_ = tool.Parameters()

	if res := tool.Execute(context.Background(), map[string]any{}); !res.IsError {
		t.Fatal("expected error for missing plan_id")
	}
	if res := tool.Execute(context.Background(), map[string]any{"plan_id": float64(id)}); res.IsError || !strings.Contains(res.ForUser, "No monitor history") {
		t.Fatalf("expected no-history message:\n%s", res.ForUser)
	}

	seedDNSnapshot(t, store, id, true)
	seedDNSnapshot(t, store, id, false)
	seedDNAlert(t, store, id)
	res := tool.Execute(context.Background(), map[string]any{"plan_id": float64(id), "limit": 10.0, "offset": 0.0})
	if res.IsError || !strings.Contains(res.ForUser, "history-test") {
		t.Fatalf("expected history output:\n%s", res.ForUser)
	}
}

func TestUpdateDeltaNeutralPlanTool_AllFields(t *testing.T) {
	store := setupTempDeltaNeutralStore(t)
	defer store.Close()
	cronSvc := newTestCronServiceForDN(t)
	id := seedDNPlan(t, store, "update-fields", "draft")

	tool := NewUpdateDeltaNeutralPlanTool(&config.Config{}, store, cronSvc)
	if tool.Name() != NameUpdateDeltaNeutralPlan {
		t.Fatalf("unexpected name %q", tool.Name())
	}
	_ = tool.Description()
	_ = tool.Parameters()

	if res := tool.Execute(context.Background(), map[string]any{}); !res.IsError {
		t.Fatal("expected error for missing plan_id")
	}
	if res := tool.Execute(context.Background(), map[string]any{"plan_id": 9999.0}); !res.IsError {
		t.Fatal("expected error for unknown plan")
	}
	if res := tool.Execute(context.Background(), map[string]any{"plan_id": float64(id)}); !res.IsError && !strings.Contains(res.ForUser, "No changes") {
		t.Fatalf("expected no-change handling, got: %v / %s", res.IsError, res.ForUser)
	}

	res := tool.Execute(context.Background(), map[string]any{
		"plan_id": float64(id),
		"name":    "update-fields-renamed",
		"enabled": false,
		"risk_policy": map[string]any{
			"min_funding_rate":             0.0002,
			"max_breakeven_days":           5.0,
			"min_liquidation_distance_pct": 20.0,
			"max_delta_drift_pct":          4.0,
			"max_slippage_bps":             25.0,
			"max_capital_usdt":             20000.0,
			"max_leverage":                 10.0,
			"reserve_margin_usdt":          750.0,
		},
		"notify": map[string]any{
			"channel": "discord",
			"chat_id": "999",
		},
	})
	if res.IsError {
		t.Fatalf("unexpected error updating fields: %v", res.ForLLM)
	}
	got, _ := store.GetPlan(context.Background(), id)
	if got.Name != "update-fields-renamed" || got.Enabled {
		t.Fatalf("name/enabled not updated: name=%q enabled=%v", got.Name, got.Enabled)
	}
	if got.RiskPolicy.MaxLeverage != 10 || got.RiskPolicy.MinFundingRate != 0.0002 {
		t.Fatalf("risk policy not updated: %+v", got.RiskPolicy)
	}
	if got.NotifyChannel != "discord" || got.NotifyChatID != "999" {
		t.Fatalf("notify not updated: %q/%q", got.NotifyChannel, got.NotifyChatID)
	}
}

func TestDeleteDeltaNeutralPlanTool(t *testing.T) {
	store := setupTempDeltaNeutralStore(t)
	defer store.Close()
	cronSvc := newTestCronServiceForDN(t)

	tool := NewDeleteDeltaNeutralPlanTool(store, cronSvc)
	if tool.Name() != NameDeleteDeltaNeutralPlan {
		t.Fatalf("unexpected name %q", tool.Name())
	}
	_ = tool.Description()
	_ = tool.Parameters()

	if res := tool.Execute(context.Background(), map[string]any{}); !res.IsError {
		t.Fatal("expected error for missing plan_id")
	}

	activeID := seedDNPlan(t, store, "active-del", "active")
	if res := tool.Execute(context.Background(), map[string]any{"plan_id": float64(activeID)}); !res.IsError {
		t.Fatal("expected refusal to delete an active plan")
	}

	draftID := seedDNPlan(t, store, "draft-del", "draft")
	res := tool.Execute(context.Background(), map[string]any{"plan_id": float64(draftID)})
	if res.IsError {
		t.Fatalf("unexpected error deleting draft: %v", res.ForLLM)
	}
	if _, err := store.GetPlan(context.Background(), draftID); err == nil {
		t.Fatal("expected plan to be gone after delete")
	}
}
