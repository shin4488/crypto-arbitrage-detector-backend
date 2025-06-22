package exchanges

import (
	"crypto-arb-backend/types"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

// BinanceClient represents Binance exchange client
type BinanceClient struct {
	conn        *websocket.Conn
	priceCh     chan types.PriceData
	symbols     []string
	stopCh      chan struct{}
	mu          sync.Mutex
	reconnectCh chan struct{}
	isConnected bool
}

// BinanceDepthResponse represents Binance partial book depth response
type BinanceDepthResponse struct {
	Stream string `json:"stream"`
	Data   struct {
		LastUpdateId int64      `json:"lastUpdateId"`
		Bids         [][]string `json:"bids"`
		Asks         [][]string `json:"asks"`
	} `json:"data"`
}

// NewBinanceClient creates new Binance client
func NewBinanceClient(symbols []string) *BinanceClient {
	return &BinanceClient{
		priceCh:     make(chan types.PriceData, 100),
		symbols:     symbols,
		stopCh:      make(chan struct{}),
		reconnectCh: make(chan struct{}, 1),
		isConnected: false,
	}
}

// Connect connects to Binance WebSocket API with auto-reconnect
func (b *BinanceClient) Connect() error {
	go b.connectionManager()
	return b.connectOnce()
}

// connectionManager manages the connection lifecycle with reconnection
func (b *BinanceClient) connectionManager() {
	for {
		select {
		case <-b.stopCh:
			return
		case <-b.reconnectCh:
			b.mu.Lock()
			wasConnected := b.isConnected
			b.isConnected = false
			if b.conn != nil {
				b.conn.Close()
				b.conn = nil
			}
			b.mu.Unlock()

			if wasConnected {
				log.Println("Binance WebSocket disconnected, attempting to reconnect...")
			}

			// Exponential backoff for reconnection
			backoff := time.Second
			maxBackoff := 60 * time.Second
			
			for {
				select {
				case <-b.stopCh:
					return
				default:
					if err := b.connectOnce(); err != nil {
						log.Printf("Binance reconnection failed: %v, retrying in %v", err, backoff)
						time.Sleep(backoff)
						if backoff < maxBackoff {
							backoff *= 2
						}
					} else {
						log.Println("Binance WebSocket reconnected successfully")
						goto nextReconnect
					}
				}
			}
		nextReconnect:
		}
	}
}

// connectOnce attempts to connect once
func (b *BinanceClient) connectOnce() error {
	// Convert symbols to stream format (e.g., "BTCUSDT" -> "btcusdt@depth5")
	var streams []string
	for _, symbol := range b.symbols {
		streams = append(streams, strings.ToLower(symbol)+"@depth5")
	}
	
	// Use faster WebSocket endpoint with better latency
	wsURL := fmt.Sprintf("wss://stream.binance.com:443/stream?streams=%s", strings.Join(streams, "/"))
	
	log.Printf("Connecting to Binance WebSocket: %s", wsURL)
	
	// Set up dialer with optimized settings for lower latency
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
	}
	
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Binance: %w", err)
	}

	// Set ping/pong handlers for connection health
	conn.SetPingHandler(func(message string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(time.Second))
	})

	// Set read deadline for timeout detection
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	b.mu.Lock()
	b.conn = conn
	b.isConnected = true
	b.mu.Unlock()

	go b.readMessages()
	go b.keepAlive() // Add keep-alive mechanism
	log.Println("Connected to Binance WebSocket")
	return nil
}

// keepAlive sends periodic ping messages to maintain connection health
func (b *BinanceClient) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.mu.Lock()
			conn := b.conn
			b.mu.Unlock()

			if conn != nil {
				// Send ping message
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second)); err != nil {
					log.Printf("Binance ping failed: %v", err)
					// Trigger reconnection
					select {
					case b.reconnectCh <- struct{}{}:
					default:
					}
					return
				}
			}
		}
	}
}

// readMessages reads messages from WebSocket connection
func (b *BinanceClient) readMessages() {
	defer func() {
		b.mu.Lock()
		if b.conn != nil {
			b.conn.Close()
		}
		b.mu.Unlock()
		
		// Trigger reconnection
		select {
		case b.reconnectCh <- struct{}{}:
		default:
		}
	}()
	
	for {
		select {
		case <-b.stopCh:
			return
		default:
			b.mu.Lock()
			conn := b.conn
			b.mu.Unlock()
			
			if conn == nil {
				return
			}

			// Reset read deadline on each message
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Binance WebSocket read error: %v", err)
				return // This will trigger reconnection via defer
			}

			var response BinanceDepthResponse
			if err := json.Unmarshal(message, &response); err != nil {
				log.Printf("Failed to unmarshal Binance message: %v", err)
				continue
			}

			// Extract symbol from stream name (e.g., "btcusdt@depth5" -> "BTCUSDT")
			streamParts := strings.Split(response.Stream, "@")
			if len(streamParts) == 0 {
				continue
			}
			symbol := strings.ToUpper(streamParts[0])

			// Process best bid and ask prices
			if len(response.Data.Bids) > 0 && len(response.Data.Asks) > 0 {
				bestBid := response.Data.Bids[0]
				bestAsk := response.Data.Asks[0]
				
				if len(bestBid) >= 2 && len(bestAsk) >= 2 {
					bidPrice, err := decimal.NewFromString(bestBid[0])
					if err != nil {
						log.Printf("Failed to parse bid price: %v", err)
						continue
					}
					
					askPrice, err := decimal.NewFromString(bestAsk[0])
					if err != nil {
						log.Printf("Failed to parse ask price: %v", err)
						continue
					}
					
					bidQuantity, err := decimal.NewFromString(bestBid[1])
					if err != nil {
						log.Printf("Failed to parse bid quantity: %v", err)
						continue
					}

					// Use high-precision timestamp for better accuracy
					timestamp := time.Now().UnixNano() / 1000000 // Convert to milliseconds

					// Send bid price data
					bidPriceData := types.PriceData{
						Exchange: "Binance",
						Symbol:   b.convertSymbol(symbol),
						Price:    bidPrice,
						Quantity: bidQuantity,
						Time:     timestamp,
						Side:     "bid",
					}

					// Send ask price data
					askQuantity, err := decimal.NewFromString(bestAsk[1])
					if err != nil {
						log.Printf("Failed to parse ask quantity: %v", err)
						continue
					}

					askPriceData := types.PriceData{
						Exchange: "Binance",
						Symbol:   b.convertSymbol(symbol),
						Price:    askPrice,
						Quantity: askQuantity,
						Time:     timestamp,
						Side:     "ask",
					}

					select {
					case b.priceCh <- bidPriceData:
					default:
						// Channel is full, skip this update
					}

					select {
					case b.priceCh <- askPriceData:
					default:
						// Channel is full, skip this update
					}
				}
			}
		}
	}
}

// convertSymbol converts Binance symbol format to standard format
func (b *BinanceClient) convertSymbol(symbol string) string {
	switch symbol {
	case "BTCUSDT":
		return "BTC/USDT"
	case "ETHUSDT":
		return "ETH/USDT"
	default:
		return symbol
	}
}

// GetPriceChannel returns price data channel
func (b *BinanceClient) GetPriceChannel() <-chan types.PriceData {
	return b.priceCh
}

// IsConnected returns connection status
func (b *BinanceClient) IsConnected() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.isConnected
}

// Close closes the connection
func (b *BinanceClient) Close() {
	close(b.stopCh)
	b.mu.Lock()
	if b.conn != nil {
		b.conn.Close()
	}
	b.mu.Unlock()
} 