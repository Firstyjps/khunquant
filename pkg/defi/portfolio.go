package defi

import (
	"context"
	"fmt"
	"strings"
)

// Holding is one asset position inside a tracked wallet.
type Holding struct {
	Chain     string
	Address   string // wallet address
	Label     string
	Asset     string // display symbol or shortened mint/contract
	TokenAddr string // contract/mint ("" = native coin)
	Amount    float64
	PriceUSD  float64
	ValueUSD  float64
	IsLP      bool
}

// lpMarkers identify liquidity-pool tokens by symbol convention.
var lpMarkers = []string{"-LP", "UNI-V2", "SLP", "CAKE-LP", "JLP", " LP"}

func looksLikeLPToken(symbol string) bool {
	u := strings.ToUpper(symbol)
	for _, m := range lpMarkers {
		if strings.Contains(u, m) {
			return true
		}
	}
	return false
}

// FetchWalletHoldings fetches native + token balances for one wallet and
// prices them in USD via DefiLlama. Price failures degrade gracefully:
// holdings are still returned unpriced plus a warning.
func FetchWalletHoldings(ctx context.Context, w Wallet, pricer *PriceClient) ([]Holding, []string, error) {
	chain := strings.ToLower(strings.TrimSpace(w.Chain))
	var holdings []Holding
	var warnings []string
	var err error
	if chain == "solana" {
		holdings, warnings, err = fetchSolanaBalances(ctx, w)
	} else {
		holdings, warnings, err = fetchEVMBalances(ctx, w)
	}
	if err != nil {
		return nil, warnings, err
	}
	if pricer != nil && len(holdings) > 0 {
		if warn := priceHoldings(ctx, holdings, pricer); warn != "" {
			warnings = append(warnings, warn)
		}
	}
	return holdings, warnings, nil
}

func fetchSolanaBalances(ctx context.Context, w Wallet) ([]Holding, []string, error) {
	var warnings []string
	sc, err := NewSolanaClient()
	if err != nil {
		return nil, nil, err
	}

	sol, err := sc.NativeBalance(ctx, w.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("solana balance: %w", err)
	}
	spl, err := sc.SPLBalances(ctx, w.Address)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("SPL token scan failed: %v", err))
	}

	var holdings []Holding
	if sol > 0 {
		holdings = append(holdings, Holding{
			Chain: "solana", Address: w.Address, Label: w.Label,
			Asset: "SOL", Amount: sol,
		})
	}
	for _, b := range spl {
		holdings = append(holdings, Holding{
			Chain: "solana", Address: w.Address, Label: w.Label,
			Asset: ShortAddress(b.Mint), TokenAddr: b.Mint, Amount: b.Amount,
		})
	}
	return holdings, warnings, nil
}

func fetchEVMBalances(ctx context.Context, w Wallet) ([]Holding, []string, error) {
	var warnings []string
	ec, err := NewEVMClient(w.Chain)
	if err != nil {
		return nil, nil, err
	}
	chain := ec.Chain()

	native, err := ec.NativeBalance(ctx, w.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("%s balance: %w", chain.Name, err)
	}

	var holdings []Holding
	if native > 0 {
		holdings = append(holdings, Holding{
			Chain: chain.Name, Address: w.Address, Label: w.Label,
			Asset: chain.NativeSymbol, Amount: native,
		})
	}

	// Watch tokens: chain defaults + wallet-specific extras (deduped).
	seen := map[string]bool{}
	var tokens []string
	for _, t := range append(append([]string{}, DefaultWatchTokens[chain.Name]...), w.Tokens...) {
		lt := strings.ToLower(strings.TrimSpace(t))
		if lt == "" || seen[lt] || !IsEVMAddress(lt) {
			continue
		}
		seen[lt] = true
		tokens = append(tokens, lt)
	}

	for _, token := range tokens {
		bal, _, err := ec.ERC20Balance(ctx, token, w.Address)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("token %s: %v", ShortAddress(token), err))
			continue
		}
		if bal <= 0 {
			continue
		}
		symbol := ec.ERC20Symbol(ctx, token)
		holdings = append(holdings, Holding{
			Chain: chain.Name, Address: w.Address, Label: w.Label,
			Asset: symbol, TokenAddr: token, Amount: bal,
			IsLP: looksLikeLPToken(symbol),
		})
	}
	return holdings, warnings, nil
}

// priceHoldings resolves USD prices for all holdings in ONE DefiLlama batch.
// Solana mints also pick up their symbol from the price feed (RPC only gives
// the mint address). Returns a warning string ("" = none).
func priceHoldings(ctx context.Context, holdings []Holding, pricer *PriceClient) string {
	keys := make([]string, 0, len(holdings))
	for _, h := range holdings {
		keys = append(keys, priceKeyFor(h))
	}
	prices, err := pricer.Prices(ctx, keys)
	if err != nil {
		return fmt.Sprintf("price lookup: %v", err)
	}
	for i := range holdings {
		p, ok := prices[strings.ToLower(priceKeyFor(holdings[i]))]
		if !ok {
			continue
		}
		holdings[i].PriceUSD = p.PriceUSD
		holdings[i].ValueUSD = holdings[i].Amount * p.PriceUSD
		// Upgrade shortened-mint display names with the feed's symbol.
		if holdings[i].TokenAddr != "" && p.Symbol != "" && strings.Contains(holdings[i].Asset, "…") {
			holdings[i].Asset = p.Symbol
			holdings[i].IsLP = looksLikeLPToken(p.Symbol)
		}
	}
	return ""
}

func priceKeyFor(h Holding) string {
	if h.TokenAddr != "" {
		return TokenKey(h.Chain, h.TokenAddr)
	}
	// Native coin: via CoinGecko id mapping.
	if h.Chain == "solana" {
		return NativeKey("solana")
	}
	if chain, ok := DefaultEVMChains[h.Chain]; ok {
		return NativeKey(chain.NativeCoinGeckoID)
	}
	return NativeKey(h.Chain)
}
