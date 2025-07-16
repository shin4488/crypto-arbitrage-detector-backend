package arbitrage

import (
	"crypto-arb-backend/types"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// Engine represents arbitrage detection engine
type Engine struct {
	bidMap      map[string]map[string]types.PriceData // symbol -> exchange -> bid priceData
	askMap      map[string]map[string]types.PriceData // symbol -> exchange -> ask priceData
	mu          sync.RWMutex
	subscribers []chan types.ArbitrageData
	lastSentArb map[string]types.ArbitrageData // symbol -> last sent arbitrage data
}

// NewEngine creates new arbitrage engine
func NewEngine() *Engine {
	return &Engine{
		bidMap:      make(map[string]map[string]types.PriceData),
		askMap:      make(map[string]map[string]types.PriceData),
		lastSentArb: make(map[string]types.ArbitrageData),
	}
}

// UpdatePrice updates price data and checks for arbitrage opportunities
func (e *Engine) UpdatePrice(priceData types.PriceData) {
	e.mu.Lock()
	defer e.mu.Unlock()

	symbol := priceData.Symbol
	exchange := priceData.Exchange

	// Debug log for price data reception
	log.Printf("📊 Received price data: %s %s %s - Price: %s, Quantity: %s", 
		exchange, symbol, priceData.Side, priceData.Price.String(), priceData.Quantity.String())

	// Update appropriate map based on side
	if priceData.Side == "bid" {
		// Initialize symbol map if not exists
		if e.bidMap[symbol] == nil {
			e.bidMap[symbol] = make(map[string]types.PriceData)
		}
		e.bidMap[symbol][exchange] = priceData
	} else if priceData.Side == "ask" {
		// Initialize symbol map if not exists
		if e.askMap[symbol] == nil {
			e.askMap[symbol] = make(map[string]types.PriceData)
		}
		e.askMap[symbol][exchange] = priceData
	}

	// Check for arbitrage opportunities
	e.checkArbitrage(symbol)
}

// checkArbitrage checks for arbitrage opportunities for given symbol
func (e *Engine) checkArbitrage(symbol string) {
	bids := e.bidMap[symbol]
	asks := e.askMap[symbol]

	if len(bids) < 1 || len(asks) < 1 {
		return // Need both bid and ask data
	}

	var validBids []types.PriceData
	var validAsks []types.PriceData

	// Filter out stale data (reduced to 5 seconds for faster detection)
	currentTime := time.Now().UnixMilli()
	staleThreshold := int64(5000) // 5 seconds

	for _, bidData := range bids {
		if currentTime-bidData.Time <= staleThreshold {
			validBids = append(validBids, bidData)
		}
	}

	for _, askData := range asks {
		if currentTime-askData.Time <= staleThreshold {
			validAsks = append(validAsks, askData)
		}
	}

	if len(validBids) < 1 || len(validAsks) < 1 {
		return
	}

	// Get prices from each exchange
	var binanceBid, binanceAsk, okxBid, okxAsk *types.PriceData

	for i := range validBids {
		if validBids[i].Exchange == "Binance" {
			binanceBid = &validBids[i]
		} else if validBids[i].Exchange == "OKX" {
			okxBid = &validBids[i]
		}
	}

	for i := range validAsks {
		if validAsks[i].Exchange == "Binance" {
			binanceAsk = &validAsks[i]
		} else if validAsks[i].Exchange == "OKX" {
			okxAsk = &validAsks[i]
		}
	}


	// Try to find arbitrage opportunities with available data (don't wait for all 4 prices)
	var availableArbitrages []*types.ArbitrageData
	
	// Check Pattern 1: Buy OKX ask, Sell Binance bid
	if okxAsk != nil && binanceBid != nil {
		if binanceBid.Price.GreaterThan(okxAsk.Price) {
			spread := binanceBid.Price.Sub(okxAsk.Price)
			// Create arbitrage opportunity (no minimum threshold)
			arb := e.createArbitrageData(symbol, "OKX", "Binance", okxAsk.Price, binanceBid.Price, spread, okxAsk.Quantity, binanceBid.Quantity)
			availableArbitrages = append(availableArbitrages, arb)
		}
	}
	
	// Check Pattern 2: Buy Binance ask, Sell OKX bid
	if binanceAsk != nil && okxBid != nil {
		if okxBid.Price.GreaterThan(binanceAsk.Price) {
			spread := okxBid.Price.Sub(binanceAsk.Price)
			// Create arbitrage opportunity (no minimum threshold)
			arb := e.createArbitrageData(symbol, "Binance", "OKX", binanceAsk.Price, okxBid.Price, spread, binanceAsk.Quantity, okxBid.Quantity)
			availableArbitrages = append(availableArbitrages, arb)
		}
	}
	
	// Select the best arbitrage opportunity
	var bestArb *types.ArbitrageData
	if len(availableArbitrages) > 0 {
		bestArb = availableArbitrages[0]
		for _, arb := range availableArbitrages[1:] {
			if arb.Spread.GreaterThan(bestArb.Spread) {
				bestArb = arb
			}
		}
	}

	// If no arbitrage found, send no-chance notification (don't show negative profits)
	if bestArb == nil {
		// No sufficient data, send no-chance notification
		noChanceData := &types.ArbitrageData{
			Pair:         symbol,
			BuyExchange:  "",
			SellExchange: "",
			BuyPrice:     decimal.Zero,
			SellPrice:    decimal.Zero,
			Amount:       decimal.Zero,
			Spread:       decimal.Zero,
			SpreadRatio:  decimal.Zero,
			Currency:     extractQuoteCurrency(symbol),
			NoChance:     true,
		}
		
		// Always send updates to ensure UI responsiveness
		e.lastSentArb[symbol] = *noChanceData
		e.notifySubscribers(*noChanceData)
		return
	}

	// Always send arbitrage data to ensure UI responsiveness
	now := time.Now()
	
	if bestArb.Spread.GreaterThan(decimal.Zero) {
		log.Printf("[%s] Arbitrage opportunity: %s - Buy %s at %s, Sell %s at %s - Spread: $%s",
			now.Format("15:04:05.000"),
			bestArb.Pair,
			bestArb.BuyExchange,
			bestArb.BuyPrice.String(),
			bestArb.SellExchange,
			bestArb.SellPrice.String(),
			bestArb.Spread.String())
		
		
		// Send profitable arbitrage opportunity
		e.lastSentArb[symbol] = *bestArb
		e.notifySubscribers(*bestArb)
	} else {
		// For negative spreads, send no-chance notification instead
		log.Printf("[%s] No profitable arbitrage for %s - Best spread: $%s (Loss)",
			now.Format("15:04:05.000"),
			bestArb.Pair,
			bestArb.Spread.String())
		
		noChanceData := &types.ArbitrageData{
			Pair:         symbol,
			BuyExchange:  "",
			SellExchange: "",
			BuyPrice:     decimal.Zero,
			SellPrice:    decimal.Zero,
			Amount:       decimal.Zero,
			Spread:       decimal.Zero,
			SpreadRatio:  decimal.Zero,
			Currency:     extractQuoteCurrency(symbol),
			NoChance:     true,
		}
		
		e.lastSentArb[symbol] = *noChanceData
		e.notifySubscribers(*noChanceData)
	}
}

// createArbitrageData creates arbitrage data structure
func (e *Engine) createArbitrageData(symbol, buyExchange, sellExchange string, buyPrice, sellPrice, spread decimal.Decimal, buyQty, sellQty decimal.Decimal) *types.ArbitrageData {
	// Calculate tradeable amount: minimum between buy and sell quantities
	// This ensures we can actually execute both sides of the arbitrage
	tradeableAmount := buyQty
	if sellQty.LessThan(tradeableAmount) {
		tradeableAmount = sellQty
	}

	// Set reasonable trade amount limits based on symbol
	var maxAmount, minAmount decimal.Decimal

	if symbol == "BTC/USDT" {
		maxAmount = decimal.NewFromFloat(1.0)      // Max 1 BTC
		minAmount = decimal.NewFromFloat(0.00001)  // Min 0.00001 BTC (5 decimal places)
	} else if symbol == "ETH/USDT" {
		maxAmount = decimal.NewFromFloat(10.0)     // Max 10 ETH
		minAmount = decimal.NewFromFloat(0.0001)   // Min 0.0001 ETH (4 decimal places)
	} else {
		// Default values for other pairs
		maxAmount = decimal.NewFromFloat(1.0)
		minAmount = decimal.NewFromFloat(0.001)
	}

	// Apply limits to tradeable amount
	if tradeableAmount.GreaterThan(maxAmount) {
		tradeableAmount = maxAmount
	}

	// Apply minimum amount for realistic trading
	if tradeableAmount.LessThan(minAmount) {
		tradeableAmount = minAmount
	}

	spreadRatio := spread.Div(buyPrice)

	// Calculate profit: (sell quantity * sell price) - (buy quantity * buy price)
	sellRevenue := tradeableAmount.Mul(sellPrice)  // sell quantity * sell price
	buyCost := tradeableAmount.Mul(buyPrice)       // buy quantity * buy price
	totalProfit := sellRevenue.Sub(buyCost)        // profit = revenue - cost


	return &types.ArbitrageData{
		Pair:         symbol,
		BuyExchange:  buyExchange,
		SellExchange: sellExchange,
		BuyPrice:     buyPrice,
		SellPrice:    sellPrice,
		Amount:       tradeableAmount,
		Spread:       totalProfit,
		SpreadRatio:  spreadRatio,
		Currency:     extractQuoteCurrency(symbol),
		NoChance:     false,
	}
}

// isSignificantlyDifferent checks if two arbitrage data are significantly different
func (e *Engine) isSignificantlyDifferent(last, current types.ArbitrageData) bool {
	// Check if exchanges changed
	if last.BuyExchange != current.BuyExchange || last.SellExchange != current.SellExchange {
		return true
	}

	// Check if any price, amount, or spread has changed (no threshold)
	if !last.BuyPrice.Equal(current.BuyPrice) ||
	   !last.SellPrice.Equal(current.SellPrice) ||
	   !last.Amount.Equal(current.Amount) ||
	   !last.Spread.Equal(current.Spread) {
		return true
	}

	return false
}

// Subscribe adds a subscriber to receive arbitrage notifications
func (e *Engine) Subscribe() <-chan types.ArbitrageData {
	e.mu.Lock()
	defer e.mu.Unlock()

	ch := make(chan types.ArbitrageData, 10)
	e.subscribers = append(e.subscribers, ch)
	return ch
}

// notifySubscribers notifies all subscribers of arbitrage opportunity
func (e *Engine) notifySubscribers(arb types.ArbitrageData) {
	for _, ch := range e.subscribers {
		select {
		case ch <- arb:
		default:
			// Channel is full, skip
		}
	}
}

// GetCurrentPrices returns current prices for all symbols
func (e *Engine) GetCurrentPrices() map[string]map[string]types.PriceData {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Deep copy to avoid race conditions
	result := make(map[string]map[string]types.PriceData)

	// Combine bid and ask data for compatibility
	for symbol, exchanges := range e.bidMap {
		if result[symbol] == nil {
			result[symbol] = make(map[string]types.PriceData)
		}
		for exchange, priceData := range exchanges {
			result[symbol][exchange+"_bid"] = priceData
		}
	}

	for symbol, exchanges := range e.askMap {
		if result[symbol] == nil {
			result[symbol] = make(map[string]types.PriceData)
		}
		for exchange, priceData := range exchanges {
			result[symbol][exchange+"_ask"] = priceData
		}
	}

	return result
}

// extractQuoteCurrency extracts the quote currency from a trading pair symbol
// e.g., "BTC/USDT" -> "USDT", "ETH/USDC" -> "USDC"
func extractQuoteCurrency(symbol string) string {
	parts := strings.Split(symbol, "/")
	if len(parts) == 2 {
		return parts[1]
	}
	return "USDT" // Default fallback
}
