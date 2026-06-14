package deltaneutral

// EntrySpreadPct returns the entry basis for a positive-carry portfolio
// (buy spot, sell perp) as a percentage.
//
//	result = (futuresPrice - spotPrice) / spotPrice * 100
//
// A higher (less negative) value means a more favorable entry.
// Zero is returned when spotPrice is zero to avoid division by zero.
func EntrySpreadPct(futuresPrice, spotPrice float64) float64 {
	if spotPrice == 0 {
		return 0
	}
	return (futuresPrice - spotPrice) / spotPrice * 100
}

// ExitSpreadPct returns the exit basis for a positive-carry portfolio
// (buy spot, sell perp) as a percentage.
//
//	result = (spotPrice - futuresPrice) / futuresPrice * 100
//
// A higher value means a more favorable exit (basis has converged or inverted).
// Zero is returned when futuresPrice is zero to avoid division by zero.
func ExitSpreadPct(spotPrice, futuresPrice float64) float64 {
	if futuresPrice == 0 {
		return 0
	}
	return (spotPrice - futuresPrice) / futuresPrice * 100
}
