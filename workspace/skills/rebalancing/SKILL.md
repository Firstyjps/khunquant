---
name: rebalancing
description: Deterministic portfolio rebalancing to target allocation weights — drift checks, trade proposals, scheduled monitoring, and optional auto-execution. All numbers come from the rebalance engine, never from manual calculation.
---

# Portfolio Rebalancing

Rebalance a portfolio to target weights using the deterministic rebalance engine. **Never compute drift, trade sizes, or allocations yourself** — the `rebalance_*` tools do all math in code and you orchestrate + explain.

## Tools

| Tool | Purpose |
|---|---|
| `rebalance_check` | Show current vs target allocation + the proposed trades (read-only, always safe) |
| `rebalance_plan_create` | Save a plan + schedule automatic drift checks (cron) |
| `rebalance_plan_list` / `rebalance_plan_delete` | Manage plans |
| `rebalance_execute` | Execute a plan's proposal now (dry-run unless confirm=true) |

## Key semantics (explain when the user sets up a plan)

- **Targets must include the quote asset** (e.g. USDT) — its weight is the stable buffer. Weights sum to 100.
- **Only target assets participate.** Other holdings on the account are ignored — tell the user this if their account has more assets than the targets.
- **Tolerance** (default 5 pts): no trades while every asset is within ±tolerance of target. **Min trade size** (default 10 USD) suppresses dust; suppressed trades are listed, not hidden.
- Trades are **spot market orders vs the quote, sells first** (frees quote before buys). Execution stops at the first failure and reports honestly.
- Allocation values include locked balances; execution can fail on locked funds — surface that rather than retrying blindly.

## Two modes

- **alert** (default): when a scheduled check finds drift beyond tolerance, the user gets the full proposal and decides. Execution then goes through `rebalance_execute confirm=true`.
- **auto**: the scheduled check executes the proposal immediately and reports fills. Before creating an auto plan, make sure the user explicitly wants hands-off execution and confirm the tolerance/min-trade values with them once. Hard gates (trade permission, daily loss limit, rate limit) still apply on every auto run.

## Workflows

### One-off rebalance
```
User: "rebalance พอร์ต binance เป็น BTC 50 / ETH 30 / USDT 20"
  → rebalance_check provider=binance targets={"BTC":50,"ETH":30,"USDT":20}
  → present the drift table + proposal from the tool output
  → user agrees → rebalance_plan_create (so execute has a plan) → rebalance_execute confirm=true
  → rebalance_check again to show the post-trade allocation
```

### Scheduled monitoring
```
rebalance_plan_create name="main" provider=binance targets={...} tolerance_pct=5 schedule="0 */6 * * *" mode=alert
  → cron checks drift every 6h, silent while within tolerance
  → on breach: user receives the proposal in chat, decides to execute or not
```

### Hands-off auto-rebalancing
```
rebalance_plan_create ... mode=auto
  → on breach the engine executes and reports fills automatically
```

## Guardrails

- Present numbers ONLY from tool output. If the user asks "what if" with different weights, run `rebalance_check` ad-hoc with those targets instead of estimating.
- After any execution, run `rebalance_check` again and show the resulting allocation.
- If execution stopped early, check `get_open_orders` for dangling orders before anything else.
- Fees (~0.1%/trade) mean rebalancing too often burns money — for small drift, suggest widening tolerance instead of trading more.
