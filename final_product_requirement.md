# KhunQuant Final Product Requirement: Delta-Neutral Funding Strategy Assistant

## 1. Executive Summary

KhunQuant will add a Delta-Neutral Funding Strategy Assistant for users who want to scan, plan, persist, monitor, alert, and execute approval-based spot + perpetual funding strategies across Binance and OKX.

The product is not a guaranteed-yield product and not a hands-off leverage bot. It is a local-first KhunQuant strategy module that helps the user reason with live data, selected portfolios, explicit risk thresholds, deterministic monitoring, and user-confirmed execution.

Core promise:

> Find funding opportunities that still make sense after fees, slippage, liquidation risk, delta drift, exchange risk, and funding reversal.

Primary MVP:

- Scan Binance/OKX funding opportunities over a bounded watchlist.
- Let the user select which KhunQuant portfolio/account performs the spot leg and which portfolio/account performs the futures leg.
- Create a persisted delta-neutral plan in its own SQLite store.
- Monitor active plans using deterministic code, not a high-frequency LLM loop.
- Invoke the agent only when thresholds breach, data is unavailable, or the user asks for explanation.
- Provide web UI inspection similar to the existing DCA panel.
- Support Approval-mode two-leg execution with a leg-aware state machine and recovery path.

## 2. Source Analysis: `idea.md` vs `idea-review.md`

`idea.md` defines the correct product direction:

- KhunQuant-native strategy workflow, not a separate orchestrator.
- Binance/OKX funding strategy assistant.
- Portfolio-per-leg selection.
- Dedicated SQLite storage modeled after DCA.
- Deterministic monitor gate with LLM invocation only on threshold breach.
- User-configurable monitor interval with 5 minute default.
- Dashboard/web UI visibility.

`idea-review.md` tightens the design into implementation requirements:

- Mirror the existing DCA blueprint across store, tools, REST, web UI, cron gate, and skill.
- Add a deterministic delta-neutral health evaluator as the main new domain component.
- Treat two-leg execution as the highest-risk area and require a state machine.
- Allow cross-exchange hedges, but always warn and recommend same-exchange legs when possible.
- Make monitor data-fetch failures alert-worthy instead of silently skipped.
- Use watchlist-scoped scans to avoid excessive API calls.
- Compute plan-level PnL from the delta-neutral store, not only account-level PnL tools.
- Store risk policy per plan, not in global config.

This final requirement merges both documents. When they differ, `idea-review.md` is treated as the stricter technical requirement.

## 3. Goals

### Product Goals

- Help users discover funding-rate carry opportunities across Binance and OKX.
- Make plan construction explicit: capital, spot leg, futures leg, selected portfolios, expected carry, costs, breakeven, and risk.
- Keep execution safe through Approval mode and leg-aware recovery.
- Keep monitoring cheap by using deterministic code for normal ticks.
- Give users durable plan history, execution history, monitor snapshots, and alerts.
- Surface cross-exchange risk clearly without blocking users who intentionally choose it.

### Engineering Goals

- Reuse existing KhunQuant tools wherever possible.
- Mirror DCA's proven architecture for persistence, cron, REST, and web UI.
- Keep delta-neutral-specific thresholds in per-plan policy JSON.
- Keep plan monitoring independent from LLM token usage except on threshold breach or explanation.
- Build implementation in phases, with execution last inside MVP.

## 4. Non-Goals

- No full-auto opening of new positions in MVP.
- No guaranteed yield, fixed return, or deposit product behavior.
- No Bitkub or BinanceTH futures support.
- No SET equities support for this strategy.
- No whole-market unbounded scanner in MVP.
- No borrowing/lending legs in MVP.
- No automatic major close without explicit approval in MVP.
- No global delta-neutral risk thresholds in `config.json` except tool enable flags and existing trading gates.

## 5. Supported Markets And Accounts

### Supported Futures Providers

- `binance`: USDT perpetuals.
- `okx`: USDT swaps.

### Supported Spot Providers

- `binance`: spot leg.
- `okx`: spot leg when account and tool support are configured.

### Explicitly Spot-Only Providers

- `bitkub`
- `binanceth`

These providers must not be accepted for the futures leg.

### Portfolio Selection Requirement

Every plan must bind two user-selected KhunQuant portfolios:

- Spot leg portfolio: `spot_provider` + `spot_account`.
- Futures leg portfolio: `futures_provider` + `futures_account`.

The plan creation flow must call `list_portfolios` or equivalent portfolio discovery before asking the user to choose portfolios. The assistant must not infer a money-moving account from a provider name alone when multiple accounts exist.

## 6. User Roles And Use Cases

### User Role

The primary user is an individual KhunQuant operator managing personal crypto accounts locally. They may use the web console or chat channels.

### Core Use Cases

- "Find the best ETH delta-neutral opportunity on Binance and OKX."
- "Create a 10,000 USDT delta-neutral plan using Binance spot and OKX futures."
- "Monitor this plan every 5 minutes and alert me if liquidation buffer drops below 25%."
- "Show my active delta-neutral plans and their health."
- "Open this plan after I confirm."
- "Unwind this plan after I confirm."
- "Why did the strategy alert me?"

## 7. Product Requirements

### 7.1 Opportunity Scanning

The scanner must:

- Use a configurable watchlist in MVP, not all markets.
- Validate futures symbols using `futures_validate_market`.
- Fetch current funding using `futures_get_funding`.
- Fetch funding history using `funding_rate_history`.
- Fetch spot/perp price and order book data using market tools.
- Rank opportunities by net carry, funding stability, liquidity, spread/slippage, and risk.
- Warn when scan data is stale, partial, unavailable, or provider-limited.

MVP watchlist default:

- BTC/USDT
- ETH/USDT
- SOL/USDT
- BNB/USDT
- XRP/USDT
- ADA/USDT
- DOGE/USDT
- AVAX/USDT
- LINK/USDT
- TON/USDT

The watchlist must be configurable later through plan creation UI or config, but MVP can hardcode a conservative default in the skill/tool design.

Output must include:

- Asset.
- Spot provider candidate.
- Futures provider candidate.
- Current funding.
- 3D, 7D, 14D funding averages when available.
- Estimated annualized carry.
- Estimated fees/slippage.
- Estimated round-trip breakeven.
- Liquidity label.
- Cross-exchange flag.
- Opportunity score.
- Recommended next action.

### 7.2 Funding Analysis

Funding analysis must include:

- Current funding rate.
- Next funding timestamp when available.
- 3D average.
- 7D average.
- 14D average.
- Minimum and maximum funding in the fetched window.
- Funding volatility.
- Positive-funding ratio.
- Funding reversal warning.
- Annualized estimate.

Funding interpretation:

- `attractive`: current funding is positive, above minimum threshold, and above recent averages.
- `watch`: current funding is positive but unstable, near zero, or deteriorating.
- `blocked`: funding is negative, unavailable, or too volatile for plan policy.

### 7.3 Plan Creation

The plan creation flow must:

1. Validate the selected opportunity.
2. Discover available portfolios.
3. Ask the user to select the spot leg portfolio.
4. Ask the user to select the futures leg portfolio.
5. Validate both selected portfolios.
6. Confirm balances and exposure for both selected leg portfolios.
7. Validate futures market metadata.
8. Estimate spot and futures sizing.
9. Estimate fees and slippage for entry and exit.
10. Estimate funding income and round-trip breakeven.
11. Set per-plan risk thresholds.
12. Set monitor interval, defaulting to 5 minutes.
13. Persist the plan in the delta-neutral SQLite store.
14. Register the cron monitor job.
15. Return a plan summary.

Required user-facing plan summary:

- Plan name.
- Asset.
- Capital.
- Spot leg portfolio.
- Futures leg portfolio.
- Spot leg action.
- Futures leg action.
- Reserve margin.
- Monitor interval.
- Estimated entry cost.
- Estimated exit cost.
- Estimated total round-trip cost.
- Expected daily funding.
- Breakeven days.
- Funding stability label.
- Liquidation buffer.
- Delta drift target.
- Same-exchange recommendation if providers differ.
- Status.

### 7.4 Same-Exchange Recommendation

Cross-exchange plans are allowed. However, same-exchange execution is safer because it usually reduces latency, transfer complexity, account mismatch, and unwind risk.

Requirement:

- If `spot_provider != futures_provider`, plan creation must show a warning.
- If `spot_provider != futures_provider`, execution review must show a warning.
- If `spot_provider != futures_provider`, monitor health must include a standing cross-exchange risk flag.
- The health evaluator must lower the exchange/execution-risk component when providers differ.
- The user may proceed after explicit acknowledgment.

Warning text shape:

```text
Cross-exchange warning
This plan uses spot on Binance and futures on OKX. This is allowed, but it has higher execution and unwind risk than running both legs on the same exchange. Recommendation: use the same exchange for both legs when available.
```

### 7.5 Plan Monitoring

The monitor must be deterministic by default and must not require an LLM for normal ticks.

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

Default:

- 5 minutes.

Sub-minute intervals:

- 30 seconds and 1 minute remain supported requirements.
- The UI and plan creation response must show a rate-limit warning when either is selected.
- The monitor must still be functional-gate based and must not invoke the LLM unless escalation is required.

Monitor behavior per due plan:

1. Load the plan from SQLite.
2. Skip disabled or archived plans.
3. Fetch spot leg balance/price from the selected spot portfolio.
4. Fetch futures position, mark price, funding, margin, and liquidation data from the selected futures portfolio.
5. Compute delta drift.
6. Compute funding state.
7. Compute liquidation distance and margin state.
8. Compute plan-level PnL estimate.
9. Compute plan health score.
10. Write a monitor snapshot.
11. If no threshold is breached, return silently with no LLM call.
12. If threshold is breached, write alert row, send alert, and invoke agent for explanation.
13. If required data is unavailable or fetch fails, write failed snapshot, send alert, and invoke agent or fallback alert path.

Data unavailable must not be silently skipped for active plans.

### 7.6 Plan Alerts

Alert triggers:

- Funding turns negative.
- Funding is below configured minimum.
- Funding is unavailable.
- Funding reverses for configured number of cycles.
- Liquidation distance is below configured minimum.
- Margin risk reaches `danger` or `critical`.
- Delta drift exceeds configured maximum.
- Spot leg balance is missing or insufficient.
- Futures position is missing, closed, or side mismatched.
- Cross-exchange monitor data mismatch occurs.
- Exchange API fetch fails.
- Profit target is reached.
- Stop loss or max drawdown is reached.
- Execution enters a failed or recovery-required state.

Alert destinations:

- Dashboard alert/event.
- Existing chat channel if `notify_channel` and `notify_chat_id` are set.
- Cron job output path for agent-driven explanation when applicable.

Alert payload must include:

- Plan ID.
- Plan name.
- Severity.
- Trigger code.
- Trigger message.
- Key metrics.
- Recommended action.
- Whether the agent was invoked.
- Timestamp.

### 7.7 Plan Reporting

The reporting flow must read from the delta-neutral store first.

Plan report must include:

- Plan status.
- Selected spot/futures portfolios.
- Current spot value.
- Current futures notional.
- Delta drift.
- Funding received or estimated.
- Fees and slippage estimate.
- Realized execution costs.
- Unrealized hedge PnL.
- Plan-level net PnL estimate.
- Health score.
- Latest alert.
- Recommendation.

Generic account PnL tools may be used as supporting data, but plan-level economics must be derived from plan execution rows and monitor snapshots.

### 7.8 Approval-Mode Execution

Live execution is in MVP, but implemented last and behind explicit confirmation.

Execution requirements:

- No live open, close, reduce, rebalance, or transfer without explicit confirmation.
- Must honor account permissions.
- Must honor `trading_risk.allow_leverage` for futures mutations.
- Must honor order rate status.
- Must display same-exchange recommendation warning for cross-exchange plans.
- Must display exact order details before execution.
- Must be leg-aware and auditable.

Execution review must include:

- Plan ID and name.
- Spot leg provider/account/symbol/side/amount/order type/estimated price.
- Futures leg provider/account/symbol/side/amount/leverage/margin mode/order type/estimated mark.
- Estimated fees.
- Estimated slippage.
- Estimated round-trip breakeven.
- Liquidation buffer.
- Delta target.
- Cross-exchange warning when applicable.
- Required confirmation text.

### 7.9 Two-Leg Execution State Machine

Two-leg execution is not atomic. The implementation must model it explicitly.

Execution attempt states:

- `pending`
- `validating`
- `awaiting_approval`
- `placing_first_leg`
- `first_leg_failed`
- `first_leg_filled`
- `placing_second_leg`
- `second_leg_failed`
- `both_legs_filled`
- `recovery_required`
- `unwinding`
- `unwound`
- `failed`
- `cancelled`

Leg states:

- `pending`
- `placing`
- `open`
- `partially_filled`
- `filled`
- `failed`
- `cancelled`
- `unwinding`
- `unwound`

Required order:

- Place the harder-to-fill or less-liquid leg first.
- Default: place the futures hedge first, then spot, unless the evaluator determines spot is materially less liquid.
- If first leg fails, abort second leg.
- If first leg fills and second leg fails, enter `recovery_required`.
- Recovery path must recommend or perform, after approval, a reduce-only futures close or spot unwind depending on which leg filled.

Recovery tools to compose:

- `futures_reduce_position`
- `futures_close_position`
- `futures_emergency_flatten`
- `create_order` for spot unwind
- `get_order`
- `futures_get_order`
- `futures_get_positions`

## 8. Functional Requirements By Feature

### 8.1 Scan

- User can request a specific symbol or watchlist scan.
- System returns ranked opportunities.
- System explains why a candidate is blocked or watchlisted.
- Scan does not create a plan unless the user requests it.

### 8.2 Create Plan

- User can create a plan from a scan result or direct symbol request.
- User must select spot and futures portfolios.
- System stores plan even before execution.
- Plan may be `draft`, `ready`, `active`, `paused`, `closing`, `closed`, `failed`, or `archived`.

### 8.3 Update Plan

User can update:

- Name.
- Enabled/paused.
- Monitor interval.
- Risk thresholds.
- Notify channel/chat ID.
- Watchlist metadata.
- Exit rules.

The system must not allow changing provider/account bindings for an active plan without requiring a new plan or explicit migration flow.

### 8.4 Delete Plan

- Draft and closed plans may be deleted.
- Active plans should require pause/close first.
- Delete must cascade monitor snapshots, alerts, and execution rows only after explicit confirmation.

### 8.5 Monitor Plan

- Cron job name prefix: `dn:<id>:<name>`.
- Monitor handler must parse plan ID from job name.
- Monitor handler must write a snapshot every tick.
- Monitor handler must skip LLM on non-breach.
- Monitor handler must escalate data failures.

### 8.6 Execute Plan

- Only `ready` plans can be opened.
- Only `active` or `recovery_required` plans can be unwound.
- Execution writes attempt and leg rows.
- Execution updates plan status after verification.
- Execution failure never hides unhedged exposure.

## 9. Technical Architecture

### 9.1 DCA Blueprint Mapping

| Layer | Existing DCA | Delta-neutral requirement |
|---|---|---|
| Store | `pkg/dca/store.go` | `pkg/deltaneutral/store.go` |
| Types | `pkg/dca/types.go` | `pkg/deltaneutral/types.go` |
| Deterministic gate | `pkg/dca/trigger.go` | `pkg/deltaneutral/health.go` |
| Cron handler | `cmd/khunquant/internal/gateway/dca_handler.go` | `cmd/khunquant/internal/gateway/delta_neutral_handler.go` |
| Tools | `pkg/tools/dca_*.go` | `pkg/tools/delta_neutral_*.go` |
| REST | `web/backend/api/agent_dca.go` | `web/backend/api/agent_delta_neutral.go` |
| Frontend API | `web/frontend/src/api/agent-dca.ts` | `web/frontend/src/api/agent-delta-neutral.ts` |
| Web panel | `dca-snapshot-panel.tsx` | `delta-neutral-panel.tsx` |
| Skill | `workspace/skills/dca/SKILL.md` | `workspace/skills/delta-neutral/SKILL.md` |

### 9.2 New Go Package

Package:

- `pkg/deltaneutral`

Files:

- `types.go`
- `store.go`
- `health.go`
- `interval.go`
- `execution.go`
- `store_test.go`
- `health_test.go`
- `interval_test.go`
- `execution_test.go`

### 9.3 Store Path

```text
{workspace}/memory/delta_neutral/delta_neutral.db
```

Store behavior:

- Create directory if missing.
- Open SQLite through `modernc.org/sqlite`.
- Use WAL mode.
- Use `PRAGMA synchronous=NORMAL`.
- Use `PRAGMA foreign_keys=ON`.
- Use idempotent migrations.
- Keep schema migrations tolerant of duplicate columns.
- Close DB cleanly.

### 9.4 Schema

#### `delta_neutral_plans`

```sql
CREATE TABLE IF NOT EXISTS delta_neutral_plans (
    id                         INTEGER PRIMARY KEY AUTOINCREMENT,
    name                       TEXT    NOT NULL UNIQUE,
    asset                      TEXT    NOT NULL,
    status                     TEXT    NOT NULL DEFAULT 'draft',
    mode                       TEXT    NOT NULL DEFAULT 'approval',

    spot_provider              TEXT    NOT NULL,
    spot_account               TEXT    NOT NULL DEFAULT '',
    spot_symbol                TEXT    NOT NULL,
    spot_side                  TEXT    NOT NULL DEFAULT 'buy',

    futures_provider           TEXT    NOT NULL,
    futures_account            TEXT    NOT NULL DEFAULT '',
    futures_symbol             TEXT    NOT NULL,
    futures_side               TEXT    NOT NULL DEFAULT 'short',
    futures_margin_mode        TEXT    NOT NULL DEFAULT 'cross',
    futures_leverage           INTEGER NOT NULL DEFAULT 1,

    capital_usdt               REAL    NOT NULL DEFAULT 0,
    spot_notional_usdt         REAL    NOT NULL DEFAULT 0,
    futures_notional_usdt      REAL    NOT NULL DEFAULT 0,
    reserve_margin_usdt        REAL    NOT NULL DEFAULT 0,

    monitor_interval           TEXT    NOT NULL DEFAULT '5m',
    cron_job_id                TEXT    NOT NULL DEFAULT '',
    enabled                    INTEGER NOT NULL DEFAULT 1,

    entry_rules_json           TEXT    NOT NULL DEFAULT '{}',
    exit_rules_json            TEXT    NOT NULL DEFAULT '{}',
    risk_policy_json           TEXT    NOT NULL DEFAULT '{}',

    estimated_entry_cost_usdt  REAL    NOT NULL DEFAULT 0,
    estimated_exit_cost_usdt   REAL    NOT NULL DEFAULT 0,
    expected_daily_funding_usdt REAL   NOT NULL DEFAULT 0,
    breakeven_days             REAL    NOT NULL DEFAULT 0,

    cross_exchange             INTEGER NOT NULL DEFAULT 0,
    notify_channel             TEXT    NOT NULL DEFAULT '',
    notify_chat_id             TEXT    NOT NULL DEFAULT '',

    opened_at                  TEXT,
    closed_at                  TEXT,
    created_at                 TEXT    NOT NULL,
    updated_at                 TEXT    NOT NULL
);
```

Indexes:

```sql
CREATE INDEX IF NOT EXISTS idx_dn_plans_status ON delta_neutral_plans(status);
CREATE INDEX IF NOT EXISTS idx_dn_plans_enabled ON delta_neutral_plans(enabled);
CREATE INDEX IF NOT EXISTS idx_dn_plans_cron_job ON delta_neutral_plans(cron_job_id);
CREATE INDEX IF NOT EXISTS idx_dn_plans_asset ON delta_neutral_plans(asset);
```

#### `delta_neutral_monitor_snapshots`

```sql
CREATE TABLE IF NOT EXISTS delta_neutral_monitor_snapshots (
    id                         INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id                    INTEGER NOT NULL REFERENCES delta_neutral_plans(id) ON DELETE CASCADE,
    checked_at                 TEXT    NOT NULL,

    spot_price                 REAL    NOT NULL DEFAULT 0,
    spot_quantity              REAL    NOT NULL DEFAULT 0,
    spot_value_usdt            REAL    NOT NULL DEFAULT 0,

    futures_mark_price         REAL    NOT NULL DEFAULT 0,
    futures_contracts          REAL    NOT NULL DEFAULT 0,
    futures_notional_usdt      REAL    NOT NULL DEFAULT 0,
    futures_unrealized_pnl_usdt REAL   NOT NULL DEFAULT 0,

    current_funding_rate       REAL    NOT NULL DEFAULT 0,
    estimated_next_funding_usdt REAL   NOT NULL DEFAULT 0,
    funding_state              TEXT    NOT NULL DEFAULT '',

    delta_drift_pct            REAL    NOT NULL DEFAULT 0,
    liquidation_price          REAL    NOT NULL DEFAULT 0,
    liquidation_distance_pct   REAL    NOT NULL DEFAULT 0,
    margin_ratio_pct           REAL    NOT NULL DEFAULT 0,
    margin_state               TEXT    NOT NULL DEFAULT '',

    health_score               INTEGER NOT NULL DEFAULT 0,
    health_label               TEXT    NOT NULL DEFAULT '',
    cross_exchange             INTEGER NOT NULL DEFAULT 0,

    threshold_breached         INTEGER NOT NULL DEFAULT 0,
    breach_codes_json          TEXT    NOT NULL DEFAULT '[]',
    data_status                TEXT    NOT NULL DEFAULT 'ok',
    error_msg                  TEXT    NOT NULL DEFAULT '',
    agent_invoked              INTEGER NOT NULL DEFAULT 0,

    created_at                 TEXT    NOT NULL
);
```

Indexes:

```sql
CREATE INDEX IF NOT EXISTS idx_dn_snapshots_plan ON delta_neutral_monitor_snapshots(plan_id);
CREATE INDEX IF NOT EXISTS idx_dn_snapshots_checked_at ON delta_neutral_monitor_snapshots(checked_at);
CREATE INDEX IF NOT EXISTS idx_dn_snapshots_breach ON delta_neutral_monitor_snapshots(threshold_breached);
```

#### `delta_neutral_alerts`

```sql
CREATE TABLE IF NOT EXISTS delta_neutral_alerts (
    id                         INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id                    INTEGER NOT NULL REFERENCES delta_neutral_plans(id) ON DELETE CASCADE,
    snapshot_id                INTEGER REFERENCES delta_neutral_monitor_snapshots(id) ON DELETE SET NULL,
    triggered_at               TEXT    NOT NULL,
    severity                   TEXT    NOT NULL,
    code                       TEXT    NOT NULL,
    message                    TEXT    NOT NULL,
    recommended_action         TEXT    NOT NULL DEFAULT '',
    agent_invoked              INTEGER NOT NULL DEFAULT 0,
    delivered_channel          TEXT    NOT NULL DEFAULT '',
    delivered_chat_id          TEXT    NOT NULL DEFAULT '',
    created_at                 TEXT    NOT NULL
);
```

#### `delta_neutral_executions`

```sql
CREATE TABLE IF NOT EXISTS delta_neutral_executions (
    id                         INTEGER PRIMARY KEY AUTOINCREMENT,
    plan_id                    INTEGER NOT NULL REFERENCES delta_neutral_plans(id) ON DELETE CASCADE,
    attempt_id                 TEXT    NOT NULL,
    state                      TEXT    NOT NULL,
    requested_at               TEXT    NOT NULL,
    approved_at                TEXT,
    completed_at               TEXT,
    error_msg                  TEXT    NOT NULL DEFAULT '',
    created_at                 TEXT    NOT NULL,
    updated_at                 TEXT    NOT NULL
);
```

#### `delta_neutral_execution_legs`

```sql
CREATE TABLE IF NOT EXISTS delta_neutral_execution_legs (
    id                         INTEGER PRIMARY KEY AUTOINCREMENT,
    execution_id               INTEGER NOT NULL REFERENCES delta_neutral_executions(id) ON DELETE CASCADE,
    leg_type                   TEXT    NOT NULL,
    provider                   TEXT    NOT NULL,
    account                    TEXT    NOT NULL DEFAULT '',
    symbol                     TEXT    NOT NULL,
    side                       TEXT    NOT NULL,
    order_type                 TEXT    NOT NULL,
    requested_amount           REAL    NOT NULL DEFAULT 0,
    requested_notional_usdt    REAL    NOT NULL DEFAULT 0,
    requested_price            REAL    NOT NULL DEFAULT 0,
    order_id                   TEXT    NOT NULL DEFAULT '',
    state                      TEXT    NOT NULL DEFAULT 'pending',
    filled_quantity            REAL    NOT NULL DEFAULT 0,
    filled_notional_usdt       REAL    NOT NULL DEFAULT 0,
    avg_fill_price             REAL    NOT NULL DEFAULT 0,
    fee_usdt                   REAL    NOT NULL DEFAULT 0,
    error_msg                  TEXT    NOT NULL DEFAULT '',
    created_at                 TEXT    NOT NULL,
    updated_at                 TEXT    NOT NULL
);
```

## 10. Core Types

### `Plan`

Required fields:

- `ID int64`
- `Name string`
- `Asset string`
- `Status PlanStatus`
- `Mode ExecutionMode`
- `SpotProvider string`
- `SpotAccount string`
- `SpotSymbol string`
- `SpotSide string`
- `FuturesProvider string`
- `FuturesAccount string`
- `FuturesSymbol string`
- `FuturesSide string`
- `FuturesMarginMode string`
- `FuturesLeverage int`
- `CapitalUSDT float64`
- `SpotNotionalUSDT float64`
- `FuturesNotionalUSDT float64`
- `ReserveMarginUSDT float64`
- `MonitorInterval string`
- `CronJobID string`
- `Enabled bool`
- `EntryRules EntryRules`
- `ExitRules ExitRules`
- `RiskPolicy RiskPolicy`
- `CrossExchange bool`
- `NotifyChannel string`
- `NotifyChatID string`
- `CreatedAt time.Time`
- `UpdatedAt time.Time`

### `RiskPolicy`

Required fields:

- `MinFundingRate float64`
- `MaxBreakevenDays float64`
- `MinLiquidationDistancePct float64`
- `MaxDeltaDriftPct float64`
- `MaxSlippageBps float64`
- `MaxCapitalUSDT float64`
- `MaxLeverage int`
- `ReserveMarginUSDT float64`
- `FundingReversalCycles int`
- `ProfitTargetUSDT float64`
- `MaxDrawdownUSDT float64`
- `EscalateOnDataFailure bool`

Defaults:

- `MinLiquidationDistancePct`: 25.
- `MaxDeltaDriftPct`: 3.
- `MaxSlippageBps`: 20.
- `FundingReversalCycles`: 2.
- `EscalateOnDataFailure`: true.

### `MonitorSnapshot`

Required fields mirror `delta_neutral_monitor_snapshots`.

### `HealthEvaluation`

Fields:

- `Snapshot MonitorSnapshot`
- `ThresholdBreached bool`
- `BreachCodes []string`
- `Severity string`
- `RecommendedAction string`
- `DataStatus string`
- `Err error`

### Enums

Plan status:

- `draft`
- `ready`
- `active`
- `paused`
- `recovery_required`
- `closing`
- `closed`
- `failed`
- `archived`

Execution mode:

- `monitor`
- `approval`
- `semi_auto`
- `full_auto`

Health labels:

- `excellent`
- `healthy`
- `watch`
- `danger`
- `critical`

Data status:

- `ok`
- `partial`
- `unavailable`
- `error`

## 11. Deterministic Health Evaluator

Function:

```go
func Evaluate(input EvaluationInput) HealthEvaluation
```

Input includes:

- Plan.
- Spot balance/value.
- Spot ticker/order book data.
- Futures position.
- Futures mark price.
- Futures funding.
- Futures risk summary.
- Current time.

Evaluator responsibilities:

- Normalize spot value in USDT.
- Normalize futures notional in USDT.
- Compute delta drift:

```text
abs(spot_value_usdt - abs(futures_notional_usdt)) / max(spot_value_usdt, abs(futures_notional_usdt)) * 100
```

- Compute liquidation distance:

```text
abs(mark_price - liquidation_price) / mark_price * 100
```

- Compute funding state.
- Compute margin state.
- Compute cross-exchange flag.
- Compute plan health score.
- Compare each metric against plan risk policy.
- Return breach codes.

Health score components:

- Funding health: 20 points.
- Margin/liquidation health: 25 points.
- Delta balance: 20 points.
- Liquidity/slippage: 10 points.
- Exchange/execution risk: 10 points.
- Profit progress: 15 points.

Cross-exchange penalty:

- If `spot_provider != futures_provider`, subtract from exchange/execution risk.
- Do not block solely because providers differ.

Data failure behavior:

- If required active-plan data cannot be fetched, return `DataStatus=error`, `ThresholdBreached=true`, and breach code `data_unavailable`.

## 12. Cron And Monitor Integration

### Job Naming

```text
dn:<plan_id>:<sanitized_plan_name>
```

### Handler

File:

- `cmd/khunquant/internal/gateway/delta_neutral_handler.go`

Function:

```go
func handleDeltaNeutralMonitorJob(
    ctx context.Context,
    job *cron.CronJob,
    cfg *config.Config,
    dnStore *deltaneutral.Store,
    cronTool *tools.CronTool,
    msgBus *bus.MessageBus,
) (string, error)
```

Behavior:

- Parse plan ID from job name.
- Load plan.
- Skip disabled, archived, or closed plans.
- Fetch live state.
- Call `deltaneutral.Evaluate`.
- Save monitor snapshot every tick.
- If no breach, return silent status.
- If breach, create alert row and deliver alert.
- If breach and `cronTool` exists, call `cronTool.ExecuteJob` for agent explanation.
- If data unavailable, alert even if `cronTool` is unavailable.

Register in `setupCronTool` next to DCA:

```go
if strings.HasPrefix(job.Name, "dn:") && dnStore != nil {
    return handleDeltaNeutralMonitorJob(...)
}
```

### Store Initialization

Like DCA, the delta-neutral store should open unconditionally in gateway setup so existing cron jobs can run even if individual tools are disabled.

## 13. Tools

### Existing Tools To Reuse

Funding/futures:

- `funding_rate_history`
- `futures_get_funding`
- `futures_validate_market`
- `futures_estimate_funding_fee`
- `futures_risk_summary`
- `futures_get_positions`
- `futures_open_position`
- `futures_close_position`
- `futures_reduce_position`
- `futures_emergency_flatten`
- `futures_get_order`

Market:

- `get_ticker`
- `get_tickers`
- `get_orderbook`
- `get_markets`
- `get_ohlcv`

Portfolio/PnL/snapshot:

- `list_portfolios`
- `get_assets_list`
- `get_total_value`
- `get_pnl_summary`
- `get_pnl_detail`
- `take_snapshot`
- `query_snapshots`
- `snapshot_summary`

Orders/alerts:

- `create_order`
- `get_order`
- `get_order_rate_status`
- `set_price_alert`
- `set_indicator_alert`
- `cron`

### New Tools

Plan tools:

- `create_delta_neutral_plan`
- `list_delta_neutral_plans`
- `get_delta_neutral_plan`
- `update_delta_neutral_plan`
- `delete_delta_neutral_plan`

Summary/history tools:

- `get_delta_neutral_summary`
- `get_delta_neutral_history`

Execution tools:

- `open_delta_neutral_position`
- `unwind_delta_neutral_position`

### Tool Categories And Config

Add tool constants in `pkg/tools/names.go`.

Add config fields in `pkg/config/config.go`:

- `CreateDeltaNeutralPlan`
- `ListDeltaNeutralPlans`
- `GetDeltaNeutralPlan`
- `UpdateDeltaNeutralPlan`
- `DeleteDeltaNeutralPlan`
- `GetDeltaNeutralSummary`
- `GetDeltaNeutralHistory`
- `OpenDeltaNeutralPosition`
- `UnwindDeltaNeutralPosition`

Add category:

```go
CatDeltaNeutral = "delta_neutral"
```

Register support metadata in `web/backend/api/tools.go`.

Register tools in `pkg/agent/instance.go` and gateway setup where cron/store dependencies are required.

## 14. REST API

File:

- `web/backend/api/agent_delta_neutral.go`

Register routes in:

- `web/backend/api/router.go`

Base:

```text
/api/agent/delta-neutral
```

Endpoints:

- `GET /api/agent/delta-neutral/plans`
- `GET /api/agent/delta-neutral/plans/{id}`
- `GET /api/agent/delta-neutral/plans/{id}/monitor-snapshots`
- `GET /api/agent/delta-neutral/plans/{id}/alerts`
- `GET /api/agent/delta-neutral/plans/{id}/executions`

Optional later:

- `POST /api/agent/delta-neutral/plans/{id}/pause`
- `POST /api/agent/delta-neutral/plans/{id}/resume`

### Plan List Response

Fields:

- `id`
- `name`
- `asset`
- `status`
- `mode`
- `spot_provider`
- `spot_account`
- `spot_symbol`
- `futures_provider`
- `futures_account`
- `futures_symbol`
- `capital_usdt`
- `monitor_interval`
- `enabled`
- `cross_exchange`
- `health_score`
- `health_label`
- `last_checked_at`
- `last_alert_at`
- `created_at`
- `updated_at`

### Monitor Snapshot Response

Fields mirror `MonitorSnapshot`.

### Execution Response

Include execution attempt plus nested leg rows.

## 15. Web UI

Frontend files:

- `web/frontend/src/api/agent-delta-neutral.ts`
- `web/frontend/src/components/agent-memory/delta-neutral-panel.tsx`
- update `web/frontend/src/components/agent-memory/agent-memory-page.tsx`

UI placement:

- Add a new "Delta-Neutral" tab beside Snapshots and DCA in Agent Memory, or create a dedicated strategy page if navigation is later expanded.

MVP layout:

- Left side: plan list.
- Right side: selected plan detail.
- Detail sections:
  - Plan summary.
  - Leg portfolios.
  - Health score.
  - Risk thresholds.
  - Latest monitor snapshot.
  - Monitor snapshot table.
  - Alert table.
  - Execution history.

Required visual indicators:

- Status badge.
- Health label badge.
- Cross-exchange warning badge.
- Data unavailable warning.
- Agent invoked flag on monitor snapshots.

## 16. Skill Design

New skill:

```text
workspace/skills/delta-neutral/SKILL.md
```

Skill responsibilities:

- Guide scan and funding analysis.
- Enforce portfolio-per-leg selection.
- Recommend same-exchange legs when possible.
- Use watchlist-scoped scanning.
- Build cost-aware plans.
- Use round-trip breakeven.
- Never execute without explicit confirmation.
- Use live tools for market and portfolio data.
- Use delta-neutral tools once they exist.

Extend existing skill:

- `workspace/skills/funding-rate-analysis/SKILL.md`

Add notes for:

- Positive funding ratio.
- Funding reversal detection.
- Comparison across Binance/OKX.
- Annualized rate caveat.

## 17. Plan-Level PnL

Plan PnL must be derived from delta-neutral records.

Inputs:

- Execution leg fills.
- Execution fees.
- Monitor snapshots.
- Funding fee history when available.
- Current spot value.
- Current futures unrealized PnL.
- Estimated exit costs.

Plan PnL output:

```text
net_plan_pnl = realized_funding
             + futures_unrealized_pnl
             + spot_unrealized_pnl
             - entry_fees
             - exit_fee_estimate
             - slippage_estimate
```

If exact funding fee history is unavailable, mark funding as estimated.

Generic account PnL tools may validate totals but must not replace plan-level accounting.

## 18. Security And Safety

Hard rules:

- No live trade without confirmation in MVP.
- No futures mutation unless `trading_risk.allow_leverage=true`.
- No futures leg on spot-only providers.
- No silent skip on active-plan data failure.
- No unbounded all-market scan in MVP.
- Do not log secrets.
- Do not expose secrets in REST responses.
- Do not store raw API keys in delta-neutral tables.

Account permissions:

- Check trade permission before live order execution.
- Allow read-only monitor where account has read capability.
- Surface permission errors clearly.

## 19. Acceptance Criteria

### Phase 1: Skill-First Analysis

Deliverables:

- `workspace/skills/delta-neutral/SKILL.md`.
- Update funding-rate skill notes.

Acceptance:

- User can ask for ETH delta-neutral analysis.
- Agent calls validation, funding, history, market, and portfolio tools in safe order.
- Agent returns cost-aware plan recommendation.
- Agent warns when legs span different exchanges.
- No Go code required.

### Phase 2: Store, Tools, Monitor Gate

Deliverables:

- `pkg/deltaneutral` store and types.
- Deterministic `Evaluate`.
- Plan CRUD tools.
- Summary/history tools.
- Cron monitor handler.
- Alert rows.

Acceptance:

- `go test ./pkg/deltaneutral/...` passes.
- Plan can be created and stored.
- Cron tick writes monitor snapshot.
- Non-breach tick does not invoke LLM.
- Breach tick writes alert and invokes agent.
- Data-fetch failure writes failed snapshot and alerts.
- Cross-exchange flag appears in health evaluation.

### Phase 3: REST And Web UI

Deliverables:

- Backend REST endpoints.
- Frontend API module.
- Delta-Neutral panel.

Acceptance:

- Web UI lists plans.
- User can inspect leg portfolios.
- User can inspect monitor snapshots.
- User can inspect alerts.
- Cross-exchange warning is visible.
- Data unavailable snapshot is visible.

### Phase 4: Approval-Mode Execution

Deliverables:

- `open_delta_neutral_position`.
- `unwind_delta_neutral_position`.
- Execution attempt and leg tables.
- State machine.
- Recovery path.

Acceptance:

- Dry-run/paper mode can simulate both legs.
- Live execution requires explicit confirmation.
- Cross-exchange execution review shows recommendation warning.
- First-leg failure aborts second leg.
- Second-leg failure enters `recovery_required`.
- Recovery action is proposed and requires confirmation.
- Final execution state is persisted.

## 20. Test Plan

### Unit Tests

- Interval parsing and validation.
- Risk policy defaults.
- Store create/list/update/delete.
- Store migrations.
- Health score computation.
- Delta drift computation.
- Liquidation distance computation.
- Cross-exchange flag.
- Threshold breach detection.
- Data-unavailable evaluation.
- Execution state transitions.

### Tool Tests

- Create plan validates portfolios.
- Create plan rejects invalid futures provider.
- Create plan persists risk policy.
- Create plan registers cron job.
- Update plan validates monitor interval.
- Summary reads store snapshots.
- History returns snapshots and alerts.
- Open position requires confirmation.
- Unwind requires confirmation.

### Gateway Tests

- `dn:<id>:<name>` parsing.
- Disabled plan skip.
- Closed plan skip.
- Non-breach monitor writes snapshot and skips LLM.
- Breach monitor writes snapshot, alert, and invokes LLM.
- Data-fetch error escalates.

### REST Tests

- List plans.
- Get plan.
- Get snapshots.
- Get alerts.
- Get executions.
- Redact sensitive fields.

### Frontend Tests

- Plan list renders.
- Empty state renders.
- Detail panel renders.
- Snapshot table renders.
- Cross-exchange badge renders.
- Alert rows render.

### Integration Tests

- Paper plan creation to monitor snapshot.
- Forced threshold breach to alert.
- Simulated execution success.
- Simulated one-leg failure to recovery.

## 21. Rollout Plan

Phase order:

1. Skill-first analysis.
2. Store, plan tools, deterministic monitor.
3. REST and web panel.
4. Approval-mode execution.

Rollout flags:

- New tools are individually enable/disable controlled through existing tools config.
- Execution tools can stay disabled until paper tests pass.
- 30 second and 1 minute monitor intervals should show warnings in UI and chat output.

## 22. Final MVP Definition

MVP is complete when KhunQuant can:

- Analyze Binance/OKX delta-neutral opportunities from a watchlist.
- Create a plan with explicit spot and futures portfolio selection.
- Persist the plan in `workspace/memory/delta_neutral/delta_neutral.db`.
- Monitor the plan on a configured interval with deterministic gates.
- Write monitor snapshots every tick.
- Alert and invoke the agent only when thresholds breach or data is unavailable.
- Show plans, snapshots, alerts, and execution history in web UI.
- Execute and unwind a plan in Approval mode with a leg-aware state machine.
- Warn but not block when a plan uses different exchanges for the two legs.

