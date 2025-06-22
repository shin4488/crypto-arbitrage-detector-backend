package main

import (
	"crypto-arb-backend/arbitrage"
	"crypto-arb-backend/config"
	"crypto-arb-backend/exchanges"
	"crypto-arb-backend/server"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log.Println("Starting Crypto Arbitrage Detection Server...")

	// Load configuration
	cfg := config.GetDefaultConfig()

	// Create arbitrage engine
	arbEngine := arbitrage.NewEngine()

	// Create WebSocket hub
	hub := server.NewHub()
	go hub.Run()

	// Subscribe to arbitrage notifications
	arbCh := arbEngine.Subscribe()
	go func() {
		for arbData := range arbCh {
			hub.Broadcast(arbData)
		}
	}()

	// Initialize exchange clients
	var clients []interface{ Close() }
	var binanceClient *exchanges.BinanceClient
	var okxClient *exchanges.OKXClient

	// Binance client
	binanceSymbols := []string{"BTCUSDT", "ETHUSDT"}
	binanceClient = exchanges.NewBinanceClient(binanceSymbols)
	if err := binanceClient.Connect(); err != nil {
		log.Printf("Failed to connect to Binance: %v", err)
	} else {
		clients = append(clients, binanceClient)
		go func() {
			for priceData := range binanceClient.GetPriceChannel() {
				arbEngine.UpdatePrice(priceData)
			}
		}()
	}

	// OKX client
	okxSymbols := []string{"BTC-USDT", "ETH-USDT"}
	okxClient = exchanges.NewOKXClient(okxSymbols)
	if err := okxClient.Connect(); err != nil {
		log.Printf("Failed to connect to OKX: %v", err)
	} else {
		clients = append(clients, okxClient)
		go func() {
			for priceData := range okxClient.GetPriceChannel() {
				arbEngine.UpdatePrice(priceData)
			}
		}()
	}

	// Start connection monitor
	go monitorConnections(binanceClient, okxClient)

	// Setup HTTP server
	http.HandleFunc("/ws", hub.HandleWebSocket)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		binanceStatus := "Disconnected"
		okxStatus := "Disconnected"
		
		if binanceClient != nil && binanceClient.IsConnected() {
			binanceStatus = "Connected"
		}
		if okxClient != nil && okxClient.IsConnected() {
			okxStatus = "Connected"
		}
		
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK - WebSocket Clients: %d, Binance: %s, OKX: %s", 
			hub.GetClientCount(), binanceStatus, okxStatus)
	})

	// Add CORS headers for development
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		binanceStatus := "Disconnected"
		okxStatus := "Disconnected"
		
		if binanceClient != nil && binanceClient.IsConnected() {
			binanceStatus = "Connected"
		}
		if okxClient != nil && okxClient.IsConnected() {
			okxStatus = "Connected"
		}
		
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Crypto Arbitrage Detection Server is running!\n")
		fmt.Fprintf(w, "WebSocket endpoint: ws://localhost:%s/ws\n", cfg.Server.Port)
		fmt.Fprintf(w, "Connected clients: %d\n", hub.GetClientCount())
		fmt.Fprintf(w, "Exchange Status - Binance: %s, OKX: %s\n", binanceStatus, okxStatus)
	})

	// Start HTTP server
	serverAddr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Server starting on %s", serverAddr)
	log.Printf("WebSocket endpoint: ws://%s/ws", serverAddr)

	go func() {
		if err := http.ListenAndServe(serverAddr, nil); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Println("Shutting down server...")

	// Close all exchange clients
	for _, client := range clients {
		client.Close()
	}

	log.Println("Server stopped")
}

// monitorConnections monitors the connection status of exchange clients
func monitorConnections(binanceClient *exchanges.BinanceClient, okxClient *exchanges.OKXClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			binanceStatus := "Disconnected"
			okxStatus := "Disconnected"
			
			if binanceClient != nil && binanceClient.IsConnected() {
				binanceStatus = "Connected"
			}
			if okxClient != nil && okxClient.IsConnected() {
				okxStatus = "Connected"
			}
			
			log.Printf("Exchange Status - Binance: %s, OKX: %s", binanceStatus, okxStatus)
		}
	}
} 