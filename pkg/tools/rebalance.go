package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/cron"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
	"github.com/cryptoquantumwave/khunquant/pkg/rebalance"
)

// rebalancePortfolioProvider resolves a provider that can read balances/prices.
var rebalancePortfolioProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.PortfolioProvider, error) {
	p, err := broker.CreateProviderForAccount(providerID, account, cfg)
	if err != nil {
		return nil, fmt.Errorf("provider %q: %w", providerID, err)
	}
	pp, ok := p.(broker.PortfolioProvider)
	if !ok {
		return nil, fmt.Errorf("provider %q does not expose balances", providerID)
	}
	_ = ctx
	return pp, nil
}

func parseTargets(v any) (map[string]float64, error) {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, fmt.Errorf(`targets must be an object of asset → weight, e.g. {"BTC": 50, "ETH": 30, "USDT": 20}`)
	}
	out := map[string]float64{}
	for asset, w := range m {
		f, ok := w.(float64)
		if !ok {
			return nil, fmt.Errorf("target weight for %s must be a number", asset)
		}
		out[strings.ToUpper(strings.TrimSpace(asset))] = f
	}
	return out, nil
}

// resolvePlanArg loads a plan by id or name from the store.
func resolvePlanArg(store *rebalance.Store, args map[string]any) (rebalance.Plan, error) {
	if id := int64(numberArg(args, "plan_id")); id > 0 {
		if p, ok := store.Get(id); ok {
			return p, nil
		}
		return rebalance.Plan{}, fmt.Errorf("rebalance plan %d not found", id)
	}
	if name := stringArg(args, "plan"); name != "" {
		if p, ok := store.FindByName(name); ok {
			return p, nil
		}
		return rebalance.Plan{}, fmt.Errorf("rebalance plan %q not found", name)
	}
	return rebalance.Plan{}, fmt.Errorf("provide plan_id or plan (name)")
}

// evaluatePlanLive fetches balances+prices and runs the pure evaluator.
func evaluatePlanLive(ctx context.Context, cfg *config.Config, plan rebalance.Plan) (rebalance.Proposal, error) {
	pp, err := rebalancePortfolioProviderFn(ctx, cfg, plan.Provider, plan.Account)
	if err != nil {
		return rebalance.Proposal{}, err
	}
	states, err := rebalance.FetchStates(ctx, pp, plan)
	if err != nil {
		return rebalance.Proposal{}, err
	}
	return rebalance.Evaluate(states, plan)
}

// ====================== rebalance_check ======================

type RebalanceCheckTool struct {
	cfg   *config.Config
	store *rebalance.Store
}

func NewRebalanceCheckTool(cfg *config.Config, store *rebalance.Store) *RebalanceCheckTool {
	return &RebalanceCheckTool{cfg: cfg, store: store}
}

func (t *RebalanceCheckTool) Name() string { return NameRebalanceCheck }

func (t *RebalanceCheckTool) Description() string {
	return "Check portfolio allocation drift against target weights and show the deterministic trade proposal (read-only, never trades). " +
		"Use a saved plan (plan/plan_id) or pass provider+targets ad-hoc."
}

func (t *RebalanceCheckTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan":          map[string]any{"type": "string", "description": "Saved plan name."},
			"plan_id":       map[string]any{"type": "integer", "description": "Saved plan id."},
			"provider":      map[string]any{"type": "string", "description": "Ad-hoc: exchange provider (binance, okx, ...)."},
			"account":       map[string]any{"type": "string", "description": "Ad-hoc: account name (empty = default)."},
			"quote":         map[string]any{"type": "string", "description": "Ad-hoc: quote currency, default USDT."},
			"targets":       map[string]any{"type": "object", "description": `Ad-hoc: asset → weight %%, e.g. {"BTC":50,"ETH":30,"USDT":20}. Must include the quote asset.`},
			"tolerance_pct": map[string]any{"type": "number", "description": "Ad-hoc: drift threshold in points (default 5)."},
			"min_trade_usd": map[string]any{"type": "number", "description": "Ad-hoc: suppress trades below this notional (default 10)."},
		},
	}
}

func (t *RebalanceCheckTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	var plan rebalance.Plan
	if stringArg(args, "plan") != "" || numberArg(args, "plan_id") > 0 {
		p, err := resolvePlanArg(t.store, args)
		if err != nil {
			return ErrorResult(err.Error())
		}
		plan = p
	} else {
		targets, err := parseTargets(args["targets"])
		if err != nil {
			return ErrorResult(err.Error())
		}
		quote := strings.ToUpper(stringArg(args, "quote"))
		if quote == "" {
			quote = "USDT"
		}
		tolerance := numberArg(args, "tolerance_pct")
		if tolerance <= 0 {
			tolerance = 5
		}
		minTrade := numberArg(args, "min_trade_usd")
		if minTrade <= 0 {
			minTrade = 10
		}
		plan = rebalance.Plan{
			Name: "ad-hoc", Provider: stringArg(args, "provider"), Account: stringArg(args, "account"),
			Quote: quote, Targets: targets, TolerancePct: tolerance, MinTradeUSD: minTrade,
		}
		if plan.Provider == "" {
			return ErrorResult("provider is required for ad-hoc checks")
		}
	}

	prop, err := evaluatePlanLive(ctx, t.cfg, plan)
	if err != nil {
		return ErrorResult(fmt.Sprintf("rebalance_check: %v", err)).WithError(err)
	}
	return UserResult(rebalance.FormatProposal(plan, prop))
}

// ====================== rebalance_plan_create ======================

type RebalancePlanCreateTool struct {
	cfg         *config.Config
	store       *rebalance.Store
	cronService *cron.CronService
}

func NewRebalancePlanCreateTool(cfg *config.Config, store *rebalance.Store, cronService *cron.CronService) *RebalancePlanCreateTool {
	return &RebalancePlanCreateTool{cfg: cfg, store: store, cronService: cronService}
}

func (t *RebalancePlanCreateTool) Name() string { return NameRebalancePlanCreate }

func (t *RebalancePlanCreateTool) Description() string {
	return "Create (or update, by same name) a scheduled rebalancing plan. mode=alert notifies with the proposal when drift exceeds tolerance; " +
		"mode=auto EXECUTES the proposed trades automatically and reports fills. Cron checks run deterministically — no LLM in the loop until something breaches."
}

func (t *RebalancePlanCreateTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":          map[string]any{"type": "string", "description": "Plan name (unique; same name updates)."},
			"provider":      map[string]any{"type": "string", "description": "Exchange provider (binance, okx, ...)."},
			"account":       map[string]any{"type": "string", "description": "Account name (empty = default)."},
			"quote":         map[string]any{"type": "string", "description": "Quote currency, default USDT."},
			"targets":       map[string]any{"type": "object", "description": `Asset → weight %%, e.g. {"BTC":50,"ETH":30,"USDT":20}. Must include the quote asset and sum to 100.`},
			"tolerance_pct": map[string]any{"type": "number", "description": "Per-asset drift threshold in points (default 5)."},
			"min_trade_usd": map[string]any{"type": "number", "description": "Suppress trades below this notional (default 10)."},
			"mode":          map[string]any{"type": "string", "enum": []string{"alert", "auto"}, "description": "alert (default) = notify proposal; auto = execute trades automatically."},
			"schedule":      map[string]any{"type": "string", "description": "Cron expression for drift checks, default '0 */6 * * *' (every 6h)."},
			"timezone":      map[string]any{"type": "string", "description": "Timezone for the schedule (default local)."},
		},
		"required": []string{"name", "provider", "targets"},
	}
}

func (t *RebalancePlanCreateTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	name := strings.TrimSpace(stringArg(args, "name"))
	providerID := stringArg(args, "provider")
	if name == "" || providerID == "" {
		return ErrorResult("name and provider are required")
	}
	targets, err := parseTargets(args["targets"])
	if err != nil {
		return ErrorResult(err.Error())
	}
	if err := rebalance.ValidateTargets(targets); err != nil {
		return ErrorResult(err.Error())
	}
	mode, err := rebalance.NormalizeMode(stringArg(args, "mode"))
	if err != nil {
		return ErrorResult(err.Error())
	}
	quote := strings.ToUpper(stringArg(args, "quote"))
	if quote == "" {
		quote = "USDT"
	}
	if _, ok := targets[quote]; !ok {
		return ErrorResult(fmt.Sprintf("targets must include the quote asset %s (its weight is the stable buffer)", quote))
	}
	tolerance := numberArg(args, "tolerance_pct")
	if tolerance <= 0 {
		tolerance = 5
	}
	minTrade := numberArg(args, "min_trade_usd")
	if minTrade <= 0 {
		minTrade = 10
	}
	schedule := stringArg(args, "schedule")
	if schedule == "" {
		schedule = "0 */6 * * *"
	}
	if t.cronService == nil {
		return ErrorResult("cron service unavailable — cannot schedule rebalance checks")
	}

	// Trade permission is required even for alert mode: the plan exists to trade eventually.
	if err := broker.CheckPermission(t.cfg, providerID, stringArg(args, "account"), config.ScopeTrade); err != nil {
		return ErrorResult(err.Error())
	}

	plan := rebalance.Plan{
		Name: name, Provider: providerID, Account: stringArg(args, "account"),
		Quote: quote, Targets: targets, TolerancePct: tolerance, MinTradeUSD: minTrade,
		Mode: mode, Schedule: schedule, Timezone: stringArg(args, "timezone"),
		Enabled:       true,
		NotifyChannel: ToolChannel(ctx),
		NotifyChatID:  ToolChatID(ctx),
	}

	// Same name = update (keep id + cron job, refresh schedule).
	if existing, ok := t.store.FindByName(name); ok {
		plan.ID = existing.ID
		plan.CronJobID = existing.CronJobID
		if existing.CronJobID != "" {
			t.cronService.RemoveJob(existing.CronJobID)
			plan.CronJobID = ""
		}
	}
	if err := t.store.Save(&plan); err != nil {
		return ErrorResult(fmt.Sprintf("save plan: %v", err))
	}

	job, err := t.cronService.AddJob(
		fmt.Sprintf("rebal:%d:%s", plan.ID, plan.Name),
		cron.CronSchedule{Kind: "cron", Expr: schedule, TZ: plan.Timezone},
		fmt.Sprintf("[REBALANCE] Check plan: %s plan_id=%d", plan.Name, plan.ID),
		false,
		plan.NotifyChannel,
		plan.NotifyChatID,
	)
	if err != nil {
		return ErrorResult(fmt.Sprintf("schedule cron job: %v", err))
	}
	job.Payload.NoHistory = true
	t.cronService.UpdateJob(job)
	plan.CronJobID = job.ID
	if err := t.store.Save(&plan); err != nil {
		return ErrorResult(fmt.Sprintf("update plan with cron id: %v", err))
	}

	modeNote := "จะแจ้ง proposal เมื่อ drift เกิน tolerance (alert mode)"
	if mode == rebalance.ModeAuto {
		modeNote = "⚠ AUTO mode: จะ EXECUTE เทรดอัตโนมัติเมื่อ drift เกิน tolerance แล้วรายงานผล"
	}
	return UserResult(fmt.Sprintf("✅ Rebalance plan #%d %q saved — %s/%s, %d assets, tolerance %.1f pts, check %s. %s",
		plan.ID, plan.Name, plan.Provider, plan.Account, len(targets), tolerance, schedule, modeNote))
}

// ====================== rebalance_plan_list ======================

type RebalancePlanListTool struct{ store *rebalance.Store }

func NewRebalancePlanListTool(store *rebalance.Store) *RebalancePlanListTool {
	return &RebalancePlanListTool{store: store}
}

func (t *RebalancePlanListTool) Name() string { return NameRebalancePlanList }

func (t *RebalancePlanListTool) Description() string { return "List saved rebalancing plans." }

func (t *RebalancePlanListTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *RebalancePlanListTool) Execute(_ context.Context, _ map[string]any) *ToolResult {
	plans := t.store.List()
	if len(plans) == 0 {
		return UserResult("No rebalancing plans yet. Use rebalance_plan_create to add one.")
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Rebalancing plans (%d):\n", len(plans)))
	for _, p := range plans {
		var targets []string
		for a, w := range p.Targets {
			targets = append(targets, fmt.Sprintf("%s %.0f%%", a, w))
		}
		sb.WriteString(fmt.Sprintf("  #%d %-14s %s/%s mode=%s tol=%.1f cron=%q targets: %s\n",
			p.ID, p.Name, p.Provider, p.Account, p.Mode, p.TolerancePct, p.Schedule, strings.Join(targets, ", ")))
	}
	return UserResult(sb.String())
}

// ====================== rebalance_plan_delete ======================

type RebalancePlanDeleteTool struct {
	store       *rebalance.Store
	cronService *cron.CronService
}

func NewRebalancePlanDeleteTool(store *rebalance.Store, cronService *cron.CronService) *RebalancePlanDeleteTool {
	return &RebalancePlanDeleteTool{store: store, cronService: cronService}
}

func (t *RebalancePlanDeleteTool) Name() string { return NameRebalancePlanDelete }

func (t *RebalancePlanDeleteTool) Description() string {
	return "Delete a rebalancing plan and its scheduled cron check."
}

func (t *RebalancePlanDeleteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan":    map[string]any{"type": "string", "description": "Plan name."},
			"plan_id": map[string]any{"type": "integer", "description": "Plan id."},
		},
	}
}

func (t *RebalancePlanDeleteTool) Execute(_ context.Context, args map[string]any) *ToolResult {
	plan, err := resolvePlanArg(t.store, args)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if plan.CronJobID != "" && t.cronService != nil {
		t.cronService.RemoveJob(plan.CronJobID)
	}
	if _, _, err := t.store.Delete(plan.ID); err != nil {
		return ErrorResult(fmt.Sprintf("delete plan: %v", err))
	}
	return UserResult(fmt.Sprintf("✅ Deleted rebalance plan #%d %q (cron check removed).", plan.ID, plan.Name))
}

// ====================== rebalance_execute ======================

type RebalanceExecuteTool struct {
	cfg   *config.Config
	store *rebalance.Store
}

func NewRebalanceExecuteTool(cfg *config.Config, store *rebalance.Store) *RebalanceExecuteTool {
	return &RebalanceExecuteTool{cfg: cfg, store: store}
}

func (t *RebalanceExecuteTool) Name() string { return NameRebalanceExecute }

func (t *RebalanceExecuteTool) Description() string {
	return "Execute a rebalancing plan NOW: re-evaluates live drift and places the proposed spot market orders (sells first). " +
		"Runs as dry-run showing the proposal unless confirm=true."
}

func (t *RebalanceExecuteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"plan":    map[string]any{"type": "string", "description": "Plan name."},
			"plan_id": map[string]any{"type": "integer", "description": "Plan id."},
			"confirm": map[string]any{"type": "boolean", "description": "Must be true to place orders; otherwise dry-run."},
		},
	}
}

func (t *RebalanceExecuteTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	plan, err := resolvePlanArg(t.store, args)
	if err != nil {
		return ErrorResult(err.Error())
	}
	confirm, _ := args["confirm"].(bool)

	// Gates (same money-path pipeline as futures/options tools).
	if err := broker.CheckPermission(t.cfg, plan.Provider, plan.Account, config.ScopeTrade); err != nil {
		return ErrorResult(err.Error())
	}
	if err := broker.GlobalLossTracker.CheckDailyLoss(t.cfg.TradingRisk.DailyLossLimitUSD); err != nil {
		return ErrorResult(err.Error())
	}
	if !broker.DefaultLimiter.Allow(plan.Provider) {
		return ErrorResult(fmt.Sprintf("rate limit exceeded for provider %q - try again in a minute", plan.Provider)).WithError(broker.ErrRateLimited)
	}

	prop, err := evaluatePlanLive(ctx, t.cfg, plan)
	if err != nil {
		return ErrorResult(fmt.Sprintf("rebalance_execute: %v", err)).WithError(err)
	}
	if !prop.NeedsRebalance {
		return UserResult(rebalance.FormatProposal(plan, prop))
	}
	if !confirm {
		return UserResult(rebalance.FormatProposal(plan, prop) + "\nDry-run — set confirm=true to place these orders.")
	}

	pp, err := rebalancePortfolioProviderFn(ctx, t.cfg, plan.Provider, plan.Account)
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}
	tp, ok := pp.(broker.TradingProvider)
	if !ok {
		return ErrorResult(fmt.Sprintf("provider %q does not support trading", plan.Provider))
	}
	results, execErr := rebalance.ExecuteProposal(ctx, tp, prop)
	out := rebalance.FormatResults(plan, results, execErr)
	if execErr != nil {
		return ErrorResult(out).WithError(execErr)
	}
	return UserResult(out)
}
