package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/bus"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
	"github.com/cryptoquantumwave/khunquant/pkg/logger"
)

// exposureRiskStates are non-terminal execution states in which one or both
// legs may already exist on the exchange. An execution found in one of these
// states at boot means a previous run died mid-flight and the position may be
// half-open: one leg exposed to the market with no hedge.
var exposureRiskStates = map[deltaneutral.ExecutionState]bool{
	deltaneutral.ExecutionStatePlacingFirstLeg:  true,
	deltaneutral.ExecutionStateFirstLegFilled:   true,
	deltaneutral.ExecutionStatePlacingSecondLeg: true,
	deltaneutral.ExecutionStateSecondLegFailed:  true,
	deltaneutral.ExecutionStateRecoveryRequired: true,
	deltaneutral.ExecutionStateUnwinding:        true,
}

// reconcileDeltaNeutralExecutions scans the execution store for executions
// stranded in a non-terminal state and alerts the plan's notify channel for
// any that may carry market exposure. Pre-trade states (pending, validating,
// awaiting_approval) have no orders on the exchange and are only logged.
func reconcileDeltaNeutralExecutions(ctx context.Context, dnStore *deltaneutral.Store, msgBus *bus.MessageBus) {
	scanCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	execs, err := dnStore.ListNonTerminalExecutions(scanCtx)
	if err != nil {
		logger.ErrorCF("dn-reconcile", "Boot execution scan failed", map[string]any{"error": err.Error()})
		return
	}
	if len(execs) == 0 {
		return
	}

	for _, exec := range execs {
		state := deltaneutral.ExecutionState(exec.State)
		if !exposureRiskStates[state] {
			logger.WarnCF("dn-reconcile", "Stale pre-trade execution found at boot", map[string]any{
				"execution_id": exec.ID, "plan_id": exec.PlanID, "state": exec.State,
			})
			continue
		}

		logger.ErrorCF("dn-reconcile", "Execution stranded mid-flight — position may be half-open", map[string]any{
			"execution_id": exec.ID, "plan_id": exec.PlanID, "state": exec.State,
			"requested_at": exec.RequestedAt,
		})

		plan, planErr := dnStore.GetPlan(scanCtx, exec.PlanID)
		if planErr != nil {
			logger.ErrorCF("dn-reconcile", "Cannot load plan for stranded execution", map[string]any{
				"execution_id": exec.ID, "plan_id": exec.PlanID, "error": planErr.Error(),
			})
			continue
		}
		if plan.NotifyChannel == "" {
			logger.WarnCF("dn-reconcile", "Plan has no notify channel; stranded-execution alert not delivered", map[string]any{
				"execution_id": exec.ID, "plan_id": exec.PlanID,
			})
			continue
		}

		msg := fmt.Sprintf(
			"🚨 CRITICAL: delta-neutral plan %d (%s) has execution %d stranded in state %q from a previous run (requested %s). "+
				"The position may be HALF-OPEN — one leg exposed with no hedge. "+
				"Verify both legs on %s/%s for %s, then unwind or re-hedge.",
			plan.ID, plan.Name, exec.ID, exec.State, exec.RequestedAt.UTC().Format(time.RFC3339),
			plan.SpotProvider, plan.FuturesProvider, plan.FuturesSymbol,
		)

		deliverCtx, deliverCancel := context.WithTimeout(ctx, 5*time.Second)
		if pubErr := msgBus.PublishOutbound(deliverCtx, bus.OutboundMessage{
			Channel: plan.NotifyChannel,
			ChatID:  plan.NotifyChatID,
			Content: msg,
		}); pubErr != nil {
			logger.ErrorCF("dn-reconcile", "Failed to deliver stranded-execution alert", map[string]any{
				"execution_id": exec.ID, "plan_id": exec.PlanID, "channel": plan.NotifyChannel, "error": pubErr.Error(),
			})
		}
		deliverCancel()
	}
}
