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
	minSpread   decimal.Decimal
	lastSentArb map[string]types.ArbitrageData // symbol -> last sent arbitrage data
}

// NewEngine creates new arbitrage engine
func NewEngine() *Engine {
	return &Engine{
		bidMap:      make(map[string]map[string]types.PriceData),
		askMap:      make(map[string]map[string]types.PriceData),
		minSpread:   decimal.NewFromFloat(0.01), // Minimum spread threshold ($0.01)
		lastSentArb: make(map[string]types.ArbitrageData),
	}
}

// UpdatePrice updates price data and checks for arbitrage opportunities
func (e *Engine) UpdatePrice(priceData types.PriceData) {
	e.mu.Lock()
	defer e.mu.Unlock()

	symbol := priceData.Symbol
	exchange := priceData.Exchange

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

	// Filter out stale data (older than 30 seconds)
	currentTime := time.Now().UnixMilli()
	
	for _, bidData := range bids {
		if currentTime-bidData.Time <= 30000 {
			validBids = append(validBids, bidData)
		}
	}
	
	for _, askData := range asks {
		if currentTime-askData.Time <= 30000 {
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

	// Need all four prices for proper arbitrage analysis
	if binanceBid == nil || binanceAsk == nil || okxBid == nil || okxAsk == nil {
		log.Printf("Missing price data for %s - Binance(bid:%v, ask:%v), OKX(bid:%v, ask:%v)", 
			symbol, binanceBid != nil, binanceAsk != nil, okxBid != nil, okxAsk != nil)
		return
	}

	// Add timestamp logging for data freshness verification
	now := time.Now()
	binanceBidAge := time.Duration(currentTime-binanceBid.Time) * time.Millisecond
	binanceAskAge := time.Duration(currentTime-binanceAsk.Time) * time.Millisecond
	okxBidAge := time.Duration(currentTime-okxBid.Time) * time.Millisecond
	okxAskAge := time.Duration(currentTime-okxAsk.Time) * time.Millisecond

	log.Printf("[%s] %s Price Data Freshness - Binance(bid:%v ago, ask:%v ago), OKX(bid:%v ago, ask:%v ago)",
		now.Format("15:04:05.000"), symbol, binanceBidAge, binanceAskAge, okxBidAge, okxAskAge)

	// Analyze the 5 price ordering patterns
	var bestArb *types.ArbitrageData = nil
	
	// Pattern 1: binanceAsk >= binanceBid >= okxAsk >= okxBid
	// Profitable: Buy at OKX ask, Sell at Binance bid
	if binanceAsk.Price.GreaterThanOrEqual(binanceBid.Price) &&
		binanceBid.Price.GreaterThanOrEqual(okxAsk.Price) &&
		okxAsk.Price.GreaterThanOrEqual(okxBid.Price) {
		
		if binanceBid.Price.GreaterThan(okxAsk.Price) {
			spread := binanceBid.Price.Sub(okxAsk.Price)
			// Pattern 1: Buy at OKX ask, Sell at Binance bid
			// Tradeable amount = min(OKX ask quantity, Binance bid quantity)
			bestArb = e.createArbitrageData(symbol, "OKX", "Binance", okxAsk.Price, binanceBid.Price, spread, okxAsk.Quantity, binanceBid.Quantity)
			log.Printf("[%s] Pattern 1 detected: Buy OKX ask(qty:%s) at %s, Sell Binance bid(qty:%s) at %s", 
				now.Format("15:04:05.000"), okxAsk.Quantity.String(), okxAsk.Price.String(), binanceBid.Quantity.String(), binanceBid.Price.String())
		}
	}
	
	// Pattern 5: okxAsk >= okxBid >= binanceAsk >= binanceBid  
	// Profitable: Buy at Binance ask, Sell at OKX bid
	if okxAsk.Price.GreaterThanOrEqual(okxBid.Price) &&
		okxBid.Price.GreaterThanOrEqual(binanceAsk.Price) &&
		binanceAsk.Price.GreaterThanOrEqual(binanceBid.Price) {
		
		if okxBid.Price.GreaterThan(binanceAsk.Price) {
			spread := okxBid.Price.Sub(binanceAsk.Price)
			// Pattern 5: Buy at Binance ask, Sell at OKX bid
			// Tradeable amount = min(Binance ask quantity, OKX bid quantity)
			candidate := e.createArbitrageData(symbol, "Binance", "OKX", binanceAsk.Price, okxBid.Price, spread, binanceAsk.Quantity, okxBid.Quantity)
			
			// Choose the better arbitrage opportunity (higher profit)
			if bestArb == nil || candidate.Spread.GreaterThan(bestArb.Spread) {
				bestArb = candidate
				log.Printf("[%s] Pattern 5 detected: Buy Binance ask(qty:%s) at %s, Sell OKX bid(qty:%s) at %s", 
					now.Format("15:04:05.000"), binanceAsk.Quantity.String(), binanceAsk.Price.String(), okxBid.Quantity.String(), okxBid.Price.String())
			}
		}
	}

	// Other patterns (2, 3, 4) result in losses, so we don't process them
	// Pattern 2: binanceAsk >= okxAsk >= binanceBid >= okxBid (Loss)
	// Pattern 3: okxAsk >= binanceAsk >= binanceBid >= okxBid (Loss)  
	// Pattern 4: okxAsk >= binanceAsk >= okxBid >= binanceBid (Loss)

	// If no profitable arbitrage found, send "no chance" notification
	if bestArb == nil {
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
			NoChance:     true, // Add this field to indicate no arbitrage opportunity
		}
		
		// Check if this is different from last sent (avoid spam)
		lastArb, exists := e.lastSentArb[symbol]
		shouldSend := !exists || !lastArb.NoChance
		
		if shouldSend {
			log.Printf("[%s] No profitable arbitrage for %s - Prices: Binance(bid:%s, ask:%s), OKX(bid:%s, ask:%s)", 
				now.Format("15:04:05.000"), symbol, binanceBid.Price.String(), binanceAsk.Price.String(), okxBid.Price.String(), okxAsk.Price.String())
			e.lastSentArb[symbol] = *noChanceData
			e.notifySubscribers(*noChanceData)
		}
		return
	}

	// Notify subscribers if arbitrage opportunity found and different from last sent
	if bestArb.Spread.GreaterThan(e.minSpread) {
		lastArb, exists := e.lastSentArb[symbol]
		shouldSend := !exists || e.isSignificantlyDifferent(lastArb, *bestArb)
		
		if shouldSend {
			log.Printf("[%s] Arbitrage opportunity found: %s - Buy %s at %s, Sell at %s - Spread: $%s",
				now.Format("15:04:05.000"),
				bestArb.Pair,
				bestArb.BuyExchange,
				bestArb.BuyPrice.String(),
				bestArb.SellExchange,
				bestArb.Spread.String())

			e.lastSentArb[symbol] = *bestArb
			e.notifySubscribers(*bestArb)
		}
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
	
	// Check price changes (threshold: $0.01)
	buyPriceDiff := current.BuyPrice.Sub(last.BuyPrice).Abs()
	sellPriceDiff := current.SellPrice.Sub(last.SellPrice).Abs()
	priceThreshold := decimal.NewFromFloat(0.01)
	
	if buyPriceDiff.GreaterThan(priceThreshold) || sellPriceDiff.GreaterThan(priceThreshold) {
		return true
	}
	
	// Check amount changes (threshold: 0.001)
	amountDiff := current.Amount.Sub(last.Amount).Abs()
	amountThreshold := decimal.NewFromFloat(0.001)
	
	if amountDiff.GreaterThan(amountThreshold) {
		return true
	}
	
	// Check spread changes (threshold: $0.01)
	spreadDiff := current.Spread.Sub(last.Spread).Abs()
	spreadThreshold := decimal.NewFromFloat(0.01)
	
	if spreadDiff.GreaterThan(spreadThreshold) {
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