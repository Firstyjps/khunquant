package tools

// Arkham-style on-chain entity tracker tools: register named entities
// (funds, exchanges, whales) with their addresses, then read aggregated
// holdings and recent transaction flows from keyless public data sources.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/defi"
	"github.com/cryptoquantumwave/khunquant/pkg/entity"
)

func entityChainList() string { return "bitcoin, " + supportedChainList() }

func validEntityAddress(chain, address string) error {
	if strings.EqualFold(chain, "bitcoin") || strings.EqualFold(chain, "btc") {
		if !entity.IsBTCAddress(address) {
			return fmt.Errorf("invalid Bitcoin address %q", address)
		}
		return nil
	}
	return validWalletAddress(chain, address)
}

// ====================== entity_add ======================

type EntityAddTool struct{ store *entity.Store }

func NewEntityAddTool(store *entity.Store) *EntityAddTool { return &EntityAddTool{store} }

func (t *EntityAddTool) Name() string { return NameEntityAdd }

func (t *EntityAddTool) Description() string {
	return "Register (or extend) an on-chain entity — a named owner of addresses, e.g. 'BlackRock' or 'Binance'. " +
		"Pass one or many addresses at once. Supported chains: " + entityChainList() + ". " +
		"Use entity_holdings for balances and entity_flows for recent transfers."
}

func (t *EntityAddTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "Entity display name, e.g. 'BlackRock'. Reusing a name merges addresses into the existing entity."},
			"note": map[string]any{"type": "string", "description": "Optional note (data source, date verified)."},
			"addresses": map[string]any{
				"type":        "array",
				"description": "Addresses owned by the entity.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"chain":   map[string]any{"type": "string", "description": "Chain: " + entityChainList() + "."},
						"address": map[string]any{"type": "string"},
						"label":   map[string]any{"type": "string", "description": "e.g. 'IBIT custody 1'."},
					},
					"required": []string{"chain", "address"},
				},
			},
		},
		"required": []string{"name", "addresses"},
	}
}

func (t *EntityAddTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	name := strings.TrimSpace(stringArg(args, "name"))
	if name == "" {
		return ErrorResult("entity_add: name is required")
	}
	rawList, ok := args["addresses"].([]any)
	if !ok || len(rawList) == 0 {
		return ErrorResult("entity_add: addresses must be a non-empty array")
	}
	var addrs []entity.Address
	for i, raw := range rawList {
		m, ok := raw.(map[string]any)
		if !ok {
			return ErrorResult(fmt.Sprintf("entity_add: addresses[%d] must be an object {chain, address, label?}", i))
		}
		a := entity.Address{
			Chain:   strings.ToLower(strings.TrimSpace(stringArg(m, "chain"))),
			Address: strings.TrimSpace(stringArg(m, "address")),
			Label:   stringArg(m, "label"),
		}
		if a.Chain == "btc" {
			a.Chain = "bitcoin"
		}
		if err := validEntityAddress(a.Chain, a.Address); err != nil {
			return ErrorResult(fmt.Sprintf("entity_add: addresses[%d]: %v", i, err))
		}
		addrs = append(addrs, a)
	}
	e, added, err := t.store.Upsert(name, "", stringArg(args, "note"), addrs)
	if err != nil {
		return ErrorResult(fmt.Sprintf("entity_add: %v", err)).WithError(err)
	}
	return UserResult(fmt.Sprintf("✅ Entity %q (slug: %s): +%d address(es), %d total. Use entity_holdings slug=%s to see balances.",
		e.Name, e.Slug, added, len(e.Addresses), e.Slug))
}

// ====================== entity_remove ======================

type EntityRemoveTool struct{ store *entity.Store }

func NewEntityRemoveTool(store *entity.Store) *EntityRemoveTool { return &EntityRemoveTool{store} }

func (t *EntityRemoveTool) Name() string { return NameEntityRemove }

func (t *EntityRemoveTool) Description() string {
	return "Remove a tracked entity entirely, or a single address from it (pass chain + address)."
}

func (t *EntityRemoveTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"slug":    map[string]any{"type": "string", "description": "Entity slug (see entity_list)."},
			"chain":   map[string]any{"type": "string", "description": "With address: remove only this address."},
			"address": map[string]any{"type": "string"},
		},
		"required": []string{"slug"},
	}
}

func (t *EntityRemoveTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	slug := strings.TrimSpace(stringArg(args, "slug"))
	removed, err := t.store.Remove(slug, stringArg(args, "chain"), stringArg(args, "address"))
	if err != nil {
		return ErrorResult(fmt.Sprintf("entity_remove: %v", err)).WithError(err)
	}
	if !removed {
		return UserResult(fmt.Sprintf("Nothing removed — no match for %q.", slug))
	}
	return UserResult("✅ Removed.")
}

// ====================== entity_list ======================

type EntityListTool struct{ store *entity.Store }

func NewEntityListTool(store *entity.Store) *EntityListTool { return &EntityListTool{store} }

func (t *EntityListTool) Name() string { return NameEntityList }

func (t *EntityListTool) Description() string {
	return "List all tracked on-chain entities with their addresses."
}

func (t *EntityListTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *EntityListTool) Execute(_ context.Context, _ map[string]any) *ToolResult {
	entities := t.store.List()
	if len(entities) == 0 {
		return UserResult("No entities tracked yet. Use entity_add first.")
	}
	var sb strings.Builder
	for _, e := range entities {
		sb.WriteString(fmt.Sprintf("• %s (slug: %s) — %d address(es)\n", e.Name, e.Slug, len(e.Addresses)))
		if e.Note != "" {
			sb.WriteString(fmt.Sprintf("  note: %s\n", e.Note))
		}
		for _, a := range e.Addresses {
			label := a.Label
			if label != "" {
				label = " — " + label
			}
			sb.WriteString(fmt.Sprintf("  %-9s %s%s\n", a.Chain, a.Address, label))
		}
	}
	return UserResult(sb.String())
}

// ====================== entity_holdings ======================

type EntityHoldingsTool struct{ store *entity.Store }

func NewEntityHoldingsTool(store *entity.Store) *EntityHoldingsTool {
	return &EntityHoldingsTool{store}
}

func (t *EntityHoldingsTool) Name() string { return NameEntityHoldings }

func (t *EntityHoldingsTool) Description() string {
	return "Aggregate on-chain balances (BTC + EVM + Solana) across all addresses of a tracked entity, priced in USD. Read-only, keyless public data."
}

func (t *EntityHoldingsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"slug": map[string]any{"type": "string", "description": "Entity slug (see entity_list)."},
		},
		"required": []string{"slug"},
	}
}

func (t *EntityHoldingsTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	slug := strings.TrimSpace(stringArg(args, "slug"))
	e, ok := t.store.Get(slug)
	if !ok {
		return ErrorResult(fmt.Sprintf("entity_holdings: unknown entity %q (see entity_list)", slug))
	}
	if len(e.Addresses) == 0 {
		return UserResult(fmt.Sprintf("Entity %q has no addresses yet — add some with entity_add.", e.Name))
	}
	pricer, err := defi.NewPriceClient()
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}
	holdings, warnings, err := entity.FetchHoldings(ctx, e, pricer)
	if err != nil {
		return ErrorResult(fmt.Sprintf("entity_holdings: %v", err)).WithError(err)
	}
	header := fmt.Sprintf("Entity: %s (%d addresses)\n\n", e.Name, len(e.Addresses))
	return UserResult(header + formatHoldings(holdings, warnings, false))
}

// ====================== entity_flows ======================

type EntityFlowsTool struct{ store *entity.Store }

func NewEntityFlowsTool(store *entity.Store) *EntityFlowsTool { return &EntityFlowsTool{store} }

func (t *EntityFlowsTool) Name() string { return NameEntityFlows }

func (t *EntityFlowsTool) Description() string {
	return "Recent on-chain transfers (inflow/outflow) for a tracked entity's Bitcoin addresses, with net flow and labeled counterparties. " +
		"EVM/Solana transfer history is not supported yet — use entity_holdings for balances there."
}

func (t *EntityFlowsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"slug":  map[string]any{"type": "string", "description": "Entity slug (see entity_list)."},
			"limit": map[string]any{"type": "integer", "description": "Max transfers per address (default 10, max 25)."},
		},
		"required": []string{"slug"},
	}
}

func (t *EntityFlowsTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	slug := strings.TrimSpace(stringArg(args, "slug"))
	e, ok := t.store.Get(slug)
	if !ok {
		return ErrorResult(fmt.Sprintf("entity_flows: unknown entity %q (see entity_list)", slug))
	}
	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	var btcAddrs []entity.Address
	skipped := 0
	for _, a := range e.Addresses {
		if a.Chain == "bitcoin" {
			btcAddrs = append(btcAddrs, a)
		} else {
			skipped++
		}
	}
	if len(btcAddrs) == 0 {
		return UserResult(fmt.Sprintf("Entity %q has no Bitcoin addresses — transfer history currently supports Bitcoin only.", e.Name))
	}
	if len(btcAddrs) > maxFlowAddresses {
		btcAddrs = btcAddrs[:maxFlowAddresses]
	}

	client, err := entity.NewBTCClient()
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Entity: %s — recent Bitcoin transfers\n", e.Name))
	if skipped > 0 {
		sb.WriteString(fmt.Sprintf("(%d non-Bitcoin address(es) skipped — flows are BTC-only for now)\n", skipped))
	}
	var netBTC float64
	var warnings []string
	for _, a := range btcAddrs {
		transfers, err := client.RecentTransfers(ctx, a.Address, limit)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", defi.ShortAddress(a.Address), err))
			continue
		}
		name := a.Label
		if name == "" {
			name = defi.ShortAddress(a.Address)
		}
		sb.WriteString(fmt.Sprintf("\n%s (%s):\n", name, defi.ShortAddress(a.Address)))
		if len(transfers) == 0 {
			sb.WriteString("  no transactions found\n")
			continue
		}
		for _, tr := range transfers {
			netBTC += tr.AmountBTC
			dir := "IN "
			if tr.AmountBTC < 0 {
				dir = "OUT"
			}
			when := "unconfirmed"
			if tr.Confirmed {
				when = tr.Time.Format("2006-01-02 15:04")
			}
			sb.WriteString(fmt.Sprintf("  %s %s %+.8f BTC  tx %s…\n", when, dir, tr.AmountBTC, tr.TxID[:12]))
			for _, cp := range tr.Counterparties {
				label := t.store.LabelFor("bitcoin", cp.Address)
				if label == "" {
					label = defi.ShortAddress(cp.Address)
				}
				sb.WriteString(fmt.Sprintf("      ↔ %s (%.8f BTC)\n", label, cp.ValueBTC))
			}
		}
	}
	sb.WriteString(fmt.Sprintf("\nNet flow over shown transfers: %+.8f BTC\n", netBTC))
	sb.WriteString(fmt.Sprintf("(as of %s — last %d tx/address; not a full history)\n",
		time.Now().UTC().Format("2006-01-02 15:04 UTC"), limit))
	if len(warnings) > 0 {
		sb.WriteString("\n⚠️ " + strings.Join(warnings, "\n⚠️ ") + "\n")
	}
	return UserResult(sb.String())
}

// maxFlowAddresses caps per-call address fan-out against public API limits.
const maxFlowAddresses = 10
