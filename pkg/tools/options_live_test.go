package tools

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/cryptoquantumwave/khunquant/pkg/config"

	// Register the deribit broker factory for live end-to-end runs.
	_ "github.com/cryptoquantumwave/khunquant/pkg/exchanges/deribit"
)

// TestLiveOptionsChainTool runs the options_chain tool against the real
// Deribit public API. Skipped unless KHUNQUANT_LIVE_TEST=1.
func TestLiveOptionsChainTool(t *testing.T) {
	if os.Getenv("KHUNQUANT_LIVE_TEST") != "1" {
		t.Skip("set KHUNQUANT_LIVE_TEST=1 to run live Deribit API tests")
	}

	cfg := config.DefaultConfig()
	tool := NewOptionsChainTool(cfg)
	res := tool.Execute(context.Background(), map[string]any{
		"underlying": "BTC",
		"limit":      10.0,
	})
	if res.IsError {
		t.Fatalf("options_chain errored: %s", res.ForLLM)
	}
	out := res.ForUser
	if !strings.Contains(out, "Options chain: BTC on deribit") {
		t.Fatalf("unexpected header: %s", out[:min(200, len(out))])
	}
	if !strings.Contains(out, "call") && !strings.Contains(out, "put") {
		t.Fatalf("no option rows in output: %s", out)
	}
	t.Logf("options_chain output:\n%s", out)

	quote := NewOptionQuoteTool(cfg)
	// Pull an instrument name out of the chain output (first data row, col 1).
	lines := strings.Split(out, "\n")
	var instrument string
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) > 3 && strings.Count(f[0], "-") == 3 {
			instrument = f[0]
			break
		}
	}
	if instrument == "" {
		t.Fatal("could not extract an instrument from chain output")
	}
	qres := quote.Execute(context.Background(), map[string]any{"symbol": instrument})
	if qres.IsError {
		t.Fatalf("option_quote(%s) errored: %s", instrument, qres.ForLLM)
	}
	if !strings.Contains(qres.ForUser, "Delta:") {
		t.Fatalf("quote output missing greeks: %s", qres.ForUser)
	}
	t.Logf("option_quote output:\n%s", qres.ForUser)
}
