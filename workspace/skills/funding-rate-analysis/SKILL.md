---
name: funding-rate-analysis
description: Analyze perpetual futures funding rate history to identify arbitrage opportunities, sentiment trends, and carry trade setups.
---

# Funding Rate Analysis

Use the `funding_rate_history` tool to fetch public funding rate records and computed statistics.

## Workflow

1. Call `funding_rate_history` with provider, symbol, and an appropriate limit (e.g. 200 for ~14d of 8h intervals).
2. Review the statistics table: 3d, 7d, 14d means, max/min extremes, volatility (std dev), and annualized rate.
3. Interpret the data:
   - **Mean > 0**: Longs paying shorts → bullish market bias (positive carry for shorts).
   - **Mean < 0**: Shorts paying longs → bearish market bias (positive carry for longs).
   - **High volatility (std dev)**: Funding rate is unstable; carry trade is risky.
   - **Annualized rate > 20%**: Significant arbitrage premium exists vs. spot borrowing cost.
4. Compare across providers (binance vs. okx) to spot cross-exchange divergences.
5. Summarize findings: current sentiment, carry opportunity, and risk level.

## Tool Reference

- `funding_rate_history` — fetch history + statistics (public, no credentials needed)
- `futures_get_funding` — current funding rate + next expected rate (also public)
- `get_ohlcv` — price history to correlate funding rate spikes with price moves

## Notes

- Funding interval is typically 8h (3×/day). Some exchanges run 4h or 1h intervals.
- The annualized rate in the output assumes 3 periods/day — adjust mentally for non-standard intervals.
- Extreme funding rates (> 0.1% per period) often revert quickly; useful for mean-reversion signals.

## Positive-Funding Ratio

The **positive-funding ratio** is the fraction of periods in the observed window where funding rate was positive. For example, if 92 out of 100 periods had positive funding, the ratio is 92%.

### How to Compute

When you fetch `funding_rate_history`, the output includes the raw funding rate series. Count the number of periods with `funding_rate > 0`, then divide by the total number of periods:

```
positive_ratio = (count of periods with funding > 0) / (total periods) × 100
```

The `funding_rate_history` tool may include this pre-computed; if not, calculate it manually.

### Interpretation

- **High ratio (> 80%)**: Funding is consistently positive, indicating persistent bullish bias and strong carry opportunity for shorts. Longs pay shorts reliably.
- **Moderate ratio (50–80%)**: Funding is mixed but leans positive. Some volatility, but carry is available on average. Good for medium-term carry if you can tolerate reversals.
- **Near 50%**: Funding is coin-flip; it flips between positive and negative frequently. No reliable carry direction; treat as unstable and watch for reversal before opening.
- **Low ratio (< 50%)**: Funding is predominantly negative, indicating bearish bias or unstable market sentiment. No carry opportunity for the direction you want to trade.

### Use in Plan Selection

Use positive-funding ratio as a stability signal:
- Before opening a delta-neutral carry position, confirm the ratio is > 80% over the past 7–14 days.
- If ratio is declining (was 90%, now 70%), mark the opportunity as "WATCH" and monitor for reversal.
- If ratio is < 50%, mark as "BLOCKED" unless your risk policy accepts high volatility.

## Funding Reversal Detection

A **funding reversal** occurs when funding shifts from positive (longs paying shorts) to negative (shorts paying longs), or when the current rate is significantly below recent averages. This is a critical risk signal for any carry or delta-neutral position.

### Detection Methods

**Method 1: Consecutive Cycles Negative**

Monitor recent funding ticks. If the current and previous N cycles (typically N=2) are negative, a reversal is underway:

```
if (current_funding < 0) and (previous_funding < 0) and (N_cycles_ago_funding < 0):
    trigger_reversal_alert()
```

Configure N in your delta-neutral plan's risk policy (default: 2 consecutive negative periods).

**Method 2: Current Rate Well Below Recent Average**

Compare the current funding rate to the recent moving average:

```
current_funding = latest_rate
avg_7d = mean of last 7 days of rates
threshold = avg_7d × 0.5  (or your configured threshold)

if (current_funding < threshold):
    trigger_reversal_warning()
```

This catches cases where funding has not yet turned fully negative but is clearly trending downward.

**Method 3: Volatility Spike + Negative Trend**

If volatility (standard deviation) is suddenly high AND the last few rates are below the mean, a reversal or squeeze may be starting:

```
if (recent_volatility > threshold) and (mean_last_3_periods < 0):
    mark_as_unstable()
```

### Risk Significance

For any open delta-neutral or carry position:

- **Reversal means your carry income stops**: If funding goes negative, you now *pay* the exchange; your position loses money daily.
- **Speed matters**: A slow drift toward zero (over days) is less urgent than a sudden flip (within 1–2 cycles). Use volatility and trend speed to gauge urgency.
- **Cross-reference the delta-neutral skill**: The delta-neutral strategy skill uses these signals to fire "funding reversal" alerts and suggest early unwind or rebalancing.

### Recommended Actions

1. **Monitor actively** before opening if a reversal warning has been triggered.
2. **Set tight alert thresholds** in your delta-neutral plan (e.g., 2 consecutive negative cycles → alert).
3. **Close early** if reversal is detected and funding is expected to stay negative for an extended period.
4. **Watch funding momentum**: Use the 3D, 7D, and 14D averages to distinguish temporary noise from sustained reversals.

## Binance vs OKX Comparison

Both Binance and OKX offer perpetual funding, but they operate independently with different liquidity pools and market dynamics. Comparing funding across exchanges helps you spot divergence and choose the best venue.

### Key Differences

**Funding Interval**

- **Binance**: 8 hours (0h, 8h, 16h UTC). Funding settles 3 times per day.
- **OKX**: 8 hours (0h, 8h, 16h UTC). Funding settles 3 times per day.

Both use the same interval, making direct comparison straightforward.

**Liquidity and Market Size**

- **Binance**: Larger perpetual market, tighter spreads, higher volume. Funding rates often more stable due to deep liquidity.
- **OKX**: Competitive perpetual market, reasonable spreads, good volume. Funding rates can diverge from Binance during regional or liquidity events.

**Settlement and Fee Structure**

- **Binance**: Taker fee ~0.05%, maker ~0.02%. Funding is added/subtracted directly to account.
- **OKX**: Taker fee ~0.05%, maker ~0.02%. Funding mechanism similar.

### How to Compare

**Step 1: Fetch Both Sides**

Call `funding_rate_history` for the same symbol on both exchanges with the same limit (e.g., 200 for ~14 days):

```json
{
  "provider": "binance",
  "symbol": "BTC/USDT",
  "limit": 200
}
```

```json
{
  "provider": "okx",
  "symbol": "BTC/USDT",
  "limit": 200
}
```

**Step 2: Compare Statistics**

Look at the 7D and 14D averages, current rate, and volatility:

```
Asset: BTC/USDT
———————————————
Binance 7D avg: 0.0082%, volatility: ±0.0015%, positive ratio: 92%
OKX     7D avg: 0.0079%, volatility: ±0.0018%, positive ratio: 88%

Divergence: Binance ~3 bps higher, OKX slightly more volatile. Both attractive.
```

**Step 3: Spot Cross-Exchange Divergence**

If one exchange has significantly higher funding (e.g., 0.02% vs 0.01%), it may indicate:

- **Temporary imbalance**: One exchange has more longs waiting to be hedged. Opportunity can shift quickly.
- **Liquidity difference**: Higher volume venue may have more stable funding.
- **Regional or time-of-day effect**: If you trade during off-peak hours for one exchange, funding may diverge.

**Step 4: Pick the Best Venue**

Rank same-exchange pairs by:
1. Net carry (funding - estimated slippage - estimated fees).
2. Funding stability (lower volatility preferred).
3. Positive-funding ratio (higher preferred).

Example:

```
Scenario A: Binance spot + Binance futures
  Funding: 0.0082%/8h → ~2.44 USDT/day on 10k USDT
  Spread (spot): 0.01%, Spread (perp): 0.015%
  Entry + exit slippage: ~60 USDT
  Net daily carry: ~2.04 USDT → BEST

Scenario B: OKX spot + OKX futures
  Funding: 0.0079%/8h → ~2.37 USDT/day on 10k USDT
  Spread (spot): 0.012%, Spread (perp): 0.02%
  Entry + exit slippage: ~65 USDT
  Net daily carry: ~1.99 USDT

Scenario C: Binance spot + OKX futures
  Binance spot spread: 0.01%, OKX futures spread: 0.02%
  Mixed slippage, cross-exchange latency risk
  Effective carry: ~1.95 USDT (after cross-exchange friction)
  Status: WATCH (higher execution risk, marginally worse carry)
```

### When Cross-Exchange Divergence Is Large

If one exchange's funding is 0.005% or more higher than the other, and both are positive:

- **Market is imbalanced**: Possible arbitrage or information asymmetry.
- **Act quickly**: Divergences can close within hours as traders reposition.
- **Check volume trends**: If one side is running high volume, funding may normalize soon.

## Annualized Rate Caveat

The annualized funding rate in the `funding_rate_history` output is computed by assuming a fixed number of funding periods per day (typically 3 for 8h venues). This is useful for rough comparison, but **must be understood correctly** and adjusted for different funding intervals.

### Standard Computation

```
annualized = current_funding_rate × periods_per_day × 365 × 100

Example (8h venue, 3 periods/day):
  current_funding = 0.01% per 8h
  annualized = 0.01% × 3 × 365 = 10.95%
```

### Adjustments for Non-Standard Intervals

If an exchange uses a different funding interval, adjust periods per day:

- **4h interval (6 periods/day)**: Annualized = `rate × 6 × 365`
- **1h interval (24 periods/day)**: Annualized = `rate × 24 × 365`
- **1D interval (1 period/day)**: Annualized = `rate × 1 × 365`

Example: If an exchange has 1h funding and shows 0.001% per period:
```
annualized = 0.001% × 24 × 365 = 87.6% ← very high, but unusual
```

### Critical Caveat: Do Not Over-Trust Annualized Estimates

The annualized figure assumes:

1. **Funding rate remains constant** for a full year — unrealistic. Markets change; funding reverts when sentiment flips.
2. **No compounding or reinvestment** — the formula is linear, not exponential.
3. **No funding reversals** — if funding turns negative for even a short period, the year-to-date return is dragged down immediately.
4. **Full window of data is representative** — a 14-day window during a bull run may not predict the next 14 days, especially if sentiment turns.
5. **Slippage and fees are zero** — in reality, entry/exit costs and ongoing fees eat into the rate.

### Practical Use

- **Use annualized for relative ranking only**: Compare 10% vs 26% to decide which asset to prioritize.
- **Use realized daily carry for breakeven estimates**: Calculate `daily_carry = capital × current_funding_rate × periods_per_day` and compare to round-trip costs.
- **Recalculate at least weekly**: Funding rates change frequently. Do not plan based on a 14-day window from a month ago.
- **Stress-test with lower rates**: Assume funding drops 20–30% before committing capital. Breakeven should still be reasonable.

### Window and Interval Mismatch Warning

If you are comparing two assets with different data windows or intervals:

- Asset A: 14 days of 8h funding (56 periods)
- Asset B: 7 days of 1h funding (168 periods)

The annualized rates may be misleading if the windows do not overlap in time. Always fetch consistent windows (ideally 14 days for both) before comparing annualized rates across assets.
