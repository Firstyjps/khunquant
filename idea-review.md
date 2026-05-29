# idea.md Review: Delta-Neutral Funding Strategy Assistant

A grounded review of `idea.md` against the actual KhunQuant codebase — concerns, recommended changes, a tools/skills design, a technical design, and a phased implementation plan.

---

## 1. Verdict

The design is unusually well-grounded. **Every one of the 34 tools `idea.md` references already exists and is registered** (`pkg/tools/*.go`, wired in `pkg/agent/instance.go` conditional on `cfg.Tools.IsToolEnabled`). More importantly, the architecture the document asks for — a dedicated SQLite store, a deterministic monitor gate that only wakes the LLM on threshold breach, a skill, plan CRUD tools, REST endpoints, and a web panel — is a feature KhunQuant **already ships once: DCA**.

So this is mostly a **replication of the DCA blueprint plus one genuinely new piece of domain logic** (a delta-neutral health evaluator). The real risks are concentrated in two places: live two-leg execution, and the cost/safety of monitoring a live hedge across two exchanges. Both are addressed below.

> **Scope decisions (agreed):** live two-leg execution **is in scope** (Approval mode). Cross-exchange hedges (spot on one venue, perp on another) **remain allowed**, but the assistant must **alert and recommend running both legs on the same exchange** whenever a plan binds the two legs to different exchanges. Rollout is **phased, skill-first** — value early, the execution state machine last within the MVP.

The DCA blueprint to mirror:

| Layer | DCA reference | Delta-neutral equivalent |
|---|---|---|
| Store | `pkg/dca/store.go` (WAL, FK cascade, idempotent migrations) | `pkg/deltaneutral/store.go` |
| Deterministic core | `pkg/dca/trigger.go` + `indicators.go` (`dca.EvaluateTrigger`) | `pkg/deltaneutral/health.go` (`deltaneutral.Evaluate`) |
| Monitor gate | `cmd/khunquant/internal/gateway/dca_handler.go` | `…/delta_neutral_handler.go` |
| Tools | `pkg/tools/dca_*.go` (7 tools) | `pkg/tools/delta_neutral_*.go` |
| REST | `web/backend/api/agent_dca.go` → `/api/agent/dca/...` | `agent_delta_neutral.go` → `/api/agent/delta-neutral/...` |
| Web panel | `web/frontend/src/components/agent-memory/dca-snapshot-panel.tsx` | new "Delta-Neutral" tab |
| Skill | `workspace/skills/dca/SKILL.md` | `workspace/skills/delta-neutral/SKILL.md` |

The key proof point: `dca_handler.go` already implements exactly the "deterministic gate → conditional LLM" pattern that `idea.md` §4.7 describes as the monitoring architecture. Cron fires → pure-Go checks (enabled / expiry / period-cap / indicator trigger) → only `cronTool.ExecuteJob` (the LLM) when all pass. The token-saving design the doc wants is already real and battle-tested.

---

## 2. Concerns & Recommended Changes

### 2.1 Two-leg execution has no cross-exchange atomicity (highest risk)

DCA places **one** market order on **one** exchange. This strategy places a spot leg **and** a perpetual leg, with no shared transaction. A partial fill or a one-leg failure leaves an **unhedged directional position** — precisely the harm the product promises to prevent. The risk is worst when the two legs sit on **different exchanges** (no shared margin, slower/serial placement, harder unwind).

`idea.md` §6.3 acknowledges failure handling, but treats it as advice text. Since execution is in scope, it must be a **hard requirement**:
- Model execution as an explicit per-leg **state machine**: `pending → leg1_filled → leg2_filled` | `leg1_filled_leg2_failed → unwind`.
- Deterministic **leg ordering** — place the harder-to-fill / less-liquid leg (typically the perp hedge) first, then the spot, so a failure happens before the second exposure is taken.
- **Abort-on-first-leg-failure** with an unwind path. Recovery primitives already exist: `futures_emergency_flatten`, `futures_reduce_position` (reduce-only), `futures_close_position`.
- Approval-mode confirmation before any live leg (per `idea.md` §4.6).

### 2.2 Same-exchange recommendation / cross-exchange alert (new requirement)

Cross-exchange delta-neutral (spot on exchange A, perp on exchange B) **stays allowed**, but it carries materially higher execution and unwind risk than running both legs on **one** exchange (where the venue often supports shared/cross margin, faster paired placement, and simpler reduce-only exits). So:

- **At plan-build and at execution review:** if `spot_provider != futures_provider`, surface a **warning + recommendation** to run both legs on the same exchange. Allow the user to proceed knowingly.
- **At monitor time:** an active plan whose legs span two exchanges should carry a standing **cross-exchange alert** flag in its health summary (lower the health score component for "exchange/execution risk"), so the dashboard and reports make the trade-off visible.
- This is a deterministic check (string compare of the two providers) — cheap, and it belongs in `deltaneutral.Evaluate` and in the plan-build skill prompt. No new tool needed.

### 2.3 The execution table must be leg-aware

DCA's `dca_executions` row represents a single order. A delta-neutral attempt has two legs plus possible recovery actions. Model two legs per attempt (or two rows linked by an `attempt_id`) so partial-fill state is representable and auditable. (Relevant only at the execution phase, but the schema decision should be made up front.)

### 2.4 A plan binds two accounts; the DCA store binds one

`dca_plans` has a single `provider` / `account`. The delta-neutral plan binds **both** legs to user-selected portfolios (`idea.md` §5.2). The schema needs `spot_provider` / `spot_account` and `futures_provider` / `futures_account`. Keep the plan-name `UNIQUE` constraint as DCA has it (`store.go:19`).

### 2.5 The monitor must distinguish "threshold not breached" from "data unavailable"

DCA's gate silently skips when the indicator condition is false ("RSI not oversold yet") — safe, because no position is at stake. For a **live hedge**, a *failed* position/margin fetch is **not** safe to skip silently: an unmonitored hedged position is dangerous. `idea.md` §4.7 mentions "a data failure needs interpretation" — elevate this to a first-class branch in the handler:

- threshold not breached → write snapshot, silent, no LLM (cheap path);
- threshold breached → write snapshot, alert + invoke LLM;
- **data unavailable / fetch error → escalate (alert), do not silently skip.**

### 2.6 Sub-minute intervals are costly and low-value

`idea.md` lists 30s and 1m intervals. Each monitor tick requires **authenticated** position + margin + ticker calls on **two** exchanges → real rate-limit/weight pressure. Funding accrues per ~8h cycle, so minute-level polling buys almost nothing.

**Recommendation:** keep the 5m default; either drop 30s/1m from MVP or gate them behind an explicit warning. (Cron *can* express them via `EveryMS`, so this is a product choice, not a technical limit.)

### 2.7 Unbounded scanning is expensive

"Discover all symbols → validate → fetch funding + history + orderbook → rank" across two exchanges is N×(several calls). There is no batch funding tool.

**Recommendation:** MVP scanner runs over a **configurable watchlist** (e.g. top 10–20 by volume), not the whole universe. Consider short-TTL caching of funding history.

### 2.8 Breakeven must be round-trip and net

`idea.md`'s 1.9-day breakeven appears entry-only. Include **exit** costs for both legs (spot sell + perp close) and both legs' fees/slippage — or label the figure explicitly as entry-only. A carry trade's true breakeven includes the unwind.

### 2.9 Plan-level PnL ≠ account-level PnL

`get_pnl_summary` / `get_pnl_detail` are account-scoped. A plan's economics (funding received, hedge PnL, fees) should be computed from the delta-neutral store's **monitor snapshots**, mirroring how `get_dca_summary` reads the DCA store — not from generic account PnL tools alone.

### 2.10 Risk policy belongs per-plan, in the store

The doc's `RiskPolicy` object (§5.6) is the right home for DN thresholds — keep it per-plan in the store. Honor the **global** `trading_risk.allow_leverage` gate for any futures mutation (already enforced in `pkg/providers/broker/permissions.go`). Avoid scattering DN-specific thresholds into global config.

### 2.11 Naming & wiring nits

- Cron job prefix: `dn:<id>:<name>` (parallels `dca:<id>:<name>` parsed in `dca_handler.go:33`).
- Store path: `{workspace}/memory/delta_neutral/delta_neutral.db` (parallels `dca/dca.db`, `store.go:80`).
- REST base: `/api/agent/delta-neutral/...`.
- Web: a new "Delta-Neutral" tab beside the DCA panel in `agent-memory-page.tsx`.

---

## 3. Tools & Skills Design

### Reuse as-is (no new code)

All 34 referenced tools, notably:
- Funding/futures: `funding_rate_history`, `futures_get_funding`, `futures_validate_market`, `futures_estimate_funding_fee`, `futures_risk_summary`, `futures_get_positions`.
- Market: `get_ticker`, `get_tickers`, `get_orderbook`, `get_markets`, `get_ohlcv`.
- Portfolio/PnL/snapshot/alert/cron: `list_portfolios`, `get_assets_list`, `get_total_value`, `get_pnl_summary/detail`, `take_snapshot`, `query_snapshots`, `snapshot_summary`, `set_price_alert`, `set_indicator_alert`, `cron`.

### New skills

- **`workspace/skills/delta-neutral/SKILL.md`** — orchestrates scan → funding analysis → plan-build → risk-check using only existing tools (the §4.1–4.5 workflows). Pure prompt; **zero Go** for the analysis phases. Models itself on the `dca` and `funding-rate-analysis` skills.
- Extend **`funding-rate-analysis`** with a positive-funding-ratio / reversal-detection note, cross-referenced from the DN skill. (It already computes 3d/7d/14d means, volatility, annualized.)

### New tools (mirror DCA's 7) — Phase 2+

- `create_delta_neutral_plan`, `list_delta_neutral_plans`, `get_delta_neutral_plan`, `update_delta_neutral_plan`, `delete_delta_neutral_plan` — plan CRUD + cron registration (mirror `pkg/tools/dca_*.go`).
- `get_delta_neutral_summary`, `get_delta_neutral_history` — plan PnL + monitor/snapshot history (mirror `get_dca_summary` / `get_dca_history`).
- Execution tools (`open` / `unwind`) — **deferred to Phase 4**.

### New deterministic core (the one genuinely novel build)

**`pkg/deltaneutral/health.go`** — analog of `dca/trigger.go` + `indicators.go`. Given live positions, margin, and funding for both legs, compute delta drift, liquidation distance, margin health, funding state, a **cross-exchange flag** (`spot_provider != futures_provider`), and a health score; return `(breached bool, snapshot)` for the monitor gate. This — not the CRUD/UI/store boilerplate — is the real engineering. Everything else is well-trodden DCA territory.

### Execution tools (Phase 4, in MVP)

- `open_delta_neutral_position` — Approval-mode two-leg open driven by the state machine in §2.1; emits the same-exchange recommendation (§2.2) before placing legs across different venues.
- `unwind_delta_neutral_position` — reduce-only / emergency unwind composed from `futures_reduce_position`, `futures_close_position`, `futures_emergency_flatten`, and spot `create_order`.

---

## 4. Technical Design (mirrors DCA, layer by layer)

**Store** — `pkg/deltaneutral/` (`types.go`, `store.go`): copy `dca/store.go` patterns (WAL, `foreign_keys=ON`, idempotent `ALTER TABLE` migrations). Tables:
- `delta_neutral_plans` — dual-account binding, risk-policy JSON, monitor interval, status, timestamps.
- `delta_neutral_monitor_snapshots` — deterministic outputs (funding, delta drift, liquidation distance, margin risk, PnL, health score) + an `agent_invoked` flag.
- `delta_neutral_alerts` — threshold events that triggered notification.
- `delta_neutral_executions` (Phase 4) — leg-aware.

**Monitor gate** — `cmd/khunquant/internal/gateway/delta_neutral_handler.go`: copy `dca_handler.go`. Parse `dn:<id>:<name>`, load plan, fetch both legs' live state, call `deltaneutral.Evaluate`, **write a snapshot every tick**, and only `cronTool.ExecuteJob` (LLM) on breach *or data-unavailable*; otherwise silent. Register in `setupCronTool` (`helpers.go`) next to the DCA handler dispatch.

**Alerts** — emit via `bus.PublishOutbound` from the handler on breach (same path as `alert_handlers.go`), plus snapshot rows for the UI.

**REST** — `web/backend/api/agent_delta_neutral.go`: copy `agent_dca.go` → `/api/agent/delta-neutral/plans`, `/plans/{id}`, `/plans/{id}/monitor-snapshots`. Register in `web/backend/api/router.go`.

**Web UI** — copy `dca-snapshot-panel.tsx` + `api/agent-dca.ts` → a "Delta-Neutral" tab in `agent-memory-page.tsx` (plan list left / detail + snapshots right).

---

## 5. Phased Implementation Plan (skill-first; value early, risk last)

> Reflects the agreed scope: **execution in MVP**, built last; **phased rollout**.

**Phase 1 — Analysis skill (no Go).**
`delta-neutral/SKILL.md` drives scan / funding / plan / risk via existing tools, over a watchlist. Ships immediately as "Monitor mode" reasoning. Includes the same-exchange recommendation (§2.2) at plan-build.
*Verify:* ask the agent "find the best ETH delta-neutral opportunity on Binance/OKX" and confirm it chains `futures_validate_market` → `futures_get_funding` → `funding_rate_history` → market tools, returns a ranked cost-aware recommendation, and warns when the two legs would span different exchanges.

**Phase 2 — Persistence + deterministic monitor.**
`pkg/deltaneutral` store, `deltaneutral.Evaluate` health core (incl. cross-exchange flag), plan CRUD tools, `delta_neutral_handler.go` cron gate, alerts.
*Verify:* `go test ./pkg/deltaneutral/...`; create a plan, let the cron tick write snapshots, force a threshold breach and confirm exactly one alert/LLM invocation fires — and that a forced data-fetch failure **escalates** rather than silently skipping.

**Phase 3 — REST + web panel.**
Endpoints + Vue tab; surface the cross-exchange flag in plan detail/health.
*Verify:* `cd web && make dev`; confirm plan list / detail / snapshots render.

**Phase 4 — Approval-mode execution (last, but in MVP).**
Leg-aware execution tools + state machine (§2.1) + unwind recovery, gated by `trading_risk.allow_leverage` and explicit confirmation; cross-exchange plans trigger the same-exchange recommendation alert before placement.
*Verify:* paper-trading mode end-to-end including a simulated one-leg failure → unwind, and confirm a different-exchange plan emits the recommendation while still being allowed to proceed.

---

## 6. Revised MVP Scope (vs. `idea.md` §10)

**MVP = Phases 1–4:** scan, analyze, plan, persist, monitor with a deterministic gate, alert, plan-level report, web panel, **and Approval-mode two-leg execution**. Watchlist-scoped scanning; 5m default monitor; 30s/1m gated or dropped.

**Execution rules:** cross-exchange hedges (spot and perp on different venues) are **allowed** but always trigger a **same-exchange recommendation alert** (§2.2); execution itself runs through the explicit state machine with abort-on-first-leg-failure and an unwind path (§2.1).

This delivers user value quickly (analysis + monitoring first), reuses the proven DCA machinery for the bulk of the build, and sequences the one dangerous capability — live execution — last, behind a well-specified state machine and the cross-exchange safeguard.
