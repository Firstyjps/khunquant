package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
)

func TestParseDeribitInstrument(t *testing.T) {
	tests := []struct {
		name    string
		expiry  string
		strike  float64
		optType string
		ok      bool
	}{
		{"BTC-26DEC25-100000-C", "26DEC25", 100000, "call", true},
		{"ETH-27JUN25-3500-P", "27JUN25", 3500, "put", true},
		{"BTC-PERPETUAL", "", 0, "", false},
		{"BTC-26DEC25-100000-X", "", 0, "", false},
		{"garbage", "", 0, "", false},
	}
	for _, tt := range tests {
		expiry, strike, optType, ok := parseDeribitInstrument(tt.name)
		if ok != tt.ok {
			t.Errorf("%s: ok = %v, want %v", tt.name, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if expiry != tt.expiry || strike != tt.strike || optType != tt.optType {
			t.Errorf("%s: got (%s, %g, %s), want (%s, %g, %s)",
				tt.name, expiry, strike, optType, tt.expiry, tt.strike, tt.optType)
		}
	}
}

func TestInfoFloat(t *testing.T) {
	info := map[string]any{
		"mark_price": 0.0525,
		"mark_iv":    "55.5",
		"missing":    nil,
	}
	if v := infoFloat(info, "mark_price"); v != 0.0525 {
		t.Errorf("float value = %g, want 0.0525", v)
	}
	if v := infoFloat(info, "mark_iv"); v != 55.5 {
		t.Errorf("string value = %g, want 55.5", v)
	}
	if v := infoFloat(info, "absent"); v != 0 {
		t.Errorf("absent key = %g, want 0", v)
	}
}

func TestHedgeSideForDelta(t *testing.T) {
	if hedgeSideForDelta(0.5) != "short" {
		t.Error("positive delta should hedge short")
	}
	if hedgeSideForDelta(-0.5) != "long" {
		t.Error("negative delta should hedge long")
	}
}

func TestOptionOrder_DryRunNoNetwork(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TradingRisk.AllowLeverage = true
	tool := NewOptionOrderTool(cfg)
	res := tool.Execute(context.Background(), map[string]any{
		"symbol":  "BTC-26DEC25-100000-C",
		"side":    "buy",
		"amount":  0.5,
		"price":   0.05,
		"confirm": false,
	})
	if res.IsError {
		t.Fatalf("dry-run errored: %s", res.ForLLM)
	}
	if !strings.Contains(res.ForUser, "Dry-run") {
		t.Fatalf("expected dry-run output, got: %s", res.ForUser)
	}
	if !strings.Contains(res.ForUser, "BTC-26DEC25-100000-C") {
		t.Fatalf("dry-run output missing instrument: %s", res.ForUser)
	}
}

func TestOptionOrder_SellWarnsInDryRun(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TradingRisk.AllowLeverage = true
	tool := NewOptionOrderTool(cfg)
	res := tool.Execute(context.Background(), map[string]any{
		"symbol":  "BTC-26DEC25-100000-C",
		"side":    "sell",
		"amount":  0.5,
		"price":   0.05,
		"confirm": false,
	})
	if !strings.Contains(res.ForUser, "SELLING") {
		t.Fatalf("expected short-premium warning in sell dry-run: %s", res.ForUser)
	}
}

func TestOptionOrder_RequiresAllowLeverage(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TradingRisk.AllowLeverage = false
	tool := NewOptionOrderTool(cfg)
	res := tool.Execute(context.Background(), map[string]any{
		"symbol":  "BTC-26DEC25-100000-C",
		"side":    "buy",
		"amount":  0.5,
		"price":   0.05,
		"confirm": false,
	})
	if !res.IsError {
		t.Fatal("expected error when allow_leverage is false")
	}
	if !strings.Contains(res.ForLLM, "allow_leverage") {
		t.Fatalf("error should mention allow_leverage: %s", res.ForLLM)
	}
}

func TestOptionOrder_LimitRequiresPrice(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.TradingRisk.AllowLeverage = true
	tool := NewOptionOrderTool(cfg)
	res := tool.Execute(context.Background(), map[string]any{
		"symbol":  "BTC-26DEC25-100000-C",
		"side":    "buy",
		"amount":  0.5,
		"confirm": false,
	})
	if !res.IsError || !strings.Contains(res.ForLLM, "price") {
		t.Fatalf("limit order without price should error: %s", res.ForLLM)
	}
}
