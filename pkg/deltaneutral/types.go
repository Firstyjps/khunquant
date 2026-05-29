package deltaneutral

import "time"

// PlanStatus enum for plan lifecycle
const (
	PlanStatusDraft            = "draft"
	PlanStatusReady            = "ready"
	PlanStatusActive           = "active"
	PlanStatusPaused           = "paused"
	PlanStatusRecoveryRequired = "recovery_required"
	PlanStatusClosing          = "closing"
	PlanStatusClosed           = "closed"
	PlanStatusFailed           = "failed"
	PlanStatusArchived         = "archived"
)

// ExecutionMode enum for plan execution mode
const (
	ExecutionModeMonitor  = "monitor"
	ExecutionModeApproval = "approval"
	ExecutionModeSemiAuto = "semi_auto"
	ExecutionModeFullAuto = "full_auto"
)

// HealthLabel enum for health evaluation
const (
	HealthLabelExcellent = "excellent"
	HealthLabelHealthy   = "healthy"
	HealthLabelWatch     = "watch"
	HealthLabelDanger    = "danger"
	HealthLabelCritical  = "critical"
)

// DataStatus enum for data fetch/availability status
const (
	DataStatusOK          = "ok"
	DataStatusPartial     = "partial"
	DataStatusUnavailable = "unavailable"
	DataStatusError       = "error"
)

// EntryRules defines policy for plan entry
type EntryRules struct {
	MinFundingRate float64 `json:"min_funding_rate"`
	MaxSlippageBps float64 `json:"max_slippage_bps"`
}

// ExitRules defines policy for plan exit
type ExitRules struct {
	ProfitTargetUSDT      float64 `json:"profit_target_usdt"`
	MaxDrawdownUSDT       float64 `json:"max_drawdown_usdt"`
	FundingReversalCycles int     `json:"funding_reversal_cycles"`
}

// RiskPolicy defines risk thresholds and escalation rules for a plan
type RiskPolicy struct {
	MinFundingRate            float64 `json:"min_funding_rate"`
	MaxBreakevenDays          float64 `json:"max_breakeven_days"`
	MinLiquidationDistancePct float64 `json:"min_liquidation_distance_pct"`
	MaxDeltaDriftPct          float64 `json:"max_delta_drift_pct"`
	MaxSlippageBps            float64 `json:"max_slippage_bps"`
	MaxCapitalUSDT            float64 `json:"max_capital_usdt"`
	MaxLeverage               int     `json:"max_leverage"`
	ReserveMarginUSDT         float64 `json:"reserve_margin_usdt"`
	FundingReversalCycles     int     `json:"funding_reversal_cycles"`
	ProfitTargetUSDT          float64 `json:"profit_target_usdt"`
	MaxDrawdownUSDT           float64 `json:"max_drawdown_usdt"`
	EscalateOnDataFailure     bool    `json:"escalate_on_data_failure"`
}

// DefaultRiskPolicy returns a RiskPolicy with conservative defaults
func DefaultRiskPolicy() RiskPolicy {
	return RiskPolicy{
		MinLiquidationDistancePct: 25,
		MaxDeltaDriftPct:          3,
		MaxSlippageBps:            20,
		FundingReversalCycles:     2,
		EscalateOnDataFailure:     true,
	}
}

// Plan represents a delta-neutral funding strategy plan
type Plan struct {
	ID                       int64      `json:"id"`
	Name                     string     `json:"name"`
	Asset                    string     `json:"asset"`
	Status                   string     `json:"status"`
	Mode                     string     `json:"mode"`
	SpotProvider             string     `json:"spot_provider"`
	SpotAccount              string     `json:"spot_account"`
	SpotSymbol               string     `json:"spot_symbol"`
	SpotSide                 string     `json:"spot_side"`
	FuturesProvider          string     `json:"futures_provider"`
	FuturesAccount           string     `json:"futures_account"`
	FuturesSymbol            string     `json:"futures_symbol"`
	FuturesSide              string     `json:"futures_side"`
	FuturesMarginMode        string     `json:"futures_margin_mode"`
	FuturesLeverage          int        `json:"futures_leverage"`
	CapitalUSDT              float64    `json:"capital_usdt"`
	SpotNotionalUSDT         float64    `json:"spot_notional_usdt"`
	FuturesNotionalUSDT      float64    `json:"futures_notional_usdt"`
	ReserveMarginUSDT        float64    `json:"reserve_margin_usdt"`
	MonitorInterval          string     `json:"monitor_interval"`
	CronJobID                string     `json:"cron_job_id"`
	Enabled                  bool       `json:"enabled"`
	EntryRules               EntryRules `json:"entry_rules"`
	ExitRules                ExitRules  `json:"exit_rules"`
	RiskPolicy               RiskPolicy `json:"risk_policy"`
	EstimatedEntryCostUSDT   float64    `json:"estimated_entry_cost_usdt"`
	EstimatedExitCostUSDT    float64    `json:"estimated_exit_cost_usdt"`
	ExpectedDailyFundingUSDT float64    `json:"expected_daily_funding_usdt"`
	BreakevenDays            float64    `json:"breakeven_days"`
	CrossExchange            bool       `json:"cross_exchange"`
	NotifyChannel            string     `json:"notify_channel"`
	NotifyChatID             string     `json:"notify_chat_id"`
	OpenedAt                 *time.Time `json:"opened_at"`
	ClosedAt                 *time.Time `json:"closed_at"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

// MonitorSnapshot represents a single monitor evaluation snapshot
type MonitorSnapshot struct {
	ID                       int64     `json:"id"`
	PlanID                   int64     `json:"plan_id"`
	CheckedAt                time.Time `json:"checked_at"`
	SpotPrice                float64   `json:"spot_price"`
	SpotQuantity             float64   `json:"spot_quantity"`
	SpotValueUSDT            float64   `json:"spot_value_usdt"`
	FuturesMarkPrice         float64   `json:"futures_mark_price"`
	FuturesContracts         float64   `json:"futures_contracts"`
	FuturesNotionalUSDT      float64   `json:"futures_notional_usdt"`
	FuturesUnrealizedPnLUSDT float64   `json:"futures_unrealized_pnl_usdt"`
	CurrentFundingRate       float64   `json:"current_funding_rate"`
	EstimatedNextFundingUSDT float64   `json:"estimated_next_funding_usdt"`
	FundingState             string    `json:"funding_state"`
	DeltaDriftPct            float64   `json:"delta_drift_pct"`
	LiquidationPrice         float64   `json:"liquidation_price"`
	LiquidationDistancePct   float64   `json:"liquidation_distance_pct"`
	MarginRatioPct           float64   `json:"margin_ratio_pct"`
	MarginState              string    `json:"margin_state"`
	HealthScore              int       `json:"health_score"`
	HealthLabel              string    `json:"health_label"`
	CrossExchange            bool      `json:"cross_exchange"`
	ThresholdBreached        bool      `json:"threshold_breached"`
	BreachCodes              []string  `json:"breach_codes"`
	DataStatus               string    `json:"data_status"`
	ErrorMsg                 string    `json:"error_msg"`
	AgentInvoked             bool      `json:"agent_invoked"`
	CreatedAt                time.Time `json:"created_at"`
}

// SpotState describes the spot leg's current balance and value
type SpotState struct {
	Available bool
	Price     float64
	Quantity  float64
	ValueUSDT float64
}

// FuturesState describes the futures leg's position and risk metrics
type FuturesState struct {
	Available         bool
	MarkPrice         float64
	Contracts         float64
	NotionalUSDT      float64
	UnrealizedPnLUSDT float64
	LiquidationPrice  float64
	MarginRatioPct    float64
}

// FundingInfo describes funding rate and next payment estimates
type FundingInfo struct {
	Available         bool
	CurrentRate       float64
	EstimatedNextUSDT float64
	RecentRates       []float64
	NextFundingTime   time.Time
}

// EvaluationInput holds all inputs required for deterministic health evaluation
type EvaluationInput struct {
	Plan         Plan
	SpotState    SpotState
	FuturesState FuturesState
	FundingInfo  FundingInfo
	Now          time.Time
}

// HealthEvaluation is the output of the deterministic health evaluator
type HealthEvaluation struct {
	Snapshot          MonitorSnapshot
	ThresholdBreached bool
	BreachCodes       []string
	Severity          string
	RecommendedAction string
	DataStatus        string
	Err               error
}
