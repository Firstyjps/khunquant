package rebalance

import (
	"context"
	"fmt"
	"strings"

	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// FetchStates pulls live balances and prices for a plan's target assets.
// Amounts include locked balances (allocation reflects total holdings);
// execution can still fail on locked funds and reports honestly.
func FetchStates(ctx context.Context, p broker.PortfolioProvider, plan Plan) ([]AssetState, error) {
	bals, err := p.GetBalances(ctx)
	if err != nil {
		return nil, fmt.Errorf("balances: %w", err)
	}
	amounts := map[string]float64{}
	for _, b := range bals {
		amounts[strings.ToUpper(b.Asset)] += b.Free + b.Locked
	}

	quote := strings.ToUpper(plan.Quote)
	var states []AssetState
	for asset := range plan.Targets {
		asset = strings.ToUpper(asset)
		st := AssetState{Asset: asset, Amount: amounts[asset]}
		if asset == quote {
			st.PriceQuote = 1
		} else {
			price, err := p.FetchPrice(ctx, asset, quote)
			if err != nil {
				return nil, fmt.Errorf("price %s/%s: %w", asset, quote, err)
			}
			st.PriceQuote = price
		}
		states = append(states, st)
	}
	return states, nil
}

// TradeResult is the outcome of one executed proposal trade.
type TradeResult struct {
	Trade
	OrderID string
	Status  string
	Err     string
}

// ExecuteProposal places the proposal's trades sequentially as market orders
// (sells first — Evaluate orders them that way). Execution stops at the first
// failure so a broken leg never cascades; completed fills are still returned.
func ExecuteProposal(ctx context.Context, tp broker.TradingProvider, prop Proposal) ([]TradeResult, error) {
	var results []TradeResult
	for _, t := range prop.Trades {
		order, err := tp.CreateOrder(ctx, t.Symbol, "market", t.Side, t.AmountAsset, nil, nil)
		res := TradeResult{Trade: t}
		if err != nil {
			res.Err = err.Error()
			results = append(results, res)
			return results, fmt.Errorf("trade %s %s failed (stopped before remaining trades): %w", t.Side, t.Symbol, err)
		}
		if order.Id != nil {
			res.OrderID = *order.Id
		}
		if order.Status != nil {
			res.Status = *order.Status
		}
		results = append(results, res)
	}
	return results, nil
}

// FormatResults renders execution results for user delivery.
func FormatResults(plan Plan, results []TradeResult, execErr error) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Rebalance execution: %s (%s/%s)\n", plan.Name, plan.Provider, accountOrDefault(plan.Account)))
	for i, r := range results {
		if r.Err != "" {
			sb.WriteString(fmt.Sprintf("  %d. ❌ %s %.6f %s — %s\n", i+1, strings.ToUpper(r.Side), r.AmountAsset, r.Symbol, r.Err))
			continue
		}
		sb.WriteString(fmt.Sprintf("  %d. ✅ %s %.6f %s (≈ %.2f %s) id=%s %s\n",
			i+1, strings.ToUpper(r.Side), r.AmountAsset, r.Symbol, r.NotionalQuote, plan.Quote, r.OrderID, r.Status))
	}
	if execErr != nil {
		sb.WriteString(fmt.Sprintf("⚠ Stopped early: %v — check open orders and holdings before retrying.\n", execErr))
	}
	return sb.String()
}
