package polymarket

// SubscriptionRequest WebSocket 订阅请求
type SubscriptionRequest struct {
	AssetIDs    []string `json:"assets_ids"`
	Type        string   `json:"type"`         // "market"
	InitialDump bool     `json:"initial_dump"` // true
	Level       int      `json:"level"`        // 2
}

// BookEvent 订单簿快照事件
type BookEvent struct {
	EventType string       `json:"event_type"` // "book"
	AssetID   string       `json:"asset_id"`
	Market    string       `json:"market"`
	Bids      []PriceLevel `json:"bids"`
	Asks      []PriceLevel `json:"asks"`
	Timestamp string       `json:"timestamp"`
	Hash      string       `json:"hash"`
}

// PriceLevel 价格档位
type PriceLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// PriceChangeEvent 价格变化事件
type PriceChangeEvent struct {
	EventType    string        `json:"event_type"` // "price_change"
	Market       string        `json:"market"`
	PriceChanges []PriceChange `json:"price_changes"`
	Timestamp    string        `json:"timestamp"`
}

// PriceChange 价格变化
type PriceChange struct {
	AssetID string `json:"asset_id"`
	Price   string `json:"price"`
	Size    string `json:"size"`
	Side    string `json:"side"` // "BUY" / "SELL"
	Hash    string `json:"hash"`
	BestBid string `json:"best_bid"`
	BestAsk string `json:"best_ask"`
}

// LastTradePriceEvent 最近成交价事件
type LastTradePriceEvent struct {
	EventType       string `json:"event_type"` // "last_trade_price"
	AssetID         string `json:"asset_id"`
	Market          string `json:"market"`
	Price           string `json:"price"`
	Size            string `json:"size"`
	FeeRateBps      string `json:"fee_rate_bps"`
	Side            string `json:"side"` // "BUY" / "SELL"
	Timestamp       string `json:"timestamp"`
	TransactionHash string `json:"transaction_hash"`
}

// UnderlyingPriceEvent 底层资产价格事件
type UnderlyingPriceEvent struct {
	Symbol string  // 资产符号，如 "btc/usd"
	Value  float64 // 当前价格
}

// PriceToBeatEvent 起始价格事件
type PriceToBeatEvent struct {
	PriceToBeat float64 // 起始价格
}

// RTDSSubscription RTDS 订阅请求
type RTDSSubscription struct {
	Action        string                 `json:"action"` // "subscribe" 或 "unsubscribe"
	Subscriptions []RTDSSubscriptionItem `json:"subscriptions"`
}

// RTDSSubscriptionItem RTDS 订阅项
type RTDSSubscriptionItem struct {
	Topic   string `json:"topic"`             // 如 "crypto_prices", "crypto_prices_chainlink"
	Type    string `json:"type"`              // 如 "update", "*"
	Filters string `json:"filters,omitempty"` // JSON 字符串格式的过滤器
}

// RTDSMessage RTDS 推送消息
type RTDSMessage struct {
	Topic     string      `json:"topic"`
	Type      string      `json:"type"`
	Timestamp int64       `json:"timestamp"`
	Payload   RTDSPayload `json:"payload"`
}

// RTDSPayload RTDS 消息负载
type RTDSPayload struct {
	Symbol    string  `json:"symbol"`
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}
