package entity

import (
	"context"
	"fmt"
	"strings"

	"github.com/cryptoquantumwave/khunquant/pkg/defi"
)

// maxAddressesPerFetch caps how many addresses one holdings call will scan —
// public endpoints rate-limit, and a chat reply doesn't need more.
const maxAddressesPerFetch = 30

// FetchHoldings aggregates balances for every address of an entity: Bitcoin
// via Esplora, EVM/Solana via the defi package. Prices come from DefiLlama;
// price failures degrade to unpriced holdings plus a warning.
func FetchHoldings(ctx context.Context, e Entity, pricer *defi.PriceClient) ([]defi.Holding, []string, error) {
	var holdings []defi.Holding
	var warnings []string

	addrs := e.Addresses
	if len(addrs) > maxAddressesPerFetch {
		warnings = append(warnings, fmt.Sprintf("entity has %d addresses; scanning the first %d only", len(addrs), maxAddressesPerFetch))
		addrs = addrs[:maxAddressesPerFetch]
	}

	var btc *BTCClient
	for _, a := range addrs {
		switch strings.ToLower(a.Chain) {
		case "bitcoin", "btc":
			if btc == nil {
				c, err := NewBTCClient()
				if err != nil {
					return nil, warnings, err
				}
				btc = c
			}
			sum, err := btc.Summary(ctx, a.Address)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("bitcoin %s: %v", defi.ShortAddress(a.Address), err))
				continue
			}
			if sum.BalanceBTC > 0 {
				holdings = append(holdings, defi.Holding{
					Chain: "bitcoin", Address: a.Address, Label: a.Label,
					Asset: "BTC", Amount: sum.BalanceBTC,
				})
			}
		default:
			w := defi.Wallet{Chain: a.Chain, Address: a.Address, Label: a.Label, Tokens: a.Tokens}
			hs, warns, err := defi.FetchWalletHoldings(ctx, w, pricer)
			warnings = append(warnings, warns...)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s %s: %v", a.Chain, defi.ShortAddress(a.Address), err))
				continue
			}
			holdings = append(holdings, hs...)
		}
	}

	// Price the BTC holdings (defi.FetchWalletHoldings already priced the rest).
	if pricer != nil {
		if warn := priceBTCHoldings(ctx, holdings, pricer); warn != "" {
			warnings = append(warnings, warn)
		}
	}
	return holdings, warnings, nil
}

func priceBTCHoldings(ctx context.Context, holdings []defi.Holding, pricer *defi.PriceClient) string {
	needed := false
	for _, h := range holdings {
		if h.Chain == "bitcoin" && h.PriceUSD == 0 {
			needed = true
			break
		}
	}
	if !needed {
		return ""
	}
	key := defi.NativeKey("bitcoin")
	prices, err := pricer.Prices(ctx, []string{key})
	if err != nil {
		return fmt.Sprintf("BTC price lookup: %v", err)
	}
	p, ok := prices[strings.ToLower(key)]
	if !ok {
		return "BTC price unavailable from DefiLlama"
	}
	for i := range holdings {
		if holdings[i].Chain == "bitcoin" {
			holdings[i].PriceUSD = p.PriceUSD
			holdings[i].ValueUSD = holdings[i].Amount * p.PriceUSD
		}
	}
	return ""
}
