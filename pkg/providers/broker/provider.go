// Package broker defines the unified provider hierarchy for all asset classes.
// It mirrors the pkg/exchanges interface hierarchy but extends it with
// market-data, trading, and transfer capabilities.
package broker

import (
	"context"

	ccxt "github.com/ccxt/ccxt/go/v4"
)

// AssetCategory identifies the class of instruments a provider handles.
type AssetCategory string

const (
	CategoryCrypto AssetCategory = "crypto"
	CategoryStock  AssetCategory = "stock"
	CategoryFX     AssetCategory = "fx"
)

// MarketStatus indicates whether a market is currently tradeable.
type MarketStatus string

const (
	MarketOpen    MarketStatus = "open"
	MarketClosed  MarketStatus = "closed"
	MarketUnknown MarketStatus = "unknown"
)

// AccountRef is a resolved (provider-id, account-name) pair returned by
// ListConfiguredAccounts.
type AccountRef struct {
	ProviderID string
	Account    string
}

// Provider is the root interface every broker provider must satisfy.
type Provider interface {
	// ID returns the canonical provider identifier (e.g. "binance", "okx").
	ID() string

	// Category returns the asset class this provider covers.
	Category() AssetCategory

	// GetMarketStatus returns whether the given symbol is currently open for trading.
	GetMarketStatus(ctx context.Context, symbol string) (MarketStatus, error)
}

// PortfolioProvider extends Provider with balance and pricing capabilities.
// It mirrors the existing pkg/exchanges.PricedExchange surface.
type PortfolioProvider interface {
	Provider

	// GetBalances returns a flat list of non-zero balances (spot / default wallet).
	GetBalances(ctx context.Context) ([]Balance, error)

	// GetWalletBalances returns balances for a specific wallet type.
	// Pass "all" to aggregate across all supported wallet types.
	GetWalletBalances(ctx context.Context, walletType string) ([]WalletBalance, error)

	// FetchPrice returns the last-traded price of asset in terms of quote (e.g. "USDT").
	// Returns (0, nil) when asset IS quote or a recognised stablecoin equivalent.
	FetchPrice(ctx context.Context, asset, quote string) (float64, error)

	// SupportedWalletTypes returns the wallet-type keys accepted by GetWalletBalances.
	SupportedWalletTypes() []string
}

// MarketDataProvider extends Provider with read-only market-data feeds.
type MarketDataProvider interface {
	Provider

	// FetchTicker returns the latest ticker for symbol (e.g. "BTC/USDT").
	FetchTicker(ctx context.Context, symbol string) (ccxt.Ticker, error)

	// FetchTickers returns tickers for a set of symbols (max 20 recommended).
	// Pass nil or empty slice to fetch all available tickers.
	FetchTickers(ctx context.Context, symbols []string) (map[string]ccxt.Ticker, error)

	// FetchOHLCV returns candlestick data.
	// timeframe is one of: 1m 5m 15m 1h 4h 1d 1w
	// limit is capped at 500 by callers.
	FetchOHLCV(ctx context.Context, symbol, timeframe string, since *int64, limit int) ([]ccxt.OHLCV, error)

	// FetchOrderBook returns the current order book.
	// depth is capped at 50 by callers.
	FetchOrderBook(ctx context.Context, symbol string, depth int) (ccxt.OrderBook, error)

	// LoadMarkets refreshes the cached market catalogue and returns the map.
	LoadMarkets(ctx context.Context) (map[string]ccxt.MarketInterface, error)
}

// TradingProvider extends Provider with order management.
type TradingProvider interface {
	Provider

	// CreateOrder submits a new order.
	// orderType: "limit" | "market" | "stop_loss" | "take_profit"
	// side: "buy" | "sell"
	CreateOrder(ctx context.Context, symbol, orderType, side string, amount float64, price *float64, params map[string]interface{}) (ccxt.Order, error)

	// CancelOrder cancels an open order by ID.
	CancelOrder(ctx context.Context, id, symbol string) (ccxt.Order, error)

	// FetchOrder retrieves a single order by ID.
	FetchOrder(ctx context.Context, id, symbol string) (ccxt.Order, error)

	// FetchOpenOrders returns all open orders, optionally filtered by symbol.
	FetchOpenOrders(ctx context.Context, symbol string) ([]ccxt.Order, error)

	// FetchClosedOrders returns closed/filled orders.
	FetchClosedOrders(ctx context.Context, symbol string, since *int64, limit int) ([]ccxt.Order, error)

	// FetchMyTrades returns the personal trade history.
	FetchMyTrades(ctx context.Context, symbol string, since *int64, limit int) ([]ccxt.Trade, error)
}

// TransferProvider extends Provider with internal fund-transfer capability.
type TransferProvider interface {
	Provider

	// Transfer moves funds between internal sub-accounts (e.g. spot → futures).
	Transfer(ctx context.Context, asset string, amount float64, fromAccount, toAccount string) (ccxt.TransferEntry, error)
}

// FuturesOrderRequest is the exchange-neutral futures order input.
// Symbols should use CCXT contract notation, e.g. "BTC/USDT:USDT" for USDT-settled
// perpetual swaps.
type FuturesOrderRequest struct {
	Symbol       string
	OrderType    string
	Side         string
	Amount       float64
	Price        *float64
	MarginMode   string
	PositionSide string
	ReduceOnly   bool
	Params       map[string]interface{}
}

// FuturesProvider extends Provider with perpetual/futures trading and account data.
// Binance TH and Bitkub intentionally do not implement this interface because they
// do not offer futures trading through this app.
type FuturesProvider interface {
	Provider

	SetFuturesLeverage(ctx context.Context, symbol string, leverage int64, marginMode, positionSide string) (map[string]interface{}, error)
	CreateFuturesOrder(ctx context.Context, req FuturesOrderRequest) (ccxt.Order, error)
	FetchFuturesOrder(ctx context.Context, id, symbol string) (ccxt.Order, error)
	FetchFuturesOpenOrders(ctx context.Context, symbol string) ([]ccxt.Order, error)
	FetchFuturesPositions(ctx context.Context, symbols []string) ([]ccxt.Position, error)
	FetchFuturesFundingRate(ctx context.Context, symbol string) (ccxt.FundingRate, error)
	FetchFuturesFundingRates(ctx context.Context, symbols []string) (map[string]ccxt.FundingRate, error)
	FetchFuturesFundingHistory(ctx context.Context, symbol string, since *int64, limit int) ([]ccxt.FundingHistory, error)
	FetchPublicFundingRateHistory(ctx context.Context, symbol string, since *int64, limit int) ([]ccxt.FundingRateHistory, error)
	LoadFuturesMarkets(ctx context.Context) (map[string]ccxt.MarketInterface, error)
	FetchFuturesMarkPrice(ctx context.Context, symbol string) (float64, error)
	CancelFuturesOrder(ctx context.Context, id, symbol string) (ccxt.Order, error)
	CancelAllFuturesOrders(ctx context.Context, symbol string) ([]ccxt.Order, error)
}

// OptionOrderRequest is the exchange-neutral option order input.
// Symbols use CCXT option notation (e.g. Deribit "BTC/USD:BTC-25-12-26-100000-C");
// adapters may also accept the venue-native instrument id and normalize it.
type OptionOrderRequest struct {
	Symbol     string
	OrderType  string // "limit" | "market"
	Side       string // "buy" | "sell"
	Amount     float64
	Price      *float64
	ReduceOnly bool
	Params     map[string]interface{}
}

// OptionsProvider extends Provider with options market data and trading.
// Deribit is the first implementation. NOTE: prefer FetchGreeks for
// per-instrument pricing — ccxt-go's typed Option struct has known
// mis-mapped fields, so chain values should be read from Option.Info.
type OptionsProvider interface {
	Provider

	// LoadOptionMarkets returns option markets keyed by CCXT symbol.
	LoadOptionMarkets(ctx context.Context) (map[string]ccxt.MarketInterface, error)
	// FetchOptionChain returns the full chain for an underlying currency code (e.g. "BTC").
	FetchOptionChain(ctx context.Context, code string) (ccxt.OptionChain, error)
	// FetchOption returns market data for a single option symbol.
	FetchOption(ctx context.Context, symbol string) (ccxt.Option, error)
	// FetchGreeks returns greeks + IV + mark data for a single option symbol.
	FetchGreeks(ctx context.Context, symbol string) (ccxt.Greeks, error)
	// FetchOptionPositions returns open option positions. currency filters by
	// settlement currency (e.g. "BTC"); empty currency returns all supported.
	FetchOptionPositions(ctx context.Context, currency string) ([]ccxt.Position, error)
	// CreateOptionOrder submits an option order. Amount is in contracts.
	CreateOptionOrder(ctx context.Context, req OptionOrderRequest) (ccxt.Order, error)
	// CancelOptionOrder cancels an open option order by id.
	CancelOptionOrder(ctx context.Context, id, symbol string) (ccxt.Order, error)
	// FetchOptionOpenOrders returns open option orders, optionally for one symbol.
	FetchOptionOpenOrders(ctx context.Context, symbol string) ([]ccxt.Order, error)
}

// EarnRatePoint represents a single historical earn rate data point.
// Rate is an annualized fraction (0.05 == 5% APY).
// Timestamp is in milliseconds.
type EarnRatePoint struct {
	Rate      float64
	Timestamp int64
}

// EarnProduct describes a flexible savings/earn product offered for an asset.
// APY is a fraction (0.05 == 5%).
type EarnProduct struct {
	Exchange      string
	Asset         string
	ProductID     string
	APY           float64
	CanSubscribe  bool
	AutoSubscribe bool
	MinSubscribe  float64
	// Type distinguishes product categories: "savings" (lending pool) or "staking-defi" (on-chain earn).
	// Empty string is treated as "savings" for backward compatibility.
	Type     string
	Protocol string // human-readable protocol name for staking-defi (e.g. "Chiliz")
}

// EarnPosition describes a currently held flexible earn position.
// APY is a fraction (0.05 == 5%).
type EarnPosition struct {
	Exchange      string
	Asset         string
	ProductID     string
	Amount        float64
	APY           float64
	AutoSubscribe bool
}

// EarnProvider is implemented by providers that expose flexible savings/earn
// products for the spot leg of a delta-neutral position. All methods require
// authenticated credentials except where the underlying APY endpoint is public.
type EarnProvider interface {
	Provider
	// FetchFlexibleEarnProducts returns flexible earn products. asset == "" returns all.
	FetchFlexibleEarnProducts(ctx context.Context, asset string) ([]EarnProduct, error)
	// FetchFlexibleEarnPositions returns currently held flexible earn positions.
	FetchFlexibleEarnPositions(ctx context.Context) ([]EarnPosition, error)
	// SubscribeFlexibleEarn moves amount of asset into the given flexible product.
	// Returns an exchange transaction/purchase id.
	SubscribeFlexibleEarn(ctx context.Context, productID, asset string, amount float64, autoSubscribe bool) (string, error)
	// RedeemFlexibleEarn redeems amount (or all) of asset from the flexible product.
	// Returns an exchange transaction/redemption id.
	RedeemFlexibleEarn(ctx context.Context, productID, asset string, amount float64, redeemAll bool) (string, error)
	// SetFlexibleAutoSubscribe enables/disables auto-subscribe for the product/asset.
	SetFlexibleAutoSubscribe(ctx context.Context, productID, asset string, enable bool) error
	// FetchFlexibleEarnRateHistory returns historical rate data points.
	// productID is required; since is optional. limit capped at 100 by callers.
	FetchFlexibleEarnRateHistory(ctx context.Context, productID, asset string, since *int64, limit int) ([]EarnRatePoint, error)
}

// Balance mirrors pkg/exchanges.Balance so callers don't need to import both.
type Balance struct {
	Asset  string
	Free   float64
	Locked float64
}

// WalletBalance extends Balance with wallet-type metadata.
type WalletBalance struct {
	Balance
	WalletType string
	Extra      map[string]string
}
