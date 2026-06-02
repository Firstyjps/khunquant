package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/cron"
	"github.com/cryptoquantumwave/khunquant/pkg/deltaneutral"
	"github.com/cryptoquantumwave/khunquant/pkg/tools"
)

// dnFeeSnapshot represents accumulated fees for a plan's futures position.
type dnFeeSnapshot struct {
	TradingFeeUSDT float64   `json:"trading_fee_usdt"`
	FundingFeeUSDT float64   `json:"funding_fee_usdt"`
	FetchedAt      time.Time `json:"fetched_at"`
}

// dnPlanListItem represents a delta-neutral plan in list responses
type dnPlanListItem struct {
	ID                  int64          `json:"id"`
	Name                string         `json:"name"`
	Asset               string         `json:"asset"`
	Status              string         `json:"status"`
	Mode                string         `json:"mode"`
	SpotProvider        string         `json:"spot_provider"`
	SpotAccount         string         `json:"spot_account"`
	SpotSymbol          string         `json:"spot_symbol"`
	FuturesProvider     string         `json:"futures_provider"`
	FuturesAccount      string         `json:"futures_account"`
	FuturesSymbol       string         `json:"futures_symbol"`
	CapitalUSDT         float64        `json:"capital_usdt"`
	SpotNotionalUSDT    float64        `json:"spot_notional_usdt"`
	FuturesNotionalUSDT float64        `json:"futures_notional_usdt"`
	MonitorInterval     string         `json:"monitor_interval"`
	Enabled             bool           `json:"enabled"`
	CrossExchange       bool           `json:"cross_exchange"`
	HealthScore         int            `json:"health_score"`
	HealthLabel         string         `json:"health_label"`
	LastCheckedAt       *time.Time     `json:"last_checked_at,omitempty"`
	LastAlertAt         *time.Time     `json:"last_alert_at,omitempty"`
	FeeSnapshot         *dnFeeSnapshot `json:"fee_snapshot,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// dnMonitorSnapshotDTO represents a monitor snapshot response
type dnMonitorSnapshotDTO struct {
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
	FundingAPYPct            float64   `json:"funding_apy_pct"`
	EarnAPYPct               float64   `json:"earn_apy_pct"`
	CombinedAPYPct           float64   `json:"combined_apy_pct"`
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
	ErrorMsg                 string    `json:"error_msg,omitempty"`
	AgentInvoked             bool      `json:"agent_invoked"`
	CreatedAt                time.Time `json:"created_at"`
}

// dnAlertDTO represents an alert response
type dnAlertDTO struct {
	ID                int64     `json:"id"`
	PlanID            int64     `json:"plan_id"`
	SnapshotID        *int64    `json:"snapshot_id,omitempty"`
	TriggeredAt       time.Time `json:"triggered_at"`
	Severity          string    `json:"severity"`
	Code              string    `json:"code"`
	Message           string    `json:"message"`
	RecommendedAction string    `json:"recommended_action,omitempty"`
	AgentInvoked      bool      `json:"agent_invoked"`
	DeliveredChannel  string    `json:"delivered_channel,omitempty"`
	DeliveredChatID   string    `json:"delivered_chat_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// dnExecutionLegDTO represents a single leg of an execution
type dnExecutionLegDTO struct {
	ID                    int64     `json:"id"`
	ExecutionID           int64     `json:"execution_id"`
	LegType               string    `json:"leg_type"`
	Provider              string    `json:"provider"`
	Account               string    `json:"account,omitempty"`
	Symbol                string    `json:"symbol"`
	Side                  string    `json:"side"`
	OrderType             string    `json:"order_type"`
	RequestedAmount       float64   `json:"requested_amount"`
	RequestedNotionalUSDT float64   `json:"requested_notional_usdt"`
	RequestedPrice        float64   `json:"requested_price"`
	OrderID               string    `json:"order_id,omitempty"`
	State                 string    `json:"state"`
	FilledQuantity        float64   `json:"filled_quantity"`
	FilledNotionalUSDT    float64   `json:"filled_notional_usdt"`
	AvgFillPrice          float64   `json:"avg_fill_price"`
	FeeUSDT               float64   `json:"fee_usdt"`
	ErrorMsg              string    `json:"error_msg,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// dnExecutionDTO represents an execution attempt with nested legs
type dnExecutionDTO struct {
	ID          int64               `json:"id"`
	PlanID      int64               `json:"plan_id"`
	AttemptID   string              `json:"attempt_id"`
	State       string              `json:"state"`
	RequestedAt time.Time           `json:"requested_at"`
	ApprovedAt  *time.Time          `json:"approved_at,omitempty"`
	CompletedAt *time.Time          `json:"completed_at,omitempty"`
	ErrorMsg    string              `json:"error_msg,omitempty"`
	Legs        []dnExecutionLegDTO `json:"legs"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

func (h *Handler) registerAgentDeltaNeutralRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agent/delta-neutral/plans", h.handleListDeltaNeutralPlans)
	mux.HandleFunc("GET /api/agent/delta-neutral/plans/{id}", h.handleGetDeltaNeutralPlan)
	mux.HandleFunc("DELETE /api/agent/delta-neutral/plans/{id}", h.handleDeleteDeltaNeutralPlan)
	mux.HandleFunc("GET /api/agent/delta-neutral/plans/{id}/monitor-snapshots", h.handleGetDeltaNeutralSnapshots)
	mux.HandleFunc("GET /api/agent/delta-neutral/plans/{id}/monitor-series", h.handleGetDeltaNeutralSnapshotSeries)
	mux.HandleFunc("GET /api/agent/delta-neutral/plans/{id}/alerts", h.handleGetDeltaNeutralAlerts)
	mux.HandleFunc("GET /api/agent/delta-neutral/plans/{id}/executions", h.handleGetDeltaNeutralExecutions)
}

func (h *Handler) dnWorkspacePath() (string, error) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		return "", fmt.Errorf("failed to load config: %w", err)
	}
	return cfg.WorkspacePath(), nil
}

func (h *Handler) handleListDeltaNeutralPlans(w http.ResponseWriter, r *http.Request) {
	workspacePath, err := h.dnWorkspacePath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	store, err := deltaneutral.NewStore(workspacePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open delta-neutral store: %v", err), http.StatusInternalServerError)
		return
	}
	defer store.Close()

	q := r.URL.Query()
	var filterEnabled *bool
	if v := q.Get("enabled"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			filterEnabled = &b
		}
	}

	var filterStatus *string
	if v := q.Get("status"); v != "" {
		filterStatus = &v
	}

	plans, err := store.ListPlans(r.Context(), deltaneutral.QueryFilter{
		Status:  filterStatus,
		Enabled: filterEnabled,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list delta-neutral plans: %v", err), http.StatusInternalServerError)
		return
	}

	items := make([]dnPlanListItem, len(plans))
	for i := range plans {
		item := dnPlanToListItem(&plans[i])

		// Enrich with latest snapshot health data
		snapshot, err := store.LatestSnapshot(r.Context(), plans[i].ID)
		if err == nil && snapshot != nil {
			item.HealthScore = snapshot.HealthScore
			item.HealthLabel = snapshot.HealthLabel
			item.LastCheckedAt = &snapshot.CheckedAt
		}

		// Enrich with latest alert timestamp
		alert, err := store.LatestAlert(r.Context(), plans[i].ID)
		if err == nil && alert != nil {
			item.LastAlertAt = &alert.TriggeredAt
		}

		// Enrich with latest fee snapshot
		feeSnap, err := store.GetLatestPlanFeeSnapshot(r.Context(), plans[i].ID)
		if err == nil && feeSnap != nil {
			item.FeeSnapshot = &dnFeeSnapshot{
				TradingFeeUSDT: feeSnap.TradingFeeUSDT,
				FundingFeeUSDT: feeSnap.FundingFeeUSDT,
				FetchedAt:      feeSnap.FetchedAt,
			}
		}

		items[i] = item
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items) //nolint:errcheck
}

func (h *Handler) handleGetDeltaNeutralPlan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	workspacePath, err := h.dnWorkspacePath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	store, err := deltaneutral.NewStore(workspacePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open delta-neutral store: %v", err), http.StatusInternalServerError)
		return
	}
	defer store.Close()

	plan, err := store.GetPlan(r.Context(), id)
	if err != nil {
		http.Error(w, fmt.Sprintf("plan not found: %v", err), http.StatusNotFound)
		return
	}

	item := dnPlanToListItem(plan)

	// Enrich with latest snapshot health data
	snapshot, err := store.LatestSnapshot(r.Context(), id)
	if err == nil && snapshot != nil {
		item.HealthScore = snapshot.HealthScore
		item.HealthLabel = snapshot.HealthLabel
		item.LastCheckedAt = &snapshot.CheckedAt
	}

	// Enrich with latest alert timestamp
	alert, err := store.LatestAlert(r.Context(), id)
	if err == nil && alert != nil {
		item.LastAlertAt = &alert.TriggeredAt
	}

	// Enrich with latest fee snapshot
	feeSnap, err := store.GetLatestPlanFeeSnapshot(r.Context(), id)
	if err == nil && feeSnap != nil {
		item.FeeSnapshot = &dnFeeSnapshot{
			TradingFeeUSDT: feeSnap.TradingFeeUSDT,
			FundingFeeUSDT: feeSnap.FundingFeeUSDT,
			FetchedAt:      feeSnap.FetchedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item) //nolint:errcheck
}

type deleteDeltaNeutralPlanRequest struct {
	ForceUnwind         bool `json:"force_unwind"`
	DeleteWithoutUnwind bool `json:"delete_without_unwind"`
}

type deleteDeltaNeutralPlanResponse struct {
	Message              string `json:"message"`
	Unwound              bool   `json:"unwound"`
	DeletedWithoutUnwind bool   `json:"deleted_without_unwind"`
	CronJobID            string `json:"cron_job_id,omitempty"`
}

func (h *Handler) handleDeleteDeltaNeutralPlan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var req deleteDeltaNeutralPlanRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	store, err := deltaneutral.NewStore(cfg.WorkspacePath())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open delta-neutral store: %v", err), http.StatusInternalServerError)
		return
	}
	defer store.Close()

	plan, err := store.GetPlan(r.Context(), id)
	if err != nil {
		http.Error(w, fmt.Sprintf("plan not found: %v", err), http.StatusNotFound)
		return
	}

	needsUnwind := plan.Status == deltaneutral.PlanStatusActive || plan.Status == deltaneutral.PlanStatusRecoveryRequired
	if needsUnwind && !req.ForceUnwind && !req.DeleteWithoutUnwind {
		http.Error(w, "plan has active or recovery-required legs; confirm force_unwind to unwind and delete it", http.StatusConflict)
		return
	}

	unwound := false
	if needsUnwind && req.ForceUnwind && !req.DeleteWithoutUnwind {
		result := tools.NewUnwindDeltaNeutralPositionTool(cfg, store).Execute(r.Context(), map[string]any{
			"plan_id": float64(id),
			"confirm": true,
		})
		if result == nil || result.IsError {
			msg := "failed to unwind plan before delete"
			if result != nil && result.ForLLM != "" {
				msg = result.ForLLM
			}
			http.Error(w, msg, http.StatusBadGateway)
			return
		}
		unwound = true
	}

	if plan.CronJobID != "" {
		if err := h.removeDeltaNeutralCronJob(r.Context(), cfg.WorkspacePath(), plan.CronJobID); err != nil {
			http.Error(w, fmt.Sprintf("failed to remove cron job %s: %v", plan.CronJobID, err), http.StatusInternalServerError)
			return
		}
	}

	if err := store.DeletePlan(r.Context(), id); err != nil {
		http.Error(w, fmt.Sprintf("failed to delete plan: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(deleteDeltaNeutralPlanResponse{
		Message:              fmt.Sprintf("Delta-neutral plan %d (%s) deleted.", id, plan.Name),
		Unwound:              unwound,
		DeletedWithoutUnwind: needsUnwind && req.DeleteWithoutUnwind,
		CronJobID:            plan.CronJobID,
	}) //nolint:errcheck
}

func (h *Handler) removeDeltaNeutralCronJob(ctx context.Context, workspacePath, jobID string) error {
	base, err := h.gatewayBase()
	if err == nil {
		httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/api/cron/jobs/"+jobID, nil)
		if reqErr != nil {
			return reqErr
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, doErr := client.Do(httpReq)
		if doErr == nil {
			defer resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == http.StatusNotFound {
				return nil
			}
			return fmt.Errorf("gateway returned %s", resp.Status)
		}
	}

	cronPath := filepath.Join(workspacePath, "cron", "jobs.json")
	cronService := cron.NewCronService(cronPath, nil)
	cronService.RemoveJob(jobID)
	return nil
}

func (h *Handler) handleGetDeltaNeutralSnapshots(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	workspacePath, err := h.dnWorkspacePath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	store, err := deltaneutral.NewStore(workspacePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open delta-neutral store: %v", err), http.StatusInternalServerError)
		return
	}
	defer store.Close()

	snapshots, err := store.ListSnapshots(r.Context(), planID, limit, offset)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list snapshots: %v", err), http.StatusInternalServerError)
		return
	}

	items := make([]dnMonitorSnapshotDTO, len(snapshots))
	for i, snap := range snapshots {
		items[i] = snapshotToDTO(&snap)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items) //nolint:errcheck
}

// dnSeriesPointDTO is the slim payload returned by the /monitor-series endpoint.
type dnSeriesPointDTO struct {
	T           time.Time `json:"t"`
	FundingRate float64   `json:"funding_rate"`
	FundingAPY  float64   `json:"funding_apy"`
	EarnAPY     float64   `json:"earn_apy"`
	CombinedAPY float64   `json:"combined_apy"`
}

func (h *Handler) handleGetDeltaNeutralSnapshotSeries(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()

	// Parse ?range= (default 7d). Supported: 7d, 14d, 30d, 3m, 6m, all.
	rangeStr := q.Get("range")
	now := time.Now().UTC()
	var since time.Time
	switch rangeStr {
	case "14d":
		since = now.AddDate(0, 0, -14)
	case "30d":
		since = now.AddDate(0, 0, -30)
	case "3m":
		since = now.AddDate(0, -3, 0)
	case "6m":
		since = now.AddDate(0, -6, 0)
	case "all":
		since = time.Time{} // zero = no filter
	default: // "7d" or unknown
		since = now.AddDate(0, 0, -7)
	}

	// Parse ?max_points= (default 500, cap 2000).
	maxPoints := 500
	if v := q.Get("max_points"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil && n > 0 {
			maxPoints = n
		}
	}
	if maxPoints > 2000 {
		maxPoints = 2000
	}

	workspacePath, err := h.dnWorkspacePath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	store, err := deltaneutral.NewStore(workspacePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open delta-neutral store: %v", err), http.StatusInternalServerError)
		return
	}
	defer store.Close()

	series, err := store.ListSnapshotSeries(r.Context(), planID, since, maxPoints)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to query series: %v", err), http.StatusInternalServerError)
		return
	}

	items := make([]dnSeriesPointDTO, len(series))
	for i, p := range series {
		items[i] = dnSeriesPointDTO{
			T:           p.CheckedAt,
			FundingRate: p.CurrentFundingRate,
			FundingAPY:  p.FundingAPYPct,
			EarnAPY:     p.EarnAPYPct,
			CombinedAPY: p.CombinedAPYPct,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items) //nolint:errcheck
}

func (h *Handler) handleGetDeltaNeutralAlerts(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	workspacePath, err := h.dnWorkspacePath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	store, err := deltaneutral.NewStore(workspacePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open delta-neutral store: %v", err), http.StatusInternalServerError)
		return
	}
	defer store.Close()

	alerts, err := store.ListAlerts(r.Context(), planID, limit, offset)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list alerts: %v", err), http.StatusInternalServerError)
		return
	}

	items := make([]dnAlertDTO, len(alerts))
	for i, alert := range alerts {
		items[i] = alertToDTO(&alert)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items) //nolint:errcheck
}

func (h *Handler) handleGetDeltaNeutralExecutions(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	workspacePath, err := h.dnWorkspacePath()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	store, err := deltaneutral.NewStore(workspacePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open delta-neutral store: %v", err), http.StatusInternalServerError)
		return
	}
	defer store.Close()

	execs, err := store.ListExecutions(r.Context(), planID, limit, offset)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list executions: %v", err), http.StatusInternalServerError)
		return
	}

	items := make([]dnExecutionDTO, len(execs))
	for i, exec := range execs {
		item := dnExecutionDTO{
			ID:          exec.ID,
			PlanID:      exec.PlanID,
			AttemptID:   exec.AttemptID,
			State:       exec.State,
			RequestedAt: exec.RequestedAt,
			ApprovedAt:  exec.ApprovedAt,
			CompletedAt: exec.CompletedAt,
			ErrorMsg:    exec.ErrorMsg,
			CreatedAt:   exec.CreatedAt,
			UpdatedAt:   exec.UpdatedAt,
		}

		// Fetch legs for this execution
		legs, err := store.ListExecutionLegs(r.Context(), exec.ID)
		if err == nil {
			item.Legs = make([]dnExecutionLegDTO, len(legs))
			for j, leg := range legs {
				item.Legs[j] = legToDTO(&leg)
			}
		}

		items[i] = item
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items) //nolint:errcheck
}

// Helper functions to convert internal types to DTOs

func dnPlanToListItem(p *deltaneutral.Plan) dnPlanListItem {
	return dnPlanListItem{
		ID:                  p.ID,
		Name:                p.Name,
		Asset:               p.Asset,
		Status:              p.Status,
		Mode:                p.Mode,
		SpotProvider:        p.SpotProvider,
		SpotAccount:         p.SpotAccount,
		SpotSymbol:          p.SpotSymbol,
		FuturesProvider:     p.FuturesProvider,
		FuturesAccount:      p.FuturesAccount,
		FuturesSymbol:       p.FuturesSymbol,
		CapitalUSDT:         p.CapitalUSDT,
		SpotNotionalUSDT:    p.SpotNotionalUSDT,
		FuturesNotionalUSDT: p.FuturesNotionalUSDT,
		MonitorInterval:     p.MonitorInterval,
		Enabled:             p.Enabled,
		CrossExchange:       p.CrossExchange,
		HealthScore:         0,   // Will be enriched from snapshot
		HealthLabel:         "",  // Will be enriched from snapshot
		LastCheckedAt:       nil, // Will be enriched from snapshot
		LastAlertAt:         nil, // Will be enriched from alert
		CreatedAt:           p.CreatedAt,
		UpdatedAt:           p.UpdatedAt,
	}
}

func snapshotToDTO(s *deltaneutral.MonitorSnapshot) dnMonitorSnapshotDTO {
	return dnMonitorSnapshotDTO{
		ID:                       s.ID,
		PlanID:                   s.PlanID,
		CheckedAt:                s.CheckedAt,
		SpotPrice:                s.SpotPrice,
		SpotQuantity:             s.SpotQuantity,
		SpotValueUSDT:            s.SpotValueUSDT,
		FuturesMarkPrice:         s.FuturesMarkPrice,
		FuturesContracts:         s.FuturesContracts,
		FuturesNotionalUSDT:      s.FuturesNotionalUSDT,
		FuturesUnrealizedPnLUSDT: s.FuturesUnrealizedPnLUSDT,
		CurrentFundingRate:       s.CurrentFundingRate,
		FundingAPYPct:            s.FundingAPYPct,
		EarnAPYPct:               s.EarnAPYPct,
		CombinedAPYPct:           s.CombinedAPYPct,
		EstimatedNextFundingUSDT: s.EstimatedNextFundingUSDT,
		FundingState:             s.FundingState,
		DeltaDriftPct:            s.DeltaDriftPct,
		LiquidationPrice:         s.LiquidationPrice,
		LiquidationDistancePct:   s.LiquidationDistancePct,
		MarginRatioPct:           s.MarginRatioPct,
		MarginState:              s.MarginState,
		HealthScore:              s.HealthScore,
		HealthLabel:              s.HealthLabel,
		CrossExchange:            s.CrossExchange,
		ThresholdBreached:        s.ThresholdBreached,
		BreachCodes:              s.BreachCodes,
		DataStatus:               s.DataStatus,
		ErrorMsg:                 s.ErrorMsg,
		AgentInvoked:             s.AgentInvoked,
		CreatedAt:                s.CreatedAt,
	}
}

func alertToDTO(a *deltaneutral.Alert) dnAlertDTO {
	return dnAlertDTO{
		ID:                a.ID,
		PlanID:            a.PlanID,
		SnapshotID:        a.SnapshotID,
		TriggeredAt:       a.TriggeredAt,
		Severity:          a.Severity,
		Code:              a.Code,
		Message:           a.Message,
		RecommendedAction: a.RecommendedAction,
		AgentInvoked:      a.AgentInvoked,
		DeliveredChannel:  a.DeliveredChannel,
		DeliveredChatID:   a.DeliveredChatID,
		CreatedAt:         a.CreatedAt,
	}
}

func legToDTO(l *deltaneutral.ExecutionLeg) dnExecutionLegDTO {
	return dnExecutionLegDTO{
		ID:                    l.ID,
		ExecutionID:           l.ExecutionID,
		LegType:               l.LegType,
		Provider:              l.Provider,
		Account:               l.Account,
		Symbol:                l.Symbol,
		Side:                  l.Side,
		OrderType:             l.OrderType,
		RequestedAmount:       l.RequestedAmount,
		RequestedNotionalUSDT: l.RequestedNotionalUSDT,
		RequestedPrice:        l.RequestedPrice,
		OrderID:               l.OrderID,
		State:                 l.State,
		FilledQuantity:        l.FilledQuantity,
		FilledNotionalUSDT:    l.FilledNotionalUSDT,
		AvgFillPrice:          l.AvgFillPrice,
		FeeUSDT:               l.FeeUSDT,
		ErrorMsg:              l.ErrorMsg,
		CreatedAt:             l.CreatedAt,
		UpdatedAt:             l.UpdatedAt,
	}
}
