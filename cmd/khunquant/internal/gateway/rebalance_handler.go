package gateway

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/bus"
	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/cron"
	"github.com/cryptoquantumwave/khunquant/pkg/logger"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
	"github.com/cryptoquantumwave/khunquant/pkg/rebalance"
)

// handleRebalanceJob is the cron dispatcher for rebalancing plans (job name
// prefix "rebal:"). Fully deterministic: fetch balances + prices, evaluate
// drift in pure Go, then either notify the proposal (mode=alert) or execute
// it (mode=auto). No LLM in the loop.
func handleRebalanceJob(
	ctx context.Context,
	job *cron.CronJob,
	cfg *config.Config,
	store *rebalance.Store,
	msgBus *bus.MessageBus,
) (string, error) {
	parts := strings.SplitN(job.Name, ":", 3)
	if len(parts) < 2 {
		return "", fmt.Errorf("rebalance: malformed job name %q", job.Name)
	}
	planID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("rebalance: parse plan_id from %q: %w", job.Name, err)
	}
	plan, ok := store.Get(planID)
	if !ok {
		logger.WarnCF("rebalance", "Plan not found for cron job, skipping", map[string]any{
			"plan_id": planID, "job": job.Name,
		})
		return "plan not found", nil
	}
	if !plan.Enabled {
		return "plan disabled", nil
	}

	deliver := func(msg string) {
		if plan.NotifyChannel == "" {
			logger.WarnCF("rebalance", "No notify channel; result not delivered", map[string]any{"plan_id": plan.ID})
			return
		}
		dctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := msgBus.PublishOutbound(dctx, bus.OutboundMessage{
			Channel: plan.NotifyChannel,
			ChatID:  plan.NotifyChatID,
			Content: msg,
		}); err != nil {
			logger.WarnCF("rebalance", "Failed to deliver rebalance message", map[string]any{
				"plan_id": plan.ID, "error": err.Error(),
			})
		}
	}

	p, err := broker.CreateProviderForAccount(plan.Provider, plan.Account, cfg)
	if err != nil {
		deliver(fmt.Sprintf("⚠ Rebalance %q: provider error: %v", plan.Name, err))
		return "", err
	}
	pp, ok := p.(broker.PortfolioProvider)
	if !ok {
		deliver(fmt.Sprintf("⚠ Rebalance %q: provider %s cannot read balances", plan.Name, plan.Provider))
		return "", fmt.Errorf("provider %s is not a PortfolioProvider", plan.Provider)
	}

	states, err := rebalance.FetchStates(ctx, pp, plan)
	if err != nil {
		// Data failure must alert, never silently skip.
		deliver(fmt.Sprintf("⚠ Rebalance %q: data fetch failed: %v", plan.Name, err))
		return "", err
	}
	prop, err := rebalance.Evaluate(states, plan)
	if err != nil {
		deliver(fmt.Sprintf("⚠ Rebalance %q: evaluation failed: %v", plan.Name, err))
		return "", err
	}

	if !prop.NeedsRebalance {
		// Within tolerance — stay silent (this runs every few hours).
		return fmt.Sprintf("within tolerance (max drift %.2f pts)", prop.MaxDriftPts), nil
	}

	if plan.Mode != rebalance.ModeAuto {
		deliver(rebalance.FormatProposal(plan, prop) +
			fmt.Sprintf("\nสั่ง execute ได้ด้วย: rebalance_execute plan_id=%d confirm=true", plan.ID))
		return "alerted proposal", nil
	}

	// AUTO mode: run the same hard gates as the manual execute tool.
	if err := broker.CheckPermission(cfg, plan.Provider, plan.Account, config.ScopeTrade); err != nil {
		deliver(fmt.Sprintf("⚠ Rebalance %q (auto): blocked by permission: %v", plan.Name, err))
		return "", err
	}
	if err := broker.GlobalLossTracker.CheckDailyLoss(cfg.TradingRisk.DailyLossLimitUSD); err != nil {
		deliver(fmt.Sprintf("⚠ Rebalance %q (auto): blocked by daily loss limit: %v", plan.Name, err))
		return "", err
	}
	if !broker.DefaultLimiter.Allow(plan.Provider) {
		deliver(fmt.Sprintf("⚠ Rebalance %q (auto): provider rate limit hit — will retry next tick", plan.Name))
		return "rate limited", nil
	}
	tp, ok := p.(broker.TradingProvider)
	if !ok {
		deliver(fmt.Sprintf("⚠ Rebalance %q (auto): provider %s does not support trading", plan.Name, plan.Provider))
		return "", fmt.Errorf("provider %s is not a TradingProvider", plan.Provider)
	}

	results, execErr := rebalance.ExecuteProposal(ctx, tp, prop)
	report := rebalance.FormatProposal(plan, prop) + "\n" + rebalance.FormatResults(plan, results, execErr)
	deliver(report)
	if execErr != nil {
		return "", execErr
	}
	logger.InfoCF("rebalance", "Auto rebalance executed", map[string]any{
		"plan_id": plan.ID, "trades": len(results),
	})
	return fmt.Sprintf("executed %d trades", len(results)), nil
}
