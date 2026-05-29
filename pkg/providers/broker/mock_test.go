package broker_test

import (
	"context"

	ccxt "github.com/ccxt/ccxt/go/v4"

	"github.com/cryptoquantumwave/khunquant/pkg/providers/broker"
)

// MockProvider implements all four broker interfaces for testing.
type MockProvider struct {
	id       string
	category broker.AssetCategory

	// Configurable responses
	MarketStatusFn      func(symbol string) (broker.MarketStatus, error)
	GetBalancesFn       func() ([]broker.Balance, error)
	GetWalletBalsFn     func(walletType string) ([]broker.WalletBalance, error)
	FetchPriceFn        func(asset, quote string) (float64, error)
	FetchTickerFn       func(symbol string) (ccxt.Ticker, error)
	FetchTickersFn      func(symbols []string) (map[string]ccxt.Ticker, error)
	FetchOHLCVFn        func(symbol, timeframe string, since *int64, limit int) ([]ccxt.OHLCV, error)
	FetchOrderBookFn    func(symbol string, depth int) (ccxt.OrderBook, error)
	LoadMarketsFn       func() (map[string]ccxt.MarketInterface, error)
	CreateOrderFn       func(symbol, orderType, side string, amount float64, price *float64, params map[string]interface{}) (ccxt.Order, error)
	CancelOrderFn       func(id, symbol string) (ccxt.Order, error)
	FetchOrderFn        func(id, symbol string) (ccxt.Order, error)
	FetchOpenOrdersFn   func(symbol string) ([]ccxt.Order, error)
	FetchClosedOrdersFn func(symbol string, since *int64, limit int) ([]ccxt.Order, error)
	FetchMyTradesFn     func(symbol string, since *int64, limit int) ([]ccxt.Trade, error)
	TransferFn          func(asset string, amount float64, fromAccount, toAccount string) (ccxt.TransferEntry, error)

	// broker.FuturesProvider
	SetFuturesLeverageFn       func(symbol string, leverage int64, marginMode, positionSide string) (map[string]interface{}, error)
	CreateFuturesOrderFn       func(req broker.FuturesOrderRequest) (ccxt.Order, error)
	FetchFuturesOrderFn        func(id, symbol string) (ccxt.Order, error)
	FetchFuturesOpenOrdersFn   func(symbol string) ([]ccxt.Order, error)
	FetchFuturesPositionsFn    func(symbols []string) ([]ccxt.Position, error)
	FetchFuturesFundingRateFn  func(symbol string) (ccxt.FundingRate, error)
	FetchFuturesFundingRatesFn func(symbols []string) (map[string]ccxt.FundingRate, error)
	FetchFuturesFundingHistFn  func(symbol string, since *int64, limit int) ([]ccxt.FundingHistory, error)
	LoadFuturesMarketsFn       func() (map[string]ccxt.MarketInterface, error)
	FetchFuturesMarkPriceFn    func(symbol string) (float64, error)
	CancelFuturesOrderFn       func(id, symbol string) (ccxt.Order, error)
	CancelAllFuturesOrdersFn   func(symbol string) ([]ccxt.Order, error)
}

func NewMockProvider(id string) *MockProvider {
	return &MockProvider{id: id, category: broker.CategoryCrypto}
}

// --- broker.Provider ---
func (m *MockProvider) ID() string                     { return m.id }
func (m *MockProvider) Category() broker.AssetCategory { return m.category }
func (m *MockProvider) GetMarketStatus(_ context.Context, symbol string) (broker.MarketStatus, error) {
	if m.MarketStatusFn != nil {
		return m.MarketStatusFn(symbol)
	}
	return broker.MarketOpen, nil
}

// --- broker.PortfolioProvider ---
func (m *MockProvider) GetBalances(_ context.Context) ([]broker.Balance, error) {
	if m.GetBalancesFn != nil {
		return m.GetBalancesFn()
	}
	return nil, nil
}
func (m *MockProvider) GetWalletBalances(_ context.Context, walletType string) ([]broker.WalletBalance, error) {
	if m.GetWalletBalsFn != nil {
		return m.GetWalletBalsFn(walletType)
	}
	return nil, nil
}
func (m *MockProvider) FetchPrice(_ context.Context, asset, quote string) (float64, error) {
	if m.FetchPriceFn != nil {
		return m.FetchPriceFn(asset, quote)
	}
	return 1.0, nil
}
func (m *MockProvider) SupportedWalletTypes() []string { return []string{"spot", "all"} }

// --- broker.MarketDataProvider ---
func (m *MockProvider) FetchTicker(_ context.Context, symbol string) (ccxt.Ticker, error) {
	if m.FetchTickerFn != nil {
		return m.FetchTickerFn(symbol)
	}
	last := 50000.0
	return ccxt.Ticker{Symbol: &symbol, Last: &last}, nil
}
func (m *MockProvider) FetchTickers(_ context.Context, symbols []string) (map[string]ccxt.Ticker, error) {
	if m.FetchTickersFn != nil {
		return m.FetchTickersFn(symbols)
	}
	return map[string]ccxt.Ticker{}, nil
}
func (m *MockProvider) FetchOHLCV(_ context.Context, symbol, timeframe string, since *int64, limit int) ([]ccxt.OHLCV, error) {
	if m.FetchOHLCVFn != nil {
		return m.FetchOHLCVFn(symbol, timeframe, since, limit)
	}
	return nil, nil
}
func (m *MockProvider) FetchOrderBook(_ context.Context, symbol string, depth int) (ccxt.OrderBook, error) {
	if m.FetchOrderBookFn != nil {
		return m.FetchOrderBookFn(symbol, depth)
	}
	return ccxt.OrderBook{}, nil
}
func (m *MockProvider) LoadMarkets(_ context.Context) (map[string]ccxt.MarketInterface, error) {
	if m.LoadMarketsFn != nil {
		return m.LoadMarketsFn()
	}
	return nil, nil
}

// --- broker.TradingProvider ---
func (m *MockProvider) CreateOrder(_ context.Context, symbol, orderType, side string, amount float64, price *float64, params map[string]interface{}) (ccxt.Order, error) {
	if m.CreateOrderFn != nil {
		return m.CreateOrderFn(symbol, orderType, side, amount, price, params)
	}
	id := "mock-order-1"
	return ccxt.Order{Id: &id}, nil
}
func (m *MockProvider) CancelOrder(_ context.Context, id, symbol string) (ccxt.Order, error) {
	if m.CancelOrderFn != nil {
		return m.CancelOrderFn(id, symbol)
	}
	return ccxt.Order{Id: &id}, nil
}
func (m *MockProvider) FetchOrder(_ context.Context, id, symbol string) (ccxt.Order, error) {
	if m.FetchOrderFn != nil {
		return m.FetchOrderFn(id, symbol)
	}
	return ccxt.Order{Id: &id}, nil
}
func (m *MockProvider) FetchOpenOrders(_ context.Context, symbol string) ([]ccxt.Order, error) {
	if m.FetchOpenOrdersFn != nil {
		return m.FetchOpenOrdersFn(symbol)
	}
	return nil, nil
}
func (m *MockProvider) FetchClosedOrders(_ context.Context, symbol string, since *int64, limit int) ([]ccxt.Order, error) {
	if m.FetchClosedOrdersFn != nil {
		return m.FetchClosedOrdersFn(symbol, since, limit)
	}
	return nil, nil
}
func (m *MockProvider) FetchMyTrades(_ context.Context, symbol string, since *int64, limit int) ([]ccxt.Trade, error) {
	if m.FetchMyTradesFn != nil {
		return m.FetchMyTradesFn(symbol, since, limit)
	}
	return nil, nil
}

// --- broker.TransferProvider ---
func (m *MockProvider) Transfer(_ context.Context, asset string, amount float64, fromAccount, toAccount string) (ccxt.TransferEntry, error) {
	if m.TransferFn != nil {
		return m.TransferFn(asset, amount, fromAccount, toAccount)
	}
	return ccxt.TransferEntry{}, nil
}

// --- broker.FuturesProvider ---
func (m *MockProvider) SetFuturesLeverage(_ context.Context, symbol string, leverage int64, marginMode, positionSide string) (map[string]interface{}, error) {
	if m.SetFuturesLeverageFn != nil {
		return m.SetFuturesLeverageFn(symbol, leverage, marginMode, positionSide)
	}
	return map[string]interface{}{}, nil
}
func (m *MockProvider) CreateFuturesOrder(_ context.Context, req broker.FuturesOrderRequest) (ccxt.Order, error) {
	if m.CreateFuturesOrderFn != nil {
		return m.CreateFuturesOrderFn(req)
	}
	id := "mock-futures-order-1"
	return ccxt.Order{Id: &id}, nil
}
func (m *MockProvider) FetchFuturesOrder(_ context.Context, id, symbol string) (ccxt.Order, error) {
	if m.FetchFuturesOrderFn != nil {
		return m.FetchFuturesOrderFn(id, symbol)
	}
	filled := 0.1
	status := "closed"
	return ccxt.Order{Id: &id, Filled: &filled, Status: &status}, nil
}
func (m *MockProvider) FetchFuturesOpenOrders(_ context.Context, symbol string) ([]ccxt.Order, error) {
	if m.FetchFuturesOpenOrdersFn != nil {
		return m.FetchFuturesOpenOrdersFn(symbol)
	}
	return nil, nil
}
func (m *MockProvider) FetchFuturesPositions(_ context.Context, symbols []string) ([]ccxt.Position, error) {
	if m.FetchFuturesPositionsFn != nil {
		return m.FetchFuturesPositionsFn(symbols)
	}
	return nil, nil
}
func (m *MockProvider) FetchFuturesFundingRate(_ context.Context, symbol string) (ccxt.FundingRate, error) {
	if m.FetchFuturesFundingRateFn != nil {
		return m.FetchFuturesFundingRateFn(symbol)
	}
	return ccxt.FundingRate{}, nil
}
func (m *MockProvider) FetchFuturesFundingRates(_ context.Context, symbols []string) (map[string]ccxt.FundingRate, error) {
	if m.FetchFuturesFundingRatesFn != nil {
		return m.FetchFuturesFundingRatesFn(symbols)
	}
	return map[string]ccxt.FundingRate{}, nil
}
func (m *MockProvider) FetchFuturesFundingHistory(_ context.Context, symbol string, since *int64, limit int) ([]ccxt.FundingHistory, error) {
	if m.FetchFuturesFundingHistFn != nil {
		return m.FetchFuturesFundingHistFn(symbol, since, limit)
	}
	return nil, nil
}
func (m *MockProvider) LoadFuturesMarkets(_ context.Context) (map[string]ccxt.MarketInterface, error) {
	if m.LoadFuturesMarketsFn != nil {
		return m.LoadFuturesMarketsFn()
	}
	return nil, nil
}
func (m *MockProvider) FetchFuturesMarkPrice(_ context.Context, symbol string) (float64, error) {
	if m.FetchFuturesMarkPriceFn != nil {
		return m.FetchFuturesMarkPriceFn(symbol)
	}
	return 50000.0, nil
}
func (m *MockProvider) CancelFuturesOrder(_ context.Context, id, symbol string) (ccxt.Order, error) {
	if m.CancelFuturesOrderFn != nil {
		return m.CancelFuturesOrderFn(id, symbol)
	}
	return ccxt.Order{Id: &id}, nil
}
func (m *MockProvider) CancelAllFuturesOrders(_ context.Context, symbol string) ([]ccxt.Order, error) {
	if m.CancelAllFuturesOrdersFn != nil {
		return m.CancelAllFuturesOrdersFn(symbol)
	}
	return nil, nil
}
