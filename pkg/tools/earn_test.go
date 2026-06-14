package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// mockEarnProvider implements broker.EarnProvider for testing.
type mockEarnProvider struct {
	id                       string
	productsErr              error
	positionsErr             error
	subscribeErr             error
	subscribeReturnID        string
	redeemErr                error
	redeemReturnID           string
	setAutoSubscribeErr      error
	callCounts               callCounter
	lastSubscribeCall        *subscribeCall
	lastRedeemCall           *redeemCall
	lastSetAutoSubscribeCall *setAutoSubscribeCall
}

type callCounter struct {
	FetchFlexibleEarnProducts  int
	FetchFlexibleEarnPositions int
	SubscribeFlexibleEarn      int
	RedeemFlexibleEarn         int
	SetFlexibleAutoSubscribe   int
}

type subscribeCall struct {
	ProductID     string
	Asset         string
	Amount        float64
	AutoSubscribe bool
}

type redeemCall struct {
	ProductID string
	Asset     string
	Amount    float64
	RedeemAll bool
}

type setAutoSubscribeCall struct {
	ProductID string
	Asset     string
	Enable    bool
}

func (m *mockEarnProvider) ID() string {
	return m.id
}

func (m *mockEarnProvider) Category() broker.AssetCategory {
	return broker.CategoryCrypto
}

func (m *mockEarnProvider) GetMarketStatus(ctx context.Context, symbol string) (broker.MarketStatus, error) {
	return broker.MarketOpen, nil
}

func (m *mockEarnProvider) FetchFlexibleEarnProducts(ctx context.Context, asset string) ([]broker.EarnProduct, error) {
	m.callCounts.FetchFlexibleEarnProducts++
	if m.productsErr != nil {
		return nil, m.productsErr
	}
	if asset != "" {
		// Filter by asset
		for _, p := range mockProducts {
			if strings.EqualFold(p.Asset, asset) {
				return []broker.EarnProduct{p}, nil
			}
		}
		return []broker.EarnProduct{}, nil
	}
	return mockProducts, nil
}

func (m *mockEarnProvider) FetchFlexibleEarnPositions(ctx context.Context) ([]broker.EarnPosition, error) {
	m.callCounts.FetchFlexibleEarnPositions++
	if m.positionsErr != nil {
		return nil, m.positionsErr
	}
	return mockPositions, nil
}

func (m *mockEarnProvider) SubscribeFlexibleEarn(ctx context.Context, productID, asset string, amount float64, autoSubscribe bool) (string, error) {
	m.callCounts.SubscribeFlexibleEarn++
	m.lastSubscribeCall = &subscribeCall{
		ProductID:     productID,
		Asset:         asset,
		Amount:        amount,
		AutoSubscribe: autoSubscribe,
	}
	if m.subscribeErr != nil {
		return "", m.subscribeErr
	}
	return m.subscribeReturnID, nil
}

func (m *mockEarnProvider) RedeemFlexibleEarn(ctx context.Context, productID, asset string, amount float64, redeemAll bool) (string, error) {
	m.callCounts.RedeemFlexibleEarn++
	m.lastRedeemCall = &redeemCall{
		ProductID: productID,
		Asset:     asset,
		Amount:    amount,
		RedeemAll: redeemAll,
	}
	if m.redeemErr != nil {
		return "", m.redeemErr
	}
	return m.redeemReturnID, nil
}

func (m *mockEarnProvider) SetFlexibleAutoSubscribe(ctx context.Context, productID, asset string, enable bool) error {
	m.callCounts.SetFlexibleAutoSubscribe++
	m.lastSetAutoSubscribeCall = &setAutoSubscribeCall{
		ProductID: productID,
		Asset:     asset,
		Enable:    enable,
	}
	return m.setAutoSubscribeErr
}

func (m *mockEarnProvider) FetchFlexibleEarnRateHistory(ctx context.Context, productID, asset string, since *int64, limit int) ([]broker.EarnRatePoint, error) {
	// Return empty for mock; tests can override if needed.
	return []broker.EarnRatePoint{}, nil
}

// Mock data for testing
var mockProducts = []broker.EarnProduct{
	{
		Exchange:      "binance",
		Asset:         "BTC",
		ProductID:     "BTC-FLEX",
		APY:           0.03, // 3% APY
		CanSubscribe:  true,
		AutoSubscribe: false,
		MinSubscribe:  0.001,
	},
	{
		Exchange:      "binance",
		Asset:         "ETH",
		ProductID:     "ETH-FLEX",
		APY:           0.05, // 5% APY
		CanSubscribe:  true,
		AutoSubscribe: true,
		MinSubscribe:  0.01,
	},
}

var mockPositions = []broker.EarnPosition{
	{
		Exchange:      "binance",
		Asset:         "BTC",
		ProductID:     "BTC-FLEX",
		Amount:        0.5,
		APY:           0.03,
		AutoSubscribe: false,
	},
}

// TestEarnOverview_HappyPath: fetch products and positions, render both tables.
func TestEarnOverview_HappyPath(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return &mockEarnProvider{id: providerID}, nil
	}

	tool := NewEarnOverviewTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider": "binance",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %v", res.ForLLM)
	}

	output := res.ForUser

	// Check products table is present.
	if !strings.Contains(output, "Earn Products") {
		t.Fatalf("expected products table:\n%s", output)
	}

	// Check positions table is present.
	if !strings.Contains(output, "Earn Positions") {
		t.Fatalf("expected positions table:\n%s", output)
	}

	// Check assets are rendered.
	if !strings.Contains(output, "BTC") || !strings.Contains(output, "ETH") {
		t.Fatalf("expected BTC and ETH assets:\n%s", output)
	}

	// Check APY is rendered as percent (3.0000%, 5.0000%).
	if !strings.Contains(output, "3.0000%") || !strings.Contains(output, "5.0000%") {
		t.Fatalf("expected APY as percentages:\n%s", output)
	}

	// Check headers.
	if !strings.Contains(output, "Exchange") {
		t.Fatalf("expected Exchange header:\n%s", output)
	}
}

// TestEarnOverview_PositionsError: products succeed, positions error (degrade gracefully).
func TestEarnOverview_PositionsError(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return &mockEarnProvider{
			id:           providerID,
			positionsErr: context.DeadlineExceeded,
		}, nil
	}

	tool := NewEarnOverviewTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider": "binance",
	})

	// Should NOT be an error—should degrade.
	if res.IsError {
		t.Fatalf("expected graceful degrade, got error: %v", res.ForLLM)
	}

	output := res.ForUser

	// Check products table is still present.
	if !strings.Contains(output, "Earn Products") {
		t.Fatalf("expected products table:\n%s", output)
	}

	// Check note about positions error.
	if !strings.Contains(output, "positions unavailable") {
		t.Fatalf("expected degrade note in output:\n%s", output)
	}

	// Check that no empty positions table is rendered.
	if !strings.Contains(output, "No positions held") {
		t.Fatalf("expected no-positions message:\n%s", output)
	}
}

// TestEarnOverview_AllProviders: "all" expands to both binance and okx.
func TestEarnOverview_AllProviders(t *testing.T) {
	callCount := 0
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		callCount++
		// Return the same mock for both providers.
		return &mockEarnProvider{id: providerID}, nil
	}

	tool := NewEarnOverviewTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider": "all",
	})

	if res.IsError {
		t.Fatalf("unexpected error: %v", res.ForLLM)
	}

	// Should have called earnProviderFn at least twice (binance and okx).
	if callCount < 2 {
		t.Fatalf("expected at least 2 provider calls, got %d", callCount)
	}

	output := res.ForUser

	// Check both assets are present.
	if !strings.Contains(output, "BTC") || !strings.Contains(output, "ETH") {
		t.Fatalf("expected assets from both providers:\n%s", output)
	}
}

// TestManageEarnPosition_Validation_BadAction: action not in {subscribe,redeem,set_auto_subscribe}.
func TestManageEarnPosition_Validation_BadAction(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return &mockEarnProvider{id: providerID}, nil
	}

	tool := NewManageEarnPositionTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider": "binance",
		"action":   "invalid_action",
		"asset":    "BTC",
	})

	if !res.IsError {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(res.ForLLM, "not valid") {
		t.Fatalf("expected validation message, got: %v", res.ForLLM)
	}
}

// TestManageEarnPosition_Validation_EmptyAsset: asset is required.
func TestManageEarnPosition_Validation_EmptyAsset(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return &mockEarnProvider{id: providerID}, nil
	}

	tool := NewManageEarnPositionTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider": "binance",
		"action":   "subscribe",
		"asset":    "",
	})

	if !res.IsError {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(res.ForLLM, "asset is required") {
		t.Fatalf("expected asset validation, got: %v", res.ForLLM)
	}
}

// TestManageEarnPosition_Validation_SubscribeNoAmount: subscribe without amount.
func TestManageEarnPosition_Validation_SubscribeNoAmount(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return &mockEarnProvider{id: providerID}, nil
	}

	tool := NewManageEarnPositionTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider": "binance",
		"action":   "subscribe",
		"asset":    "BTC",
		// no amount
	})

	if !res.IsError {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(res.ForLLM, "amount > 0") {
		t.Fatalf("expected amount validation, got: %v", res.ForLLM)
	}
}

// TestManageEarnPosition_Validation_RedeemNoAmountOrAll: redeem without amount and redeem_all=false.
func TestManageEarnPosition_Validation_RedeemNoAmountOrAll(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return &mockEarnProvider{id: providerID}, nil
	}

	tool := NewManageEarnPositionTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider":   "binance",
		"action":     "redeem",
		"asset":      "BTC",
		"redeem_all": false,
		// no amount
	})

	if !res.IsError {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(res.ForLLM, "amount > 0 or redeem_all") {
		t.Fatalf("expected redeem validation, got: %v", res.ForLLM)
	}
}

// TestManageEarnPosition_Validation_ProviderAll: provider "all" not allowed.
func TestManageEarnPosition_Validation_ProviderAll(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return &mockEarnProvider{id: providerID}, nil
	}

	tool := NewManageEarnPositionTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider": "all",
		"action":   "subscribe",
		"asset":    "BTC",
		"amount":   1.0,
	})

	if !res.IsError {
		t.Fatal("expected validation error for provider all")
	}
	if !strings.Contains(res.ForLLM, "not 'all'") {
		t.Fatalf("expected provider validation, got: %v", res.ForLLM)
	}
}

// TestManageEarnPosition_DryRun: confirm=false returns preview without calling write methods.
func TestManageEarnPosition_DryRun(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	mock := &mockEarnProvider{id: "binance"}
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return mock, nil
	}

	tool := NewManageEarnPositionTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider":       "binance",
		"action":         "subscribe",
		"asset":          "BTC",
		"amount":         1.5,
		"auto_subscribe": true,
		"confirm":        false,
	})

	if res.IsError {
		t.Fatalf("unexpected error: %v", res.ForLLM)
	}

	output := res.ForUser

	// Check for DRY-RUN marker.
	if !strings.Contains(output, "DRY-RUN") {
		t.Fatalf("expected DRY-RUN in output:\n%s", output)
	}

	// Check that write method was NOT called.
	if mock.callCounts.SubscribeFlexibleEarn != 0 {
		t.Fatal("expected SubscribeFlexibleEarn NOT to be called in dry-run")
	}
}

// TestManageEarnPosition_Subscribe_Confirm: confirm=true calls SubscribeFlexibleEarn.
func TestManageEarnPosition_Subscribe_Confirm(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	mock := &mockEarnProvider{
		id:                "binance",
		subscribeReturnID: "sub-123",
	}
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return mock, nil
	}

	tool := NewManageEarnPositionTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider":       "binance",
		"action":         "subscribe",
		"asset":          "BTC",
		"amount":         1.5,
		"auto_subscribe": true,
		"confirm":        true,
	})

	if res.IsError {
		t.Fatalf("unexpected error: %v", res.ForLLM)
	}

	output := res.ForUser

	// Check success message.
	if !strings.Contains(output, "executed successfully") {
		t.Fatalf("expected success message:\n%s", output)
	}

	// Check result ID is shown.
	if !strings.Contains(output, "sub-123") {
		t.Fatalf("expected result ID:\n%s", output)
	}

	// Verify the call was made with correct args.
	if mock.callCounts.SubscribeFlexibleEarn != 1 {
		t.Fatal("expected SubscribeFlexibleEarn to be called once")
	}
	if mock.lastSubscribeCall == nil {
		t.Fatal("expected subscribe call to be recorded")
	}
	if mock.lastSubscribeCall.Asset != "BTC" ||
		mock.lastSubscribeCall.Amount != 1.5 ||
		!mock.lastSubscribeCall.AutoSubscribe {
		t.Fatalf("subscribe call args mismatch: %+v", mock.lastSubscribeCall)
	}
}

// TestManageEarnPosition_Redeem_All: redeem_all=true redeems entire position.
func TestManageEarnPosition_Redeem_All(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	mock := &mockEarnProvider{
		id:             "binance",
		redeemReturnID: "red-456",
	}
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return mock, nil
	}

	tool := NewManageEarnPositionTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider":   "binance",
		"action":     "redeem",
		"asset":      "ETH",
		"redeem_all": true,
		"confirm":    true,
	})

	if res.IsError {
		t.Fatalf("unexpected error: %v", res.ForLLM)
	}

	if mock.callCounts.RedeemFlexibleEarn != 1 {
		t.Fatal("expected RedeemFlexibleEarn to be called once")
	}
	if mock.lastRedeemCall.Asset != "ETH" || !mock.lastRedeemCall.RedeemAll {
		t.Fatalf("redeem call args mismatch: %+v", mock.lastRedeemCall)
	}
}

// TestManageEarnPosition_SetAutoSubscribe_Error: error on set_auto_subscribe surfaces as IsError.
func TestManageEarnPosition_SetAutoSubscribe_Error(t *testing.T) {
	oldFn := earnProviderFn
	t.Cleanup(func() { earnProviderFn = oldFn })
	mock := &mockEarnProvider{
		id:                  "okx",
		setAutoSubscribeErr: context.DeadlineExceeded,
	}
	earnProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.EarnProvider, error) {
		return mock, nil
	}

	tool := NewManageEarnPositionTool(&config.Config{})
	res := tool.Execute(context.Background(), map[string]any{
		"provider":       "okx",
		"action":         "set_auto_subscribe",
		"asset":          "BTC",
		"auto_subscribe": true,
		"confirm":        true,
	})

	if !res.IsError {
		t.Fatal("expected error on set_auto_subscribe failure")
	}
	if !strings.Contains(res.ForLLM, "action failed") {
		t.Fatalf("expected action failed message, got: %v", res.ForLLM)
	}
}

// TestManageEarnPosition_NameDescParams: tool metadata.
func TestManageEarnPosition_NameDescParams(t *testing.T) {
	tool := NewManageEarnPositionTool(&config.Config{})
	if tool.Name() != NameManageEarnPosition {
		t.Fatalf("unexpected name %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Fatal("description must be non-empty")
	}
	if p := tool.Parameters(); p == nil || p["properties"] == nil {
		t.Fatal("parameters must define properties")
	}
}

// TestEarnOverview_NameDescParams: tool metadata.
func TestEarnOverview_NameDescParams(t *testing.T) {
	tool := NewEarnOverviewTool(&config.Config{})
	if tool.Name() != NameEarnOverview {
		t.Fatalf("unexpected name %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Fatal("description must be non-empty")
	}
	if p := tool.Parameters(); p == nil || p["properties"] == nil {
		t.Fatal("parameters must define properties")
	}
}
