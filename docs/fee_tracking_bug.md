# Fee Tracking Bug Report

## Context

`pkg/deltaneutral/store.go` has `SumPlanExecutionFees` which sums `fee_usdt`
from `delta_neutral_execution_legs` for a plan. This value is saved as
`trading_fee_usdt` in `plan_fee_snapshots` and displayed in the UI as
**Accumulated Fees → Trading Fee**.

The user sees **+0.0499 USDT** (positive), which is wrong. All trading fees
are costs and must be negative. When the plan is closed, the fee should grow
more negative to include close-leg fees.

---

## Bug 1 — Wrong sign: fees stored as positive

**Files:** `pkg/tools/delta_neutral_open.go`

### Spot leg (`executeSpotLeg`)

```go
// WRONG — order.Fee.Cost from CCXT/OKX is the absolute fee amount (positive).
// For a spot buy, OKX deducts 0.678 ALGO as fee. CCXT returns Fee.Cost = +0.678.
// Multiplying by avgFill gives a positive USDT value → stored as positive cost.
if order.Fee.Cost != nil && *order.Fee.Cost > 0 && avgFill > 0 {
    leg.FeeUSDT = *order.Fee.Cost * avgFill   // e.g. +0.0801 — SHOULD BE -0.0801
}
```

### Futures leg (`executeFuturesLeg`)

```go
// WRONG — same issue. OKX/CCXT returns Fee.Cost as a positive absolute amount.
if order.Fee.Cost != nil && *order.Fee.Cost > 0 {
    leg.FeeUSDT = *order.Fee.Cost   // e.g. +0.0148 — SHOULD BE -0.0148
}
```

**Fix:** Negate both assignments. Fees are costs; store them as negative values.
Maker rebates (rare) would have `Fee.Cost <= 0` from OKX's convention, so the
`> 0` guard already skips them — just negate the stored value.

```go
leg.FeeUSDT = -(*order.Fee.Cost * avgFill)  // spot
leg.FeeUSDT = -(*order.Fee.Cost)            // futures
```

---

## Bug 2 — Close legs never record fees

**Files:** `pkg/tools/delta_neutral_unwind.go`

`closeFuturesLeg` and `closeSpotLeg` do **not** call `FetchFuturesOrder` /
`FetchOrder` after placing the close order, and do **not** set `leg.FeeUSDT`.

When the user unwinds a plan, the close-leg fees (taker fee on close order for
futures + spot sell fee) are never recorded. `SumPlanExecutionFees` therefore
only reflects the open-leg fees, and the Accumulated Fee never grows at close
time.

### `closeFuturesLeg` — required changes

1. After `fp.CreateFuturesOrder(...)`, call `fp.FetchFuturesOrder(ctx, oid, symbol)`
   to get actual fill data (same pattern as `executeFuturesLeg` in open.go).
2. Set `leg.FeeUSDT = -(*order.Fee.Cost)` if `Fee.Cost != nil && *Fee.Cost > 0`.

### `closeSpotLeg` — required changes

1. After `tp.CreateOrder(...)`, call `tp.FetchOrder(ctx, oid, symbol)`
   to get actual fill data.
2. Set `leg.FeeUSDT = -(*order.Fee.Cost * avgFill)` if applicable.

---

## Bug 3 — Resize legs never record fees

**File:** `pkg/tools/delta_neutral_resize.go`

`resizeFuturesLeg` and `resizeSpotLeg` have the same problems as close legs:
no `FetchOrder` after `CreateOrder`, and `leg.FeeUSDT` is never set.

Resize fees (both increase and decrease) must be captured so that
`SumPlanExecutionFees` reflects all trading activity for the plan.

### `resizeFuturesLeg` — required changes

Same as `closeFuturesLeg`: add `FetchFuturesOrder` + set `leg.FeeUSDT`.

### `resizeSpotLeg` — required changes

Same as `closeSpotLeg`: add `FetchOrder` + set `leg.FeeUSDT`.

---

## Expected behaviour after fix

| Event | `fee_usdt` on the leg | Running sum |
|---|---|---|
| Open futures (taker $0.0148) | −0.0148 | −0.0148 |
| Open spot (0.678 ALGO × 0.118) | −0.0801 | −0.0949 |
| Resize futures (taker $0.010) | −0.010 | −0.1049 |
| Close futures (taker $0.0148) | −0.0148 | −0.1197 |
| Close spot (0.566 ALGO × 0.118) | −0.0668 | −0.1865 |

`SumPlanExecutionFees` returns −0.1865 → displayed as **−0.1865 USDT** ✓

---

## Key files to edit

| File | Functions |
|---|---|
| `pkg/tools/delta_neutral_open.go` | `executeFuturesLeg`, `executeSpotLeg` |
| `pkg/tools/delta_neutral_unwind.go` | `closeFuturesLeg`, `closeSpotLeg` |
| `pkg/tools/delta_neutral_resize.go` | `resizeFuturesLeg`, `resizeSpotLeg` |

No schema changes needed — `fee_usdt` column already exists on
`delta_neutral_execution_legs`.

No changes needed to `SumPlanExecutionFees` or `refreshPlanFees` — the query
and the snapshot logic are correct once the sign and completeness issues are fixed.

---

## How to verify

After the fix, open a new plan and check:

```sql
SELECT leg_type, side, fee_usdt
FROM delta_neutral_execution_legs l
JOIN delta_neutral_executions e ON l.execution_id = e.id
WHERE e.plan_id = <new_plan_id> AND l.state = 'filled';
```

All `fee_usdt` values must be **negative** (or zero for maker rebates).
After unwind, two additional rows (close legs) must appear, also negative.
