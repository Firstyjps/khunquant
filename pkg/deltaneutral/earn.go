package deltaneutral

import (
	"time"

	ccxt "github.com/ccxt/ccxt/go/v4"

	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// EarnWindowMeanPct returns the mean of EarnRatePoint.Rate (a fraction) over the
// trailing window ending at now, expressed as a percent, plus the number of
// points used. Returns (0, 0) when no points fall within the window.
func EarnWindowMeanPct(points []broker.EarnRatePoint, window time.Duration, now time.Time) (float64, int) {
	cutoffMs := now.Add(-window).UnixMilli()
	var sum float64
	var n int
	for _, pt := range points {
		if pt.Timestamp >= cutoffMs {
			sum += pt.Rate
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return (sum / float64(n)) * 100, n
}

// FundingWindowMeanRate returns the mean per-period funding rate over the trailing
// window ending at now, plus the number of records used. Returns (0, 0) when no
// records fall within the window. Annualize the result with AnnualizeFundingRatePct.
func FundingWindowMeanRate(history []ccxt.FundingRateHistory, window time.Duration, now time.Time) (float64, int) {
	cutoffMs := now.Add(-window).UnixMilli()
	var sum float64
	var n int
	for _, r := range history {
		if r.Timestamp != nil && *r.Timestamp >= cutoffMs && r.FundingRate != nil {
			sum += *r.FundingRate
			n++
		}
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}
