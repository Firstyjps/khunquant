package defi

import (
	"context"
	"os"
	"testing"
)

// Live tests hit real public RPCs + CoinGecko. Gated behind
// KHUNQUANT_LIVE_TEST=1 so CI stays hermetic. Uses well-known public
// addresses (read-only lookups).
func TestLiveEVMHoldings(t *testing.T) {
	if os.Getenv("KHUNQUANT_LIVE_TEST") != "1" {
		t.Skip("set KHUNQUANT_LIVE_TEST=1 to run live RPC tests")
	}
	ctx := context.Background()
	pricer, err := NewPriceClient()
	if err != nil {
		t.Fatalf("pricer: %v", err)
	}
	// vitalik.eth — guaranteed to hold ETH.
	w := Wallet{Chain: "ethereum", Address: "0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", Label: "vitalik"}
	holdings, warnings, err := FetchWalletHoldings(ctx, w, pricer)
	if err != nil {
		t.Fatalf("FetchWalletHoldings: %v", err)
	}
	if len(holdings) == 0 {
		t.Fatal("expected at least the native ETH holding")
	}
	foundETH := false
	for _, h := range holdings {
		t.Logf("%s %s amount=%.6f price=%.2f value=%.2f", h.Chain, h.Asset, h.Amount, h.PriceUSD, h.ValueUSD)
		if h.Asset == "ETH" && h.Amount > 0 {
			foundETH = true
			if h.PriceUSD <= 0 {
				t.Log("warning: ETH price is 0 (CoinGecko rate limit?)")
			}
		}
	}
	if !foundETH {
		t.Fatal("no ETH holding found for vitalik.eth")
	}
	for _, w := range warnings {
		t.Logf("warning: %s", w)
	}
}

func TestLiveSolanaHoldings(t *testing.T) {
	if os.Getenv("KHUNQUANT_LIVE_TEST") != "1" {
		t.Skip("set KHUNQUANT_LIVE_TEST=1 to run live RPC tests")
	}
	ctx := context.Background()
	sc, err := NewSolanaClient()
	if err != nil {
		t.Fatalf("solana client: %v", err)
	}
	// Solana Foundation-associated address with permanent SOL balance
	// (binance cold wallet — large, stable, public).
	addr := "9WzDXwBbmkg8ZTbNMqUxvQRAyrZzDsGYdLVL9zYtAWWM"
	sol, err := sc.NativeBalance(ctx, addr)
	if err != nil {
		t.Fatalf("NativeBalance: %v", err)
	}
	if sol <= 0 {
		t.Fatalf("expected positive SOL balance, got %g", sol)
	}
	t.Logf("SOL balance: %.4f", sol)

	spl, err := sc.SPLBalances(ctx, addr)
	if err != nil {
		t.Fatalf("SPLBalances: %v", err)
	}
	t.Logf("SPL holdings: %d", len(spl))
}
