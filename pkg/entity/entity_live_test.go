package entity

import (
	"context"
	"os"
	"testing"

	"github.com/cryptoquantumwave/khunquant/pkg/defi"
)

// Live tests hit mempool.space/blockstream.info + DefiLlama. Gated behind
// KHUNQUANT_LIVE_TEST=1 so CI stays hermetic. Read-only lookups on a
// well-known public address.
func TestLiveBTCEntityHoldings(t *testing.T) {
	if os.Getenv("KHUNQUANT_LIVE_TEST") != "1" {
		t.Skip("set KHUNQUANT_LIVE_TEST=1 to run live esplora tests")
	}
	ctx := context.Background()
	pricer, err := defi.NewPriceClient()
	if err != nil {
		t.Fatalf("pricer: %v", err)
	}
	e := Entity{
		Slug: "binance", Name: "Binance",
		Addresses: []Address{{Chain: "bitcoin", Address: "34xp4vRoCGJym3xR7yCVPFHoCNxv4Twseo", Label: "cold"}},
	}
	holdings, warnings, err := FetchHoldings(ctx, e, pricer)
	if err != nil {
		t.Fatalf("FetchHoldings: %v", err)
	}
	t.Logf("warnings: %v", warnings)
	if len(holdings) != 1 || holdings[0].Asset != "BTC" || holdings[0].Amount <= 0 {
		t.Fatalf("unexpected holdings: %+v", holdings)
	}
	if holdings[0].ValueUSD <= 0 {
		t.Errorf("expected priced BTC holding, got %+v", holdings[0])
	}

	c, err := NewBTCClient()
	if err != nil {
		t.Fatal(err)
	}
	trs, err := c.RecentTransfers(ctx, e.Addresses[0].Address, 5)
	if err != nil {
		t.Fatalf("RecentTransfers: %v", err)
	}
	if len(trs) == 0 {
		t.Fatal("expected at least one transfer for an active address")
	}
}
