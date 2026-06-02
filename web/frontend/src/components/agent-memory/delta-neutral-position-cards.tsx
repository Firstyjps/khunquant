import type { DeltaNeutralMonitorSnapshot, DeltaNeutralPlanListItem } from "@/api/agent-delta-neutral"

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
  const spotValue   = safeNum(snap.spot_value_usdt)
  const spotNotional = safeNum(plan.spot_notional_usdt)
  const futuresPnL  = safeNum(snap.futures_unrealized_pnl_usdt)

  // entry notional unknown (plan DTO not yet populated / legacy zero value)
  const entryKnown = spotNotional > 0
  const spotPnL    = entryKnown ? spotValue - spotNotional : null
  const netPnL     = entryKnown ? spotPnL! + futuresPnL : null
  const netAbsSmall = netPnL !== null && Math.abs(netPnL) < 0.01

  return (
    <div className="overflow-hidden rounded-lg border">
      <div className="border-border/50 border-b px-3 py-2">
        <span className="text-foreground/80 text-xs font-medium uppercase tracking-wide">
          Position P&amp;L
        </span>
      </div>

      <div className="p-3">
        {/* Net P&L — prominent center */}
        <div className="mb-3 flex flex-col items-center gap-0.5 py-2">
          <span className="text-muted-foreground text-xs">Net Unrealized P&amp;L</span>
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

        {/* Futures-only P&L always available */}
        {!entryKnown && (
          <div className="mb-3 flex items-center justify-center gap-2 text-xs text-muted-foreground">
            <span>Spot entry notional unavailable — net P&L cannot be computed.</span>
            <span className="font-mono">Futures: <span className={signedColor(futuresPnL)}>{signed(futuresPnL)} USDT</span></span>
          </div>
        )}

        <div className="border-border/40 mb-3 border-t" />

        {/* Two-column leg breakdown */}
        <div className="grid grid-cols-2 gap-3">
          {/* Spot leg */}
          <div className="rounded-md bg-muted/30 p-2.5">
            <div className="text-muted-foreground mb-2 text-xs font-medium">Spot Leg (long)</div>
            <div className="space-y-1">
              <Row
                label="Entry notional"
                value={entryKnown ? `${fmt(spotNotional, 2)} USDT` : "—"}
              />
              <Row
                label="Current value"
                value={`${fmt(spotValue, 2)} USDT`}
                sub={`${fmt(snap.spot_price, 6)} × ${fmt(snap.spot_quantity, 2)}`}
              />
              <div className="border-border/30 border-t pt-1">
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground text-xs">Unrealized</span>
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
            <div className="text-muted-foreground mb-2 text-xs font-medium">Futures Leg (short)</div>
            <div className="space-y-1">
              <Row label="Notional" value={`${fmt(snap.futures_notional_usdt, 2)} USDT`} />
              <Row label="Mark price" value={fmt(snap.futures_mark_price, 6)} />
              <div className="border-border/30 border-t pt-1">
                <div className="flex items-center justify-between">
                  <span className="text-muted-foreground text-xs">Unrealized (upl)</span>
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
  const exitCost = entryCost
  const roundTrip = entryCost + exitCost

  const fundingApy = safeNum(snap.funding_apy_pct)
  const futuresNotionalSnap = safeNum(snap.futures_notional_usdt)
  const earnApy = safeNum(snap.earn_apy_pct)
  const spotValue = safeNum(snap.spot_value_usdt)

  // Use stored annualised APY so the correct interval (1h/4h/8h) is already baked in
  const dailyFunding = fundingApy > 0 ? (fundingApy / 100 / 365) * futuresNotionalSnap : 0
  const dailyEarn = earnApy > 0 && spotValue > 0 ? (earnApy / 100 / 365) * spotValue : 0
  const dailyCombined = dailyFunding + dailyEarn

  const breakevenFunding = dailyFunding > 0 ? roundTrip / dailyFunding : null
  const breakevenCombined = dailyCombined > 0 ? roundTrip / dailyCombined : null

  const hasData = dailyFunding > 0 || dailyEarn > 0

  return (
    <div className="overflow-hidden rounded-lg border">
      <div className="border-border/50 border-b px-3 py-2">
        <span className="text-foreground/80 text-xs font-medium uppercase tracking-wide">
          Yield &amp; Breakeven
        </span>
      </div>

      <div className="p-3">
        {!hasData ? (
          <p className="text-muted-foreground py-2 text-center text-xs">
            No snapshot data yet — values will appear after the first monitor tick.
          </p>
        ) : (
          <>
            {/* Top summary row — most important numbers large */}
            <div className="mb-3 grid grid-cols-2 gap-3">
              <div className="rounded-md bg-muted/30 p-2.5 text-center">
                <div className="text-muted-foreground mb-0.5 text-xs">Daily Combined</div>
                <div className="font-mono text-lg font-bold text-green-500 dark:text-green-400">
                  +{fmt(dailyCombined, 4)} USDT
                </div>
                <div className="text-muted-foreground mt-0.5 text-xs">per day</div>
              </div>
              <div className="rounded-md bg-muted/30 p-2.5 text-center">
                <div className="text-muted-foreground mb-0.5 text-xs">Breakeven</div>
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

            {/* Daily breakdown */}
            <div className="mb-3 grid grid-cols-3 gap-2 text-center">
              <DailyPill
                label="Funding"
                value={dailyFunding}
                sub={`${fmt(snap.funding_apy_pct, 2)}% APY/365`}
                color="text-indigo-500 dark:text-indigo-400"
              />
              <DailyPill
                label="Earn"
                value={dailyEarn}
                sub={`${fmt(snap.earn_apy_pct, 2)}% APY/365`}
                color="text-amber-500 dark:text-amber-400"
              />
              <DailyPill
                label="Combined"
                value={dailyCombined}
                sub="total / day"
                color="text-green-500 dark:text-green-400"
              />
            </div>

            {/* Divider */}
            <div className="border-border/40 mb-2.5 border-t" />

            {/* Entry/exit cost */}
            <div className="flex items-center justify-between">
              <div className="text-muted-foreground text-xs">
                Entry cost{" "}
                <span className="opacity-60">(spot 0.10% + futures 0.05%)</span>
              </div>
              <div className="font-mono text-xs font-medium">
                {fmt(entryCost, 4)} USDT each way
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
  value,
  sub,
}: {
  label: string
  value: string
  sub?: string
}) {
  return (
    <div>
      <div className="flex items-start justify-between gap-2">
        <span className="text-muted-foreground shrink-0 text-xs">{label}</span>
        <span className="font-mono text-xs font-medium text-right">{value}</span>
      </div>
      {sub && <div className="text-muted-foreground text-right font-mono text-xs opacity-60">{sub}</div>}
    </div>
  )
}

function DailyPill({
  label,
  value,
  sub,
  color,
}: {
  label: string
  value: number
  sub: string
  color: string
}) {
  return (
    <div className="rounded-md bg-muted/30 px-2 py-2">
      <div className="text-muted-foreground mb-0.5 text-xs">{label}</div>
      <div className={`font-mono text-sm font-semibold ${color}`}>
        +{fmt(value, 4)}
      </div>
      <div className="text-muted-foreground mt-0.5 text-xs opacity-70">{sub}</div>
    </div>
  )
}
