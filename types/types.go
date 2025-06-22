package types

import "github.com/shopspring/decimal"

// ArbitrageData represents arbitrage opportunity data
type ArbitrageData struct {
	Pair         string          `json:"pair"`
	BuyExchange  string          `json:"buy_exchange"`
	SellExchange string          `json:"sell_exchange"`
	BuyPrice     decimal.Decimal `json:"buy_price"`
	SellPrice    decimal.Decimal `json:"sell_price"`
	Amount       decimal.Decimal `json:"amount"`
	Spread       decimal.Decimal `json:"spread"`
	SpreadRatio  decimal.Decimal `json:"spread_ratio"`
	Currency     string          `json:"currency"` // Quote currency (USDT, USDC, etc.)
	NoChance     bool            `json:"no_chance"` // Indicates no profitable arbitrage opportunity
}

// ArbitrageDataJSON represents arbitrage data for JSON serialization
type ArbitrageDataJSON struct {
	Pair         string  `json:"pair"`
	BuyExchange  string  `json:"buy_exchange"`
	SellExchange string  `json:"sell_exchange"`
	BuyPrice     float64 `json:"buy_price"`
	SellPrice    float64 `json:"sell_price"`
	Amount       float64 `json:"amount"`
	Spread       float64 `json:"spread"`
	SpreadRatio  float64 `json:"spread_ratio"`
	Currency     string  `json:"currency"` // Quote currency (USDT, USDC, etc.)
	NoChance     bool    `json:"no_chance"` // Indicates no profitable arbitrage opportunity
}

// ToJSON converts ArbitrageData to ArbitrageDataJSON for WebSocket transmission
func (a *ArbitrageData) ToJSON() ArbitrageDataJSON {
	buyPrice, _ := a.BuyPrice.Float64()
	sellPrice, _ := a.SellPrice.Float64()
	amount, _ := a.Amount.Float64()
	spread, _ := a.Spread.Float64()
	spreadRatio, _ := a.SpreadRatio.Float64()

	return ArbitrageDataJSON{
		Pair:         a.Pair,
		BuyExchange:  a.BuyExchange,
		SellExchange: a.SellExchange,
		BuyPrice:     buyPrice,
		SellPrice:    sellPrice,
		Amount:       amount,
		Spread:       spread,
		SpreadRatio:  spreadRatio,
		Currency:     a.Currency,
		NoChance:     a.NoChance,
	}
}

// PriceData represents price information from exchange
type PriceData struct {
	Exchange string          `json:"exchange"`
	Symbol   string          `json:"symbol"`
	Price    decimal.Decimal `json:"price"`
	Quantity decimal.Decimal `json:"quantity"`
	Time     int64           `json:"time"`
	Side     string          `json:"side"` // "bid" or "ask"
}

// TradingPair represents a trading pair configuration
type TradingPair struct {
	Symbol       string `json:"symbol"`
	BaseAsset    string `json:"base_asset"`
	QuoteAsset   string `json:"quote_asset"`
	MinTradeSize string `json:"min_trade_size"`
}

// ExchangeConfig represents exchange configuration
type ExchangeConfig struct {
	Name     string        `json:"name"`
	WSUrl    string        `json:"ws_url"`
	RestUrl  string        `json:"rest_url"`
	Pairs    []TradingPair `json:"pairs"`
	Enabled  bool          `json:"enabled"`
} 