package tools

import (
	"testing"

	ccxt "github.com/ccxt/ccxt/go/v4"
)

// fp is a small constructor for *float64 used throughout these tests.
func fpf(f float64) *float64 { return &f }
func fps(s string) *string   { return &s }

// ── mergeOrderFillData ──────────────────────────────────────────────────────

// TestMergeOrderFillData_OKXPath: create has no fill data; fetched has all.
// OKX returns only order ID from CreateOrder; fill+fee come from FetchOrder.
func TestMergeOrderFillData_OKXPath(t *testing.T) {
	create := ccxt.Order{} // no fill, no fee (OKX create response)
	fetched := ccxt.Order{
		Filled:  fpf(120.5),
		Average: fpf(0.1182),
		Cost:    fpf(14.23),
		Status:  fps("closed"),
		Fee:     ccxt.Fee{Cost: fpf(0.01423)},
	}

	got := mergeOrderFillData(create, fetched)

	if got.Filled == nil || *got.Filled != 120.5 {
		t.Errorf("Filled: got %v, want 120.5", got.Filled)
	}
	if got.Average == nil || *got.Average != 0.1182 {
		t.Errorf("Average: got %v, want 0.1182", got.Average)
	}
	if got.Cost == nil || *got.Cost != 14.23 {
		t.Errorf("Cost: got %v, want 14.23", got.Cost)
	}
	if got.Fee.Cost == nil || *got.Fee.Cost != 0.01423 {
		t.Errorf("Fee.Cost: got %v, want 0.01423", got.Fee.Cost)
	}
}

// TestMergeOrderFillData_BinancePath: create has fill data AND fee (Binance
// FULL response carries fills[]); FetchOrder returns fill data but NO fee.
// The merged result must preserve the fee from the create response.
func TestMergeOrderFillData_BinancePath(t *testing.T) {
	create := ccxt.Order{
		Filled:  fpf(0.119),
		Average: fpf(42000.0),
		Cost:    fpf(4998.0),
		Status:  fps("closed"),
		Fee:     ccxt.Fee{Cost: fpf(0.4998)}, // commission from fills[]
	}
	// Binance GET /api/v3/order: has fill data but NO fee field.
	fetched := ccxt.Order{
		Filled:  fpf(0.119),
		Average: fpf(42010.0),
		Cost:    fpf(4999.19),
		Status:  fps("filled"),
		Fee:     ccxt.Fee{Cost: nil}, // no commission in FetchOrder response
	}

	got := mergeOrderFillData(create, fetched)

	// Fill fields: prefer fetched (more accurate post-fill data from exchange).
	if got.Average == nil || *got.Average != 42010.0 {
		t.Errorf("Average: got %v, want 42010.0 (from fetched)", got.Average)
	}
	if got.Cost == nil || *got.Cost != 4999.19 {
		t.Errorf("Cost: got %v, want 4999.19 (from fetched)", got.Cost)
	}

	// Fee: must come from create (Binance does not include fee in FetchOrder).
	if got.Fee.Cost == nil {
		t.Fatal("Fee.Cost is nil — create fee was discarded (the Binance H1 bug)")
	}
	if *got.Fee.Cost != 0.4998 {
		t.Errorf("Fee.Cost: got %v, want 0.4998 (from create)", *got.Fee.Cost)
	}
}

// TestMergeOrderFillData_BothHaveFee: when both carry fee, prefer fetched
// (OKX fetched fee is the accurate post-fill settled amount).
func TestMergeOrderFillData_BothHaveFee(t *testing.T) {
	create := ccxt.Order{Fee: ccxt.Fee{Cost: fpf(0.010)}}
	fetched := ccxt.Order{
		Filled: fpf(10.0),
		Fee:    ccxt.Fee{Cost: fpf(0.0148)}, // post-fill settled (OKX)
	}

	got := mergeOrderFillData(create, fetched)

	if got.Fee.Cost == nil || *got.Fee.Cost != 0.0148 {
		t.Errorf("Fee.Cost: got %v, want 0.0148 (prefer fetched when both have fee)", got.Fee.Cost)
	}
}

// TestMergeOrderFillData_NeitherHasFee: when neither has fee, result fee is nil.
func TestMergeOrderFillData_NeitherHasFee(t *testing.T) {
	create := ccxt.Order{}
	fetched := ccxt.Order{Filled: fpf(5.0)}

	got := mergeOrderFillData(create, fetched)

	if got.Fee.Cost != nil {
		t.Errorf("Fee.Cost: expected nil (no fee in either order), got %v", *got.Fee.Cost)
	}
}

// TestMergeOrderFillData_PartialFetch: fetched has 0 for fill fields — do not
// overwrite create's actual fill data with zeros.
func TestMergeOrderFillData_PartialFetch(t *testing.T) {
	create := ccxt.Order{Filled: fpf(50.0), Average: fpf(0.5), Cost: fpf(25.0)}
	fetched := ccxt.Order{Filled: fpf(0), Average: nil, Cost: nil} // empty fetch

	got := mergeOrderFillData(create, fetched)

	if got.Filled == nil || *got.Filled != 50.0 {
		t.Errorf("Filled: got %v, want 50.0 (create preserved when fetched is 0)", got.Filled)
	}
	if got.Average == nil || *got.Average != 0.5 {
		t.Errorf("Average: got %v, want 0.5 (create preserved when fetched is nil)", got.Average)
	}
}

// TestMergeOrderFillData_PreservesOrderID: order ID must always come from
// the create response (fetched may have a different internal representation).
func TestMergeOrderFillData_PreservesOrderID(t *testing.T) {
	oid := fps("create-order-123")
	create := ccxt.Order{Id: oid}
	fetched := ccxt.Order{Id: fps("fetch-order-456"), Filled: fpf(10.0)}

	got := mergeOrderFillData(create, fetched)

	if got.Id == nil || *got.Id != "create-order-123" {
		t.Errorf("Id: got %v, want create-order-123", got.Id)
	}
}

// ── spotFeeCostUSDT ─────────────────────────────────────────────────────────

// TestSpotFeeCostUSDT_BuyConvertsToUSDT: spot BUY fee is in base currency
// (e.g. ALGO); must multiply by fill price to get USDT.
func TestSpotFeeCostUSDT_BuyConvertsToUSDT(t *testing.T) {
	order := ccxt.Order{Fee: ccxt.Fee{Cost: fpf(0.678)}} // 0.678 ALGO fee
	fillPrice := 0.1182                                    // 0.1182 USDT/ALGO

	got := spotFeeCostUSDT(order, "buy", fillPrice)
	want := 0.678 * 0.1182 // ≈ 0.0801 USDT

	if diff := got - want; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("buy fee: got %.6f, want %.6f", got, want)
	}
	if got <= 0 {
		t.Error("buy fee must be positive (cost)")
	}
}

// TestSpotFeeCostUSDT_SellIsAlreadyUSDT: spot SELL fee is deducted from
// received USDT; Fee.Cost is already in USDT — do NOT multiply by price.
// This is the H2 bug: previous code did * avgFillPrice on sells.
func TestSpotFeeCostUSDT_SellIsAlreadyUSDT(t *testing.T) {
	order := ccxt.Order{Fee: ccxt.Fee{Cost: fpf(0.0080)}} // 0.0080 USDT fee
	fillPrice := 0.1182                                    // would produce wrong result if multiplied

	got := spotFeeCostUSDT(order, "sell", fillPrice)
	want := 0.0080 // USDT, no conversion

	if diff := got - want; diff > 0.0001 || diff < -0.0001 {
		t.Errorf("sell fee: got %.6f, want %.6f", got, want)
	}
	// Regression: old code returned 0.0080 * 0.1182 ≈ 0.000946 — 8.5× too small.
	wrongValue := 0.0080 * 0.1182
	if got == wrongValue {
		t.Errorf("sell fee equals the old wrong value (%.6f = Cost * price); H2 bug not fixed", wrongValue)
	}
}

// TestSpotFeeCostUSDT_ZeroWhenNoFee: nil Fee.Cost returns 0.
func TestSpotFeeCostUSDT_ZeroWhenNoFee(t *testing.T) {
	order := ccxt.Order{Fee: ccxt.Fee{Cost: nil}}
	if got := spotFeeCostUSDT(order, "buy", 100.0); got != 0 {
		t.Errorf("expected 0 for nil fee, got %v", got)
	}
}

// TestSpotFeeCostUSDT_ZeroWhenNegativeFee: negative Fee.Cost is ignored
// (maker rebate path; not a cost, do not record as negative cost).
func TestSpotFeeCostUSDT_ZeroWhenNegativeFee(t *testing.T) {
	order := ccxt.Order{Fee: ccxt.Fee{Cost: fpf(-0.001)}} // rebate
	if got := spotFeeCostUSDT(order, "buy", 100.0); got != 0 {
		t.Errorf("expected 0 for negative fee (rebate), got %v", got)
	}
}

// TestSpotFeeCostUSDT_ZeroWhenFillPriceZeroOnBuy: if fillPrice is 0 on a buy,
// cannot convert → return 0 rather than NaN or Inf.
func TestSpotFeeCostUSDT_ZeroWhenFillPriceZeroOnBuy(t *testing.T) {
	order := ccxt.Order{Fee: ccxt.Fee{Cost: fpf(0.5)}}
	if got := spotFeeCostUSDT(order, "buy", 0); got != 0 {
		t.Errorf("expected 0 when fillPrice=0 on buy, got %v", got)
	}
}

// TestSpotFeeCostUSDT_SellDoesNotNeedFillPrice: sell fee does not use price.
func TestSpotFeeCostUSDT_SellDoesNotNeedFillPrice(t *testing.T) {
	order := ccxt.Order{Fee: ccxt.Fee{Cost: fpf(0.005)}}
	// fillPrice=0 should still work for sells (fee is already in USDT).
	got := spotFeeCostUSDT(order, "sell", 0)
	if got != 0.005 {
		t.Errorf("sell fee with fillPrice=0: got %v, want 0.005", got)
	}
}

// TestSpotFeeCostUSDT_CaseInsensitiveSide: "BUY" and "Buy" should work.
func TestSpotFeeCostUSDT_CaseInsensitiveSide(t *testing.T) {
	order := ccxt.Order{Fee: ccxt.Fee{Cost: fpf(1.0)}}
	for _, side := range []string{"BUY", "Buy", "buy"} {
		got := spotFeeCostUSDT(order, side, 2.0)
		if got != 2.0 {
			t.Errorf("side=%q: got %v, want 2.0 (1.0 * 2.0)", side, got)
		}
	}
	for _, side := range []string{"SELL", "Sell", "sell"} {
		got := spotFeeCostUSDT(order, side, 999.0) // price must be ignored
		if got != 1.0 {
			t.Errorf("side=%q: got %v, want 1.0 (no price multiply)", side, got)
		}
	}
}

// ── leg.FeeUSDT sign invariants ─────────────────────────────────────────────
// These tests verify the contract that callers negate spotFeeCostUSDT before
// storing in leg.FeeUSDT, so all recorded fees are negative (costs).

func TestLegFeeSign_BuyResultIsNegative(t *testing.T) {
	order := ccxt.Order{Fee: ccxt.Fee{Cost: fpf(0.678)}}
	fee := spotFeeCostUSDT(order, "buy", 0.1182)
	stored := -fee // what the caller does: leg.FeeUSDT = -fee
	if stored >= 0 {
		t.Errorf("leg.FeeUSDT for a buy should be negative (cost), got %v", stored)
	}
}

func TestLegFeeSign_SellResultIsNegative(t *testing.T) {
	order := ccxt.Order{Fee: ccxt.Fee{Cost: fpf(0.0080)}}
	fee := spotFeeCostUSDT(order, "sell", 0.1182)
	stored := -fee // what the caller does: leg.FeeUSDT = -fee
	if stored >= 0 {
		t.Errorf("leg.FeeUSDT for a sell should be negative (cost), got %v", stored)
	}
}

// TestSellFeeVsBuyFeeUSDT: for the same nominal fee amount, sell fee should
// be LARGER (more negative) than buy fee when price < 1 — the H2 bug caused
// sells to record 1/price of the correct value, severely understating close fees.
func TestSellFeeVsBuyFeeUSDT_SellLargerThanBuyWhenPriceLessThan1(t *testing.T) {
	// Scenario: ALGO/USDT @ 0.1182. Fee.Cost = 0.678 (same number, different currencies).
	// Buy fee: 0.678 ALGO × 0.1182 = 0.0801 USDT
	// Sell fee: 0.0678 USDT (already USDT, but let's use a realistic sell value)
	// Use the same Fee.Cost to prove the formula diverges by ~price factor.
	feeCost := 0.678
	price := 0.1182
	order := ccxt.Order{Fee: ccxt.Fee{Cost: &feeCost}}

	buyFee := spotFeeCostUSDT(order, "buy", price)  // 0.678 * 0.1182 ≈ 0.0801
	sellFee := spotFeeCostUSDT(order, "sell", price) // 0.678 (no multiply)

	// Sell fee should be ~1/price times larger than buy fee.
	if sellFee <= buyFee {
		t.Errorf("sell fee (%.6f) should be larger than buy fee (%.6f) when price<1 and same Cost", sellFee, buyFee)
	}
	// Old buggy code returned buyFee for both: assert they are different.
	if buyFee == sellFee {
		t.Errorf("buy and sell returned identical values (%.6f); H2 not fixed", buyFee)
	}
}

// TestMergeOrderFillData_BinanceH1Regression is the exact scenario that broke
// fee tracking on Binance in the original commit: create carries fee, the
// subsequent unconditional overwrite with fetchedOrder (which has no fee) zeroes
// the fee field. This test pins the fixed behaviour.
func TestMergeOrderFillData_BinanceH1Regression(t *testing.T) {
	createFee := 4.9980 // e.g. BTC taker fee from Binance FULL response
	create := ccxt.Order{
		Filled:  fpf(0.119),
		Average: fpf(42000.0),
		Cost:    fpf(4998.0),
		Fee:     ccxt.Fee{Cost: &createFee},
	}

	// Simulate what Binance GET /api/v3/order returns: no fee field.
	fetched := ccxt.Order{
		Filled:  fpf(0.119),
		Average: fpf(42000.0),
		Cost:    fpf(4998.0),
		Fee:     ccxt.Fee{Cost: nil},
	}

	// --- OLD (buggy) behaviour:
	// order = fetched → discards create.Fee.Cost → leg.FeeUSDT = 0

	// --- NEW (fixed) behaviour:
	merged := mergeOrderFillData(create, fetched)

	if merged.Fee.Cost == nil {
		t.Fatal("BinanceH1 regression: fee was nil after merge — create fee discarded")
	}
	if *merged.Fee.Cost != createFee {
		t.Errorf("BinanceH1 regression: fee = %.4f, want %.4f", *merged.Fee.Cost, createFee)
	}
}
