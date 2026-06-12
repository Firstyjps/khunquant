package broker

import (
	"testing"
	"time"
)

func TestEarnRateHistoryCache_SetGet(t *testing.T) {
	t.Cleanup(func() {
		earnRateCacheMu.Lock()
		earnRateCache = make(map[string]earnRateCacheEntry)
		earnRateCacheMu.Unlock()
	})

	since := time.Now().Add(-364 * 24 * time.Hour).UnixMilli()
	pts := []EarnRatePoint{{Rate: 0.05, Timestamp: 1}, {Rate: 0.06, Timestamp: 2}}

	if _, ok := EarnRateHistoryCacheGet("okx", "BTC", &since); ok {
		t.Fatal("expected cache miss before set")
	}

	EarnRateHistoryCacheSet("okx", "BTC", &since, pts)

	got, ok := EarnRateHistoryCacheGet("okx", "BTC", &since)
	if !ok || len(got) != 2 {
		t.Fatalf("expected 2 cached points, got ok=%v len=%d", ok, len(got))
	}
}

func TestEarnRateHistoryCache_BucketsBySinceDay(t *testing.T) {
	t.Cleanup(func() {
		earnRateCacheMu.Lock()
		earnRateCache = make(map[string]earnRateCacheEntry)
		earnRateCacheMu.Unlock()
	})

	base := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	sameDay := base.Add(3 * time.Hour).UnixMilli() // same calendar day → same bucket
	nextDay := base.Add(28 * time.Hour).UnixMilli()
	s0 := base.UnixMilli()

	EarnRateHistoryCacheSet("okx", "BTC", &s0, []EarnRatePoint{{Rate: 1, Timestamp: 1}})

	if _, ok := EarnRateHistoryCacheGet("okx", "BTC", &sameDay); !ok {
		t.Fatal("expected hit for a since within the same day bucket")
	}
	if _, ok := EarnRateHistoryCacheGet("okx", "BTC", &nextDay); ok {
		t.Fatal("expected miss for a since in a different day bucket")
	}
	// Different exchange / asset must not collide.
	if _, ok := EarnRateHistoryCacheGet("binance", "BTC", &s0); ok {
		t.Fatal("expected miss for a different exchange")
	}
	if _, ok := EarnRateHistoryCacheGet("okx", "ETH", &s0); ok {
		t.Fatal("expected miss for a different asset")
	}
}

func TestEarnRateHistoryCache_Expiry(t *testing.T) {
	t.Cleanup(func() {
		earnRateCacheMu.Lock()
		earnRateCache = make(map[string]earnRateCacheEntry)
		earnRateCacheMu.Unlock()
	})

	since := int64(0)
	key := earnRateCacheKey("okx", "BTC", &since)
	earnRateCacheMu.Lock()
	earnRateCache[key] = earnRateCacheEntry{
		points:  []EarnRatePoint{{Rate: 1, Timestamp: 1}},
		expires: time.Now().Add(-time.Minute), // already expired
	}
	earnRateCacheMu.Unlock()

	if _, ok := EarnRateHistoryCacheGet("okx", "BTC", &since); ok {
		t.Fatal("expected miss for an expired entry")
	}
}
