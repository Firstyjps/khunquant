package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	ccxt "github.com/ccxt/ccxt/go/v4"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// optionsProviderFn can be overridden in tests.
var optionsProviderFn = func(ctx context.Context, cfg *config.Config, providerID, account string) (broker.OptionsProvider, error) {
	if providerID == "" {
		providerID = "deribit"
	}
	p, err := broker.CreateProviderForAccount(providerID, account, cfg)
	if err != nil {
		return nil, fmt.Errorf("provider %q: %w", providerID, err)
	}
	op, ok := p.(broker.OptionsProvider)
	if !ok {
		return nil, fmt.Errorf("provider %q does not support options trading (currently: deribit)", providerID)
	}
	_ = ctx
	return op, nil
}

func optionsProvider(ctx context.Context, cfg *config.Config, providerID, account string) (broker.OptionsProvider, error) {
	return optionsProviderFn(ctx, cfg, providerID, account)
}

// resolveOptionSymbol maps user input to the CCXT unified symbol.
// Accepts either the unified form (contains "/") or the venue-native
// instrument id (e.g. "BTC-26DEC25-100000-C"), resolved via option markets.
func resolveOptionSymbol(ctx context.Context, op broker.OptionsProvider, symbol string) (string, ccxt.MarketInterface, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	markets, err := op.LoadOptionMarkets(ctx)
	if err != nil {
		return "", ccxt.MarketInterface{}, fmt.Errorf("load option markets: %w", err)
	}
	if m, ok := markets[symbol]; ok {
		return symbol, m, nil
	}
	for sym, m := range markets {
		if m.Id != nil && strings.EqualFold(*m.Id, symbol) {
			return sym, m, nil
		}
	}
	return "", ccxt.MarketInterface{}, fmt.Errorf("option %q not found on %s (use the instrument id, e.g. BTC-26DEC25-100000-C)", symbol, op.ID())
}

// infoFloat extracts a float from a ccxt raw-Info map, tolerating string values.
func infoFloat(info map[string]any, key string) float64 {
	switch v := info[key].(type) {
	case float64:
		return v
	case string:
		var f float64
		fmt.Sscanf(v, "%g", &f)
		return f
	}
	return 0
}

func infoString(info map[string]any, key string) string {
	if s, ok := info[key].(string); ok {
		return s
	}
	return ""
}

// ====================== options_chain ======================

type OptionsChainTool struct{ cfg *config.Config }

func NewOptionsChainTool(cfg *config.Config) *OptionsChainTool { return &OptionsChainTool{cfg: cfg} }

func (t *OptionsChainTool) Name() string { return NameOptionsChain }

func (t *OptionsChainTool) Description() string {
	return "List the options chain for an underlying (e.g. BTC, ETH) on Deribit: strikes, expiries, mark price, mark IV, open interest. " +
		"Values are read from the venue's raw book summary. Use option_quote for per-instrument greeks."
}

func (t *OptionsChainTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider":   map[string]any{"type": "string", "description": "Options venue (default deribit)."},
			"underlying": map[string]any{"type": "string", "description": "Underlying currency code, e.g. BTC or ETH."},
			"expiry":     map[string]any{"type": "string", "description": "Optional expiry filter, venue-native format e.g. 26DEC25."},
			"type":       map[string]any{"type": "string", "enum": []string{"call", "put"}, "description": "Optional option-type filter."},
			"strike_min": map[string]any{"type": "number", "description": "Optional minimum strike."},
			"strike_max": map[string]any{"type": "number", "description": "Optional maximum strike."},
			"limit":      map[string]any{"type": "integer", "description": "Max rows (default 30, max 60)."},
		},
		"required": []string{"underlying"},
	}
}

type chainRow struct {
	instrument string
	expiry     string
	strike     float64
	optType    string
	markPrice  float64
	markIVPct  float64
	openInt    float64
	bid        float64
	ask        float64
	underlying float64
}

// parseDeribitInstrument splits "BTC-26DEC25-100000-C" into expiry/strike/type.
func parseDeribitInstrument(name string) (expiry string, strike float64, optType string, ok bool) {
	parts := strings.Split(name, "-")
	if len(parts) < 4 {
		return "", 0, "", false
	}
	last := parts[len(parts)-1]
	switch last {
	case "C":
		optType = "call"
	case "P":
		optType = "put"
	default:
		return "", 0, "", false
	}
	fmt.Sscanf(parts[len(parts)-2], "%g", &strike)
	expiry = parts[1]
	return expiry, strike, optType, strike > 0
}

func (t *OptionsChainTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	providerID := stringArg(args, "provider")
	underlying := strings.ToUpper(stringArg(args, "underlying"))
	expiryFilter := strings.ToUpper(stringArg(args, "expiry"))
	typeFilter := strings.ToLower(stringArg(args, "type"))
	strikeMin := numberArg(args, "strike_min")
	strikeMax := numberArg(args, "strike_max")
	limit := int(numberArg(args, "limit"))
	if limit <= 0 {
		limit = 30
	}
	if limit > 60 {
		limit = 60
	}
	if underlying == "" {
		return ErrorResult("underlying is required (e.g. BTC)")
	}

	op, err := optionsProvider(ctx, t.cfg, providerID, stringArg(args, "account"))
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}

	chain, err := op.FetchOptionChain(ctx, underlying)
	if err != nil {
		return ErrorResult(fmt.Sprintf("options_chain: %v", err)).WithError(err)
	}
	if len(chain.Chains) == 0 {
		return UserResult(fmt.Sprintf("No options found for %s on %s.", underlying, op.ID()))
	}

	var rows []chainRow
	var underlyingPrice float64
	for _, opt := range chain.Chains {
		// ccxt-go's typed Option fields are mis-mapped upstream — read the
		// venue's raw book summary from Info instead.
		info := opt.Info
		if info == nil {
			continue
		}
		name := infoString(info, "instrument_name")
		expiry, strike, optType, ok := parseDeribitInstrument(name)
		if !ok {
			continue
		}
		if expiryFilter != "" && expiry != expiryFilter {
			continue
		}
		if typeFilter != "" && optType != typeFilter {
			continue
		}
		if strikeMin > 0 && strike < strikeMin {
			continue
		}
		if strikeMax > 0 && strike > strikeMax {
			continue
		}
		row := chainRow{
			instrument: name,
			expiry:     expiry,
			strike:     strike,
			optType:    optType,
			markPrice:  infoFloat(info, "mark_price"),
			markIVPct:  infoFloat(info, "mark_iv"),
			openInt:    infoFloat(info, "open_interest"),
			bid:        infoFloat(info, "bid_price"),
			ask:        infoFloat(info, "ask_price"),
			underlying: infoFloat(info, "underlying_price"),
		}
		if row.underlying > 0 {
			underlyingPrice = row.underlying
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return UserResult(fmt.Sprintf("No %s options matched the filters on %s.", underlying, op.ID()))
	}

	// No explicit strike window: focus around spot (±25%) to keep output usable.
	clamped := false
	if strikeMin == 0 && strikeMax == 0 && underlyingPrice > 0 {
		var focused []chainRow
		for _, r := range rows {
			if r.strike >= underlyingPrice*0.75 && r.strike <= underlyingPrice*1.25 {
				focused = append(focused, r)
			}
		}
		if len(focused) > 0 && len(focused) < len(rows) {
			rows = focused
			clamped = true
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].expiry != rows[j].expiry {
			return rows[i].expiry < rows[j].expiry
		}
		if rows[i].strike != rows[j].strike {
			return rows[i].strike < rows[j].strike
		}
		return rows[i].optType < rows[j].optType
	})

	total := len(rows)
	if len(rows) > limit {
		rows = rows[:limit]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Options chain: %s on %s", underlying, op.ID()))
	if underlyingPrice > 0 {
		sb.WriteString(fmt.Sprintf(" (underlying ≈ %.2f)", underlyingPrice))
	}
	sb.WriteString(fmt.Sprintf(" — showing %d/%d\n", len(rows), total))
	if clamped {
		sb.WriteString("(no strike filter given — showing strikes within ±25% of spot; pass strike_min/strike_max for more)\n")
	}
	sb.WriteString(fmt.Sprintf("%-26s %6s %10s %5s %12s %8s %10s %10s %10s\n",
		"Instrument", "Expiry", "Strike", "Type", "Mark", "IV%", "Bid", "Ask", "OI"))
	sb.WriteString(strings.Repeat("-", 104) + "\n")
	for _, r := range rows {
		sb.WriteString(fmt.Sprintf("%-26s %6s %10.0f %5s %12.6f %8.1f %10.6f %10.6f %10.1f\n",
			r.instrument, r.expiry, r.strike, r.optType, r.markPrice, r.markIVPct, r.bid, r.ask, r.openInt))
	}
	sb.WriteString("\nMark/bid/ask are premiums in the underlying currency (Deribit convention). Use option_quote <instrument> for greeks.")
	return UserResult(sb.String())
}

// ====================== option_quote ======================

type OptionQuoteTool struct{ cfg *config.Config }

func NewOptionQuoteTool(cfg *config.Config) *OptionQuoteTool { return &OptionQuoteTool{cfg: cfg} }

func (t *OptionQuoteTool) Name() string { return NameOptionQuote }

func (t *OptionQuoteTool) Description() string {
	return "Fetch a single option's greeks (delta/gamma/theta/vega/rho), implied volatility, mark price, and underlying price. " +
		"Accepts the venue instrument id (e.g. BTC-26DEC25-100000-C) or CCXT symbol."
}

func (t *OptionQuoteTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider": map[string]any{"type": "string", "description": "Options venue (default deribit)."},
			"symbol":   map[string]any{"type": "string", "description": "Option instrument, e.g. BTC-26DEC25-100000-C."},
		},
		"required": []string{"symbol"},
	}
}

func (t *OptionQuoteTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	op, err := optionsProvider(ctx, t.cfg, stringArg(args, "provider"), stringArg(args, "account"))
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}
	symbol, market, err := resolveOptionSymbol(ctx, op, stringArg(args, "symbol"))
	if err != nil {
		return ErrorResult(err.Error())
	}

	g, err := op.FetchGreeks(ctx, symbol)
	if err != nil {
		return ErrorResult(fmt.Sprintf("option_quote: %v", err)).WithError(err)
	}

	f := func(p *float64) string {
		if p == nil {
			return "-"
		}
		return fmt.Sprintf("%.6g", *p)
	}

	var sb strings.Builder
	instrument := symbol
	if market.Id != nil {
		instrument = *market.Id
	}
	sb.WriteString(fmt.Sprintf("Option quote: %s (%s)\n\n", instrument, op.ID()))
	sb.WriteString(fmt.Sprintf("  Underlying price: %s\n", f(g.UnderlyingPrice)))
	sb.WriteString(fmt.Sprintf("  Mark price:       %s (premium, underlying ccy)\n", f(g.MarkPrice)))
	sb.WriteString(fmt.Sprintf("  Bid / Ask:        %s / %s\n", f(g.BidPrice), f(g.AskPrice)))
	sb.WriteString(fmt.Sprintf("  Mark IV:          %s\n", f(g.MarkImpliedVolatility)))
	sb.WriteString(fmt.Sprintf("  Bid IV / Ask IV:  %s / %s\n\n", f(g.BidImpliedVolatility), f(g.AskImpliedVolatility)))
	sb.WriteString(fmt.Sprintf("  Delta: %s   Gamma: %s\n", f(g.Delta), f(g.Gamma)))
	sb.WriteString(fmt.Sprintf("  Theta: %s   Vega:  %s   Rho: %s\n", f(g.Theta), f(g.Vega), f(g.Rho)))
	if market.ContractSize != nil {
		sb.WriteString(fmt.Sprintf("\n  Contract size: %g", *market.ContractSize))
	}
	if market.Limits.Amount.Min != nil {
		sb.WriteString(fmt.Sprintf("   Min amount: %g", *market.Limits.Amount.Min))
	}
	return UserResult(sb.String())
}

// ====================== option_positions ======================

type OptionPositionsTool struct{ cfg *config.Config }

func NewOptionPositionsTool(cfg *config.Config) *OptionPositionsTool {
	return &OptionPositionsTool{cfg: cfg}
}

func (t *OptionPositionsTool) Name() string { return NameOptionPositions }

func (t *OptionPositionsTool) Description() string {
	return "List open option positions with size, entry/mark price, PnL, and per-position greeks; aggregates net delta per settlement currency."
}

func (t *OptionPositionsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider": map[string]any{"type": "string", "description": "Options venue (default deribit)."},
			"account":  map[string]any{"type": "string", "description": "Account name (empty = default)."},
			"currency": map[string]any{"type": "string", "description": "Settlement currency filter, e.g. BTC or ETH (empty = all)."},
		},
	}
}

func (t *OptionPositionsTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	op, err := optionsProvider(ctx, t.cfg, stringArg(args, "provider"), stringArg(args, "account"))
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}
	positions, err := op.FetchOptionPositions(ctx, stringArg(args, "currency"))
	if err != nil {
		return ErrorResult(fmt.Sprintf("option_positions: %v", err)).WithError(err)
	}
	if len(positions) == 0 {
		return UserResult(fmt.Sprintf("No open option positions on %s.", op.ID()))
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Open option positions on %s (%d):\n\n", op.ID(), len(positions)))
	sb.WriteString(fmt.Sprintf("%-30s %6s %10s %12s %12s %12s %10s\n",
		"Instrument", "Side", "Size", "Avg Price", "Mark", "uPnL", "Delta"))
	sb.WriteString(strings.Repeat("-", 98) + "\n")

	netDelta := map[string]float64{}
	for _, p := range positions {
		symbol := ""
		if p.Symbol != nil {
			symbol = *p.Symbol
		}
		instrument := symbol
		if name := infoString(p.Info, "instrument_name"); name != "" {
			instrument = name
		}
		side := ""
		if p.Side != nil {
			side = *p.Side
		}
		size := derefOr(p.Contracts, infoFloat(p.Info, "size"))
		delta := infoFloat(p.Info, "delta")
		cur := strings.SplitN(instrument, "-", 2)[0]
		netDelta[cur] += delta

		sb.WriteString(fmt.Sprintf("%-30s %6s %10.4g %12.6g %12.6g %12.6g %10.4f\n",
			instrument, side, size,
			derefOr(p.EntryPrice, infoFloat(p.Info, "average_price")),
			derefOr(p.MarkPrice, infoFloat(p.Info, "mark_price")),
			derefOr(p.UnrealizedPnl, infoFloat(p.Info, "total_profit_loss")),
			delta))
	}
	sb.WriteString("\nNet delta by currency (underlying units):\n")
	for cur, d := range netDelta {
		sb.WriteString(fmt.Sprintf("  %s: %+.4f (hedge with %s perp: %s ≈ %.4f %s)\n",
			cur, d, cur, hedgeSideForDelta(d), absFloat(d), cur))
	}
	return UserResult(sb.String())
}

func derefOr(p *float64, fallback float64) float64 {
	if p != nil && *p != 0 {
		return *p
	}
	return fallback
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func hedgeSideForDelta(d float64) string {
	if d > 0 {
		return "short"
	}
	return "long"
}

// ====================== option_order ======================

type OptionOrderTool struct{ cfg *config.Config }

func NewOptionOrderTool(cfg *config.Config) *OptionOrderTool { return &OptionOrderTool{cfg: cfg} }

func (t *OptionOrderTool) Name() string { return NameOptionOrder }

func (t *OptionOrderTool) Description() string {
	return "Place an option order on Deribit (buy/sell, limit/market). Amount is in CONTRACTS; premium prices are quoted in the underlying currency. " +
		"Runs as dry-run unless confirm=true. SELLING options creates short-premium exposure with margin requirements."
}

func (t *OptionOrderTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"provider":    map[string]any{"type": "string", "description": "Options venue (default deribit)."},
			"account":     map[string]any{"type": "string", "description": "Account name (empty = default)."},
			"symbol":      map[string]any{"type": "string", "description": "Option instrument, e.g. BTC-26DEC25-100000-C."},
			"side":        map[string]any{"type": "string", "enum": []string{"buy", "sell"}, "description": "buy = long option, sell = short/write option."},
			"order_type":  map[string]any{"type": "string", "enum": []string{"limit", "market"}, "description": "Default limit."},
			"amount":      map[string]any{"type": "number", "description": "Number of contracts (Deribit BTC options: 1 contract = 1 BTC notional; min 0.1)."},
			"price":       map[string]any{"type": "number", "description": "Limit premium per contract in underlying ccy (required for limit orders)."},
			"reduce_only": map[string]any{"type": "boolean", "description": "Only reduce an existing position."},
			"confirm":     map[string]any{"type": "boolean", "description": "Must be true to place the order; otherwise dry-run."},
		},
		"required": []string{"symbol", "side", "amount"},
	}
}

func (t *OptionOrderTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	providerID := stringArg(args, "provider")
	if providerID == "" {
		providerID = "deribit"
	}
	account := stringArg(args, "account")
	rawSymbol := stringArg(args, "symbol")
	side := strings.ToLower(stringArg(args, "side"))
	orderType := strings.ToLower(stringArg(args, "order_type"))
	if orderType == "" {
		orderType = "limit"
	}
	amount := numberArg(args, "amount")
	reduceOnly, _ := args["reduce_only"].(bool)
	confirm, _ := args["confirm"].(bool)

	var price *float64
	if v := numberArg(args, "price"); v > 0 {
		price = &v
	}

	// Step 1: basic validation
	if rawSymbol == "" {
		return ErrorResult("symbol is required (e.g. BTC-26DEC25-100000-C)")
	}
	if side != "buy" && side != "sell" {
		return ErrorResult("side must be buy or sell")
	}
	if amount <= 0 {
		return ErrorResult("amount (contracts) must be > 0")
	}
	if orderType == "limit" && price == nil {
		return ErrorResult("price is required for limit option orders")
	}

	// Step 2: leverage/derivatives opt-in (options are margined derivatives)
	if err := broker.CheckLeverage(t.cfg, "place option order"); err != nil {
		return ErrorResult(err.Error())
	}

	// Step 3: permission
	if err := broker.CheckPermission(t.cfg, providerID, account, config.ScopeTrade); err != nil {
		return ErrorResult(err.Error())
	}

	// Step 4: daily loss limit and rate limit
	if err := broker.GlobalLossTracker.CheckDailyLoss(t.cfg.TradingRisk.DailyLossLimitUSD); err != nil {
		return ErrorResult(err.Error())
	}
	if !broker.DefaultLimiter.Allow(providerID) {
		return ErrorResult(fmt.Sprintf("rate limit exceeded for provider %q - try again in a minute", providerID)).WithError(broker.ErrRateLimited)
	}

	// Step 5 (dry-run): all gates above validated; no network calls yet.
	if !confirm {
		riskNote := ""
		if side == "sell" && !reduceOnly {
			riskNote = " ⚠ SELLING an option: short-premium position with margin requirements and potentially large downside."
		}
		premiumNote := ""
		if price != nil {
			premiumNote = fmt.Sprintf(" @ %.6g (est. premium %.6g in underlying ccy)", *price, *price*amount)
		}
		return UserResult(fmt.Sprintf("Dry-run: would %s %s %.4g contracts of %s on %s%s.%s Set confirm=true to execute.",
			side, orderType, amount, strings.ToUpper(rawSymbol), providerID, premiumNote, riskNote))
	}

	// Step 6: acquire provider, resolve + validate the option market
	op, err := optionsProvider(ctx, t.cfg, providerID, account)
	if err != nil {
		return ErrorResult(err.Error()).WithError(err)
	}
	symbol, market, err := resolveOptionSymbol(ctx, op, rawSymbol)
	if err != nil {
		return ErrorResult(err.Error())
	}
	if market.Active != nil && !*market.Active {
		return ErrorResult(fmt.Sprintf("option market %s is not active", symbol))
	}
	if market.Limits.Amount.Min != nil && amount < *market.Limits.Amount.Min {
		return ErrorResult(fmt.Sprintf("amount %.4g is below the market minimum %.4g contracts", amount, *market.Limits.Amount.Min))
	}

	order, err := op.CreateOptionOrder(ctx, broker.OptionOrderRequest{
		Symbol:     symbol,
		OrderType:  orderType,
		Side:       side,
		Amount:     amount,
		Price:      price,
		ReduceOnly: reduceOnly,
	})
	if err != nil {
		return ErrorResult(fmt.Sprintf("option_order: %v", err)).WithError(err)
	}

	id := ""
	if order.Id != nil {
		id = *order.Id
	}
	status := ""
	if order.Status != nil {
		status = *order.Status
	}
	return UserResult(fmt.Sprintf("✅ Option order placed on %s: %s %s %.4g %s%s — id=%s status=%s",
		op.ID(), side, orderType, amount, symbol, priceText(price), id, status))
}
