package polymarket

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// NewMarketWatcher 创建市场监听器
func NewMarketWatcher(client CLOBReaderClient, assetIDs []string) *MarketWatcher {
	w := &MarketWatcher{
		client:        client,
		watchAssetIDs: make(map[string]struct{}, len(assetIDs)),
	}
	for _, assetID := range assetIDs {
		w.watchAssetIDs[assetID] = struct{}{}
	}
	return w
}

// MarketWatcher 市场监听器
type MarketWatcher struct {
	ctx    context.Context
	cancel context.CancelFunc

	lock          sync.RWMutex
	client        CLOBReaderClient
	conn          CLOBChannelConn
	closed        bool
	watchAssetIDs map[string]struct{}

	outChan chan *CLOBEvent
}

// Start 启动
func (w *MarketWatcher) Start(ctx context.Context) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.outChan = make(chan *CLOBEvent)

	go w.runLoop(w.ctx)
}

// runLoop 运行循环
func (w *MarketWatcher) runLoop(ctx context.Context) {
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
		conn, err := w.client.ConnectMarketChannel(ctx)
		if err != nil {
			logger.Error(err, "connect market channel error")
			continue
		}
		w.lock.Lock()

		w.conn = conn

		// 订阅资产
		assetIDs := make([]string, 0, len(w.watchAssetIDs))
		for id := range w.watchAssetIDs {
			assetIDs = append(assetIDs, id)
		}

		w.lock.Unlock()

		if err := conn.SendSubscriptionRequest(ctx, &CLOBSubscriptionRequest{
			AssetIDs:             assetIDs,
			Type:                 SubTypeMarket,
			CustomFeatureEnabled: true,
		}); err != nil {
			logger.Error(err, "subscription markets error")
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
			logger.Error(err, "watch market channel error")
		}
	}
}

// Channel 获取接收市场事件的通道
func (w *MarketWatcher) Channel() <-chan *CLOBEvent {
	return w.outChan
}

// Close 关闭
func (w *MarketWatcher) Close() error {
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
func (w *MarketWatcher) Subscribe(ctx context.Context, assetIDs ...string) {
	if w.closed {
		return
	}

	w.lock.Lock()
	for _, assetID := range assetIDs {
		w.watchAssetIDs[assetID] = struct{}{}
	}
	w.lock.Unlock()

	logger := logr.FromContextOrDiscard(ctx)
	for i := 0; i < 3; i++ {
		if w.closed {
			return
		}
		if err := w.conn.SendSubscriptionUpdate(ctx, &CLOBSubscriptionUpdateRequest{
			Operation:            OpSubscribe,
			AssetIDs:             assetIDs,
			CustomFeatureEnabled: true,
		}); err != nil {
			logger.Error(err, fmt.Sprintf("send subscription update error (retries: %d)", i))
			time.Sleep(time.Second)
			continue
		}
		break
	}
}

// Unsubscribe 取消订阅
func (w *MarketWatcher) Unsubscribe(ctx context.Context, assetIDs ...string) {
	if w.closed {
		return
	}

	w.lock.Lock()
	for _, assetID := range assetIDs {
		delete(w.watchAssetIDs, assetID)
	}
	w.lock.Unlock()

	logger := logr.FromContextOrDiscard(ctx)
	for i := 0; i < 3; i++ {
		if w.closed {
			return
		}
		if err := w.conn.SendSubscriptionUpdate(ctx, &CLOBSubscriptionUpdateRequest{
			Operation:            OpUnsubscribe,
			AssetIDs:             assetIDs,
			CustomFeatureEnabled: true,
		}); err != nil {
			logger.Error(err, "send subscription update error (retries: %d)", i)
			time.Sleep(time.Second)
			continue
		}
		break
	}
}
