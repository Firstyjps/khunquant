import type { DeltaNeutralMonitorSnapshot, DeltaNeutralPlanListItem } from "@/api/agent-delta-neutral"

import { InfoHint, LabelWithHint } from "./info-hint"

// ─── helpers ────────────────────────────────────────────────────────────────

function fmt(n: number | undefined | null, digits = 4) {
  const v = n ?? 0
  return v.toLocaleString(undefined, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  })
}

function safeNum(n: number | undefined | null): number {
  return n ?? 0
}

function signedColor(n: number) {
  if (n > 0.0001) return "text-green-500 dark:text-green-400"
  if (n < -0.0001) return "text-red-500 dark:text-red-400"
  return "text-foreground"
}

function signed(n: number, digits = 4) {
  return (n >= 0 ? "+" : "") + fmt(n, digits)
}

// ─── P&L Card ────────────────────────────────────────────────────────────────

interface PnLCardProps {
  plan: DeltaNeutralPlanListItem
  snap: DeltaNeutralMonitorSnapshot
}

export function DeltaNeutralPnLCard({ plan, snap }: PnLCardProps) {
  const spotValue    = safeNum(snap.spot_value_usdt)
  const spotNotional = safeNum(plan.spot_notional_usdt)
  const futuresPnL   = safeNum(snap.futures_unrealized_pnl_usdt)

  // entry notional unknown (plan DTO not yet populated / legacy zero value)
  const entryKnown  = spotNotional > 0
  const spotPnL     = entryKnown ? spotValue - spotNotional : null
  const netPnL      = entryKnown ? spotPnL! + futuresPnL : null
  const netAbsSmall = netPnL !== null && Math.abs(netPnL) < 0.01

  return (
    <div className="overflow-hidden rounded-lg border">
      {/* Header */}
      <div className="border-border/50 flex items-center gap-1.5 border-b px-3 py-2">
        <span className="text-foreground/80 text-xs font-medium uppercase tracking-wide">
          Position P&amp;L
        </span>
        <InfoHint text="Unrealized profit/loss of the open delta-neutral position. Net P&L = (spot current value − spot entry notional) + futures unrealized P&L. Should stay near zero while the hedge is working." />
      </div>

      <div className="p-3">
        {/* Net P&L — prominent center */}
        <div className="mb-4 flex flex-col items-center gap-0.5 rounded-md bg-muted/20 py-3">
          <span className="text-muted-foreground flex items-center gap-1 text-xs">
            <LabelWithHint
              label="Net Unrealized P&L"
              hint="(Spot current value − Spot entry notional) + Futures uPnL. A value near zero confirms the delta-neutral hedge is working."
              className="text-muted-foreground text-xs"
            />
          </span>
          {netPnL !== null ? (
            <>
              <span className={`font-mono text-2xl font-bold ${signedColor(netPnL)}`}>
                {signed(netPnL, 4)} USDT
              </span>
              {netAbsSmall && (
                <span className="text-muted-foreground text-xs">(≈ flat — delta-neutral is working)</span>
              )}
            </>
          ) : (
            <span className="text-muted-foreground font-mono text-lg">—</span>
          )}
        </div>

        {/* Futures-only P&L available when entry notional not known */}
        {!entryKnown && (
          <div className="mb-3 flex items-center justify-center gap-2 text-xs text-muted-foreground">
            <span>Spot entry notional unavailable — net P&L cannot be computed.</span>
            <span className="font-mono">
              Futures:{" "}
              <span className={signedColor(futuresPnL)}>{signed(futuresPnL)} USDT</span>
            </span>
          </div>
        )}

        <div className="border-border/40 mb-3 border-t" />

        {/* Two-column leg breakdown */}
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {/* Spot leg */}
          <div className="rounded-md bg-muted/30 p-2.5">
            <div className="text-muted-foreground mb-2 flex items-center gap-1 text-xs font-medium">
              Spot Leg (long)
              <InfoHint text="The spot position bought at strategy open. We hold the asset and benefit from any earn/lending yield." />
            </div>
            <div className="space-y-1.5">
              <Row
                label="Entry notional"
                hint="USDT value paid to acquire the spot position at open. Derived from filled quantity × average fill price at execution time."
                value={entryKnown ? `${fmt(spotNotional, 2)} USDT` : "—"}
              />
              <Row
                label="Current value"
                hint={`Spot quantity × current mark price: ${fmt(snap.spot_quantity, 4)} × ${fmt(snap.spot_price, 6)}`}
                value={`${fmt(spotValue, 2)} USDT`}
                sub={`${fmt(snap.spot_price, 6)} × ${fmt(snap.spot_quantity, 2)}`}
              />
              <div className="border-border/30 border-t pt-1">
                <div className="flex items-center justify-between">
                  <LabelWithHint
                    label="Unrealized"
                    hint="Current spot value − entry notional. Negative when price has fallen since entry; partially offset by futures short gain."
                    className="text-muted-foreground text-xs"
                  />
                  {spotPnL !== null ? (
                    <span className={`font-mono text-sm font-semibold ${signedColor(spotPnL)}`}>
                      {signed(spotPnL)} USDT
                    </span>
                  ) : (
                    <span className="text-muted-foreground font-mono text-sm">—</span>
                  )}
                </div>
              </div>
            </div>
          </div>

          {/* Futures leg */}
          <div className="rounded-md bg-muted/30 p-2.5">
            <div className="text-muted-foreground mb-2 flex items-center gap-1 text-xs font-medium">
              Futures Leg (short)
              <InfoHint text="The perpetual futures short position that hedges spot price risk. Earns funding when long-side funding rate is positive." />
            </div>
            <div className="space-y-1.5">
              <Row
                label="Notional"
                hint="Mark price × contracts × contract size in USDT. Live value from the exchange."
                value={`${fmt(snap.futures_notional_usdt, 2)} USDT`}
              />
              <Row
                label="Mark price"
                hint="Exchange fair-value mark price used for unrealized P&L and liquidation calculations. Updated each monitor tick."
                value={fmt(snap.futures_mark_price, 6)}
              />
              <div className="border-border/30 border-t pt-1">
                <div className="flex items-center justify-between">
                  <LabelWithHint
                    label="Unrealized (upl)"
                    hint="Exchange-reported unrealized P&L for the short futures position. Positive when price has fallen (short profits), negative when price rises."
                    className="text-muted-foreground text-xs"
                  />
                  <span className={`font-mono text-sm font-semibold ${signedColor(futuresPnL)}`}>
                    {signed(futuresPnL)} USDT
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

// ─── Yield & Breakeven Card ───────────────────────────────────────────────────

interface YieldCardProps {
  plan: DeltaNeutralPlanListItem
  snap: DeltaNeutralMonitorSnapshot
}

export function DeltaNeutralYieldCard({ plan, snap }: YieldCardProps) {
  const SPOT_FEE = 0.001
  const FUTURES_FEE = 0.0005
  // Fall back to snapshot values when plan notionals aren't populated yet
  const spotNotional    = safeNum(plan.spot_notional_usdt)    || safeNum(snap.spot_value_usdt)
  const futuresNotional = safeNum(plan.futures_notional_usdt) || safeNum(snap.futures_notional_usdt)
  const entryCost = spotNotional * SPOT_FEE + futuresNotional * FUTURES_FEE
  const exitCost  = entryCost
  const roundTrip = entryCost + exitCost

  const fundingApy        = safeNum(snap.funding_apy_pct)
  const futuresNotionalSnap = safeNum(snap.futures_notional_usdt)
  const earnApy           = safeNum(snap.earn_apy_pct)
  const spotValue         = safeNum(snap.spot_value_usdt)

  // Use stored annualised APY so the correct interval (1h/4h/8h) is already baked in
  const dailyFunding  = fundingApy > 0 ? (fundingApy / 100 / 365) * futuresNotionalSnap : 0
  const dailyEarn     = earnApy > 0 && spotValue > 0 ? (earnApy / 100 / 365) * spotValue : 0
  const dailyCombined = dailyFunding + dailyEarn

  const breakevenFunding  = dailyFunding > 0 ? roundTrip / dailyFunding : null
  const breakevenCombined = dailyCombined > 0 ? roundTrip / dailyCombined : null

  const hasData = dailyFunding > 0 || dailyEarn > 0

  return (
    <div className="overflow-hidden rounded-lg border">
      {/* Header */}
      <div className="border-border/50 flex items-center gap-1.5 border-b px-3 py-2">
        <span className="text-foreground/80 text-xs font-medium uppercase tracking-wide">
          Yield &amp; Breakeven
        </span>
        <InfoHint text="Estimated daily income from funding rates (futures short) and earn/lending yield (spot). Breakeven is how many days of yield it takes to recover round-trip trading fees." />
      </div>

      <div className="p-3">
        {!hasData ? (
          <p className="text-muted-foreground py-2 text-center text-xs">
            No snapshot data yet — values will appear after the first monitor tick.
          </p>
        ) : (
          <>
            {/* Top summary row — most important numbers large */}
            <div className="mb-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="rounded-md bg-muted/30 p-3 text-center">
                <div className="text-muted-foreground mb-0.5 flex items-center justify-center gap-1 text-xs">
                  <LabelWithHint
                    label="Daily Combined"
                    hint={`Daily funding income + daily earn/lending income.\nFunding: ${fmt(fundingApy, 2)}% APY ÷ 365 × ${fmt(futuresNotionalSnap, 2)} USDT notional\nEarn: ${fmt(snap.earn_apy_pct, 2)}% APY ÷ 365 × ${fmt(spotValue, 2)} USDT spot value`}
                    className="text-muted-foreground text-xs"
                  />
                </div>
                <div className="font-mono text-lg font-bold text-green-500 dark:text-green-400">
                  +{fmt(dailyCombined, 4)} USDT
                </div>
                <div className="text-muted-foreground mt-0.5 text-xs">per day</div>
              </div>
              <div className="rounded-md bg-muted/30 p-3 text-center">
                <div className="text-muted-foreground mb-0.5 flex items-center justify-center gap-1 text-xs">
                  <LabelWithHint
                    label="Breakeven"
                    hint={`Days until yield covers round-trip trading fees.\nRound-trip fees: ${fmt(roundTrip, 4)} USDT (entry + exit each at spot 0.10% + futures 0.05%).\nBreakeven (combined) = ${fmt(roundTrip, 4)} ÷ ${fmt(dailyCombined, 4)} per day`}
                    className="text-muted-foreground text-xs"
                  />
                </div>
                {breakevenCombined !== null && (
                  <div className="font-mono text-lg font-bold text-foreground">
                    {breakevenCombined.toFixed(1)}d
                  </div>
                )}
                <div className="text-muted-foreground mt-0.5 text-xs">
                  {breakevenFunding !== null && `${breakevenFunding.toFixed(1)}d funding only`}
                </div>
              </div>
            </div>

            {/* Divider */}
            <div className="border-border/40 mb-3 border-t" />

            {/* Daily breakdown pills */}
            <div className="mb-3 grid grid-cols-3 gap-2 text-center">
              <DailyPill
                label="Funding"
                hint={`Annualised funding rate (${fmt(fundingApy, 2)}% APY) ÷ 365 × futures notional (${fmt(futuresNotionalSnap, 2)} USDT). Paid by longs to shorts when market is bullish.`}
                value={dailyFunding}
                sub={`${fmt(snap.funding_apy_pct, 2)}% APY/365`}
                color="text-indigo-500 dark:text-indigo-400"
              />
              <DailyPill
                label="Earn"
                hint={`Spot lending/earn APY (${fmt(earnApy, 2)}%) ÷ 365 × spot value (${fmt(spotValue, 2)} USDT). Zero if no earn position is active.`}
                value={dailyEarn}
                sub={`${fmt(snap.earn_apy_pct, 2)}% APY/365`}
                color="text-amber-500 dark:text-amber-400"
              />
              <DailyPill
                label="Combined"
                hint="Daily funding income + daily earn income."
                value={dailyCombined}
                sub="total / day"
                color="text-green-500 dark:text-green-400"
              />
            </div>

            {/* Divider */}
            <div className="border-border/40 mb-2.5 border-t" />

            {/* Entry/exit cost */}
            <div className="flex items-center justify-between gap-2">
              <LabelWithHint
                label="Round-trip fee"
                hint={`Entry: spot ${(SPOT_FEE * 100).toFixed(2)}% taker (${fmt(spotNotional * SPOT_FEE, 4)} USDT) + futures ${(FUTURES_FEE * 100).toFixed(3)}% taker (${fmt(futuresNotional * FUTURES_FEE, 4)} USDT) = ${fmt(entryCost, 4)} USDT.\nSame estimated cost to exit. Total round-trip: ${fmt(roundTrip, 4)} USDT.`}
                className="text-muted-foreground text-xs"
              />
              <div className="font-mono text-xs font-medium shrink-0">
                {fmt(roundTrip, 4)} USDT total
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

// ─── tiny shared sub-components ──────────────────────────────────────────────

function Row({
  label,
  hint,
  value,
  sub,
}: {
  label: string
  hint?: string
  value: string
  sub?: string
}) {
  return (
    <div>
      <div className="flex items-start justify-between gap-2">
        {hint ? (
          <LabelWithHint
            label={label}
            hint={hint}
            className="text-muted-foreground shrink-0 text-xs"
          />
        ) : (
          <span className="text-muted-foreground shrink-0 text-xs">{label}</span>
        )}
        <span className="font-mono text-xs font-medium text-right">{value}</span>
      </div>
      {sub && (
        <div className="text-muted-foreground text-right font-mono text-xs opacity-60">{sub}</div>
      )}
    </div>
  )
}

function DailyPill({
  label,
  hint,
  value,
  sub,
  color,
}: {
  label: string
  hint: string
  value: number
  sub: string
  color: string
}) {
  return (
    <div className="rounded-md bg-muted/30 px-2 py-2">
      <div className="text-muted-foreground mb-0.5 flex items-center justify-center gap-0.5 text-xs">
        {label}
        <InfoHint text={hint} />
      </div>
      <div className={`font-mono text-sm font-semibold ${color}`}>
        +{fmt(value, 4)}
      </div>
      <div className="text-muted-foreground mt-0.5 text-xs opacity-70">{sub}</div>
    </div>
  )
}
