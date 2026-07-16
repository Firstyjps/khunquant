package rebalance

import (
	"math"
	"strings"
	"testing"
)

func basePlan() Plan {
	return Plan{
		Name: "test", Provider: "binance", Quote: "USDT",
		Targets:      map[string]float64{"BTC": 50, "ETH": 30, "USDT": 20},
		TolerancePct: 5, MinTradeUSD: 10,
	}
}

func states(btc, btcPx, eth, ethPx, usdt float64) []AssetState {
	return []AssetState{
		{Asset: "BTC", Amount: btc, PriceQuote: btcPx},
		{Asset: "ETH", Amount: eth, PriceQuote: ethPx},
		{Asset: "USDT", Amount: usdt, PriceQuote: 1},
	}
}

func TestEvaluateBalancedPortfolioNoTrades(t *testing.T) {
	// 5000/3000/2000 on a 10k portfolio = exactly 50/30/20.
	prop, err := Evaluate(states(0.05, 100000, 1.5, 2000, 2000), basePlan())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if prop.NeedsRebalance {
		t.Errorf("balanced portfolio must not need rebalance, trades=%v", prop.Trades)
	}
	if math.Abs(prop.TotalValueQuote-10000) > 0.01 {
		t.Errorf("total = %g, want 10000", prop.TotalValueQuote)
	}
}

func TestEvaluateOverweightSellsFirst(t *testing.T) {
	// BTC 70% (7000), ETH 10% (1000), USDT 20% (2000): sell BTC, buy ETH.
	prop, err := Evaluate(states(0.07, 100000, 0.5, 2000, 2000), basePlan())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !prop.NeedsRebalance || len(prop.Trades) != 2 {
		t.Fatalf("expected 2 trades, got %+v", prop.Trades)
	}
	if prop.Trades[0].Side != "sell" || prop.Trades[0].Asset != "BTC" {
		t.Errorf("first trade must be the BTC sell, got %+v", prop.Trades[0])
	}
	if math.Abs(prop.Trades[0].NotionalQuote-2000) > 0.01 {
		t.Errorf("BTC sell notional = %g, want 2000", prop.Trades[0].NotionalQuote)
	}
	if prop.Trades[1].Side != "buy" || prop.Trades[1].Asset != "ETH" {
		t.Errorf("second trade must be the ETH buy, got %+v", prop.Trades[1])
	}
	if math.Abs(prop.Trades[1].NotionalQuote-2000) > 0.01 {
		t.Errorf("ETH buy notional = %g, want 2000", prop.Trades[1].NotionalQuote)
	}
	// Amounts derived from price: 2000 USDT of BTC @100k = 0.02 BTC.
	if math.Abs(prop.Trades[0].AmountAsset-0.02) > 1e-9 {
		t.Errorf("BTC sell amount = %g, want 0.02", prop.Trades[0].AmountAsset)
	}
}

func TestEvaluateWithinToleranceSilent(t *testing.T) {
	// BTC 53% vs target 50 with tolerance 5 pts → no trade.
	plan := basePlan()
	prop, err := Evaluate(states(0.053, 100000, 1.35, 2000, 2000), plan)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if prop.NeedsRebalance {
		t.Errorf("drift within tolerance must not trade: %+v", prop.Trades)
	}
	if prop.MaxDriftPts <= 0 {
		t.Error("MaxDriftPts should still report the actual drift")
	}
}

func TestEvaluateMinTradeSuppression(t *testing.T) {
	plan := basePlan()
	plan.TolerancePct = 0.1
	plan.MinTradeUSD = 500
	// ETH off by ~2% of 10k = 200 USDT — above tolerance but below min trade.
	prop, err := Evaluate(states(0.05, 100000, 1.4, 2000, 2200), plan)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	for _, tr := range prop.Trades {
		if tr.Asset == "ETH" {
			t.Errorf("ETH trade should be suppressed below min size: %+v", tr)
		}
	}
	if len(prop.Skipped) == 0 {
		t.Error("suppressed trades must be listed in Skipped, not silently dropped")
	}
}

func TestEvaluateRejectsBadInputs(t *testing.T) {
	plan := basePlan()
	plan.Targets = map[string]float64{"BTC": 50, "ETH": 30} // sums to 80, no quote
	if _, err := Evaluate(states(1, 100, 1, 100, 100), plan); err == nil {
		t.Error("expected error for weights not summing to 100")
	}

	plan = basePlan()
	delete(plan.Targets, "USDT")
	plan.Targets["SOL"] = 20
	if _, err := Evaluate(states(1, 100, 1, 100, 100), plan); err == nil {
		t.Error("expected error when quote asset is missing from targets")
	}

	plan = basePlan()
	if _, err := Evaluate([]AssetState{{Asset: "BTC", Amount: 1, PriceQuote: 0}, {Asset: "ETH", Amount: 1, PriceQuote: 100}, {Asset: "USDT", Amount: 1, PriceQuote: 1}}, plan); err == nil {
		t.Error("expected error for zero price")
	}
}

func TestFormatProposalContainsTrades(t *testing.T) {
	plan := basePlan()
	prop, err := Evaluate(states(0.07, 100000, 0.5, 2000, 2000), plan)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	out := FormatProposal(plan, prop)
	for _, want := range []string{"SELL", "BUY", "BTC/USDT", "ETH/USDT", "Max drift"} {
		if !strings.Contains(out, want) {
			t.Errorf("formatted proposal missing %q:\n%s", want, out)
		}
	}
}

func TestStoreCRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	p := basePlan()
	p.Schedule = "0 */6 * * *"
	p.Enabled = true
	if err := s.Save(&p); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if p.ID != 1 {
		t.Errorf("first plan ID = %d, want 1", p.ID)
	}

	p.TolerancePct = 7
	if err := s.Save(&p); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, ok := s.Get(p.ID)
	if !ok || got.TolerancePct != 7 {
		t.Errorf("Get after update = %+v, ok=%v", got, ok)
	}
	if _, ok := s.FindByName("TEST"); !ok {
		t.Error("FindByName should be case-insensitive")
	}

	// Persistence.
	s2, err := NewStore(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(s2.List()) != 1 {
		t.Fatal("plans did not persist")
	}

	if _, removed, err := s2.Delete(p.ID); err != nil || !removed {
		t.Fatalf("Delete = %v, %v", removed, err)
	}
	if len(s2.List()) != 0 {
		t.Error("plan not deleted")
	}
}
