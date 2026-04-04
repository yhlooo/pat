package polymarket

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// NewRTDSWatcher 创建 RTDS 监听器
func NewRTDSWatcher(client RTDSClient, subscriptions []RTDSSubscription) *RTDSWatcher {
	w := &RTDSWatcher{
		client:        client,
		subscriptions: make(map[RTDSSubscription]struct{}, len(subscriptions)),
	}
	for _, sub := range subscriptions {
		w.subscriptions[sub] = struct{}{}
	}
	return w
}

// RTDSWatcher RTDS 监听器
type RTDSWatcher struct {
	ctx    context.Context
	cancel context.CancelFunc

	lock          sync.RWMutex
	client        RTDSClient
	conn          RTDSConn
	closed        bool
	subscriptions map[RTDSSubscription]struct{}

	outChan chan *RTDSEvent
}

// Start 启动
func (w *RTDSWatcher) Start(ctx context.Context) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.outChan = make(chan *RTDSEvent)

	go w.runLoop(w.ctx)
}

// runLoop 运行循环
func (w *RTDSWatcher) runLoop(ctx context.Context) {
	logger := logr.FromContextOrDiscard(ctx)
	waitTime := time.Duration(0)

	defer func() {
		close(w.outChan)
		w.cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(waitTime):
		}
		waitTime = time.Second

		// 连接 market channel
		conn, err := w.client.ConnectRTDS(ctx)
		if err != nil {
			logger.Error(err, "connect rtds error")
			continue
		}
		w.lock.Lock()
		w.conn = conn
		w.lock.Unlock()

		// 订阅
		subscriptions := make([]RTDSSubscription, 0, len(w.subscriptions))
		for sub := range w.subscriptions {
			subscriptions = append(subscriptions, sub)
		}
		if err := conn.SendSubscriptionUpdate(ctx, &RTDSSubscriptionUpdateRequest{
			Action:        RTDSActionSubscribe,
			Subscriptions: subscriptions,
		}); err != nil {
			logger.Error(err, "subscribe rtds error")
			continue
		}

		for event := range conn.Channel() {
			select {
			case <-ctx.Done():
				return
			case w.outChan <- event:
			}
		}

		if err := conn.Err(); err != nil {
			logger.Error(err, "watch rtds error")
		}
	}
}

// Channel 获取接收市场事件的通道
func (w *RTDSWatcher) Channel() <-chan *RTDSEvent {
	return w.outChan
}

// Close 关闭
func (w *RTDSWatcher) Close() error {
	w.lock.Lock()
	defer w.lock.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	if w.cancel != nil {
		w.cancel()
	}

	return nil
}

// Subscribe 添加订阅
func (w *RTDSWatcher) Subscribe(ctx context.Context, subscriptions ...RTDSSubscription) {
	if w.closed {
		return
	}

	w.lock.Lock()
	for _, sub := range subscriptions {
		w.subscriptions[sub] = struct{}{}
	}
	w.lock.Unlock()

	logger := logr.FromContextOrDiscard(ctx)
	for i := 0; i < 3; i++ {
		if w.closed {
			return
		}
		if err := w.conn.SendSubscriptionUpdate(ctx, &RTDSSubscriptionUpdateRequest{
			Action:        RTDSActionSubscribe,
			Subscriptions: subscriptions,
		}); err != nil {
			logger.Error(err, fmt.Sprintf("send subscription update error (retries: %d)", i))
			time.Sleep(time.Second)
			continue
		}
		break
	}
}

// Unsubscribe 取消订阅
func (w *RTDSWatcher) Unsubscribe(ctx context.Context, subscriptions ...RTDSSubscription) {
	if w.closed {
		return
	}

	w.lock.Lock()
	for _, sub := range subscriptions {
		delete(w.subscriptions, sub)
	}
	w.lock.Unlock()

	logger := logr.FromContextOrDiscard(ctx)
	for i := 0; i < 3; i++ {
		if w.closed {
			return
		}
		if err := w.conn.SendSubscriptionUpdate(ctx, &RTDSSubscriptionUpdateRequest{
			Action:        RTDSActionUnsubscribe,
			Subscriptions: subscriptions,
		}); err != nil {
			logger.Error(err, "send subscription update error (retries: %d)", i)
			time.Sleep(time.Second)
			continue
		}
		break
	}
}
