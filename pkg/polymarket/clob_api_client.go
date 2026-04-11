package polymarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
)

// CLOBReaderClient CLOB 读 API 客户端
type CLOBReaderClient interface {
	// ConnectMarketChannel 连接市场频道
	ConnectMarketChannel(ctx context.Context) (CLOBChannelConn, error)
}

// CLOBWriterClient CLOB 写 API 客户端
type CLOBWriterClient interface {
	// CreateOrDeriveAPIKey 创建或派生 API 密钥
	//
	// 如果指定 nonce 还未生成密钥则创建密钥，若已生成则获取已有密钥
	CreateOrDeriveAPIKey(ctx context.Context, nonce int64) (*APIKeyInfo, error)
	// CreateAPIKey 创建 API 密钥
	CreateAPIKey(ctx context.Context, nonce int64) (*APIKeyInfo, error)
	// DeriveAPIKey 获取已有 API 密钥
	DeriveAPIKey(ctx context.Context, nonce int64) (*APIKeyInfo, error)
	// SendHeartbeat 发送心跳
	SendHeartbeat(ctx context.Context) (*HeartbeatStatus, error)
}

// CLOBChannelConn CLOB 频道连接
type CLOBChannelConn interface {
	// SendSubscriptionRequest 开始订阅
	SendSubscriptionRequest(ctx context.Context, req *CLOBSubscriptionRequest) error
	// SendSubscriptionUpdate 更新订阅
	SendSubscriptionUpdate(ctx context.Context, req *CLOBSubscriptionUpdateRequest) error
	// Channel 获取接收事件的通道
	Channel() <-chan *CLOBEvent
	// Err 获取运行错误
	Err() error
}

// CreateOrDeriveAPIKey 创建或派生 API 密钥
//
// 如果指定 nonce 还未生成密钥则创建密钥，若已生成则获取已有密钥
func (c *Client) CreateOrDeriveAPIKey(ctx context.Context, nonce int64) (*APIKeyInfo, error) {
	info, err := c.CreateAPIKey(ctx, nonce)
	if err != nil {
		if errors.Is(err, ErrNonceAlreadyUsed) {
			return c.DeriveAPIKey(ctx, nonce)
		}
		return nil, err
	}
	return info, nil
}

// CreateAPIKey 创建 API 密钥
func (c *Client) CreateAPIKey(ctx context.Context, nonce int64) (*APIKeyInfo, error) {
	info := &APIKeyInfo{}
	err := c.Do(ctx, &RawRequest{
		Method:     http.MethodPost,
		Endpoint:   CLOBEndpoint,
		URI:        "/auth/api-key",
		WithL1Auth: true,
		L1Nonce:    nonce,
	}, info)
	if err != nil {
		return nil, err
	}

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(1).Info(fmt.Sprintf(
		"created api key: apiKey: %s, secret: %s, passphrase: %s",
		info.APIKey, info.Secret, info.Passphrase,
	))
	return info, nil
}

// DeriveAPIKey 获取已有 API 密钥
func (c *Client) DeriveAPIKey(ctx context.Context, nonce int64) (*APIKeyInfo, error) {
	info := &APIKeyInfo{}
	err := c.Do(ctx, &RawRequest{
		Method:     http.MethodGet,
		Endpoint:   CLOBEndpoint,
		URI:        "/auth/derive-api-key",
		WithL1Auth: true,
		L1Nonce:    nonce,
	}, info)
	if err != nil {
		return nil, err
	}

	logger := logr.FromContextOrDiscard(ctx)
	logger.V(1).Info(fmt.Sprintf(
		"derived api key: apiKey: %s, secret: %s, passphrase: %s",
		info.APIKey, info.Secret, info.Passphrase,
	))
	return info, nil
}

// SendHeartbeat 发送心跳
func (c *Client) SendHeartbeat(ctx context.Context) (*HeartbeatStatus, error) {
	status := &HeartbeatStatus{}
	err := c.Do(ctx, &RawRequest{
		Method:     http.MethodPost,
		Endpoint:   CLOBEndpoint,
		URI:        "/heartbeats",
		WithL2Auth: true,
	}, status)
	if err != nil {
		return nil, err
	}
	return status, nil
}

// ConnectMarketChannel 连接市场频道
func (c *Client) ConnectMarketChannel(ctx context.Context) (CLOBChannelConn, error) {
	conn, err := c.ConnectWebSocket(ctx, &RawRequest{
		Method:   http.MethodGet,
		Endpoint: CLOBWebSocketEndpoint,
		URI:      "/ws/market",
	})
	if err != nil {
		return nil, err
	}
	return NewCLOBChannelConn(ctx, conn), nil
}

// NewCLOBChannelConn 创建基于 WebSocket 的 CLOBChannelConn
func NewCLOBChannelConn(ctx context.Context, conn *websocket.Conn) CLOBChannelConn {
	w := &wsCLOBChannelConn{
		conn:       conn,
		eventsChan: make(chan *CLOBEvent),
	}
	go w.pingLoop(ctx)
	go w.receiveLoop(ctx)
	return w
}

// wsCLOBChannelConn 基于的 WebSocket 的 CLOB 频道连接
type wsCLOBChannelConn struct {
	conn       *websocket.Conn
	err        error
	eventsChan chan *CLOBEvent
}

var _ CLOBChannelConn = (*wsCLOBChannelConn)(nil)

// SendSubscriptionRequest 开始订阅
func (w *wsCLOBChannelConn) SendSubscriptionRequest(_ context.Context, req *CLOBSubscriptionRequest) error {
	return w.conn.WriteJSON(req)
}

// SendSubscriptionUpdate 更新订阅
func (w *wsCLOBChannelConn) SendSubscriptionUpdate(_ context.Context, req *CLOBSubscriptionUpdateRequest) error {
	return w.conn.WriteJSON(req)
}

// pingLoop 发送 PING 的循环
func (w *wsCLOBChannelConn) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	logger := logr.FromContextOrDiscard(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		if err := w.conn.WriteControl(websocket.PingMessage, []byte("1"), time.Now().Add(3*time.Second)); err != nil {
			if errors.Is(err, websocket.ErrCloseSent) || websocket.IsUnexpectedCloseError(err) {
				w.err = fmt.Errorf("ping error: %w", err)
				return
			}
			logger.Error(err, "ping error")
		}
	}
}

// receiveLoop 接收循环
func (w *wsCLOBChannelConn) receiveLoop(ctx context.Context) {
	logger := logr.FromContextOrDiscard(ctx).WithName("clob-channel-conn")

	logger.V(1).Info("clob channel receive loop started")

	defer func() {
		close(w.eventsChan)
		_ = w.conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			if w.err == nil {
				w.err = ctx.Err()
			}
			logger.V(1).Info("clob channel receive loop done")
			return
		default:
		}

		_, raw, err := w.conn.ReadMessage()
		if err != nil {
			if w.err == nil {
				w.err = fmt.Errorf("read message error: %w", err)
			}
			logger.V(1).Error(err, "read message from clob channel error")
			return
		}
		logger.V(1).Info(fmt.Sprintf("received message: %s", string(raw)))

		e := CLOBEvent{}
		if err := json.Unmarshal(raw, &e); err != nil {
			logger.V(1).Error(err, fmt.Sprintf("unmarshal clob event from json error, raw: %s", string(raw)))
			continue
		}

		select {
		case <-ctx.Done():
			if w.err == nil {
				w.err = ctx.Err()
			}
			logger.V(1).Info("clob channel receive loop done")
			return
		case w.eventsChan <- &e:
		}
	}
}

// Channel 获取接收事件的通道
func (w *wsCLOBChannelConn) Channel() <-chan *CLOBEvent {
	return w.eventsChan
}

// Err 获取运行错误
func (w *wsCLOBChannelConn) Err() error {
	return w.err
}

// APIKeyInfo API 密钥信息
type APIKeyInfo struct {
	APIKey     string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

// HeartbeatStatus 心跳状态
type HeartbeatStatus struct {
	Status string `json:"status"`
}

const (
	SubTypeMarket = "market"
)

// CLOBSubscriptionRequest 订阅请求
type CLOBSubscriptionRequest struct {
	// 订阅资产 ID
	AssetIDs []string `json:"assets_ids"`
	// 订阅类型
	// market / user
	Type string `json:"type"`
	// 是否接收初始状态
	InitialDump *bool `json:"initial_dump"`
	// 订阅级别，默认 2
	Level int `json:"level,omitempty"`
	// 是否开启自定义特性
	CustomFeatureEnabled bool `json:"custom_feature_enabled,omitempty"`
}

const (
	OpSubscribe   = "subscribe"
	OpUnsubscribe = "unsubscribe"
)

// CLOBSubscriptionUpdateRequest 订阅更新请求
type CLOBSubscriptionUpdateRequest struct {
	// 操作
	// subscribe / unsubscribe
	Operation string `json:"operation"`
	// 订阅资产 ID
	AssetIDs []string `json:"assets_ids"`
	// 订阅级别，默认 2
	Level int `json:"level,omitempty"`
	// 是否开启自定义特性
	CustomFeatureEnabled bool `json:"custom_feature_enabled,omitempty"`
}

// CLOBEvent CLOB 事件
type CLOBEvent struct {
	CLOBEventMeta

	OrderbookSnapshot *OrderbookSnapshot `json:"-"`
	PriceChange       *PriceChange       `json:"-"`
	LastTradePrice    *LastTradePrice    `json:"-"`
	TickSizeChange    *TickSizeChange    `json:"-"`
	BestBidOrAsk      *BestBidOrAsk      `json:"-"`
	NewMarket         *NewMarket         `json:"-"`
	MarketResolved    *MarketResolved    `json:"-"`
}

const (
	EventBook           = "book"
	EventPriceChange    = "price_change"
	EventLastTradePrice = "last_trade_price"
	EventTickSizeChange = "tick_size_change"
	EventBestBidAsk     = "best_bid_ask"
	EventNewMarket      = "new_market"
	EventMarketResolved = "market_resolved"
)

// UnmarshalJSON 从 JSON 反序列化
func (e *CLOBEvent) UnmarshalJSON(data []byte) error {
	meta := CLOBEventMeta{}
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}

	e.CLOBEventMeta = meta

	var err error
	switch meta.EventType {
	case EventBook:
		e.OrderbookSnapshot = &OrderbookSnapshot{}
		err = json.Unmarshal(data, e.OrderbookSnapshot)
	case EventPriceChange:
		e.PriceChange = &PriceChange{}
		err = json.Unmarshal(data, e.PriceChange)
	case EventLastTradePrice:
		e.LastTradePrice = &LastTradePrice{}
		err = json.Unmarshal(data, e.LastTradePrice)
	case EventTickSizeChange:
		e.TickSizeChange = &TickSizeChange{}
		err = json.Unmarshal(data, e.TickSizeChange)
	case EventBestBidAsk:
		e.BestBidOrAsk = &BestBidOrAsk{}
		err = json.Unmarshal(data, e.BestBidOrAsk)
	case EventNewMarket:
		e.NewMarket = &NewMarket{}
		err = json.Unmarshal(data, e.NewMarket)
	case EventMarketResolved:
		e.MarketResolved = &MarketResolved{}
		err = json.Unmarshal(data, e.MarketResolved)
	}

	return err
}

// CLOBEventMeta CLOB 事件元信息
type CLOBEventMeta struct {
	// 事件类型
	// book: OrderbookSnapshot
	// price_change: PriceChange
	// last_trade_price: LastTradePrice
	// tick_size_change: TickSizeChange
	// best_bid_ask: BestBidOrAsk
	// new_market: NewMarket
	// market_resolved: MarketResolved
	EventType string `json:"event_type"`
	// 事件 UNIX 时间戳（毫秒）
	Timestamp int64 `json:"timestamp,string"`
}

// OrderbookSnapshot 订单簿快照事件
type OrderbookSnapshot struct {
	AssetID string       `json:"asset_id"`
	Market  string       `json:"market"`
	Bids    []PriceLevel `json:"bids"`
	Asks    []PriceLevel `json:"asks"`
	Hash    string       `json:"hash"`
}

// PriceLevel 分档价格
type PriceLevel struct {
	Price decimal.Decimal `json:"price"`
	Size  decimal.Decimal `json:"size"`
}

// PriceChange 价格变化事件
type PriceChange struct {
	Market       string             `json:"market"`
	PriceChanges []AssetPriceChange `json:"price_changes"`
}

// AssetPriceChange 价格变化
type AssetPriceChange struct {
	AssetID string          `json:"asset_id"`
	Price   decimal.Decimal `json:"price"`
	Size    decimal.Decimal `json:"size"`
	// 交易方向
	// BUY / SELL
	Side    string          `json:"side"`
	Hash    string          `json:"hash"`
	BestBid decimal.Decimal `json:"best_bid"`
	BestAsk decimal.Decimal `json:"best_ask"`
}

// LastTradePrice 最近成交价事件
type LastTradePrice struct {
	AssetID    string          `json:"asset_id"`
	Market     string          `json:"market"`
	Price      decimal.Decimal `json:"price"`
	Size       decimal.Decimal `json:"size"`
	FeeRateBps decimal.Decimal `json:"fee_rate_bps"`
	// 交易方向
	// BUY / SELL
	Side string `json:"side"`
	// 交易哈希
	TransactionHash string `json:"transaction_hash"`
}

// TickSizeChange 市场最小价格变动单位更新事件
type TickSizeChange struct {
	AssetID     string          `json:"asset_id"`
	Market      string          `json:"market"`
	OldTickSize decimal.Decimal `json:"old_tick_size"`
	NewTickSize decimal.Decimal `json:"new_tick_size"`
}

// BestBidOrAsk 最佳出价/要价更新事件
type BestBidOrAsk struct {
	AssetID string          `json:"asset_id"`
	Market  string          `json:"market"`
	BestBid decimal.Decimal `json:"best_bid"`
	BestAsk decimal.Decimal `json:"best_ask"`
	Spread  decimal.Decimal `json:"spread"`
}

// NewMarket 新市场事件
type NewMarket struct {
	ID                    string       `json:"id"`
	Question              string       `json:"question"`
	Market                string       `json:"market"`
	Slug                  string       `json:"slug"`
	Description           string       `json:"description,omitempty"`
	AssetsIDs             []string     `json:"assets_ids"`
	Outcomes              []string     `json:"outcomes"`
	EventMessage          EventMessage `json:"event_message,omitempty"`
	Tags                  []string     `json:"tags,omitempty"`
	ConditionID           string       `json:"condition_id,omitempty"`
	Active                bool         `json:"active,omitempty"`
	ClobTokenIDs          []string     `json:"clob_token_ids,omitempty"`
	SportsMarketType      string       `json:"sports_market_type,omitempty"`
	Line                  string       `json:"line,omitempty"`
	GameStartTime         string       `json:"game_start_time,omitempty"`
	OrderPriceMinTickSize string       `json:"order_price_min_tick_size,omitempty"`
	GroupItemTitle        string       `json:"group_item_title,omitempty"`
}

// MarketResolved 市场解决事件
type MarketResolved struct {
	ID             string       `json:"id"`
	Market         string       `json:"market"`
	AssetIDs       []string     `json:"assets_ids"`
	WinningAssetId string       `json:"winning_asset_id"`
	WinningOutcome string       `json:"winning_outcome"`
	EventMessage   EventMessage `json:"event_message,omitempty"`
	Tags           []string     `json:"tags,omitempty"`
}

// EventMessage 事件消息
type EventMessage struct {
	ID          string `json:"id"`
	Ticker      string `json:"ticker"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Description string `json:"description"`
}
