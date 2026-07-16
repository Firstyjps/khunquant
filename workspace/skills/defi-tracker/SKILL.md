---
name: defi-tracker
description: Track DeFi wallets read-only across Ethereum/BSC/Polygon/Arbitrum/Base/Optimism/Solana — balances, USD values, LP detection, and portfolio snapshots. No API keys required.
---

# DeFi Wallet Tracker

Read-only, on-chain portfolio tracking via public RPC endpoints and DefiLlama prices. No API keys, no signing — the bot can never move on-chain funds with these tools.

## Tools

| Tool | Purpose |
|---|---|
| `defi_wallet_add` | Track a wallet (chain, address, label, extra ERC-20 contracts to watch) |
| `defi_wallet_remove` / `defi_wallet_list` | Manage the watchlist |
| `defi_wallet_balances` | Ad-hoc balance lookup for ANY address — no tracking needed |
| `defi_portfolio` | Aggregate all tracked wallets: per-chain + grand USD totals; `save_snapshot=true` persists into the portfolio snapshot store |
| `query_snapshots` / `snapshot_summary` | Historical DeFi portfolio value (`source` starts with `defi:`) |

## Chain coverage & discovery semantics (explain to the user when relevant)

- **Solana**: full SPL holdings discovered automatically (RPC supports enumeration). Token symbols resolve via the price feed.
- **EVM chains** (ethereum, bsc, polygon, arbitrum, base, optimism): plain RPC cannot enumerate token holdings — the tracker checks the native coin + well-known tokens (USDT/USDC/WETH/WBTC/DAI per chain) + any contracts registered on the wallet. If the user holds an unusual token, ask for the contract address and add it via `defi_wallet_add` `tokens`.
- **LP tokens** are flagged `[LP]` by symbol convention (UNI-V2, Cake-LP, SLP, …). Underlying pool composition and farming rewards are NOT decomposed — for deep protocol positions (Uniswap V3 NFTs, staked farms), suggest checking the protocol dashboard, or use web_fetch on a public explorer page if the user asks.
- **Prices** come from DefiLlama (keyless). Assets missing from the feed display `-` and are excluded from totals — say so rather than guessing a value.

## Workflows

### Onboard wallets
```
User: "track กระเป๋า 0xABC… บน Arbitrum ชื่อ hot wallet"
  → defi_wallet_add chain=arbitrum address=0xABC… label="hot wallet"
  → defi_portfolio  (show them what it sees immediately)
```

### Portfolio review
```
defi_portfolio → per-chain totals + TOTAL USD
  → answer questions from tool output only (never estimate prices)
  → save_snapshot=true when the user wants history kept
```

### Trend over time
```
Snapshots saved with source "defi:<chain>", category "defi", quote USD
  → snapshot_summary / query_snapshots filtered by source prefix "defi:"
  → report change vs previous snapshot
```

### Proactive monitoring (optional)
Create a cron job that runs `defi_portfolio save_snapshot=true` on a schedule (e.g. daily 07:00) and alerts when TOTAL moves more than a user-set percentage vs the previous snapshot.

## Guardrails

- These tools are **read-only**: no private keys, no transactions, no approvals. If the user asks to move on-chain funds, explain that on-chain execution is not supported (exchange trading via the trading tools is).
- Never fabricate USD values for unpriced assets; show `-` and name the asset.
- Public RPCs are best-effort: each EVM chain has fallback endpoints, but a chain can still be temporarily unreachable — report failures per wallet and continue with the rest.
- Wallet addresses are public data, but treat labels + the combined portfolio as sensitive: don't repeat them into group chats uninvited.
