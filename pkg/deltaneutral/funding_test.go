package deltaneutral

import (
	"testing"
)

func strp(s string) *string { return &s }

// TestFundingPeriodsPerDay covers all documented interval strings, the nil/unknown
// fallback, and case-insensitivity. Default is 3 (8-hour intervals).
func TestFundingPeriodsPerDay(t *testing.T) {
	tests := []struct {
		name     string
		interval *string
		want     float64
	}{
		{"1h — 24 periods/day", strp("1h"), 24.0},
		{"4h — 6 periods/day", strp("4h"), 6.0},
		{"8h — 3 periods/day", strp("8h"), 3.0},
		{"nil — defaults to 3", nil, 3.0},
		{"unknown 12h — defaults to 3", strp("12h"), 3.0},
		{"empty string — defaults to 3", strp(""), 3.0},
		{"uppercase 1H — case-insensitive", strp("1H"), 24.0},
		{"uppercase 4H — case-insensitive", strp("4H"), 6.0},
		{"uppercase 8H — case-insensitive", strp("8H"), 3.0},
		{"mixed-case 1h — case-insensitive", strp("1h"), 24.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FundingPeriodsPerDay(tt.interval)
			if got != tt.want {
				t.Errorf("FundingPeriodsPerDay(%v) = %v, want %v", tt.interval, got, tt.want)
			}
		})
	}
}

// TestAnnualizeFundingRatePct verifies the formula: rate × periodsPerDay × 365 × 100.
func TestAnnualizeFundingRatePct(t *testing.T) {
	interval8h := "8h"
	interval1h := "1h"

	tests := []struct {
		name     string
		rate     float64
		interval *string
		want     float64
	}{
		{
			name:     "positive 8h rate (OKX/Binance default)",
			rate:     0.0001,
			interval: &interval8h,
			// 0.0001 * 3 * 365 * 100 = 10.95%
			want: 0.0001 * 3 * 365 * 100,
		},
		{
			name:     "positive 1h rate (OKX 1h funding)",
			rate:     0.0001,
			interval: &interval1h,
			// 0.0001 * 24 * 365 * 100 = 87.6%
			want: 0.0001 * 24 * 365 * 100,
		},
		{
			name:     "nil interval falls back to 8h (3/day)",
			rate:     0.0002,
			interval: nil,
			want:     0.0002 * 3 * 365 * 100,
		},
		{
			name:     "negative rate — shorts pay, longs earn",
			rate:     -0.0001,
			interval: &interval8h,
			want:     -0.0001 * 3 * 365 * 100,
		},
		{
			name:     "zero rate",
			rate:     0,
			interval: &interval8h,
			want:     0,
		},
	}

	const tol = 1e-9
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnnualizeFundingRatePct(tt.rate, tt.interval)
			diff := got - tt.want
			if diff > tol || diff < -tol {
				t.Errorf("AnnualizeFundingRatePct(%.6f, %v) = %.8f, want %.8f",
					tt.rate, tt.interval, got, tt.want)
			}
		})
	}
}

// TestAnnualizeFundingRatePct_Consistency checks that AnnualizeFundingRatePct is
// consistent with FundingPeriodsPerDay (they must compose correctly).
func TestAnnualizeFundingRatePct_Consistency(t *testing.T) {
	rate := 0.00015
	for _, iv := range []*string{strp("1h"), strp("4h"), strp("8h"), nil} {
		periods := FundingPeriodsPerDay(iv)
		want := rate * periods * 365 * 100
		got := AnnualizeFundingRatePct(rate, iv)
		if got != want {
			t.Errorf("interval %v: AnnualizeFundingRatePct=%v, manual=%v — mismatch", iv, got, want)
		}
	}
}
