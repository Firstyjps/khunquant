---
name: onchain-entity
description: Arkham-style on-chain entity tracker — register named entities (funds, exchanges, whales) with their Bitcoin/EVM/Solana addresses, then read aggregated holdings and recent BTC transfer flows from keyless public APIs. No Arkham login needed.
---

# On-chain Entity Tracker

Track what big players (BlackRock, exchanges, whales) hold and move on-chain — the same questions Arkham answers, but from keyless public data sources that don't block bots (Esplora: mempool.space/blockstream.info for Bitcoin; public JSON-RPC + DefiLlama for EVM/Solana).

## Tools

| Tool | Purpose |
|---|---|
| `entity_add` | Register/extend an entity with addresses (batch). Chains: bitcoin + EVM chains + solana |
| `entity_list` / `entity_remove` | Manage the registry |
| `entity_holdings` | Aggregate balances across all the entity's addresses, priced in USD |
| `entity_flows` | Recent transfers per Bitcoin address: direction, net flow, labeled counterparties |

## Where addresses come from (important — accuracy over guessing)

**Never invent or "recall" entity addresses.** Wrong attribution = wrong conclusions about money. Get addresses from:

1. **Arkham entity pages** (e.g. `arkm.com/explorer/entity/blackrock`) — plain fetch fails on Cloudflare, so use the **agent-browser skill** to open the page, read the tagged address list, then `entity_add` them with a note like "source: Arkham, 2026-07-20".
2. User pastes addresses directly (from Arkham, Etherscan, news, ETF disclosures).
3. Well-known published addresses only when highly confident, and say so in the note.

Counterparty labeling in `entity_flows` uses this same registry — the more entities registered (Coinbase Prime, Binance, …), the more readable the flow output becomes.

## Limits (state honestly when relevant)

- `entity_flows` covers **Bitcoin only** for now (last ≤25 tx per address, first 10 BTC addresses). EVM/Solana transfer history is a future phase — holdings still work there.
- EVM balances check native + well-known tokens (plus per-address extra contracts) — not a full token enumeration.
- Public endpoints rate-limit: holdings scan caps at 30 addresses per call.
- Net flow shown = over the fetched window, **not** total/all-time flow.

## Workflow example

```
User: "BlackRock ถือ BTC เท่าไหร่ ขยับล่าสุดยังไง"
  → entity_list — registered already?
  → if not: agent-browser → arkm.com/explorer/entity/blackrock → collect BTC addresses
  → entity_add name="BlackRock" addresses=[...] note="source: Arkham <date>"
  → entity_holdings slug=blackrock   (balances + USD)
  → entity_flows slug=blackrock      (recent moves + counterparties)
  → answer with numbers from tools; state the data window and source date
```
