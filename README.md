# Crypto Arbitrage Detection Backend

リアルタイム暗号通貨アービトラージ検知システムのバックエンドサーバーです。

## 機能

- **リアルタイム価格監視**: Binance と OKX から WebSocket 経由でリアルタイム価格データを取得
- **アービトラージ検知**: 取引所間の価格差を検知して利益機会を特定
- **WebSocket API**: フロントエンドとのリアルタイム通信
- **拡張可能な設計**: 新しい通貨ペアや取引所を簡単に追加可能

## サポート対象

### 取引所
- Binance
- OKX

### 通貨ペア
- BTC/USDT
- ETH/USDT

## セットアップ

### 前提条件
- Go 1.21以上

### インストール

1. 依存関係のインストール:
```bash
go mod download
```

2. サーバーの起動:
```bash
go run main.go
```

または Makefile を使用:
```bash
make run
```

## API エンドポイント

### WebSocket
- **エンドポイント**: `ws://localhost:8080/ws`
- **説明**: アービトラージ機会をリアルタイムで受信

### HTTP
- **ヘルスチェック**: `GET http://localhost:8080/health`
- **ルート**: `GET http://localhost:8080/`

## データ形式

### ArbitrageData
```json
{
  "pair": "BTC/USDT",
  "buy_exchange": "Binance",
  "sell_exchange": "OKX", 
  "buy_price": "65425.50",
  "sell_price": "65440.20",
  "amount": "0.168",
  "spread": "14.70",
  "spread_ratio": "0.0002"
}
```

## 新しい通貨ペアの追加

新しい通貨ペアを追加するには、`main.go` の以下の部分を編集してください：

```go
// Binance
binanceSymbols := []string{"BTCUSDT", "ETHUSDT", "ADAUSDT"} // 新しいペアを追加

// OKX  
okxSymbols := []string{"BTC-USDT", "ETH-USDT", "ADA-USDT"} // 新しいペアを追加
```

## ログ

サーバーは以下の情報をログ出力します：
- 取引所への接続状況
- アービトラージ機会の検知
- WebSocket クライアントの接続/切断
- エラー情報

## 開発

### ディレクトリ構造
```
backend/
├── main.go              # メインアプリケーション
├── go.mod              # Go モジュール定義
├── types/              # データ構造定義
├── config/             # 設定管理
├── exchanges/          # 取引所クライアント
├── arbitrage/          # アービトラージ検知エンジン
└── server/             # WebSocket サーバー
```

### 新しい取引所の追加

新しい取引所を追加するには：

1. `exchanges/` ディレクトリに新しい取引所のクライアントを実装
2. `types.PriceData` 形式でデータを送信するチャネルを実装
3. `main.go` で新しいクライアントを初期化

## トラブルシューティング

### よくある問題

1. **取引所への接続失敗**
   - ネットワーク接続を確認
   - 取引所のAPI状態を確認

2. **WebSocket接続エラー**
   - ポート8080が利用可能か確認
   - ファイアウォール設定を確認

3. **アービトラージ機会が検知されない**
   - 価格データが正常に受信されているかログを確認
   - 最小スプレッド閾値 ($0.01) を確認 