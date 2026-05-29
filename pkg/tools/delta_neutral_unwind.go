package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// UnwindDeltaNeutralPositionTool closes a delta-neutral position (both legs) via the
// approval-mode state machine (T2.5), driving recovery from partial fills or manual unwind.
type UnwindDeltaNeutralPositionTool struct {
	cfg   *config.Config
	store *deltaneutral.Store
}

func NewUnwindDeltaNeutralPositionTool(cfg *config.Config, store *deltaneutral.Store) *UnwindDeltaNeutralPositionTool {
	return &UnwindDeltaNeutralPositionTool{cfg: cfg, store: store}
}

func (t *UnwindDeltaNeutralPositionTool) Name() string { return NameUnwindDeltaNeutralPosition }

func (t *UnwindDeltaNeutralPositionTool) Description() string {
	return "Close a delta-neutral position (unwind both legs): reduce-only close on futures + sell on spot. " +
		"Only for active or recovery_required plans. Requires explicit approval (confirm=true). " +
		"Dry-run mode (confirm=false) shows closure summary without executing. Recovery tool for unhedged exposure."
}

func (t *UnwindDeltaNeutralPositionTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan_id": map[string]any{
				"type":        "integer",
				"description": "ID of the active or recovery_required delta-neutral plan to unwind.",
			},
			"confirm": map[string]any{
				"type":        "boolean",
				"description": "Must be true to execute. Use false for dry-run review.",
			},
		},
		"required": []string{"plan_id"},
	}
}

func (t *UnwindDeltaNeutralPositionTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	planID, _ := args["plan_id"].(float64)
	confirm, _ := args["confirm"].(bool)

	if planID <= 0 {
		return ErrorResult("plan_id must be a positive integer")
	}

	// Load the plan
	plan, err := t.store.GetPlan(ctx, int64(planID))
	if err != nil {
		return ErrorResult(fmt.Sprintf("cannot load plan %d: %v", int64(planID), err))
	}

	// Only active or recovery_required plans can be unwound (§8.6)
	if plan.Status != "active" && plan.Status != "recovery_required" {
		return ErrorResult(fmt.Sprintf("plan status is %q, not active or recovery_required. Cannot unwind.", plan.Status))
	}

	// --- Safety gates (same sequence as open) ---

	// Gate 1: leverage opt-in
	if err := broker.CheckLeverage(t.cfg, "unwind delta-neutral position"); err != nil {
		return ErrorResult(err.Error())
	}

	// Gate 2: permission for futures leg
	if err := broker.CheckPermission(t.cfg, plan.FuturesProvider, plan.FuturesAccount, config.ScopeTrade); err != nil {
		return ErrorResult(fmt.Sprintf("futures leg permission denied: %v", err))
	}

	// Gate 2b: permission for spot leg
	if err := broker.CheckPermission(t.cfg, plan.SpotProvider, plan.SpotAccount, config.ScopeTrade); err != nil {
		return ErrorResult(fmt.Sprintf("spot leg permission denied: %v", err))
	}

	// Gate 3: daily loss limit
	if err := broker.GlobalLossTracker.CheckDailyLoss(t.cfg.TradingRisk.DailyLossLimitUSD); err != nil {
		return ErrorResult(err.Error())
	}

	// Gate 4: rate limit on both providers
	if !broker.DefaultLimiter.Allow(plan.FuturesProvider) {
		return ErrorResult(fmt.Sprintf("rate limit exceeded for futures provider %q", plan.FuturesProvider)).
			WithError(broker.ErrRateLimited)
	}
	if !broker.DefaultLimiter.Allow(plan.SpotProvider) {
		return ErrorResult(fmt.Sprintf("rate limit exceeded for spot provider %q", plan.SpotProvider)).
			WithError(broker.ErrRateLimited)
	}

	// --- Dry-run gate ---
	if !confirm {
		review := formatUnwindReview(plan)
		review += "\n\nSet confirm=true to execute closure."
		return UserResult(review)
	}

	// --- On confirm: execute the unwind (close both legs) ---

	// Create an Execution record for the unwind attempt
	now := time.Now()
	exec := &deltaneutral.Execution{
		PlanID:      plan.ID,
		AttemptID:   fmt.Sprintf("unwind_%d_%d", plan.ID, now.UnixNano()),
		State:       string(deltaneutral.ExecutionStateUnwinding),
		RequestedAt: now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	execID, err := t.store.SaveExecution(ctx, exec)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to save unwind execution record: %v", err))
	}
	exec.ID = execID

	// Close futures leg (reduce-only)
	futuresErr := t.closeFuturesLeg(ctx, plan, exec)
	if futuresErr != nil {
		// Log the error but continue to spot leg
		return ErrorResult(fmt.Sprintf(
			"CRITICAL: Futures leg closure failed — UNHEDGED EXPOSURE. "+
				"Spot leg may still be open. Error: %v. "+
				"Manual intervention required.",
			futuresErr))
	}

	// Close spot leg (sell)
	spotErr := t.closeSpotLeg(ctx, plan, exec)
	if spotErr != nil {
		return ErrorResult(fmt.Sprintf(
			"CRITICAL: Spot leg closure failed — UNHEDGED EXPOSURE. "+
				"Futures leg closed but spot leg may still be open. Error: %v. "+
				"Manual intervention required.",
			spotErr))
	}

	// Both legs closed successfully
	exec.State = string(deltaneutral.ExecutionStateUnwound)
	completedNow := time.Now()
	exec.CompletedAt = &completedNow
	if err := t.store.UpdateExecution(ctx, exec); err != nil {
		return ErrorResult(fmt.Sprintf("failed to finalize unwind execution: %v", err))
	}

	// Update plan: mark closed_at, set status to closed
	plan.ClosedAt = &completedNow
	plan.Status = "closed"
	if err := t.store.UpdatePlan(ctx, plan); err != nil {
		return ErrorResult(fmt.Sprintf("failed to update plan status: %v", err))
	}

	return UserResult(fmt.Sprintf(
		"Delta-neutral position successfully closed:\n"+
			"  Plan:       %s (ID %d)\n"+
			"  Status:     closed\n"+
			"  Closed At:  %s\n",
		plan.Name, plan.ID, completedNow.Format(time.RFC3339)))
}

// closeFuturesLeg closes the futures hedge position (reduce-only).
func (t *UnwindDeltaNeutralPositionTool) closeFuturesLeg(ctx context.Context, plan *deltaneutral.Plan, exec *deltaneutral.Execution) error {
	fp, err := futuresProvider(ctx, t.cfg, plan.FuturesProvider, plan.FuturesAccount)
	if err != nil {
		return fmt.Errorf("futures provider: %w", err)
	}

	// Fetch current position to get size
	positions, err := fp.FetchFuturesPositions(ctx, []string{plan.FuturesSymbol})
	if err != nil {
		return fmt.Errorf("fetch positions: %w", err)
	}

	var closeAmount float64
	for _, p := range positions {
		if p.Contracts == nil || *p.Contracts == 0 {
			continue
		}
		sym := ""
		if p.Symbol != nil {
			sym = normalizeFuturesSymbol(*p.Symbol)
		}
		if sym != normalizeFuturesSymbol(plan.FuturesSymbol) {
			continue
		}
		// Use absolute value (position is already in the correct side)
		closeAmount = *p.Contracts
		if closeAmount < 0 {
			closeAmount = -closeAmount
		}
		break
	}

	if closeAmount == 0 {
		return fmt.Errorf("no active %s position found", plan.FuturesSymbol)
	}

	// Determine close side (opposite of position side)
	closeSide := futuresCloseSide(plan.FuturesSide)

	// Create reduce-only close order
	order, err := fp.CreateFuturesOrder(ctx, broker.FuturesOrderRequest{
		Symbol:       plan.FuturesSymbol,
		OrderType:    "market",
		Side:         closeSide,
		Amount:       closeAmount,
		PositionSide: plan.FuturesSide,
		ReduceOnly:   true,
	})
	if err != nil {
		return fmt.Errorf("close order failed: %w", err)
	}

	// Record the leg
	leg := &deltaneutral.ExecutionLeg{
		ExecutionID:     exec.ID,
		LegType:         string(deltaneutral.LegTypeFutures),
		Provider:        plan.FuturesProvider,
		Account:         plan.FuturesAccount,
		Symbol:          plan.FuturesSymbol,
		Side:            closeSide,
		OrderType:       "market",
		RequestedAmount: closeAmount,
		OrderID:         orderID(order),
		State:           string(deltaneutral.LegStateFilled),
		FilledQuantity:  closeAmount,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	_, err = t.store.SaveExecutionLeg(ctx, leg)
	return err
}

// closeSpotLeg closes the spot position (sell).
func (t *UnwindDeltaNeutralPositionTool) closeSpotLeg(ctx context.Context, plan *deltaneutral.Plan, exec *deltaneutral.Execution) error {
	sp, err := broker.CreateProviderForAccount(plan.SpotProvider, plan.SpotAccount, t.cfg)
	if err != nil {
		return fmt.Errorf("spot provider: %w", err)
	}

	tp, ok := sp.(broker.TradingProvider)
	if !ok {
		return fmt.Errorf("spot provider does not support order execution")
	}

	// Fetch balance to determine sell amount
	pp, ok := sp.(broker.PortfolioProvider)
	if !ok {
		return fmt.Errorf("spot provider does not support balance fetch")
	}

	balances, err := pp.GetBalances(ctx)
	if err != nil {
		return fmt.Errorf("fetch balances: %w", err)
	}

	// Extract base currency from symbol (e.g., "BTC/USDT" -> "BTC")
	var baseCur string
	parts := strings.SplitN(plan.SpotSymbol, "/", 2)
	if len(parts) >= 1 {
		baseCur = parts[0]
	}
	if baseCur == "" {
		return fmt.Errorf("cannot parse base currency from symbol %s", plan.SpotSymbol)
	}

	var sellAmount float64
	for _, b := range balances {
		if b.Asset == baseCur {
			sellAmount = b.Free
			break
		}
	}

	if sellAmount <= 0 {
		return fmt.Errorf("no balance of %s to sell", baseCur)
	}

	// Determine sell side (opposite of buy side)
	sellSide := "sell"
	if plan.SpotSide == "sell" {
		sellSide = "buy"
	}

	// Create sell order
	order, err := tp.CreateOrder(ctx, plan.SpotSymbol, "market", sellSide, sellAmount, nil, nil)
	if err != nil {
		return fmt.Errorf("sell order failed: %w", err)
	}

	// Record the leg
	leg := &deltaneutral.ExecutionLeg{
		ExecutionID:     exec.ID,
		LegType:         string(deltaneutral.LegTypeSpot),
		Provider:        plan.SpotProvider,
		Account:         plan.SpotAccount,
		Symbol:          plan.SpotSymbol,
		Side:            sellSide,
		OrderType:       "market",
		RequestedAmount: sellAmount,
		OrderID:         orderID(order),
		State:           string(deltaneutral.LegStateFilled),
		FilledQuantity:  sellAmount,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	_, err = t.store.SaveExecutionLeg(ctx, leg)
	return err
}

// formatUnwindReview formats the unwind review for dry-run output.
func formatUnwindReview(plan *deltaneutral.Plan) string {
	return fmt.Sprintf(
		"Unwind review (DRY-RUN):\n"+
			"  Plan:                    %s (ID %d)\n"+
			"  Status:                  %s\n\n"+
			"Leg 1 (Futures Close):\n"+
			"  Provider/Account:        %s / %s\n"+
			"  Symbol:                  %s\n"+
			"  Side:                    (opposite of %s)\n"+
			"  Amount:                  (current position)\n"+
			"  Order Type:              market (reduce-only)\n\n"+
			"Leg 2 (Spot Sell):\n"+
			"  Provider/Account:        %s / %s\n"+
			"  Symbol:                  %s\n"+
			"  Side:                    (opposite of %s)\n"+
			"  Amount:                  (available balance)\n"+
			"  Order Type:              market\n\n"+
			"Estimated Closure Costs:\n"+
			"  Est. Exit Cost (USDT):   %.2f\n",
		plan.Name, plan.ID, plan.Status,
		plan.FuturesProvider, plan.FuturesAccount,
		plan.FuturesSymbol, plan.FuturesSide,
		plan.SpotProvider, plan.SpotAccount,
		plan.SpotSymbol, plan.SpotSide,
		plan.EstimatedExitCostUSDT)
}
