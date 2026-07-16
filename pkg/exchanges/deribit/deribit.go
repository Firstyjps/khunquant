// Package deribit implements the broker provider hierarchy for Deribit —
// the options + perpetuals venue. Public market data (chains, greeks, tickers)
// works without credentials; trading and balances require an API key.
package deribit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	ccxt "github.com/ccxt/ccxt/go/v4"

	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/logger"
	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// Name is the canonical provider identifier.
const Name = "deribit"

// optionCurrencies are the settlement currencies scanned when no explicit
// currency filter is given (Deribit lists options per settlement currency).
var optionCurrencies = []string{"BTC", "ETH"}

// Compile-time interface assertions.
var (
	_ broker.PortfolioProvider  = (*DeribitAdapter)(nil)
	_ broker.MarketDataProvider = (*DeribitAdapter)(nil)
	_ broker.TradingProvider    = (*DeribitAdapter)(nil)
	_ broker.FuturesProvider    = (*DeribitAdapter)(nil)
	_ broker.OptionsProvider    = (*DeribitAdapter)(nil)
)

// DeribitAdapter implements broker.PortfolioProvider, broker.MarketDataProvider,
// broker.TradingProvider, broker.FuturesProvider, and broker.OptionsProvider.
type DeribitAdapter struct {
	auth    *ccxt.Deribit // credentialed instance (private endpoints)
	public  *ccxt.Deribit // credential-free instance (market data)
	hasAuth bool
}

func catchPanic(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(compactCCXTMessage(fmt.Sprint(r)))
		}
	}()
	if err := fn(); err != nil {
		return compactCCXTError(err)
	}
	return nil
}

func compactCCXTError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(compactCCXTMessage(err.Error()))
}

func compactCCXTMessage(msg string) string {
	for _, marker := range []string{"\nStack trace:", "Stack trace:"} {
		if idx := strings.Index(msg, marker); idx >= 0 {
			msg = msg[:idx]
			break
		}
	}
	return strings.TrimSpace(msg)
}

func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func newAdapter(creds config.ExchangeAccount, testnet bool) (*DeribitAdapter, error) {
	hasAuth := creds.APIKey.String() != "" && creds.Secret.String() != ""

	var ccxtCreds map[string]interface{}
	if hasAuth {
		ccxtCreds = map[string]interface{}{
			"apiKey": creds.APIKey.String(),
			"secret": creds.Secret.String(),
		}
		logger.RegisterSecret(creds.APIKey.String())
		logger.RegisterSecret(creds.Secret.String())
	}

	auth := ccxt.NewDeribit(ccxtCreds)
	public := ccxt.NewDeribit(nil)
	if testnet {
		auth.Core.SetSandboxMode(true)
		public.Core.SetSandboxMode(true)
	}

	if creds.Proxy != "" {
		isHTTPS := strings.HasPrefix(strings.ToLower(creds.Proxy), "https")
		for _, ex := range []*ccxt.Deribit{auth, public} {
			if isHTTPS {
				ex.HttpsProxy = creds.Proxy
			} else {
				ex.HttpProxy = creds.Proxy
			}
			ex.UpdateProxySettings()
		}
	}

	return &DeribitAdapter{auth: auth, public: public, hasAuth: hasAuth}, nil
}

func (a *DeribitAdapter) requireAuth() error {
	if !a.hasAuth {
		return fmt.Errorf("deribit: this action requires API credentials — add exchanges.deribit.accounts to config")
	}
	return nil
}

// --- broker.Provider ---

func (a *DeribitAdapter) ID() string { return Name }

func (a *DeribitAdapter) Category() broker.AssetCategory { return broker.CategoryCrypto }

func (a *DeribitAdapter) GetMarketStatus(_ context.Context, symbol string) (broker.MarketStatus, error) {
	var t ccxt.Ticker
	err := catchPanic(func() error {
		var e error
		t, e = a.public.FetchTicker(symbol)
		return e
	})
	if err != nil {
		return broker.MarketUnknown, fmt.Errorf("deribit: GetMarketStatus: %w", err)
	}
	if t.Last != nil || t.Close != nil {
		return broker.MarketOpen, nil
	}
	return broker.MarketUnknown, nil
}

// --- broker.PortfolioProvider ---

func (a *DeribitAdapter) GetBalances(_ context.Context) ([]broker.Balance, error) {
	if err := a.requireAuth(); err != nil {
		return nil, err
	}
	var bal ccxt.Balances
	err := catchPanic(func() error {
		var e error
		bal, e = a.auth.FetchBalance()
		return e
	})
	if err != nil {
		return nil, fmt.Errorf("deribit: FetchBalance: %w", err)
	}
	var out []broker.Balance
	for currency, b := range bal.Balances {
		free := derefFloat(b.Free)
		used := derefFloat(b.Used)
		if free == 0 && used == 0 {
			continue
		}
		out = append(out, broker.Balance{Asset: currency, Free: free, Locked: used})
	}
	return out, nil
}

func (a *DeribitAdapter) GetWalletBalances(ctx context.Context, walletType string) ([]broker.WalletBalance, error) {
	bals, err := a.GetBalances(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]broker.WalletBalance, len(bals))
	for i, b := range bals {
		out[i] = broker.WalletBalance{Balance: b, WalletType: "main"}
	}
	return out, nil
}

func (a *DeribitAdapter) SupportedWalletTypes() []string { return []string{"main"} }

// FetchPrice returns the price of asset in quote terms. Deribit's spot markets
// are thin; fall back to the linear (USDC) then inverse (USD) perp mark.
func (a *DeribitAdapter) FetchPrice(ctx context.Context, asset, quote string) (float64, error) {
	asset = strings.ToUpper(strings.TrimSpace(asset))
	quote = strings.ToUpper(strings.TrimSpace(quote))
	if asset == quote || asset == "" {
		return 0, nil
	}
	candidates := []string{
		asset + "/" + quote,
		asset + "/USDC:USDC",
		asset + "/USD:" + asset,
	}
	var lastErr error
	for _, sym := range candidates {
		var t ccxt.Ticker
		err := catchPanic(func() error {
			var e error
			t, e = a.public.FetchTicker(sym)
			return e
		})
		if err != nil {
			lastErr = err
			continue
		}
		if t.Last != nil && *t.Last > 0 {
			return *t.Last, nil
		}
		if t.Close != nil && *t.Close > 0 {
			return *t.Close, nil
		}
	}
	return 0, fmt.Errorf("deribit: no price for %s/%s: %v", asset, quote, lastErr)
}

// --- broker.MarketDataProvider ---

func (a *DeribitAdapter) FetchTicker(_ context.Context, symbol string) (t ccxt.Ticker, err error) {
	err = catchPanic(func() error { t, err = a.public.FetchTicker(symbol); return err })
	return
}

func (a *DeribitAdapter) FetchTickers(_ context.Context, symbols []string) (map[string]ccxt.Ticker, error) {
	var tickers ccxt.Tickers
	err := catchPanic(func() error {
		var e error
		if len(symbols) == 0 {
			tickers, e = a.public.FetchTickers()
		} else {
			tickers, e = a.public.FetchTickers(ccxt.WithFetchTickersSymbols(symbols))
		}
		return e
	})
	if err != nil {
		return nil, err
	}
	return tickers.Tickers, nil
}

func (a *DeribitAdapter) FetchOHLCV(_ context.Context, symbol, timeframe string, since *int64, limit int) (candles []ccxt.OHLCV, err error) {
	opts := []ccxt.FetchOHLCVOptions{}
	if timeframe != "" {
		opts = append(opts, ccxt.WithFetchOHLCVTimeframe(timeframe))
	}
	if since != nil {
		opts = append(opts, ccxt.WithFetchOHLCVSince(*since))
	}
	if limit > 0 {
		opts = append(opts, ccxt.WithFetchOHLCVLimit(int64(limit)))
	}
	err = catchPanic(func() error { candles, err = a.public.FetchOHLCV(symbol, opts...); return err })
	return
}

func (a *DeribitAdapter) FetchOrderBook(_ context.Context, symbol string, depth int) (ob ccxt.OrderBook, err error) {
	opts := []ccxt.FetchOrderBookOptions{}
	if depth > 0 {
		opts = append(opts, ccxt.WithFetchOrderBookLimit(int64(depth)))
	}
	err = catchPanic(func() error { ob, err = a.public.FetchOrderBook(symbol, opts...); return err })
	return
}

func (a *DeribitAdapter) LoadMarkets(_ context.Context) (markets map[string]ccxt.MarketInterface, err error) {
	err = catchPanic(func() error { markets, err = a.public.LoadMarkets(); return err })
	return
}

// --- broker.TradingProvider ---

func (a *DeribitAdapter) CreateOrder(_ context.Context, symbol, orderType, side string, amount float64, price *float64, params map[string]interface{}) (o ccxt.Order, err error) {
	if err := a.requireAuth(); err != nil {
		return o, err
	}
	opts := []ccxt.CreateOrderOptions{}
	if price != nil {
		opts = append(opts, ccxt.WithCreateOrderPrice(*price))
	}
	if len(params) > 0 {
		opts = append(opts, ccxt.WithCreateOrderParams(params))
	}
	err = catchPanic(func() error {
		o, err = a.auth.CreateOrder(symbol, orderType, side, amount, opts...)
		return err
	})
	return
}

func (a *DeribitAdapter) CancelOrder(_ context.Context, id, symbol string) (o ccxt.Order, err error) {
	if err := a.requireAuth(); err != nil {
		return o, err
	}
	err = catchPanic(func() error {
		var e error
		o, e = a.auth.CancelOrder(id, ccxt.WithCancelOrderSymbol(symbol))
		return e
	})
	return
}

func (a *DeribitAdapter) FetchOrder(_ context.Context, id, symbol string) (o ccxt.Order, err error) {
	if err := a.requireAuth(); err != nil {
		return o, err
	}
	err = catchPanic(func() error { o, err = a.auth.FetchOrder(id, ccxt.WithFetchOrderSymbol(symbol)); return err })
	return
}

func (a *DeribitAdapter) FetchOpenOrders(_ context.Context, symbol string) (orders []ccxt.Order, err error) {
	if err := a.requireAuth(); err != nil {
		return nil, err
	}
	err = catchPanic(func() error {
		if symbol != "" {
			orders, err = a.auth.FetchOpenOrders(ccxt.WithFetchOpenOrdersSymbol(symbol))
		} else {
			orders, err = a.auth.FetchOpenOrders()
		}
		return err
	})
	return
}

func (a *DeribitAdapter) FetchClosedOrders(_ context.Context, symbol string, since *int64, limit int) (orders []ccxt.Order, err error) {
	if err := a.requireAuth(); err != nil {
		return nil, err
	}
	opts := []ccxt.FetchClosedOrdersOptions{}
	if symbol != "" {
		opts = append(opts, ccxt.WithFetchClosedOrdersSymbol(symbol))
	}
	if since != nil {
		opts = append(opts, ccxt.WithFetchClosedOrdersSince(*since))
	}
	if limit > 0 {
		opts = append(opts, ccxt.WithFetchClosedOrdersLimit(int64(limit)))
	}
	err = catchPanic(func() error { orders, err = a.auth.FetchClosedOrders(opts...); return err })
	return
}

func (a *DeribitAdapter) FetchMyTrades(_ context.Context, symbol string, since *int64, limit int) (trades []ccxt.Trade, err error) {
	if err := a.requireAuth(); err != nil {
		return nil, err
	}
	opts := []ccxt.FetchMyTradesOptions{}
	if symbol != "" {
		opts = append(opts, ccxt.WithFetchMyTradesSymbol(symbol))
	}
	if since != nil {
		opts = append(opts, ccxt.WithFetchMyTradesSince(*since))
	}
	if limit > 0 {
		opts = append(opts, ccxt.WithFetchMyTradesLimit(int64(limit)))
	}
	err = catchPanic(func() error { trades, err = a.auth.FetchMyTrades(opts...); return err })
	return
}

// --- broker.FuturesProvider ---

// SetFuturesLeverage is not supported: Deribit margins at the account/portfolio
// level and has no per-symbol leverage endpoint.
func (a *DeribitAdapter) SetFuturesLeverage(_ context.Context, symbol string, leverage int64, marginMode, positionSide string) (map[string]interface{}, error) {
	return nil, fmt.Errorf("deribit: per-symbol leverage is not supported (account-level margin); order size determines exposure")
}

func (a *DeribitAdapter) CreateFuturesOrder(_ context.Context, req broker.FuturesOrderRequest) (o ccxt.Order, err error) {
	if err := a.requireAuth(); err != nil {
		return o, err
	}
	opts := []ccxt.CreateOrderOptions{}
	if req.Price != nil {
		opts = append(opts, ccxt.WithCreateOrderPrice(*req.Price))
	}
	params := map[string]interface{}{}
	for k, v := range req.Params {
		params[k] = v
	}
	if req.ReduceOnly {
		params["reduceOnly"] = true
	}
	if len(params) > 0 {
		opts = append(opts, ccxt.WithCreateOrderParams(params))
	}
	err = catchPanic(func() error {
		o, err = a.auth.CreateOrder(req.Symbol, req.OrderType, req.Side, req.Amount, opts...)
		return err
	})
	return
}

func (a *DeribitAdapter) FetchFuturesOrder(ctx context.Context, id, symbol string) (ccxt.Order, error) {
	return a.FetchOrder(ctx, id, symbol)
}

func (a *DeribitAdapter) FetchFuturesOpenOrders(ctx context.Context, symbol string) ([]ccxt.Order, error) {
	return a.FetchOpenOrders(ctx, symbol)
}

func (a *DeribitAdapter) FetchFuturesPositions(_ context.Context, symbols []string) (positions []ccxt.Position, err error) {
	if err := a.requireAuth(); err != nil {
		return nil, err
	}
	opts := []ccxt.FetchPositionsOptions{}
	if len(symbols) > 0 {
		opts = append(opts, ccxt.WithFetchPositionsSymbols(symbols))
	}
	err = catchPanic(func() error { positions, err = a.auth.FetchPositions(opts...); return err })
	return
}

func (a *DeribitAdapter) FetchFuturesFundingRate(_ context.Context, symbol string) (rate ccxt.FundingRate, err error) {
	err = catchPanic(func() error { rate, err = a.public.FetchFundingRate(symbol); return err })
	return
}

func (a *DeribitAdapter) FetchFuturesFundingRates(ctx context.Context, symbols []string) (map[string]ccxt.FundingRate, error) {
	out := map[string]ccxt.FundingRate{}
	for _, sym := range symbols {
		rate, err := a.FetchFuturesFundingRate(ctx, sym)
		if err != nil {
			return out, fmt.Errorf("deribit: funding rate %s: %w", sym, err)
		}
		out[sym] = rate
	}
	return out, nil
}

func (a *DeribitAdapter) FetchFuturesFundingHistory(_ context.Context, symbol string, since *int64, limit int) (history []ccxt.FundingHistory, err error) {
	if err := a.requireAuth(); err != nil {
		return nil, err
	}
	opts := []ccxt.FetchFundingHistoryOptions{}
	if symbol != "" {
		opts = append(opts, ccxt.WithFetchFundingHistorySymbol(symbol))
	}
	if since != nil {
		opts = append(opts, ccxt.WithFetchFundingHistorySince(*since))
	}
	if limit > 0 {
		opts = append(opts, ccxt.WithFetchFundingHistoryLimit(int64(limit)))
	}
	err = catchPanic(func() error { history, err = a.auth.FetchFundingHistory(opts...); return err })
	return
}

func (a *DeribitAdapter) FetchPublicFundingRateHistory(_ context.Context, symbol string, since *int64, limit int) (history []ccxt.FundingRateHistory, err error) {
	opts := []ccxt.FetchFundingRateHistoryOptions{
		ccxt.WithFetchFundingRateHistorySymbol(symbol),
	}
	if since != nil {
		opts = append(opts, ccxt.WithFetchFundingRateHistorySince(*since))
	}
	if limit > 0 {
		opts = append(opts, ccxt.WithFetchFundingRateHistoryLimit(int64(limit)))
	}
	err = catchPanic(func() error {
		history, err = a.public.FetchFundingRateHistory(opts...)
		return err
	})
	if err != nil {
		return history, fmt.Errorf("deribit: funding rate history %s: %w", symbol, err)
	}
	return history, nil
}

func (a *DeribitAdapter) LoadFuturesMarkets(ctx context.Context) (map[string]ccxt.MarketInterface, error) {
	all, err := a.LoadMarkets(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]ccxt.MarketInterface{}
	for sym, m := range all {
		if m.Swap != nil && *m.Swap {
			out[sym] = m
		}
	}
	return out, nil
}

func (a *DeribitAdapter) FetchFuturesMarkPrice(ctx context.Context, symbol string) (float64, error) {
	t, err := a.FetchTicker(ctx, symbol)
	if err != nil {
		return 0, err
	}
	if mp, ok := t.Info["mark_price"].(float64); ok && mp > 0 {
		return mp, nil
	}
	if t.Last != nil && *t.Last > 0 {
		return *t.Last, nil
	}
	return 0, fmt.Errorf("deribit: no mark price for %s", symbol)
}

func (a *DeribitAdapter) CancelFuturesOrder(ctx context.Context, id, symbol string) (ccxt.Order, error) {
	return a.CancelOrder(ctx, id, symbol)
}

func (a *DeribitAdapter) CancelAllFuturesOrders(_ context.Context, symbol string) (orders []ccxt.Order, err error) {
	if err := a.requireAuth(); err != nil {
		return nil, err
	}
	err = catchPanic(func() error {
		var e error
		var res interface{}
		if symbol != "" {
			res, e = a.auth.CancelAllOrders(ccxt.WithCancelAllOrdersSymbol(symbol))
		} else {
			res, e = a.auth.CancelAllOrders()
		}
		if e != nil {
			return e
		}
		if list, ok := res.([]ccxt.Order); ok {
			orders = list
		}
		return nil
	})
	return
}

// --- broker.OptionsProvider ---

func (a *DeribitAdapter) LoadOptionMarkets(ctx context.Context) (map[string]ccxt.MarketInterface, error) {
	all, err := a.LoadMarkets(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]ccxt.MarketInterface{}
	for sym, m := range all {
		if m.Option != nil && *m.Option {
			out[sym] = m
		}
	}
	return out, nil
}

func (a *DeribitAdapter) FetchOptionChain(_ context.Context, code string) (chain ccxt.OptionChain, err error) {
	err = catchPanic(func() error {
		chain, err = a.public.FetchOptionChain(strings.ToUpper(strings.TrimSpace(code)))
		return err
	})
	return
}

func (a *DeribitAdapter) FetchOption(_ context.Context, symbol string) (opt ccxt.Option, err error) {
	err = catchPanic(func() error { opt, err = a.public.FetchOption(symbol); return err })
	return
}

func (a *DeribitAdapter) FetchGreeks(_ context.Context, symbol string) (g ccxt.Greeks, err error) {
	err = catchPanic(func() error { g, err = a.public.FetchGreeks(symbol); return err })
	return
}

func (a *DeribitAdapter) FetchOptionPositions(_ context.Context, currency string) ([]ccxt.Position, error) {
	if err := a.requireAuth(); err != nil {
		return nil, err
	}
	currencies := optionCurrencies
	if c := strings.ToUpper(strings.TrimSpace(currency)); c != "" {
		currencies = []string{c}
	}
	var out []ccxt.Position
	var errs []error
	for _, cur := range currencies {
		var positions []ccxt.Position
		err := catchPanic(func() error {
			var e error
			positions, e = a.auth.FetchPositions(ccxt.WithFetchPositionsParams(map[string]any{
				"currency": cur,
				"kind":     "option",
			}))
			return e
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", cur, err))
			continue
		}
		out = append(out, positions...)
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("deribit: option positions: %w", errors.Join(errs...))
	}
	return out, nil
}

func (a *DeribitAdapter) CreateOptionOrder(_ context.Context, req broker.OptionOrderRequest) (o ccxt.Order, err error) {
	if err := a.requireAuth(); err != nil {
		return o, err
	}
	opts := []ccxt.CreateOrderOptions{}
	if req.Price != nil {
		opts = append(opts, ccxt.WithCreateOrderPrice(*req.Price))
	}
	params := map[string]interface{}{}
	for k, v := range req.Params {
		params[k] = v
	}
	if req.ReduceOnly {
		params["reduceOnly"] = true
	}
	if len(params) > 0 {
		opts = append(opts, ccxt.WithCreateOrderParams(params))
	}
	err = catchPanic(func() error {
		o, err = a.auth.CreateOrder(req.Symbol, req.OrderType, req.Side, req.Amount, opts...)
		return err
	})
	return
}

func (a *DeribitAdapter) CancelOptionOrder(ctx context.Context, id, symbol string) (ccxt.Order, error) {
	return a.CancelOrder(ctx, id, symbol)
}

func (a *DeribitAdapter) FetchOptionOpenOrders(ctx context.Context, symbol string) ([]ccxt.Order, error) {
	return a.FetchOpenOrders(ctx, symbol)
}

// --- factory registration ---

func init() {
	broker.RegisterFactory(Name, func(cfg *config.Config) (broker.Provider, error) {
		acc, ok := cfg.Exchanges.Deribit.ResolveAccount("")
		if !ok {
			// No credentials — public-only instance for chains/greeks/tickers.
			return newAdapter(config.ExchangeAccount{}, cfg.Exchanges.Deribit.Testnet)
		}
		return newAdapter(acc, cfg.Exchanges.Deribit.Testnet)
	})
	broker.RegisterAccountFactory(Name, func(cfg *config.Config, accountName string) (broker.Provider, error) {
		acc, ok := cfg.Exchanges.Deribit.ResolveAccount(accountName)
		if !ok {
			if accountName == "" {
				return newAdapter(config.ExchangeAccount{}, cfg.Exchanges.Deribit.Testnet)
			}
			var names []string
			for i, a := range cfg.Exchanges.Deribit.Accounts {
				n := a.Name
				if n == "" {
					n = fmt.Sprintf("%d", i+1)
				}
				names = append(names, n)
			}
			return nil, fmt.Errorf("%s: account %q not found (available: %v)", Name, accountName, names)
		}
		return newAdapter(acc, cfg.Exchanges.Deribit.Testnet)
	})
}
