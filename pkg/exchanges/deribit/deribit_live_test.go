package deribit

import (
	"context"
	"os"
	"testing"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
)

// TestLivePublicData exercises the real Deribit public API (no credentials).
// Skipped unless KHUNQUANT_LIVE_TEST=1 so CI stays hermetic.
func TestLivePublicData(t *testing.T) {
	if os.Getenv("KHUNQUANT_LIVE_TEST") != "1" {
		t.Skip("set KHUNQUANT_LIVE_TEST=1 to run live Deribit API tests")
	}

	a, err := newAdapter(config.ExchangeAccount{}, false)
	if err != nil {
		t.Fatalf("newAdapter: %v", err)
	}
	ctx := context.Background()

	markets, err := a.LoadOptionMarkets(ctx)
	if err != nil {
		t.Fatalf("LoadOptionMarkets: %v", err)
	}
	if len(markets) == 0 {
		t.Fatal("no option markets returned")
	}
	t.Logf("option markets: %d", len(markets))

	chain, err := a.FetchOptionChain(ctx, "BTC")
	if err != nil {
		t.Fatalf("FetchOptionChain: %v", err)
	}
	if len(chain.Chains) == 0 {
		t.Fatal("empty BTC option chain")
	}
	t.Logf("BTC chain entries: %d", len(chain.Chains))

	// Verify the raw Info keys the chain tool depends on actually exist.
	checked := 0
	var sampleSymbol string
	for sym, opt := range chain.Chains {
		if opt.Info == nil {
			continue
		}
		name, _ := opt.Info["instrument_name"].(string)
		if name == "" {
			t.Fatalf("chain entry %s missing instrument_name in Info", sym)
		}
		if _, ok := opt.Info["mark_price"]; !ok {
			t.Fatalf("chain entry %s missing mark_price in Info (keys: %v)", sym, infoKeys(opt.Info))
		}
		if _, ok := opt.Info["mark_iv"]; !ok {
			t.Fatalf("chain entry %s missing mark_iv in Info", sym)
		}
		if _, ok := opt.Info["underlying_price"]; !ok {
			t.Fatalf("chain entry %s missing underlying_price in Info", sym)
		}
		sampleSymbol = sym
		checked++
		if checked >= 5 {
			break
		}
	}
	if checked == 0 {
		t.Fatal("no chain entries had raw Info to verify")
	}

	g, err := a.FetchGreeks(ctx, sampleSymbol)
	if err != nil {
		t.Fatalf("FetchGreeks(%s): %v", sampleSymbol, err)
	}
	if g.Delta == nil {
		t.Fatalf("FetchGreeks(%s): nil delta", sampleSymbol)
	}
	t.Logf("greeks %s: delta=%v markIV=%v mark=%v underlying=%v",
		sampleSymbol, *g.Delta, deref(g.MarkImpliedVolatility), deref(g.MarkPrice), deref(g.UnderlyingPrice))

	price, err := a.FetchPrice(ctx, "BTC", "USDT")
	if err != nil {
		t.Fatalf("FetchPrice BTC/USDT: %v", err)
	}
	if price <= 0 {
		t.Fatalf("FetchPrice returned %g", price)
	}
	t.Logf("BTC price: %.2f", price)
}

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func infoKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
