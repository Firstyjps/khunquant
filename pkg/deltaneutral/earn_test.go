package deltaneutral

import (
	"testing"
	"time"

	ccxt "github.com/ccxt/ccxt/go/v4"

	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

func TestEarnWindowMeanPct(t *testing.T) {
	now := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	day := func(d int) int64 { return now.AddDate(0, 0, -d).UnixMilli() }

	points := []broker.EarnRatePoint{
		{Rate: 0.10, Timestamp: day(1)},  // 10%  in 7d
		{Rate: 0.20, Timestamp: day(3)},  // 20%  in 7d
		{Rate: 0.30, Timestamp: day(10)}, // 30%  outside 7d, in 14d
		{Rate: 0.40, Timestamp: day(40)}, // 40%  outside 30d
	}

	t.Run("7d mean in percent", func(t *testing.T) {
		mean, n := EarnWindowMeanPct(points, 7*24*time.Hour, now)
		if n != 2 {
			t.Fatalf("count = %d, want 2", n)
		}
		// (0.10 + 0.20)/2 * 100 = 15
		if mean < 14.999 || mean > 15.001 {
			t.Fatalf("mean = %v, want 15", mean)
		}
	})

	t.Run("14d includes the 10-day point", func(t *testing.T) {
		_, n := EarnWindowMeanPct(points, 14*24*time.Hour, now)
		if n != 3 {
			t.Fatalf("count = %d, want 3", n)
		}
	})

	t.Run("empty window returns zero", func(t *testing.T) {
		mean, n := EarnWindowMeanPct(points, time.Hour, now)
		if n != 0 || mean != 0 {
			t.Fatalf("got (%v, %d), want (0, 0)", mean, n)
		}
	})

	t.Run("no points", func(t *testing.T) {
		mean, n := EarnWindowMeanPct(nil, 7*24*time.Hour, now)
		if n != 0 || mean != 0 {
			t.Fatalf("got (%v, %d), want (0, 0)", mean, n)
		}
	})
}

func TestFundingWindowMeanRate(t *testing.T) {
	now := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	ts := func(d int) *int64 { v := now.AddDate(0, 0, -d).UnixMilli(); return &v }
	rate := func(r float64) *float64 { return &r }

	history := []ccxt.FundingRateHistory{
		{FundingRate: rate(0.0001), Timestamp: ts(1)},
		{FundingRate: rate(0.0003), Timestamp: ts(2)},
		{FundingRate: rate(0.0009), Timestamp: ts(20)}, // outside 7d
	}

	mean, n := FundingWindowMeanRate(history, 7*24*time.Hour, now)
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	// (0.0001 + 0.0003)/2 = 0.0002 (raw rate, not annualised)
	if mean < 0.00019999 || mean > 0.00020001 {
		t.Fatalf("mean = %v, want 0.0002", mean)
	}

	if _, n := FundingWindowMeanRate(nil, 7*24*time.Hour, now); n != 0 {
		t.Fatalf("nil history count = %d, want 0", n)
	}
}
