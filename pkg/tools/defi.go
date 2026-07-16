package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/defi"
	"github.com/cryptoquantumwave/khunquant/pkg/snapshot"
)

func validWalletAddress(chain, address string) error {
	if strings.EqualFold(chain, "solana") {
		if !defi.IsSolanaAddress(address) {
			return fmt.Errorf("invalid Solana address %q", address)
		}
		return nil
	}
	if _, err := defi.NewEVMClient(chain); err != nil {
		return err
	}
	if !defi.IsEVMAddress(address) {
		return fmt.Errorf("invalid EVM address %q (expect 0x + 40 hex chars)", address)
	}
	return nil
}

func supportedChainList() string {
	names := make([]string, 0, len(defi.DefaultEVMChains)+1)
	for n := range defi.DefaultEVMChains {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(append(names, "solana"), ", ")
}

// ====================== defi_wallet_add ======================

type DeFiWalletAddTool struct{ store *defi.Store }

func NewDeFiWalletAddTool(store *defi.Store) *DeFiWalletAddTool { return &DeFiWalletAddTool{store} }

func (t *DeFiWalletAddTool) Name() string { return NameDeFiWalletAdd }

func (t *DeFiWalletAddTool) Description() string {
	return "Track a DeFi wallet (read-only). Supported chains: " + supportedChainList() + ". " +
		"EVM chains check native + well-known tokens plus any extra contracts you register; Solana enumerates all SPL holdings automatically."
}

func (t *DeFiWalletAddTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chain":   map[string]any{"type": "string", "description": "Chain: " + supportedChainList() + "."},
			"address": map[string]any{"type": "string", "description": "Wallet address (0x… or base58)."},
			"label":   map[string]any{"type": "string", "description": "Human label, e.g. 'hot wallet'."},
			"tokens":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Extra ERC-20 contract addresses to watch (EVM only)."},
		},
		"required": []string{"chain", "address"},
	}
}

func (t *DeFiWalletAddTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	chain := strings.ToLower(stringArg(args, "chain"))
	address := stringArg(args, "address")
	if err := validWalletAddress(chain, address); err != nil {
		return ErrorResult(err.Error())
	}
	var tokens []string
	if list, ok := args["tokens"].([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok && s != "" {
				tokens = append(tokens, s)
			}
		}
	}
	w := defi.Wallet{Chain: chain, Address: address, Label: stringArg(args, "label"), Tokens: tokens}
	if err := t.store.Add(w); err != nil {
		return ErrorResult(fmt.Sprintf("defi_wallet_add: %v", err)).WithError(err)
	}
	label := w.Label
	if label == "" {
		label = defi.ShortAddress(address)
	}
	return UserResult(fmt.Sprintf("✅ Tracking %s wallet %s (%s). Use defi_portfolio to see holdings.", chain, defi.ShortAddress(address), label))
}

// ====================== defi_wallet_remove ======================

type DeFiWalletRemoveTool struct{ store *defi.Store }

func NewDeFiWalletRemoveTool(store *defi.Store) *DeFiWalletRemoveTool {
	return &DeFiWalletRemoveTool{store}
}

func (t *DeFiWalletRemoveTool) Name() string { return NameDeFiWalletRemove }

func (t *DeFiWalletRemoveTool) Description() string {
	return "Stop tracking a DeFi wallet."
}

func (t *DeFiWalletRemoveTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chain":   map[string]any{"type": "string", "description": "Chain of the tracked wallet."},
			"address": map[string]any{"type": "string", "description": "Wallet address."},
		},
		"required": []string{"chain", "address"},
	}
}

func (t *DeFiWalletRemoveTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	removed, err := t.store.Remove(stringArg(args, "chain"), stringArg(args, "address"))
	if err != nil {
		return ErrorResult(fmt.Sprintf("defi_wallet_remove: %v", err)).WithError(err)
	}
	if !removed {
		return UserResult("Wallet not found in the watchlist. Use defi_wallet_list to see tracked wallets.")
	}
	return UserResult("✅ Wallet removed from tracking.")
}

// ====================== defi_wallet_list ======================

type DeFiWalletListTool struct{ store *defi.Store }

func NewDeFiWalletListTool(store *defi.Store) *DeFiWalletListTool { return &DeFiWalletListTool{store} }

func (t *DeFiWalletListTool) Name() string { return NameDeFiWalletList }

func (t *DeFiWalletListTool) Description() string {
	return "List tracked DeFi wallets."
}

func (t *DeFiWalletListTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *DeFiWalletListTool) Execute(_ context.Context, _ map[string]any) *ToolResult {
	wallets := t.store.List()
	if len(wallets) == 0 {
		return UserResult("No DeFi wallets tracked yet. Use defi_wallet_add to start.")
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tracked DeFi wallets (%d):\n", len(wallets)))
	for _, w := range wallets {
		label := w.Label
		if label == "" {
			label = "-"
		}
		extra := ""
		if len(w.Tokens) > 0 {
			extra = fmt.Sprintf(" (+%d watched tokens)", len(w.Tokens))
		}
		sb.WriteString(fmt.Sprintf("  %-10s %-46s %s%s\n", w.Chain, w.Address, label, extra))
	}
	return UserResult(sb.String())
}

// ====================== defi_wallet_balances ======================

type DeFiWalletBalancesTool struct{}

func NewDeFiWalletBalancesTool() *DeFiWalletBalancesTool { return &DeFiWalletBalancesTool{} }

func (t *DeFiWalletBalancesTool) Name() string { return NameDeFiWalletBalances }

func (t *DeFiWalletBalancesTool) Description() string {
	return "Fetch on-chain balances (with USD values) for any wallet address — no tracking required. Chains: " + supportedChainList() + "."
}

func (t *DeFiWalletBalancesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chain":   map[string]any{"type": "string", "description": "Chain: " + supportedChainList() + "."},
			"address": map[string]any{"type": "string", "description": "Wallet address."},
			"tokens":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Extra ERC-20 contracts to check (EVM only)."},
		},
		"required": []string{"chain", "address"},
	}
}

func (t *DeFiWalletBalancesTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	chain := strings.ToLower(stringArg(args, "chain"))
	address := stringArg(args, "address")
	if err := validWalletAddress(chain, address); err != nil {
		return ErrorResult(err.Error())
	}
	var tokens []string
	if list, ok := args["tokens"].([]any); ok {
		for _, v := range list {
			if s, ok := v.(string); ok {
				tokens = append(tokens, s)
			}
		}
	}
	pricer, err := defi.NewPriceClient()
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}
	holdings, warnings, err := defi.FetchWalletHoldings(ctx, defi.Wallet{Chain: chain, Address: address, Tokens: tokens}, pricer)
	if err != nil {
		return ErrorResult(fmt.Sprintf("defi_wallet_balances: %v", err)).WithError(err)
	}
	return UserResult(formatHoldings(holdings, warnings, false))
}

// ====================== defi_portfolio ======================

type DeFiPortfolioTool struct {
	store    *defi.Store
	snapshot *snapshot.Store // optional; nil disables save_snapshot
}

func NewDeFiPortfolioTool(store *defi.Store, snap *snapshot.Store) *DeFiPortfolioTool {
	return &DeFiPortfolioTool{store: store, snapshot: snap}
}

func (t *DeFiPortfolioTool) Name() string { return NameDeFiPortfolio }

func (t *DeFiPortfolioTool) Description() string {
	return "Aggregate all tracked DeFi wallets into one portfolio view with USD totals per chain. " +
		"Set save_snapshot=true to persist positions into the portfolio snapshot store (source=defi)."
}

func (t *DeFiPortfolioTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"save_snapshot": map[string]any{"type": "boolean", "description": "Persist holdings into the snapshot store."},
		},
	}
}

func (t *DeFiPortfolioTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	wallets := t.store.List()
	if len(wallets) == 0 {
		return UserResult("No DeFi wallets tracked yet. Use defi_wallet_add first.")
	}
	pricer, err := defi.NewPriceClient()
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}

	var all []defi.Holding
	var warnings []string
	for _, w := range wallets {
		holdings, warns, err := defi.FetchWalletHoldings(ctx, w, pricer)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s %s: %v", w.Chain, defi.ShortAddress(w.Address), err))
			continue
		}
		for _, warn := range warns {
			warnings = append(warnings, fmt.Sprintf("%s %s: %s", w.Chain, defi.ShortAddress(w.Address), warn))
		}
		all = append(all, holdings...)
	}
	if len(all) == 0 && len(warnings) > 0 {
		return ErrorResult("defi_portfolio: every wallet fetch failed:\n  " + strings.Join(warnings, "\n  "))
	}

	out := formatHoldings(all, warnings, true)

	if save, _ := args["save_snapshot"].(bool); save {
		if t.snapshot == nil {
			out += "\n⚠ Snapshot store unavailable — holdings NOT saved."
		} else {
			positions := make([]snapshot.Position, 0, len(all))
			for _, h := range all {
				account := h.Label
				if account == "" {
					account = defi.ShortAddress(h.Address)
				}
				meta := map[string]string{"chain": h.Chain, "address": h.Address}
				if h.TokenAddr != "" {
					meta["token"] = h.TokenAddr
				}
				if h.IsLP {
					meta["lp"] = "true"
				}
				positions = append(positions, snapshot.Position{
					Source:   "defi:" + h.Chain,
					Account:  account,
					Category: "defi",
					Asset:    h.Asset,
					Quantity: h.Amount,
					Quote:    "USD",
					Price:    h.PriceUSD,
					Value:    h.ValueUSD,
					Meta:     meta,
				})
			}
			snap := &snapshot.Snapshot{TakenAt: time.Now().UTC(), Positions: positions}
			if id, err := t.snapshot.SaveSnapshot(ctx, snap); err != nil {
				out += fmt.Sprintf("\n⚠ Snapshot save failed: %v", err)
			} else {
				out += fmt.Sprintf("\n💾 Saved snapshot #%d (%d positions, source=defi).", id, len(positions))
			}
		}
	}
	return UserResult(out)
}

// formatHoldings renders holdings as a table with per-chain and grand totals.
func formatHoldings(holdings []defi.Holding, warnings []string, groupByWallet bool) string {
	var sb strings.Builder
	if len(holdings) == 0 {
		sb.WriteString("No holdings found (empty wallet, or tokens outside the watched set).\n")
	} else {
		sort.Slice(holdings, func(i, j int) bool {
			if holdings[i].Chain != holdings[j].Chain {
				return holdings[i].Chain < holdings[j].Chain
			}
			return holdings[i].ValueUSD > holdings[j].ValueUSD
		})
		sb.WriteString(fmt.Sprintf("%-10s %-14s %-14s %16s %14s %14s\n",
			"Chain", "Wallet", "Asset", "Amount", "Price USD", "Value USD"))
		sb.WriteString(strings.Repeat("-", 88) + "\n")
		chainTotals := map[string]float64{}
		var total float64
		for _, h := range holdings {
			wallet := h.Label
			if wallet == "" {
				wallet = defi.ShortAddress(h.Address)
			}
			asset := h.Asset
			if h.IsLP {
				asset += " [LP]"
			}
			priceStr, valueStr := "-", "-"
			if h.PriceUSD > 0 {
				priceStr = fmt.Sprintf("%.4f", h.PriceUSD)
				valueStr = fmt.Sprintf("%.2f", h.ValueUSD)
			}
			sb.WriteString(fmt.Sprintf("%-10s %-14s %-14s %16.6f %14s %14s\n",
				h.Chain, wallet, asset, h.Amount, priceStr, valueStr))
			chainTotals[h.Chain] += h.ValueUSD
			total += h.ValueUSD
		}
		sb.WriteString("\nTotals (priced holdings only):\n")
		var chains []string
		for c := range chainTotals {
			chains = append(chains, c)
		}
		sort.Strings(chains)
		for _, c := range chains {
			sb.WriteString(fmt.Sprintf("  %-10s $%.2f\n", c, chainTotals[c]))
		}
		sb.WriteString(fmt.Sprintf("  %-10s $%.2f\n", "TOTAL", total))
	}
	if len(warnings) > 0 {
		sb.WriteString("\n⚠ Warnings:\n")
		for _, w := range warnings {
			sb.WriteString("  - " + w + "\n")
		}
	}
	sb.WriteString("\nNote: EVM chains check native + watched tokens only (plain RPC cannot enumerate all holdings); Solana lists all SPL tokens. Unpriced assets show '-' (CoinGecko may not cover them).")
	return sb.String()
}
