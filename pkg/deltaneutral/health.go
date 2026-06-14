package deltaneutral

import (
	"math"
)

// Evaluate is the deterministic health evaluator for delta-neutral plans.
// It takes an EvaluationInput and returns a HealthEvaluation with no I/O, network, or DB.
func Evaluate(input EvaluationInput) HealthEvaluation {
	result := HealthEvaluation{
		Snapshot: MonitorSnapshot{
			PlanID:                   input.Plan.ID,
			CheckedAt:                input.Now,
			SpotPrice:                input.SpotState.Price,
			SpotQuantity:             input.SpotState.Quantity,
			SpotValueUSDT:            input.SpotState.ValueUSDT,
			FuturesMarkPrice:         input.FuturesState.MarkPrice,
			FuturesContracts:         input.FuturesState.Contracts,
			FuturesNotionalUSDT:      input.FuturesState.NotionalUSDT,
			FuturesUnrealizedPnLUSDT: input.FuturesState.UnrealizedPnLUSDT,
			CurrentFundingRate:       input.FundingInfo.CurrentRate,
			EstimatedNextFundingUSDT: input.FundingInfo.EstimatedNextUSDT,
			LiquidationPrice:         input.FuturesState.LiquidationPrice,
			MarginRatioPct:           input.FuturesState.MarginRatioPct,
			CrossExchange:            input.Plan.SpotProvider != input.Plan.FuturesProvider,
			DataStatus:               DataStatusOK,
		},
		DataStatus: DataStatusOK,
	}

	// Step 1: Check data availability first
	if input.Plan.Status != PlanStatusDraft && input.Plan.Status != PlanStatusReady {
		// For active plans, check data availability. A zero mark price means the
		// futures position could not be read, so liquidation distance cannot be
		// computed — treat it as a data failure rather than letting
		// computeLiquidationDistance report a misleading "100% safe".
		futuresDataMissing := !input.FuturesState.Available || input.FuturesState.MarkPrice == 0
		if !input.SpotState.Available || futuresDataMissing || !input.FundingInfo.Available {
			if input.Plan.RiskPolicy.EscalateOnDataFailure {
				result.DataStatus = DataStatusError
				result.ThresholdBreached = true
				result.BreachCodes = []string{"data_unavailable"}
				result.Severity = "critical"
				result.RecommendedAction = "Data fetch failed for active plan. Check network connectivity and API keys. Manual intervention may be required."
				result.Snapshot.DataStatus = DataStatusError
				result.Snapshot.ThresholdBreached = true
				result.Snapshot.BreachCodes = []string{"data_unavailable"}
				return result
			}
			// If escalation is off, continue with unavailable status
			result.DataStatus = DataStatusUnavailable
			result.Snapshot.DataStatus = DataStatusUnavailable
		}
	}

	// Step 2: Normalize spot and futures values
	spotValueUSDT := input.SpotState.ValueUSDT
	futuresNotionalUSDT := math.Abs(input.FuturesState.NotionalUSDT)

	// Step 3: Compute delta drift
	deltaDrift := computeDeltaDrift(spotValueUSDT, futuresNotionalUSDT)
	result.Snapshot.DeltaDriftPct = deltaDrift

	// Step 3.5: Propagate spread values from input
	result.Snapshot.EntrySpreadPct = input.EntrySpreadPct
	result.Snapshot.ExitSpreadPct = input.ExitSpreadPct

	// Step 4: Compute liquidation distance
	liquidationDistance := computeLiquidationDistance(input.FuturesState.MarkPrice, input.FuturesState.LiquidationPrice)
	result.Snapshot.LiquidationDistancePct = liquidationDistance

	// Step 5: Classify funding state
	fundingState := classifyFunding(input.FundingInfo, input.Plan.RiskPolicy)
	result.Snapshot.FundingState = fundingState

	// Step 6: Classify margin state
	marginState := classifyMargin(input.FuturesState.MarginRatioPct, liquidationDistance, input.Plan.RiskPolicy)
	result.Snapshot.MarginState = marginState

	// Step 7: Cross-exchange flag (already set above)
	crossExchange := result.Snapshot.CrossExchange

	// Step 8: Compute health score
	healthScore := computeHealthScore(
		fundingState,
		marginState,
		deltaDrift,
		liquidationDistance,
		input.FuturesState.MarginRatioPct,
		input.Plan.RiskPolicy,
		crossExchange,
		input.FundingInfo.CurrentRate,
	)
	result.Snapshot.HealthScore = healthScore
	result.Snapshot.HealthLabel = labelForScore(healthScore)

	// Step 9: Detect threshold breaches
	breachCodes := detectBreaches(
		deltaDrift,
		liquidationDistance,
		fundingState,
		marginState,
		input.FundingInfo,
		input.Plan.RiskPolicy,
		input.ExitSpreadPct,
		input.Plan.ExitRules,
	)
	result.BreachCodes = breachCodes
	result.ThresholdBreached = len(breachCodes) > 0
	result.Snapshot.BreachCodes = breachCodes
	result.Snapshot.ThresholdBreached = len(breachCodes) > 0

	// Step 10: Determine severity and recommended action
	if len(breachCodes) > 0 {
		result.Severity = determineSeverity(breachCodes)
		result.RecommendedAction = recommendAction(breachCodes, result.Snapshot)
	}

	// Set agent invoked flag to false (the handler decides)
	result.Snapshot.AgentInvoked = false

	return result
}

// computeDeltaDrift computes the delta drift percentage
// Formula: abs(spot_value - abs(futures_notional)) / max(spot_value, abs(futures_notional)) * 100
func computeDeltaDrift(spotValueUSDT, futuresNotionalUSDT float64) float64 {
	spotAbs := math.Abs(spotValueUSDT)
	futuresAbs := math.Abs(futuresNotionalUSDT)

	// Guard against divide-by-zero
	if spotAbs == 0 && futuresAbs == 0 {
		return 0
	}

	maxVal := math.Max(spotAbs, futuresAbs)
	if maxVal == 0 {
		return 0
	}

	drift := math.Abs(spotAbs-futuresAbs) / maxVal * 100
	return math.Min(drift, 100) // Cap at 100%
}

// computeLiquidationDistance computes the liquidation distance percentage
// Formula: abs(mark_price - liquidation_price) / mark_price * 100
func computeLiquidationDistance(markPrice, liquidationPrice float64) float64 {
	// Guard against mark_price == 0
	if markPrice == 0 {
		// If mark price is zero, assume safe (no valid liq price)
		return 100
	}

	// If liquidation price is 0, treat as "no liq price" => safe distance
	if liquidationPrice == 0 {
		return 100
	}

	distance := math.Abs(markPrice-liquidationPrice) / math.Abs(markPrice) * 100
	return math.Min(distance, 100) // Cap at 100%
}

// classifyFunding classifies funding state from FundingInfo
func classifyFunding(fundingInfo FundingInfo, policy RiskPolicy) string {
	if !fundingInfo.Available {
		return "unavailable"
	}

	rate := fundingInfo.CurrentRate

	// Check negative first (single negative takes priority over reversal detection)
	if rate < 0 {
		// Also check for reversal if we have recent rates with N consecutive negatives
		if len(fundingInfo.RecentRates) > 0 {
			consecutiveNeg := 0
			for i := len(fundingInfo.RecentRates) - 1; i >= 0; i-- {
				if fundingInfo.RecentRates[i] < 0 {
					consecutiveNeg++
				} else {
					break
				}
			}
			if consecutiveNeg >= policy.FundingReversalCycles && policy.FundingReversalCycles > 0 {
				return "reversing"
			}
		}
		return "negative"
	}

	// Check below minimum
	if rate < policy.MinFundingRate && policy.MinFundingRate > 0 {
		return "watch"
	}

	// Positive above minimum
	if rate > 0 {
		return "positive"
	}

	return "neutral"
}

// classifyMargin classifies margin state based on ratio and liquidation distance
func classifyMargin(marginRatioPct, liquidationDistancePct float64, policy RiskPolicy) string {
	// Critical: liquidation distance very low
	if liquidationDistancePct < (policy.MinLiquidationDistancePct * 0.5) {
		return "critical"
	}

	// Danger: liquidation distance below minimum
	if liquidationDistancePct < policy.MinLiquidationDistancePct {
		return "danger"
	}

	// Watch: margin ratio is getting tight (< 50%) but liquidation distance is ok
	if marginRatioPct > 0 && marginRatioPct < 50 {
		return "watch"
	}

	// Safe: good margin and liquidation distance
	return "safe"
}

// computeHealthScore computes a 0-100 health score based on components
// Components:
// - Funding: 20 points
// - Margin/liquidation: 25 points
// - Delta balance: 20 points
// - Liquidity/slippage: 10 points (assume ok for now)
// - Exchange/execution risk: 10 points
// - Profit progress: 15 points (assume ok for now)
func computeHealthScore(
	fundingState string,
	marginState string,
	deltaDriftPct float64,
	liquidationDistancePct float64,
	marginRatioPct float64,
	policy RiskPolicy,
	crossExchange bool,
	fundingRate float64,
) int {
	score := 0

	// Funding health: 20 points
	fundingScore := 0
	switch fundingState {
	case "positive":
		fundingScore = 20
	case "neutral":
		fundingScore = 10
	case "watch":
		fundingScore = 5
	case "negative":
		fundingScore = 0
	case "reversing":
		fundingScore = 2
	case "unavailable":
		fundingScore = 0
	}
	score += fundingScore

	// Margin/liquidation health: 25 points
	marginScore := 0
	switch marginState {
	case "safe":
		marginScore = 25
	case "watch":
		marginScore = 15
	case "danger":
		marginScore = 5
	case "critical":
		marginScore = 0
	}
	score += marginScore

	// Delta balance: 20 points
	// Excellent: < 1% drift, Good: < 3%, Watch: < 10%, Poor: >= 10%
	deltaScore := 0
	if deltaDriftPct < 1 {
		deltaScore = 20
	} else if deltaDriftPct < 3 {
		deltaScore = 15
	} else if deltaDriftPct < 10 {
		deltaScore = 8
	} else if deltaDriftPct < 20 {
		deltaScore = 3
	} else {
		deltaScore = 0
	}
	score += deltaScore

	// Liquidity/slippage: 10 points (assume ok for now)
	score += 10

	// Exchange/execution risk: 10 points
	exchangeScore := 10
	if crossExchange {
		// Subtract from exchange score for cross-exchange
		exchangeScore = 6
	}
	score += exchangeScore

	// Profit progress: 15 points (assume ok for now)
	score += 15

	// Clamp to 0-100
	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	return score
}

// labelForScore returns the health label for a given score
func labelForScore(score int) string {
	switch {
	case score >= 90:
		return HealthLabelExcellent
	case score >= 75:
		return HealthLabelHealthy
	case score >= 60:
		return HealthLabelWatch
	case score >= 40:
		return HealthLabelDanger
	default:
		return HealthLabelCritical
	}
}

// detectBreaches detects threshold breaches and returns breach codes
func detectBreaches(
	deltaDriftPct float64,
	liquidationDistancePct float64,
	fundingState string,
	marginState string,
	fundingInfo FundingInfo,
	policy RiskPolicy,
	exitSpreadPct float64,
	exitRules ExitRules,
) []string {
	var breaches []string

	// Funding breaches
	if fundingState == "negative" {
		breaches = append(breaches, "funding_negative")
	}

	if fundingState == "unavailable" {
		breaches = append(breaches, "funding_unavailable")
	}

	if fundingInfo.Available && fundingInfo.CurrentRate < policy.MinFundingRate && policy.MinFundingRate > 0 {
		breaches = append(breaches, "funding_below_min")
	}

	if fundingState == "reversing" {
		breaches = append(breaches, "funding_reversal")
	}

	// Liquidation distance breach
	if liquidationDistancePct < policy.MinLiquidationDistancePct {
		breaches = append(breaches, "liquidation_distance_low")
	}

	// Delta drift breach
	if deltaDriftPct > policy.MaxDeltaDriftPct {
		breaches = append(breaches, "delta_drift_high")
	}

	// Margin danger/critical
	if marginState == "danger" {
		breaches = append(breaches, "margin_danger")
	} else if marginState == "critical" {
		breaches = append(breaches, "margin_critical")
	}

	// Exit spread target: profit-taking signal (non-critical)
	if exitRules.TargetExitSpreadPct != 0 && exitSpreadPct >= exitRules.TargetExitSpreadPct {
		breaches = append(breaches, "exit_spread_target_met")
	}

	return breaches
}

// criticalBreachCodes are breach codes whose condition can deteriorate further
// under the same code (e.g. liquidation distance shrinking, margin ratio climbing).
// They always map to "critical" severity and must never be throttled by the alert
// cooldown — see IsCriticalBreachCode.
var criticalBreachCodes = map[string]bool{
	"data_unavailable":         true,
	"margin_critical":          true,
	"liquidation_distance_low": true,
}

// IsCriticalBreachCode reports whether a breach code is critical severity. The cron
// monitor uses this to bypass alert-cooldown silencing for critical breaches so a
// worsening position keeps re-alerting on every tick.
func IsCriticalBreachCode(code string) bool { return criticalBreachCodes[code] }

// determineSeverity determines the worst severity among breaches
func determineSeverity(breachCodes []string) string {
	// Severity levels: critical > danger > warn > info
	hasCritical := false
	hasDanger := false

	dangerCodes := map[string]bool{
		"funding_negative":    true,
		"funding_reversal":    true,
		"margin_danger":       true,
		"delta_drift_high":    true,
		"funding_unavailable": true,
	}

	for _, code := range breachCodes {
		if criticalBreachCodes[code] {
			hasCritical = true
		}
		if dangerCodes[code] {
			hasDanger = true
		}
	}

	if hasCritical {
		return "critical"
	}
	if hasDanger {
		return "danger"
	}
	return "warn"
}

// recommendAction returns a recommended action based on breach codes and snapshot
func recommendAction(breachCodes []string, snapshot MonitorSnapshot) string {
	// Build recommendation based on breach codes
	if len(breachCodes) == 0 {
		return ""
	}

	// Critical breaches
	for _, code := range breachCodes {
		if code == "data_unavailable" {
			return "Data fetch failed for active plan. Check network connectivity and API keys. Manual intervention may be required."
		}
		if code == "margin_critical" {
			return "Margin ratio is critical. Consider reducing position size or unwinding immediately."
		}
		if code == "liquidation_distance_low" {
			return "Liquidation distance is dangerously low. Reduce position or add margin immediately."
		}
	}

	// Danger breaches
	if containsAny(breachCodes, []string{"funding_negative", "funding_unavailable", "margin_danger"}) {
		return "Plan health has degraded. Review position and consider unwinding or rebalancing."
	}

	if containsAny(breachCodes, []string{"delta_drift_high"}) {
		return "Delta drift has exceeded policy threshold. Rebalance positions to restore hedge."
	}

	if containsAny(breachCodes, []string{"funding_reversal"}) {
		return "Funding rates are reversing. Monitor closely; consider unwinding if reversal sustains."
	}

	if containsAny(breachCodes, []string{"funding_below_min"}) {
		return "Current funding rate is below policy minimum. Plan profitability is declining."
	}

	if containsAny(breachCodes, []string{"exit_spread_target_met"}) {
		return "Exit spread target met. Consider unwinding to lock in gains from basis convergence."
	}

	return "Plan threshold breached. Review metrics and consider appropriate action."
}

// containsAny checks if any string in the list is contained in the codes slice
func containsAny(codes []string, list []string) bool {
	for _, code := range codes {
		for _, target := range list {
			if code == target {
				return true
			}
		}
	}
	return false
}
