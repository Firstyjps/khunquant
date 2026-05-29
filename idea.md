# KhunQuant Product Design: Delta-Neutral Funding Strategy Assistant

## 1. Product Concept

KhunQuant should support an AI-assisted delta-neutral funding strategy workflow for users who want to earn perpetual futures funding while controlling liquidation, exchange, and execution risk.

This is not a blind auto-trading bot. It is a KhunQuant-native strategy assistant that scans, plans, monitors, explains, alerts, and only executes when the configured safety policy allows it.

Core promise:

> Do not chase the highest funding rate. Use KhunQuant to find funding opportunities that survive fees, slippage, margin risk, delta drift, and funding reversal.

Primary MVP use case:

> Find a Binance/OKX delta-neutral opportunity, build a spot + perpetual hedge plan, estimate breakeven and risk, then monitor it with alerts and approval-based execution.

## 2. System Fit In KhunQuant

KhunQuant already has the system primitives needed for this product:

- Orchestrator: the existing agent loop that understands user intent, calls tools, applies workspace skills, and responds through web/chat channels.
- Skills: reusable strategy instructions in `workspace/skills`, including trading, PnL, funding-rate analysis, DCA, alerts, and future delta-neutral workflows.
- Tools: existing market, futures, order, PnL, snapshot, alert, cron, and dashboard tools.
- Policies: confirmation-before-execution rules, account permissions, tool enable flags, and `trading_risk` controls.
- Storage: existing memory snapshots, DCA stores, cron jobs, logs, and a dedicated delta-neutral SQLite store modeled after the DCA feature.
- Interfaces: web console plus Telegram, LINE, Discord, Slack, WhatsApp, and other configured channels.

The delta-neutral product should be implemented as a first-class KhunQuant strategy workflow, not as a separate orchestration system.

## 3. Supported MVP Exchanges

MVP futures venues:

- Binance USDT perpetuals
- OKX USDT swaps

MVP spot venues:

- Binance spot
- OKX spot where supported by configured account and tools

Explicit exclusions:

- Bitkub and BinanceTH are spot-only in KhunQuant and must not be used for futures legs.
- SET equities are outside this strategy.
- Full-auto trade opening is outside MVP.

## 4. Product Roles

The original multi-agent concept maps into KhunQuant as specialized workflows. These can initially be implemented as skills and tool-calling patterns, then promoted into typed services if needed.

### 4.1 KhunQuant Orchestrator

Responsibilities:

- Understand the user's request.
- Decide whether the request is scan, plan, monitor, execute, rebalance, or exit.
- Call the correct tools in a safe order.
- Combine funding, market, risk, and portfolio data.
- Enforce confirmation and risk policies.
- Produce a clear recommendation with assumptions and next actions.

Example user request:

> Find the best ETH delta-neutral opportunity using Binance and OKX.

Expected flow:

1. Validate relevant futures symbols with `futures_validate_market`.
2. Fetch current funding with `futures_get_funding`.
3. Fetch funding history with `funding_rate_history`.
4. Fetch spot/perp prices and order books with market tools.
5. Estimate fees, slippage, breakeven, and liquidation buffer.
6. Return a ranked opportunity and explain the risk.

### 4.2 Opportunity Scanner Workflow

Purpose:

Find and rank assets where a spot position plus opposite perpetual position may produce attractive positive carry.

Capabilities:

- Asset scanning
- Binance vs OKX comparison
- Funding opportunity detection
- Spread and basis analysis
- Order book/liquidity review
- Slippage estimation
- Opportunity scoring

Existing tools to use:

- `get_ticker`
- `get_tickers`
- `get_orderbook`
- `get_markets`
- `futures_validate_market`
- `futures_get_funding`
- `funding_rate_history`

Output shape:

```text
Opportunity: ETH Binance Spot + OKX Short Perp
Current funding: +0.021% per period
7D average funding: +0.014% per period
Estimated APR: 18.2%
Spread/slippage risk: Medium
Liquidity: Acceptable
Opportunity score: 82/100
Recommendation: Review plan with low leverage and reserve margin.
```

### 4.3 Funding Analysis Workflow

Purpose:

Decide whether the funding rate is stable enough to justify a plan.

Capabilities:

- Current funding interpretation
- 3D, 7D, and 14D average review
- Min/max funding review
- Funding volatility analysis
- Funding trend detection
- Positive-funding ratio estimation
- Funding reversal warning

Existing tools to use:

- `funding_rate_history`
- `futures_get_funding`
- `get_ohlcv`

Output shape:

```text
Funding Analysis: ETH/USDT:USDT on OKX
Current funding: +0.021%
3D average: +0.018%
7D average: +0.014%
14D average: +0.011%
Funding trend: Improving
Risk: Funding has turned negative before; add reversal alert.
```

Product interpretation:

- Attractive: current funding is positive and above recent averages.
- Watch: funding is positive but unstable or near zero.
- Block: funding is negative, too volatile, or unavailable.

### 4.4 Plan Builder Workflow

Purpose:

Convert an opportunity into a concrete delta-neutral plan.

Capabilities:

- Strategy selection
- Capital allocation
- Portfolio selection for each leg
- Spot/futures position sizing
- Breakeven estimation
- Fee and slippage estimation
- Reserve margin planning
- Entry rule and exit rule design
- Plan status classification

Existing tools to use:

- `list_portfolios`
- `get_ticker`
- `get_orderbook`
- `futures_validate_market`
- `futures_estimate_funding_fee`
- `futures_risk_summary`
- `get_assets_list`
- `get_total_value`
- `take_snapshot`

Output shape:

```text
Plan: ETH Binance Spot + OKX Short Perp
Capital: 10,000 USDT
Spot leg portfolio: Binance main
Futures leg portfolio: OKX futures-main
Spot leg: Buy 5,000 USDT ETH on Binance main
Futures leg: Short 5,000 USDT ETH/USDT:USDT on OKX futures-main
Reserve margin: 2,000 USDT recommended
Monitor interval: 5 minutes
Estimated entry cost: 13 USDT
Expected funding: 7 USDT/day
Breakeven: 1.9 days
Open condition: liquidation buffer remains above 30%
Status: Ready for review
```

### 4.5 Risk Workflow

Purpose:

Protect the user from liquidation, margin stress, exchange instability, and unbalanced exposure.

Capabilities:

- Liquidation-distance review
- Margin health review
- Delta drift calculation
- Position mismatch detection
- Stress scenario analysis
- Fee/slippage risk review
- Exchange support and maintenance review
- Risk score generation

Existing tools to use:

- `futures_risk_summary`
- `futures_get_positions`
- `futures_estimate_funding_fee`
- `get_ticker`
- `get_orderbook`
- `take_snapshot`
- `get_pnl_summary`

Risk labels:

- Safe
- Watch
- Danger
- Critical

Output shape:

```text
Risk Check: ETH Binance Spot + OKX Short Perp
Margin risk: Watch
Liquidation distance: 24.7%
Delta drift: 2.1%
Status: Position is acceptable, but margin risk is rising.
Suggested action: Add 300-500 USDT margin if liquidation distance drops below 20%.
```

### 4.6 Execution Workflow

Purpose:

Prepare and, after approval, execute both legs safely.

Capabilities:

- Pre-trade validation
- Spot order preparation
- Futures order preparation
- Two-leg execution management
- Partial-fill handling
- Failure recovery
- Fill verification
- Emergency close suggestion

Existing tools to use:

- `get_assets_list`
- `get_order_rate_status`
- `create_order`
- `get_order`
- `futures_open_position`
- `futures_get_order`
- `futures_get_positions`
- `futures_close_position`
- `futures_reduce_position`
- `futures_emergency_flatten`

Required execution policy:

- Present asset, quantity, side, estimated price, estimated notional, fees, slippage, and risk.
- Ask for explicit confirmation before live execution.
- Recheck price, funding, liquidity, balances, and margin immediately before execution.
- If one leg fails, stop new orders, inspect exposure, and recommend hedge, rollback, or emergency action.

Execution approval screen:

```text
Ready to Execute
Plan: Buy ETH spot on Binance + short ETH perpetual on OKX
Capital: 10,000 USDT
Estimated entry cost: 13 USDT
Expected daily funding: 7 USDT
Liquidation buffer: 32%
Required approval: Confirm open position
```

### 4.7 Position Monitor Workflow

Purpose:

Track active plans and recommend hold, rebalance, add margin, reduce, or exit.

Capabilities:

- Position state tracking
- Spot/futures exposure comparison
- Real-time PnL tracking
- Funding received/paid tracking
- Margin monitoring
- Plan health scoring
- Action recommendation

Monitoring architecture:

- The normal monitoring loop must be deterministic code, not an LLM loop.
- Each active plan has a configured monitoring interval and threshold policy.
- A functional gate reads live data, computes risk/funding/delta state, writes monitor snapshots, and compares the result with thresholds.
- The LLM/agent is invoked only after a threshold is met, a data failure needs interpretation, or a user asks for explanation.
- This prevents high-frequency checks such as every 30 seconds or every minute from burning tokens.

Supported monitor intervals:

- 30 seconds
- 1 minute
- 3 minutes
- 5 minutes
- 10 minutes
- 15 minutes
- 30 minutes
- 1 hour
- 2 hours
- 3 hours
- 4 hours
- 8 hours
- 1 day

Default monitor interval:

> 5 minutes

Existing tools to use:

- `futures_get_positions`
- `futures_get_funding`
- `futures_risk_summary`
- `futures_estimate_funding_fee`
- `get_pnl_summary`
- `get_pnl_detail`
- `take_snapshot`
- `query_snapshots`
- `snapshot_summary`

Output shape:

```text
Active Plan Summary
Plan: ETH Binance Spot + OKX Short Perp
Status: Active
Total invested: 10,000 USDT
Current net profit: +42.80 USDT
Funding received: +61.20 USDT
Fees and slippage: -13.00 USDT
Unrealized hedge PnL: -5.40 USDT
Margin risk: Watch
Recommendation: Hold, but keep margin alert active.
```

### 4.8 Alert Workflow

Purpose:

Notify the user when opportunity, risk, or exit conditions change.

Capabilities:

- Margin danger alert
- Liquidation danger alert
- Funding reversal alert
- Delta drift alert
- Profit target alert
- Exit suggestion alert
- Execution failure alert
- Daily/weekly summary alert

Existing tools and services to use:

- `set_price_alert`
- `set_indicator_alert`
- `cron`
- channel message delivery
- dashboard events/logs

Example alerts:

```text
Margin Danger Alert
Plan: ETH Binance Spot + OKX Short Perp
Problem: OKX liquidation distance dropped below 20%.
Suggested action: Add margin, reduce position, or close the plan.
```

```text
Funding Reversal Alert
Plan: ETH Binance Spot + OKX Short Perp
Problem: ETH funding turned negative for 2 consecutive cycles.
Suggested action: Review exit plan.
```

### 4.9 Reporting Workflow

Purpose:

Explain strategy performance in language the user can act on.

Capabilities:

- Plan summary
- Funding income report
- Realized and unrealized PnL explanation
- Risk explanation
- Daily report
- Weekly performance report
- CSV/dashboard export in future phases

Existing tools to use:

- `get_pnl_summary`
- `get_pnl_detail`
- `snapshot_summary`
- `query_snapshots`
- `futures_get_funding`
- `futures_get_positions`

Daily report shape:

```text
Daily Delta-Neutral Report
Active plans: 3
Total invested: 25,000 USDT
Net profit: +86.40 USDT
Funding received: +112.70 USDT
Fees: -18.30 USDT
Highest risk plan: SOL Binance Spot + OKX Short Perp
Recommendation: Keep ETH and BTC active. Review SOL because margin risk is rising.
```

## 5. Conceptual Product Objects

These objects describe the product shape. They do not require immediate public API changes for the document rewrite.

### 5.1 DeltaNeutralOpportunity

Represents a candidate strategy before a user creates a plan.

Fields:

- `asset`
- `spot_provider`
- `spot_account`
- `futures_provider`
- `futures_account`
- `spot_symbol`
- `futures_symbol`
- `funding_current`
- `funding_avg_3d`
- `funding_avg_7d`
- `funding_avg_14d`
- `estimated_apr`
- `spread_bps`
- `slippage_estimate`
- `liquidity_label`
- `risk_label`
- `score`
- `recommendation`

### 5.2 DeltaNeutralPlan

Represents a reviewed strategy that may be monitored or executed. A plan must explicitly bind each leg to a selected KhunQuant portfolio/account so the user controls which account performs the spot leg and which account performs the futures leg.

Fields:

- `id`
- `name`
- `asset`
- `status`
- `mode`
- `capital_usdt`
- `spot_leg_portfolio`
- `futures_leg_portfolio`
- `spot_leg`
- `futures_leg`
- `reserve_margin_usdt`
- `monitor_interval`
- `estimated_entry_cost_usdt`
- `expected_daily_funding_usdt`
- `breakeven_days`
- `entry_rules`
- `exit_rules`
- `risk_policy`
- `created_at`
- `updated_at`

### 5.3 DeltaNeutralStore

Delta-neutral plans should have their own persistent storage, following the same product pattern as DCA.

Storage requirements:

- Store path: `workspace/memory/delta_neutral/delta_neutral.db`.
- SQLite schema with WAL mode, foreign keys, idempotent migrations, and plan/execution/monitoring history tables.
- A Go package equivalent to `pkg/dca` for typed plan, execution, monitor snapshot, alert event, and store APIs.
- Backend APIs equivalent to the DCA read APIs, for example `/api/agent/delta-neutral/plans`, `/plans/{id}`, `/plans/{id}/executions`, and `/plans/{id}/monitor-snapshots`.
- A web UI panel equivalent to the DCA panel so users can inspect active plans, leg portfolios, health, execution history, and risk events.

Core tables:

- `delta_neutral_plans`: plan configuration, selected portfolios, risk thresholds, monitor interval, status, and timestamps.
- `delta_neutral_executions`: spot/futures order attempts, fills, partial fills, errors, and recovery actions.
- `delta_neutral_monitor_snapshots`: deterministic monitor outputs such as funding, delta drift, liquidation distance, margin risk, PnL, and health score.
- `delta_neutral_alerts`: threshold events that triggered agent/user notification.

### 5.4 PlanHealthScore

Represents the current quality of an active or pending plan.

Score bands:

- 90-100: Excellent
- 75-89: Healthy
- 60-74: Watch
- 40-59: Danger
- 0-39: Critical

Score components:

- Funding health
- Margin health
- Delta balance
- Fee efficiency
- Liquidity health
- Exchange health
- Profit progress

Example:

```text
Plan Health: 78/100
Funding health: Good
Margin health: Watch
Delta balance: Good
Profit progress: Above breakeven
Recommendation: Hold, but monitor margin risk.
```

### 5.5 ExecutionMode

Supported modes:

- Monitor: scan, analyze, create plans, send alerts, and report. No live orders.
- Approval: prepare orders and execute only after explicit confirmation.
- Semi-auto: future mode for pre-approved small maintenance actions.
- Full-auto: future mode requiring strict capital, loss, and emergency policies.

Recommended default:

> Approval mode for execution, Monitor mode for new users.

### 5.6 RiskPolicy

Plan-level policy should include:

- Minimum funding threshold
- Maximum breakeven days
- Minimum liquidation buffer
- Maximum allowed slippage
- Maximum capital per plan
- Maximum single-asset allocation
- Maximum leverage
- Required reserve margin
- Funding reversal trigger
- Profit target trigger
- Emergency exit behavior
- Monitor interval, defaulting to 5 minutes
- Agent invocation policy: invoke the agent only when deterministic thresholds fire or the user asks for explanation

## 6. Core User Flows

### 6.1 Scan Opportunity

User asks:

> Find the best delta-neutral opportunity.

KhunQuant flow:

1. Discover available Binance/OKX accounts and symbols.
2. Validate futures markets.
3. Fetch current funding and funding history.
4. Fetch spot/perp prices and order books.
5. Estimate carry, slippage, liquidity, and risk.
6. Rank opportunities.
7. Present the best candidates with a recommendation.

Final output:

```text
Best Opportunity: ETH Binance Spot + OKX Short Perp
Score: 82/100
Expected profit: 7.00 USDT/day
Breakeven: 1.9 days
Risk: Medium
Recommendation: Create a plan, use low leverage, and keep reserve margin.
```

### 6.2 Create Plan

User asks:

> Create a plan with 10,000 USDT.

KhunQuant flow:

1. Validate selected opportunity.
2. Ask the user to select the portfolio/account for the spot leg.
3. Ask the user to select the portfolio/account for the futures leg.
4. Confirm account balances and portfolio exposure for both selected leg portfolios.
5. Build spot and futures leg sizing.
6. Set monitoring interval, defaulting to 5 minutes when the user does not choose one.
7. Estimate fees, slippage, funding income, and breakeven.
8. Run risk checks.
9. Save the plan into the dedicated delta-neutral SQLite store and present it as ready for review.

Final output:

```text
Plan Created
Spot leg portfolio: Binance main
Futures leg portfolio: OKX futures-main
Buy: 5,000 USDT ETH spot on Binance main
Short: 5,000 USDT ETH perpetual on OKX futures-main
Reserve: 2,000 USDT suggested
Monitor interval: 5 minutes
Breakeven: 1.9 days
Liquidation buffer: 32%
Status: Ready for review
```

### 6.3 Execute Plan

User action:

> Open the position.

KhunQuant flow:

1. Recheck current prices.
2. Recheck funding and next funding time.
3. Recheck order book liquidity.
4. Recheck balances and margin.
5. Show exact execution details.
6. Wait for explicit user confirmation.
7. Place both legs.
8. Verify order status and filled exposure.
9. Mark plan active or failed.

Failure handling:

- If the spot leg fills and futures leg fails, stop further orders and recommend hedge or unwind.
- If the futures leg fills and spot leg fails, stop further orders and recommend hedge or reduce-only close.
- If protection orders fail, surface an unprotected-position warning and recommend immediate protection or close.

### 6.4 Monitor Plan

Schedule:

- Per plan, using the user's selected monitor interval.
- Every funding cycle for carry and reversal checks.
- Daily for reporting.

Allowed plan monitor intervals:

- 30 seconds
- 1 minute
- 3 minutes
- 5 minutes
- 10 minutes
- 15 minutes
- 30 minutes
- 1 hour
- 2 hours
- 3 hours
- 4 hours
- 8 hours
- 1 day

Default:

> 5 minutes

KhunQuant flow:

1. Read active plans.
2. For each due plan, fetch spot balances from the selected spot-leg portfolio and futures positions from the selected futures-leg portfolio.
3. Recalculate delta drift.
4. Recheck margin and liquidation distance.
5. Estimate upcoming funding.
6. Compute the plan health score and compare deterministic values against stored thresholds.
7. Write a monitor snapshot to the delta-neutral SQLite store.
8. Update dashboard status.
9. If no threshold is breached, stop without invoking the LLM.
10. If a threshold is breached, invoke the agent to explain the condition and alert the user.

### 6.5 Exit Plan

Exit triggers:

- Funding turns negative.
- Margin risk becomes critical.
- Delta drift is too high.
- Profit target is reached.
- Breakeven is unlikely within policy.
- Exchange health becomes unacceptable.
- User manually requests exit.

KhunQuant flow:

1. Explain exit reason.
2. Estimate exit costs and expected final result.
3. Ask for explicit user approval.
4. Close futures leg with reduce-only order.
5. Sell or rebalance spot leg if approved.
6. Verify final exposure.
7. Record funding, PnL, and plan outcome.

## 7. Safety Rules

The orchestrator must block or require review when:

- Funding is negative or unavailable.
- Breakeven is too long.
- Slippage is above policy.
- Liquidity is too low.
- Margin buffer is too small.
- Liquidation distance is too close.
- Position size exceeds policy.
- User has insufficient reserve margin.
- Exchange API is unstable or maintenance is detected.
- Account permissions do not allow the requested action.
- Required futures symbol validation fails.

Example blocked action:

```text
Action Blocked
Reason: OKX ETH funding is attractive, but liquidation buffer is only 9%.
Recommendation: Reduce leverage or increase margin before opening.
```

Execution invariants:

- No trade, close, rebalance, or transfer without explicit user confirmation in MVP.
- All live futures mutation tools must respect `trading_risk.allow_leverage`.
- All order tools must respect account permissions and rate limits.
- The assistant must use live tools for market data and portfolio state.
- The assistant must state stale or unavailable data clearly.

## 8. Memory And Preferences

KhunQuant should remember user preferences when available:

- Preferred exchanges
- Risk profile
- Maximum capital per plan
- Minimum expected APR
- Maximum breakeven days
- Minimum liquidation buffer
- Maximum allowed slippage
- Preferred execution mode
- Alert preferences
- Past plan performance

Example memory:

```text
User prefers:
- Binance and OKX only
- Breakeven under 3 days
- Liquidation buffer above 25%
- Approval required before live execution
- Alert through Telegram and dashboard
```

## 9. Dashboard Product Surface

The web console should provide a dedicated Delta-Neutral page or panel, following the DCA product pattern of SQLite-backed backend APIs plus a web UI plan list/detail view.

Primary views:

- Opportunity table: asset, venue pair, current funding, 7D average, estimated APR, liquidity, risk, score.
- Plan detail: capital, selected spot-leg portfolio, selected futures-leg portfolio, spot leg, futures leg, reserve, breakeven, monitor interval, health score, entry/exit rules.
- Active monitoring: spot value, futures notional, delta drift, funding received, margin risk, liquidation distance.
- Monitor snapshots: timestamped deterministic checks, threshold results, health score, and whether the agent was invoked.
- Alerts: funding reversal, margin danger, profit target, execution failure.
- Execution review: exact orders, estimated costs, risk checklist, confirmation controls.

MVP should include persistent plan storage from the start. The first web UI can be compact like the existing DCA memory panel: plan list on the left, selected plan detail and history on the right.

## 10. MVP Scope

MVP includes:

- Scan Binance/OKX funding opportunities.
- Analyze current and historical funding.
- Build a spot + perpetual delta-neutral plan.
- Require the user to select which portfolio/account performs the spot leg and which portfolio/account performs the futures leg.
- Persist plans in a dedicated SQLite store modeled after DCA.
- Show plans and monitoring history in a web UI panel.
- Estimate fees, slippage, expected funding, and breakeven.
- Check liquidation/margin risk.
- Monitor active plans with deterministic functional gates, not an LLM loop.
- Support monitor intervals of 30s, 1min, 3min, 5min, 10min, 15min, 30min, 1h, 2h, 3h, 4h, 8h, and 1d, defaulting to 5min.
- Invoke the agent only when thresholds are met or user explanation is needed.
- Send alerts after threshold breaches.
- Execute only in Approval mode.
- Report funding, PnL, and plan health.

MVP should not include:

- Full-auto opening of new positions.
- Automatic major position closes.
- Cross-asset portfolio optimizer.
- Historical backtesting engine.
- Borrow/lending legs.
- SET, Bitkub futures, or BinanceTH futures.

## 11. Future Enhancements

### Portfolio Allocation Workflow

Find the best split across BTC, ETH, SOL, and other assets so the user does not allocate too much capital to one funding opportunity.

### Backtesting Workflow

Test whether funding was stable enough historically before recommending real capital.

### Exchange Health Workflow

Monitor API status, abnormal spread, order book depth, and maintenance warnings before execution.

### Auto-Rebalance Workflow

Keep spot and futures exposure balanced when delta drift exceeds policy.

### Emergency Workflow

Protect capital when liquidation risk becomes critical.

Possible actions:

- Send urgent alert.
- Pause strategy.
- Suggest adding margin.
- Suggest reducing position.
- Suggest closing the plan.
- In future full-auto mode, execute only if emergency auto-close is explicitly enabled.

## 12. Product Positioning

KhunQuant should position this feature as:

> An AI funding strategy assistant that scans, plans, monitors, and protects delta-neutral crypto positions across Binance and OKX.

It should not be positioned as:

> A guaranteed yield product or hands-off leverage bot.

Strongest differentiator:

- Scanner finds opportunities.
- Funding workflow checks stability.
- Plan workflow estimates real profit after costs.
- Risk workflow protects against liquidation.
- Alert workflow warns before conditions break.
- Execution workflow acts only when allowed.

Final product promise:

> KhunQuant helps Thai investors reason about funding strategies with live data, local credentials, explicit approvals, and risk-first automation.
