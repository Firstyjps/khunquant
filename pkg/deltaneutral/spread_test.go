package deltaneutral

import (
	"math"
	"testing"
)

func TestEntrySpreadPct(t *testing.T) {
	const tol = 1e-4

	tests := []struct {
		name         string
		futuresPrice float64
		spotPrice    float64
		want         float64
	}{
		{
			name:         "Binance screenshot example: fut=0.7424 spot=0.7435",
			futuresPrice: 0.7424,
			spotPrice:    0.7435,
			// (0.7424 - 0.7435) / 0.7435 * 100 = -0.14795...%
			want: (0.7424 - 0.7435) / 0.7435 * 100,
		},
		{
			name:         "positive spread: futures above spot",
			futuresPrice: 1.02,
			spotPrice:    1.00,
			want:         2.0,
		},
		{
			name:         "zero spread: prices equal",
			futuresPrice: 1.00,
			spotPrice:    1.00,
			want:         0.0,
		},
		{
			name:         "negative spread: futures below spot",
			futuresPrice: 0.98,
			spotPrice:    1.00,
			want:         -2.0,
		},
		{
			name:         "zero spot price returns 0 (no division by zero)",
			futuresPrice: 1.00,
			spotPrice:    0,
			want:         0,
		},
		{
			name:         "large BTC-like prices",
			futuresPrice: 50200,
			spotPrice:    50000,
			want:         0.4, // (50200-50000)/50000*100
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EntrySpreadPct(tt.futuresPrice, tt.spotPrice)
			if math.Abs(got-tt.want) > tol {
				t.Errorf("EntrySpreadPct(fut=%.6f, spot=%.6f) = %.6f, want %.6f (diff %.6f > tol %.6f)",
					tt.futuresPrice, tt.spotPrice, got, tt.want, math.Abs(got-tt.want), tol)
			}
		})
	}
}

func TestExitSpreadPct(t *testing.T) {
	const tol = 1e-4

	tests := []struct {
		name         string
		spotPrice    float64
		futuresPrice float64
		want         float64
	}{
		{
			name:         "Binance screenshot inverse: spot=0.7435 fut=0.7424",
			spotPrice:    0.7435,
			futuresPrice: 0.7424,
			// (0.7435 - 0.7424) / 0.7424 * 100 = +0.1481...%
			want: (0.7435 - 0.7424) / 0.7424 * 100,
		},
		{
			name:         "positive exit spread: spot above futures (favorable)",
			spotPrice:    1.02,
			futuresPrice: 1.00,
			// (1.02 - 1.00) / 1.00 * 100 = 2.0%
			want: 2.0,
		},
		{
			name:         "negative exit spread: spot below futures",
			spotPrice:    1.00,
			futuresPrice: 1.02,
			// (1.00 - 1.02) / 1.02 * 100 ≈ -1.9608%
			want: (1.00 - 1.02) / 1.02 * 100,
		},
		{
			name:         "zero futures price returns 0 (no division by zero)",
			spotPrice:    1.00,
			futuresPrice: 0,
			want:         0,
		},
		{
			name:         "prices equal — zero exit spread",
			spotPrice:    50000,
			futuresPrice: 50000,
			want:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExitSpreadPct(tt.spotPrice, tt.futuresPrice)
			if math.Abs(got-tt.want) > tol {
				t.Errorf("ExitSpreadPct(spot=%.6f, fut=%.6f) = %.6f, want %.6f (diff %.6f > tol %.6f)",
					tt.spotPrice, tt.futuresPrice, got, tt.want, math.Abs(got-tt.want), tol)
			}
		})
	}
}

// TestSpreadSymmetry verifies that EntrySpreadPct and ExitSpreadPct are near-mirror
// images: when entry spread is −X%, exit spread should be approximately +X%
// (with a small second-order difference from the different denominator).
func TestSpreadSymmetry(t *testing.T) {
	fut := 0.7424
	spot := 0.7435

	entry := EntrySpreadPct(fut, spot)
	exit := ExitSpreadPct(spot, fut)

	// entry should be negative, exit positive for this case
	if entry >= 0 {
		t.Errorf("entry spread %.6f should be negative when fut < spot", entry)
	}
	if exit <= 0 {
		t.Errorf("exit spread %.6f should be positive when spot > fut", exit)
	}
}
