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
)

// RTDSClient RTDS 接口客户端
type RTDSClient interface {
	// ConnectRTDS WebSocket 连接 RTDS 实时数据服务
	ConnectRTDS(ctx context.Context) (RTDSConn, error)
}

// ConnectRTDS WebSocket 连接 RTDS 实时数据服务
func (c *Client) ConnectRTDS(ctx context.Context) (RTDSConn, error) {
	conn, err := c.ConnectWebSocket(ctx, &RawRequest{
		Method:   http.MethodGet,
		Endpoint: RTDSWebSocketEndpoint,
	})
	if err != nil {
		return nil, err
	}
	return NewRTDSConn(ctx, conn), nil
}

// RTDSConn RTDS 连接
type RTDSConn interface {
	// SendSubscriptionUpdate 更新订阅
	SendSubscriptionUpdate(ctx context.Context, req *RTDSSubscriptionUpdateRequest) error
	// Channel 获取接收事件的通道
	Channel() <-chan *RTDSEvent
	// Err 获取运行错误
	Err() error
}

// NewRTDSConn 创建基于 WebSocket 的 CLOBChannelConn
func NewRTDSConn(ctx context.Context, conn *websocket.Conn) RTDSConn {
	w := &wsRTDSConn{
		conn:       conn,
		eventsChan: make(chan *RTDSEvent),
	}
	go w.pingLoop(ctx)
	go w.receiveLoop(ctx)
	return w
}

// wsRTDSConn 基于的 WebSocket 的 RTDS 连接
type wsRTDSConn struct {
	conn       *websocket.Conn
	err        error
	eventsChan chan *RTDSEvent
}

var _ RTDSConn = (*wsRTDSConn)(nil)

// SendSubscriptionUpdate 更新订阅
func (w *wsRTDSConn) SendSubscriptionUpdate(_ context.Context, req *RTDSSubscriptionUpdateRequest) error {
	return w.conn.WriteJSON(req)
}

// pingLoop 发送 PING 的循环
func (w *wsRTDSConn) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
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
func (w *wsRTDSConn) receiveLoop(ctx context.Context) {
	logger := logr.FromContextOrDiscard(ctx).WithName("rtds-conn")

	logger.V(1).Info("rtds receive loop started")

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
			logger.V(1).Info("rtds receive loop done")
			return
		default:
		}

		_, raw, err := w.conn.ReadMessage()
		if err != nil {
			if w.err == nil {
				w.err = fmt.Errorf("read message error: %w", err)
			}
			logger.V(1).Error(err, "read message from rtds error")
			return
		}
		logger.V(1).Info(fmt.Sprintf("received message: %s", string(raw)))

		e := RTDSEvent{}
		if err := json.Unmarshal(raw, &e); err != nil {
			logger.V(1).Error(err, "unmarshal rtds event from json error")
			continue
		}

		select {
		case <-ctx.Done():
			if w.err == nil {
				w.err = ctx.Err()
			}
			logger.V(1).Info("rtds receive loop done")
			return
		case w.eventsChan <- &e:
		}
	}
}

// Channel 获取接收事件的通道
func (w *wsRTDSConn) Channel() <-chan *RTDSEvent {
	return w.eventsChan
}

// Err 获取运行错误
func (w *wsRTDSConn) Err() error {
	return w.err
}

// RTDSSubscriptionUpdateRequest RTDS 订阅更新请求
type RTDSSubscriptionUpdateRequest struct {
	Action        string             `json:"action"`
	Subscriptions []RTDSSubscription `json:"subscriptions"`
}

const (
	RTDSActionSubscribe   = "subscribe"
	RTDSActionUnsubscribe = "unsubscribe"
)

// RTDSSubscription RTDS 订阅
type RTDSSubscription struct {
	Topic     string         `json:"topic"`
	Type      string         `json:"type"`
	Filters   string         `json:"filters"`
	GammaAuth *RTDSGammaAuth `json:"gamma_auth,omitempty"`
}

// RTDSGammaAuth RTDS Gamma 认证
type RTDSGammaAuth struct {
	Address string `json:"address,omitempty"`
}

// RTDSEvent RTDS 事件
type RTDSEvent struct {
	// 主题
	Topic string `json:"topic"`
	// 事件类型
	Type string `json:"type"`
	// 时间戳（毫秒）
	Timestamp int64 `json:"timestamp"`
	// 载荷
	Payload RTDSEventPayload `json:"payload"`
}

// RTDSEventPayload RTDS 事件载荷
type RTDSEventPayload struct {
	// 代号
	Symbol string `json:"symbol"`
	// 时间戳（毫秒）
	Timestamp int64 `json:"timestamp"`
	// 值
	Value float64 `json:"value"`
}
