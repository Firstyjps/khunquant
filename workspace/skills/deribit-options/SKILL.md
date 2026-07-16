---
name: deribit-options
description: Read option chains, quote greeks/IV, track option positions, place option orders, and delta-hedge with perpetuals on Deribit with approval-based execution.
---

# Deribit Options Trading

Trade crypto options on Deribit: read chains, analyze greeks and implied volatility, open/close option positions, and keep net delta hedged with perpetuals. All order placement is dry-run first and requires explicit confirmation.

## Tools

| Tool | Purpose |
|---|---|
| `options_chain` | List strikes/expiries with mark price, mark IV, bid/ask, open interest for an underlying (BTC, ETH) |
| `option_quote` | Single instrument detail: delta, gamma, theta, vega, rho, IV, mark, underlying price |
| `option_positions` | Open option positions with per-position greeks + net delta per currency |
| `option_order` | Place buy/sell limit/market option orders (dry-run → confirm) |
| `order_cancel` / `get_open_orders` | Manage resting option orders (provider=deribit) |
| `futures_open_position` / `futures_get_positions` | Delta-hedge leg via Deribit perpetuals |

## Deribit Conventions (critical — get these right)

- **Instrument naming**: `BTC-26DEC25-100000-C` = BTC option, expiry 26 Dec 2025, strike 100,000, Call. `P` = Put. Expiry is 08:00 UTC.
- **Premiums are quoted in the underlying currency**, not USD. A BTC option marked 0.05 costs 0.05 BTC per contract. Convert to USD via underlying price when explaining to the user.
- **Amount is in contracts**: 1 BTC contract = 1 BTC notional (min 0.1); 1 ETH contract = 1 ETH notional (min 1). Never pass USD notional as amount.
- **European-style, cash-settled** in the underlying currency at expiry.
- **Selling options** creates short-premium positions with margin requirements — always surface the risk explicitly before confirming.
- Deribit margins at the **account level**; there is no per-symbol leverage setting.

## Workflows

### Explore / analyze
```
User asks about BTC options
  → options_chain underlying=BTC (optionally expiry=26DEC25, type=call, strike range)
  → for candidates: option_quote for greeks + IV
  → explain premium in both BTC and USD terms, breakeven, and theta decay
```

### Open a position
```
User picks an instrument and size
  → option_quote to show current mark/greeks
  → option_order side=buy/sell amount=<contracts> price=<premium> (dry-run)
  → present: premium cost (underlying + USD), max loss/gain, breakeven, margin note if selling
  → user confirms → option_order confirm=true
  → verify with option_positions
```

### Delta hedging
```
User wants delta-neutral book
  → option_positions — read "Net delta by currency"
  → positive net delta → SHORT the perp; negative → LONG the perp
  → hedge size ≈ |net delta| in underlying units on BTC/USD:BTC or ETH/USD:ETH (or USDC-linear)
  → futures_open_position provider=deribit ... (dry-run → confirm)
  → re-check option_positions + futures_get_positions; re-hedge when |net delta| drifts beyond the user's threshold
```

### Strategy patterns to suggest when asked
- **Covered call**: hold spot/long perp + sell OTM call (income, capped upside).
- **Cash-secured put**: sell OTM put with collateral (acquire-lower or income).
- **Straddle/strangle**: buy call + put around spot before expected volatility.
- **Vertical spread**: buy/sell same-expiry different strikes to cap premium and risk.
Explain max loss / max gain / breakeven for the chosen structure before any order.

## Guardrails

- `option_order` enforces: `trading_risk.allow_leverage`, per-account trade permission, daily-loss circuit breaker, rate limit, and dry-run-before-confirm. Do not attempt to bypass them.
- Quote every financial figure from tool output — never estimate greeks, IV, or premiums yourself.
- For sells, state margin impact and assignment-at-expiry behavior in the confirmation summary.
- Options on Deribit expire 08:00 UTC — surface time-to-expiry when it is under 48h.
- API credentials go in `exchanges.deribit.accounts` (config); public data (chains/quotes) works without keys. `exchanges.deribit.testnet=true` routes to test.deribit.com.
