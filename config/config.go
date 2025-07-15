package config

import "crypto-arb-backend/types"

// Config represents application configuration
type Config struct {
	Server    ServerConfig            `json:"server"`
	Exchanges map[string]types.ExchangeConfig `json:"exchanges"`
	Pairs     []string                `json:"pairs"`
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Port    string `json:"port"`
	Host    string `json:"host"`
	WSPath  string `json:"ws_path"`
}

// GetDefaultConfig returns default configuration
func GetDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:   "8080",
			Host:   "0.0.0.0",
			WSPath: "/ws",
		},
		Exchanges: map[string]types.ExchangeConfig{
			"binance": {
				Name:    "Binance",
				WSUrl:   "wss://stream.binance.com:9443/ws/",
				RestUrl: "https://api.binance.com",
				Pairs: []types.TradingPair{
					{
						Symbol:       "BTCUSDT",
						BaseAsset:    "BTC",
						QuoteAsset:   "USDT",
						MinTradeSize: "0.001",
					},
					{
						Symbol:       "ETHUSDT",
						BaseAsset:    "ETH",
						QuoteAsset:   "USDT",
						MinTradeSize: "0.01",
					},
				},
				Enabled: true,
			},
			"okx": {
				Name:    "OKX",
				WSUrl:   "wss://ws.okx.com:8443/ws/v5/public",
				RestUrl: "https://www.okx.com",
				Pairs: []types.TradingPair{
					{
						Symbol:       "BTC-USDT",
						BaseAsset:    "BTC",
						QuoteAsset:   "USDT",
						MinTradeSize: "0.001",
					},
					{
						Symbol:       "ETH-USDT",
						BaseAsset:    "ETH",
						QuoteAsset:   "USDT",
						MinTradeSize: "0.01",
					},
				},
				Enabled: true,
			},
		},
		Pairs: []string{"BTC/USDT", "ETH/USDT"},
	}
}
