package gateway

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	ccxt "github.com/ccxt/ccxt/go/v4"
	"github.com/cryptoquantumwave/khunquant/pkg/bus"
	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/cron"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
	"github.com/cryptoquantumwave/khunquant/pkg/logger"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
	"github.com/cryptoquantumwave/khunquant/pkg/tools"
)

// handleDeltaNeutralMonitorJob is the cron dispatcher for delta-neutral monitor plans (job name prefix "dn:").
//
// It fetches live state for both legs (spot and futures), evaluates health deterministically in pure Go,
// saves a snapshot, and escalates to the LLM if thresholds are breached.
// Alerts are always delivered via the message bus if a breach occurs, even if cronTool is nil.
func handleDeltaNeutralMonitorJob(
	ctx context.Context,
	job *cron.CronJob,
	cfg *config.Config,
	dnStore *deltaneutral.Store,
	cronTool *tools.CronTool,
	msgBus *bus.MessageBus,
) (string, error) {
	// Parse plan ID from "dn:<id>:<name>".
	parts := strings.SplitN(job.Name, ":", 3)
	if len(parts) < 2 {
		return "", fmt.Errorf("dn-monitor: malformed job name %q", job.Name)
	}
	planID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("dn-monitor: parse plan_id from %q: %w", job.Name, err)
	}

	plan, err := dnStore.GetPlan(ctx, planID)
	if err != nil {
		logger.DebugCF("dn-monitor", "Plan not found, skipping", map[string]any{
			"plan_id": planID, "job": job.Name, "error": err.Error(),
		})
		return "plan not found", nil
	}

	// Skip if plan is not monitorable.
	if !plan.Enabled {
		logger.DebugCF("dn-monitor", "Plan disabled, skipping", map[string]any{"plan_id": planID})
		return "plan disabled", nil
	}

	// Skip closed/archived/failed/draft plans.
	if plan.Status == deltaneutral.PlanStatusClosed ||
		plan.Status == deltaneutral.PlanStatusArchived ||
		plan.Status == deltaneutral.PlanStatusFailed ||
		plan.Status == deltaneutral.PlanStatusDraft {
		logger.DebugCF("dn-monitor", "Plan not monitorable (status: "+plan.Status+"), skipping", map[string]any{"plan_id": planID})
		return "plan status not monitorable", nil
	}

	now := time.Now().UTC()

	// Fetch live state for futures leg.
	futuresState := deltaneutral.FuturesState{}
	var futureFundingRate *ccxt.FundingRate // Store funding rate if available.
	futureFetcher, err := broker.CreateProviderForAccount(plan.FuturesProvider, plan.FuturesAccount, cfg)
	if err != nil {
		logger.WarnCF("dn-monitor", "Failed to create futures provider", map[string]any{
			"plan_id": planID, "provider": plan.FuturesProvider, "error": err.Error(),
		})
		futuresState.Available = false
	} else {
		futuresProv, ok := futureFetcher.(broker.FuturesProvider)
		if !ok {
			logger.WarnCF("dn-monitor", "Provider does not support futures", map[string]any{
				"plan_id": planID, "provider": plan.FuturesProvider,
			})
			futuresState.Available = false
		} else {
			// Fetch positions.
			positions, err := futuresProv.FetchFuturesPositions(ctx, []string{plan.FuturesSymbol})
			if err != nil {
				logger.WarnCF("dn-monitor", "Failed to fetch futures positions", map[string]any{
					"plan_id": planID, "error": err.Error(),
				})
				futuresState.Available = false
			} else {
				futuresState.Available = true
				// Extract position data (assume first position matches the symbol).
				if len(positions) > 0 {
					pos := positions[0]
					if pos.Contracts != nil {
						futuresState.Contracts = *pos.Contracts
					}
					if pos.MarkPrice != nil && pos.ContractSize != nil {
						futuresState.NotionalUSDT = *pos.ContractSize * futuresState.Contracts * *pos.MarkPrice
					}
					if pos.UnrealizedPnl != nil {
						futuresState.UnrealizedPnLUSDT = *pos.UnrealizedPnl
					}
					if pos.LiquidationPrice != nil {
						futuresState.LiquidationPrice = *pos.LiquidationPrice
					}
					if pos.MarginRatio != nil {
						futuresState.MarginRatioPct = *pos.MarginRatio * 100 // Convert to percentage
					}
				}
			}

			// Fetch mark price (also gets latest mark price).
			markPrice, err := futuresProv.FetchFuturesMarkPrice(ctx, plan.FuturesSymbol)
			if err != nil {
				logger.WarnCF("dn-monitor", "Failed to fetch futures mark price", map[string]any{
					"plan_id": planID, "error": err.Error(),
				})
				futuresState.Available = false
			} else {
				futuresState.MarkPrice = markPrice
			}

			// Fetch funding rate.
			fr, err := futuresProv.FetchFuturesFundingRate(ctx, plan.FuturesSymbol)
			if err != nil {
				logger.WarnCF("dn-monitor", "Failed to fetch funding rate", map[string]any{
					"plan_id": planID, "error": err.Error(),
				})
				// Don't set Available=false here; funding info is handled separately.
			} else {
				futureFundingRate = &fr
			}
		}
	}

	// Fetch live state for spot leg.
	spotState := deltaneutral.SpotState{}
	spotFetcher, err := broker.CreateProviderForAccount(plan.SpotProvider, plan.SpotAccount, cfg)
	if err != nil {
		logger.WarnCF("dn-monitor", "Failed to create spot provider", map[string]any{
			"plan_id": planID, "provider": plan.SpotProvider, "error": err.Error(),
		})
		spotState.Available = false
	} else {
		spotMD, ok := spotFetcher.(broker.MarketDataProvider)
		if !ok {
			logger.WarnCF("dn-monitor", "Provider does not support market data", map[string]any{
				"plan_id": planID, "provider": plan.SpotProvider,
			})
			spotState.Available = false
		} else {
			ticker, err := spotMD.FetchTicker(ctx, plan.SpotSymbol)
			if err != nil {
				logger.WarnCF("dn-monitor", "Failed to fetch spot ticker", map[string]any{
					"plan_id": planID, "error": err.Error(),
				})
				spotState.Available = false
			} else {
				spotState.Available = true
				if ticker.Last != nil {
					spotState.Price = *ticker.Last
				}
				// For now, use the plan's spot notional value if we can't get balance.
				// In a more complete implementation, we'd fetch the actual balance.
				spotState.Quantity = plan.SpotNotionalUSDT / spotState.Price
				spotState.ValueUSDT = plan.SpotNotionalUSDT
			}
		}
	}

	// Build funding info from futures fetching.
	fundingInfo := deltaneutral.FundingInfo{Available: false}
	if futureFundingRate != nil && futuresState.Available {
		fundingInfo.Available = true
		if futureFundingRate.FundingRate != nil {
			fundingInfo.CurrentRate = *futureFundingRate.FundingRate
			// Estimate next funding payment: contracts * mark_price * funding_rate.
			fundingInfo.EstimatedNextUSDT = futuresState.Contracts * futuresState.MarkPrice * *futureFundingRate.FundingRate
		}
		if futureFundingRate.FundingTimestamp != nil {
			fundingInfo.NextFundingTime = time.UnixMilli(int64(*futureFundingRate.FundingTimestamp)).UTC()
		}
		// For now, keep RecentRates empty; in production, you'd fetch history.
	}

	// Build evaluation input.
	input := deltaneutral.EvaluationInput{
		Plan:         *plan,
		SpotState:    spotState,
		FuturesState: futuresState,
		FundingInfo:  fundingInfo,
		Now:          now,
	}

	// Run deterministic health evaluation.
	eval := deltaneutral.Evaluate(input)

	// Always save snapshot.
	eval.Snapshot.PlanID = planID
	eval.Snapshot.CreatedAt = now
	snapID, err := dnStore.SaveSnapshot(ctx, &eval.Snapshot)
	if err != nil {
		logger.ErrorCF("dn-monitor", "Failed to save snapshot", map[string]any{
			"plan_id": planID, "error": err.Error(),
		})
		// Continue; don't fail the job.
	}

	// If no breach, return early.
	if !eval.ThresholdBreached {
		logger.DebugCF("dn-monitor", "No breach detected", map[string]any{
			"plan_id": planID,
			"health":  eval.Snapshot.HealthLabel,
			"score":   eval.Snapshot.HealthScore,
		})
		return "ok: no breach", nil
	}

	// Breach detected: build and save alert.
	logger.WarnCF("dn-monitor", "Threshold breached", map[string]any{
		"plan_id":  planID,
		"codes":    eval.BreachCodes,
		"severity": eval.Severity,
	})

	alertCode := "breach"
	if len(eval.BreachCodes) > 0 {
		alertCode = eval.BreachCodes[0]
	}

	alertMsg := fmt.Sprintf("Delta-neutral plan %d (%s) health alert: %v", planID, plan.Name, eval.BreachCodes)

	alert := &deltaneutral.Alert{
		PlanID:            planID,
		SnapshotID:        &snapID,
		TriggeredAt:       now,
		Severity:          eval.Severity,
		Code:              alertCode,
		Message:           alertMsg,
		RecommendedAction: eval.RecommendedAction,
		AgentInvoked:      false,
		DeliveredChannel:  plan.NotifyChannel,
		DeliveredChatID:   plan.NotifyChatID,
		CreatedAt:         now,
	}

	if _, err := dnStore.SaveAlert(ctx, alert); err != nil {
		logger.ErrorCF("dn-monitor", "Failed to save alert", map[string]any{
			"plan_id": planID, "error": err.Error(),
		})
		// Continue; deliver it anyway.
	}

	// Deliver alert via message bus if a channel is configured.
	if plan.NotifyChannel != "" {
		// Use a short timeout to avoid blocking the cron job.
		deliverCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := msgBus.PublishOutbound(deliverCtx, bus.OutboundMessage{
			Channel: plan.NotifyChannel,
			ChatID:  plan.NotifyChatID,
			Content: alertMsg,
		})
		if err != nil {
			logger.WarnCF("dn-monitor", "Failed to deliver alert via message bus", map[string]any{
				"plan_id": planID, "channel": plan.NotifyChannel, "error": err.Error(),
			})
		} else {
			logger.DebugCF("dn-monitor", "Alert delivered", map[string]any{
				"plan_id": planID, "channel": plan.NotifyChannel,
			})
		}
	}

	// If cronTool is available, escalate to LLM for agent explanation.
	if cronTool != nil {
		return cronTool.ExecuteJob(ctx, job), nil
	}

	// Alert delivered but no agent explanation.
	return "breach alerted (no agent)", nil
}
