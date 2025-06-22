package exchanges

import (
	"crypto-arb-backend/types"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

// OKXClient represents OKX exchange client
type OKXClient struct {
	conn        *websocket.Conn
	priceCh     chan types.PriceData
	symbols     []string
	stopCh      chan struct{}
	mu          sync.Mutex
	reconnectCh chan struct{}
	isConnected bool
}

// OKXSubscribeRequest represents OKX subscription request
type OKXSubscribeRequest struct {
	Op   string `json:"op"`
	Args []struct {
		Channel string `json:"channel"`
		InstId  string `json:"instId"`
	} `json:"args"`
}

// OKXOrderBookResponse represents OKX order book response
type OKXOrderBookResponse struct {
	Arg struct {
		Channel string `json:"channel"`
		InstId  string `json:"instId"`
	} `json:"arg"`
	Data []struct {
		Asks      [][]string `json:"asks"`
		Bids      [][]string `json:"bids"`
		Timestamp string     `json:"ts"`
		Checksum  string     `json:"checksum"`
	} `json:"data"`
}

// NewOKXClient creates new OKX client
func NewOKXClient(symbols []string) *OKXClient {
	return &OKXClient{
		priceCh:     make(chan types.PriceData, 100),
		symbols:     symbols,
		stopCh:      make(chan struct{}),
		reconnectCh: make(chan struct{}, 1),
		isConnected: false,
	}
}

// Connect connects to OKX WebSocket API with auto-reconnect
func (o *OKXClient) Connect() error {
	go o.connectionManager()
	return o.connectOnce()
}

// connectionManager manages the connection lifecycle with reconnection
func (o *OKXClient) connectionManager() {
	for {
		select {
		case <-o.stopCh:
			return
		case <-o.reconnectCh:
			o.mu.Lock()
			wasConnected := o.isConnected
			o.isConnected = false
			if o.conn != nil {
				o.conn.Close()
				o.conn = nil
			}
			o.mu.Unlock()

			if wasConnected {
				log.Println("OKX WebSocket disconnected, attempting to reconnect...")
			}

			// Exponential backoff for reconnection
			backoff := time.Second
			maxBackoff := 60 * time.Second
			
			for {
				select {
				case <-o.stopCh:
					return
				default:
					if err := o.connectOnce(); err != nil {
						log.Printf("OKX reconnection failed: %v, retrying in %v", err, backoff)
						time.Sleep(backoff)
						if backoff < maxBackoff {
							backoff *= 2
						}
					} else {
						log.Println("OKX WebSocket reconnected successfully")
						goto nextReconnect
					}
				}
			}
		nextReconnect:
		}
	}
}

// connectOnce attempts to connect once
func (o *OKXClient) connectOnce() error {
	wsURL := "wss://ws.okx.com:8443/ws/v5/public"
	
	log.Printf("Connecting to OKX WebSocket: %s", wsURL)
	
	// Set up dialer with optimized settings for lower latency
	dialer := &websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   1024,
		WriteBufferSize:  1024,
	}
	
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to OKX: %w", err)
	}

	// Set ping/pong handlers for connection health
	conn.SetPingHandler(func(message string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(time.Second))
	})

	// Set read deadline for timeout detection
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	o.mu.Lock()
	o.conn = conn
	o.isConnected = true
	o.mu.Unlock()

	// Subscribe to tickers
	if err := o.subscribe(); err != nil {
		o.mu.Lock()
		o.conn.Close()
		o.conn = nil
		o.isConnected = false
		o.mu.Unlock()
		return err
	}

	go o.readMessages()
	go o.keepAlive() // Add keep-alive mechanism
	log.Println("Connected to OKX WebSocket")
	return nil
}

// subscribe subscribes to order book streams
func (o *OKXClient) subscribe() error {
	req := OKXSubscribeRequest{
		Op: "subscribe",
	}

	for _, symbol := range o.symbols {
		req.Args = append(req.Args, struct {
			Channel string `json:"channel"`
			InstId  string `json:"instId"`
		}{
			Channel: "books5",
			InstId:  symbol,
		})
	}

	o.mu.Lock()
	conn := o.conn
	o.mu.Unlock()
	
	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	return conn.WriteJSON(req)
}

// readMessages reads messages from WebSocket connection
func (o *OKXClient) readMessages() {
	defer func() {
		o.mu.Lock()
		if o.conn != nil {
			o.conn.Close()
		}
		o.mu.Unlock()
		
		// Trigger reconnection
		select {
		case o.reconnectCh <- struct{}{}:
		default:
		}
	}()
	
	for {
		select {
		case <-o.stopCh:
			return
		default:
			o.mu.Lock()
			conn := o.conn
			o.mu.Unlock()
			
			if conn == nil {
				return
			}

			// Reset read deadline on each message
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))

			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Printf("OKX WebSocket read error: %v", err)
				return // This will trigger reconnection via defer
			}

			var response OKXOrderBookResponse
			if err := json.Unmarshal(message, &response); err != nil {
				// Skip non-order book messages (like subscription confirmations)
				continue
			}

			if response.Arg.Channel != "books5" || len(response.Data) == 0 {
				continue
			}

			data := response.Data[0]
			
			// Parse timestamp with fallback to current time
			var timestamp int64
			if data.Timestamp != "" {
				if ts, err := decimal.NewFromString(data.Timestamp); err == nil {
					timestamp = ts.IntPart()
				} else {
					timestamp = time.Now().UnixNano() / 1000000 // Use high-precision current time
				}
			} else {
				timestamp = time.Now().UnixNano() / 1000000 // Use high-precision current time
			}

			// Process bids (buy orders)
			if len(data.Bids) > 0 && len(data.Bids[0]) >= 4 {
				bestBid := data.Bids[0]
				bidPrice, err := decimal.NewFromString(bestBid[0])
				if err != nil {
					log.Printf("Failed to parse OKX bid price: %v", err)
				} else {
					bidQuantity, err := decimal.NewFromString(bestBid[1])
					if err != nil {
						log.Printf("Failed to parse OKX bid quantity: %v", err)
					} else {
						bidPriceData := types.PriceData{
							Exchange: "OKX",
							Symbol:   o.convertSymbol(response.Arg.InstId),
							Price:    bidPrice,
							Quantity: bidQuantity,
							Time:     timestamp,
							Side:     "bid",
						}

						select {
						case o.priceCh <- bidPriceData:
						default:
							// Channel is full, skip this update
						}
					}
				}
			}

			// Process asks (sell orders)
			if len(data.Asks) > 0 && len(data.Asks[0]) >= 4 {
				bestAsk := data.Asks[0]
				askPrice, err := decimal.NewFromString(bestAsk[0])
				if err != nil {
					log.Printf("Failed to parse OKX ask price: %v", err)
				} else {
					askQuantity, err := decimal.NewFromString(bestAsk[1])
					if err != nil {
						log.Printf("Failed to parse OKX ask quantity: %v", err)
					} else {
						askPriceData := types.PriceData{
							Exchange: "OKX",
							Symbol:   o.convertSymbol(response.Arg.InstId),
							Price:    askPrice,
							Quantity: askQuantity,
							Time:     timestamp,
							Side:     "ask",
						}

						select {
						case o.priceCh <- askPriceData:
						default:
							// Channel is full, skip this update
						}
					}
				}
			}
		}
	}
}

// convertSymbol converts OKX symbol format to standard format
func (o *OKXClient) convertSymbol(symbol string) string {
	switch symbol {
	case "BTC-USDT":
		return "BTC/USDT"
	case "ETH-USDT":
		return "ETH/USDT"
	default:
		return symbol
	}
}

// GetPriceChannel returns price data channel
func (o *OKXClient) GetPriceChannel() <-chan types.PriceData {
	return o.priceCh
}

// IsConnected returns connection status
func (o *OKXClient) IsConnected() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.isConnected
}

// Close closes the connection
func (o *OKXClient) Close() {
	close(o.stopCh)
	o.mu.Lock()
	if o.conn != nil {
		o.conn.Close()
	}
	o.mu.Unlock()
}

// keepAlive adds keep-alive mechanism
func (o *OKXClient) keepAlive() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.mu.Lock()
			conn := o.conn
			o.mu.Unlock()
			
			if conn == nil {
				return
			}

			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
				log.Printf("OKX WebSocket keep-alive failed: %v", err)
				o.mu.Lock()
				o.conn.Close()
				o.conn = nil
				o.isConnected = false
				o.mu.Unlock()
				return
			}
		}
	}
} 