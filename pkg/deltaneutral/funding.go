package deltaneutral

import "strings"

// FundingPeriodsPerDay returns the number of funding periods in a calendar day
// based on the exchange-reported interval string (e.g. "1h", "4h", "8h").
// Defaults to 3 (8-hour intervals) when the interval is nil or unknown — the
// most common value across OKX and Binance perpetual swaps.
func FundingPeriodsPerDay(interval *string) float64 {
	if interval == nil {
		return 3.0
	}
	switch strings.ToLower(*interval) {
	case "1h":
		return 24.0
	case "4h":
		return 6.0
	case "8h":
		return 3.0
	default:
		return 3.0
	}
}

// AnnualizeFundingRatePct converts a raw per-period funding rate (e.g. 0.0001)
// to an annualised percentage using the given interval.
//
//	result = rate * periodsPerDay * 365 * 100
func AnnualizeFundingRatePct(rate float64, interval *string) float64 {
	return rate * FundingPeriodsPerDay(interval) * 365 * 100
}
