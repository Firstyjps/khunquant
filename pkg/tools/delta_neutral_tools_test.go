package tools

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/cron"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
)

func newTestCronServiceForDN(t *testing.T) *cron.CronService {
	t.Helper()
	return cron.NewCronService(filepath.Join(t.TempDir(), "cron.json"), nil)
}

func TestCreateDeltaNeutralPlanTool_RejectBitkubFutures(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())
	cronService := newTestCronServiceForDN(t)

	tool := NewCreateDeltaNeutralPlanTool(&config.Config{}, store, cronService)

	// Try to use bitkub as futures_provider (should reject)
	args := map[string]any{
		"plan_name":        "Test Plan",
		"asset":            "BTC",
		"spot_provider":    "binance",
		"spot_symbol":      "BTC/USDT",
		"futures_provider": "bitkub", // spot-only provider
		"futures_symbol":   "BTC/USDT",
		"capital_usdt":     1000.0,
	}

	result := tool.Execute(context.Background(), args)
	if !result.IsError {
		t.Errorf("Expected error for bitkub futures_provider, got success")
	}
	if result.ForLLM == "" {
		t.Errorf("Expected error message, got empty string")
	}
}

func TestCreateDeltaNeutralPlanTool_RejectBinanceEthFutures(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())
	cronService := newTestCronServiceForDN(t)

	tool := NewCreateDeltaNeutralPlanTool(&config.Config{}, store, cronService)

	// Try to use binanceth as futures_provider (should reject)
	args := map[string]any{
		"plan_name":        "Test Plan",
		"asset":            "ETH",
		"spot_provider":    "binance",
		"spot_symbol":      "ETH/USDT",
		"futures_provider": "binanceth", // spot-only provider
		"futures_symbol":   "ETH/USDT:USDT",
		"capital_usdt":     5000.0,
	}

	result := tool.Execute(context.Background(), args)
	if !result.IsError {
		t.Errorf("Expected error for binanceth futures_provider, got success")
	}
}

func TestCreateDeltaNeutralPlanTool_RejectInvalidMonitorInterval(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())
	cronService := newTestCronServiceForDN(t)

	tool := NewCreateDeltaNeutralPlanTool(&config.Config{}, store, cronService)

	args := map[string]any{
		"plan_name":        "Test Plan",
		"asset":            "BTC",
		"spot_provider":    "binance",
		"spot_symbol":      "BTC/USDT",
		"futures_provider": "binance",
		"futures_symbol":   "BTC/USDT:USDT",
		"capital_usdt":     1000.0,
		"monitor_interval": "2m30s", // invalid
	}

	result := tool.Execute(context.Background(), args)
	if !result.IsError {
		t.Errorf("Expected error for invalid monitor_interval, got success")
	}
}

func TestCreateDeltaNeutralPlanTool_Success(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())
	cronService := newTestCronServiceForDN(t)

	tool := NewCreateDeltaNeutralPlanTool(&config.Config{}, store, cronService)

	args := map[string]any{
		"plan_name":        "BTC Funding Harvest Q1",
		"asset":            "BTC",
		"spot_provider":    "binance",
		"spot_account":     "my_spot",
		"spot_symbol":      "BTC/USDT",
		"futures_provider": "binance",
		"futures_account":  "my_futures",
		"futures_symbol":   "BTC/USDT:USDT",
		"capital_usdt":     10000.0,
		"leverage":         5.0,
		"monitor_interval": "5m",
	}

	result := tool.Execute(context.Background(), args)
	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.ForLLM)
	}

	// Verify the plan was saved
	plans, err := store.ListPlans(context.Background(), deltaneutral.QueryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Failed to list plans: %v", err)
	}
	if len(plans) == 0 {
		t.Errorf("Expected 1 plan to be saved, got 0")
	}
	if len(plans) > 0 {
		plan := plans[0]
		if plan.Name != "BTC Funding Harvest Q1" {
			t.Errorf("Expected plan name 'BTC Funding Harvest Q1', got '%s'", plan.Name)
		}
		if plan.Status != deltaneutral.PlanStatusDraft {
			t.Errorf("Expected status 'draft', got '%s'", plan.Status)
		}
		if plan.FuturesLeverage != 5 {
			t.Errorf("Expected leverage 5, got %d", plan.FuturesLeverage)
		}
		if !plan.Enabled {
			t.Errorf("Expected plan to be enabled by default")
		}
		if plan.CronJobID == "" {
			t.Errorf("Expected cron job ID to be set")
		}
	}
}

func TestCreateDeltaNeutralPlanTool_CrossExchange(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())
	cronService := newTestCronServiceForDN(t)

	tool := NewCreateDeltaNeutralPlanTool(&config.Config{}, store, cronService)

	args := map[string]any{
		"plan_name":        "Cross-Exchange BTC",
		"asset":            "BTC",
		"spot_provider":    "bitkub",
		"spot_symbol":      "BTC/THB",
		"futures_provider": "binance",
		"futures_symbol":   "BTC/USDT:USDT",
		"capital_usdt":     5000.0,
	}

	result := tool.Execute(context.Background(), args)
	if result.IsError {
		t.Fatalf("Expected success for cross-exchange setup, got error: %s", result.ForLLM)
	}

	plans, _ := store.ListPlans(context.Background(), deltaneutral.QueryFilter{Limit: 10})
	if len(plans) > 0 && !plans[0].CrossExchange {
		t.Errorf("Expected CrossExchange flag to be true for different providers")
	}
}

func TestUpdateDeltaNeutralPlanTool_ChangeMonitorInterval(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())
	cronService := newTestCronServiceForDN(t)

	// Create a plan first
	plan := &deltaneutral.Plan{
		Name:            "Test Plan",
		Asset:           "BTC",
		Status:          deltaneutral.PlanStatusDraft,
		Mode:            deltaneutral.ExecutionModeApproval,
		SpotProvider:    "binance",
		SpotSymbol:      "BTC/USDT",
		FuturesProvider: "binance",
		FuturesSymbol:   "BTC/USDT:USDT",
		CapitalUSDT:     1000.0,
		MonitorInterval: "5m",
		Enabled:         true,
		RiskPolicy:      deltaneutral.DefaultRiskPolicy(),
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	planID, _ := store.SavePlan(context.Background(), plan)
	plan.ID = planID

	// Add a dummy cron job
	ms, _ := deltaneutral.IntervalToMS("5m")
	job, _ := cronService.AddJob(
		"test-dn-job",
		cron.CronSchedule{Kind: "every", EveryMS: &ms},
		"test message",
		false,
		"",
		"",
	)
	plan.CronJobID = job.ID
	store.UpdatePlan(context.Background(), plan)

	// Now update the monitor interval
	tool := NewUpdateDeltaNeutralPlanTool(store, cronService)
	args := map[string]any{
		"plan_id":          float64(planID),
		"monitor_interval": "10m",
	}

	result := tool.Execute(context.Background(), args)
	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.ForLLM)
	}

	// Verify the plan was updated
	updated, _ := store.GetPlan(context.Background(), planID)
	if updated.MonitorInterval != "10m" {
		t.Errorf("Expected monitor_interval to be '10m', got '%s'", updated.MonitorInterval)
	}
}

func TestDeleteDeltaNeutralPlanTool_RejectActivePlan(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())
	cronService := newTestCronServiceForDN(t)

	// Create an ACTIVE plan
	plan := &deltaneutral.Plan{
		Name:            "Active Plan",
		Asset:           "BTC",
		Status:          deltaneutral.PlanStatusActive, // ACTIVE
		Mode:            deltaneutral.ExecutionModeApproval,
		SpotProvider:    "binance",
		SpotSymbol:      "BTC/USDT",
		FuturesProvider: "binance",
		FuturesSymbol:   "BTC/USDT:USDT",
		CapitalUSDT:     1000.0,
		Enabled:         true,
		RiskPolicy:      deltaneutral.DefaultRiskPolicy(),
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	planID, _ := store.SavePlan(context.Background(), plan)

	tool := NewDeleteDeltaNeutralPlanTool(store, cronService)
	args := map[string]any{
		"plan_id": float64(planID),
	}

	result := tool.Execute(context.Background(), args)
	if !result.IsError {
		t.Errorf("Expected error when deleting active plan, got success")
	}

	// Verify plan still exists
	existing, _ := store.GetPlan(context.Background(), planID)
	if existing == nil {
		t.Errorf("Expected plan to still exist after failed deletion")
	}
}

func TestDeleteDeltaNeutralPlanTool_AllowDraftPlan(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())
	cronService := newTestCronServiceForDN(t)

	// Create a DRAFT plan
	plan := &deltaneutral.Plan{
		Name:            "Draft Plan",
		Asset:           "BTC",
		Status:          deltaneutral.PlanStatusDraft, // DRAFT
		Mode:            deltaneutral.ExecutionModeApproval,
		SpotProvider:    "binance",
		SpotSymbol:      "BTC/USDT",
		FuturesProvider: "binance",
		FuturesSymbol:   "BTC/USDT:USDT",
		CapitalUSDT:     1000.0,
		Enabled:         true,
		RiskPolicy:      deltaneutral.DefaultRiskPolicy(),
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	planID, _ := store.SavePlan(context.Background(), plan)

	tool := NewDeleteDeltaNeutralPlanTool(store, cronService)
	args := map[string]any{
		"plan_id": float64(planID),
	}

	result := tool.Execute(context.Background(), args)
	if result.IsError {
		t.Fatalf("Expected success deleting draft plan, got error: %s", result.ForLLM)
	}

	// Verify plan was deleted
	existing, err := store.GetPlan(context.Background(), planID)
	if existing != nil {
		t.Errorf("Expected plan to be deleted, but it still exists")
	}
	if err == nil {
		t.Errorf("Expected error when fetching deleted plan")
	}
}

func TestGetDeltaNeutralSummaryTool(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())

	// Create a plan
	plan := &deltaneutral.Plan{
		Name:            "Test Plan",
		Asset:           "BTC",
		Status:          deltaneutral.PlanStatusActive,
		SpotProvider:    "binance",
		SpotSymbol:      "BTC/USDT",
		FuturesProvider: "binance",
		FuturesSymbol:   "BTC/USDT:USDT",
		CapitalUSDT:     10000.0,
		MonitorInterval: "5m",
		RiskPolicy:      deltaneutral.DefaultRiskPolicy(),
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	planID, _ := store.SavePlan(context.Background(), plan)

	tool := NewGetDeltaNeutralSummaryTool(store)
	args := map[string]any{
		"plan_id": float64(planID),
	}

	result := tool.Execute(context.Background(), args)
	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.ForLLM)
	}
	if result.ForUser == "" {
		t.Errorf("Expected output for user, got empty string")
	}
}

func TestGetDeltaNeutralHistoryTool(t *testing.T) {
	store, _ := deltaneutral.NewStore(t.TempDir())

	// Create a plan
	plan := &deltaneutral.Plan{
		Name:            "Test Plan",
		Asset:           "BTC",
		Status:          deltaneutral.PlanStatusActive,
		SpotProvider:    "binance",
		SpotSymbol:      "BTC/USDT",
		FuturesProvider: "binance",
		FuturesSymbol:   "BTC/USDT:USDT",
		CapitalUSDT:     10000.0,
		RiskPolicy:      deltaneutral.DefaultRiskPolicy(),
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	planID, _ := store.SavePlan(context.Background(), plan)

	tool := NewGetDeltaNeutralHistoryTool(store)
	args := map[string]any{
		"plan_id": float64(planID),
	}

	result := tool.Execute(context.Background(), args)
	if result.IsError {
		t.Fatalf("Expected success, got error: %s", result.ForLLM)
	}
	if result.ForUser == "" {
		t.Errorf("Expected output for user, got empty string")
	}
}
