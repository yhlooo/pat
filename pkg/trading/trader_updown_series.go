package trading

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/yhlooo/pat/pkg/polymarket"
)

// NewUpdownSeriesTrader 创建涨或跌系列交易员
func NewUpdownSeriesTrader(
	series UpdownSeries,
	client polymarket.ClientInterface,
	strategy Strategy,
	opts TraderOptions,
) *UpdownSeriesTrader {
	return &UpdownSeriesTrader{
		opts:     opts,
		client:   client,
		series:   series,
		strategy: strategy,
	}
}

// UpdownSeriesTrader “某资产涨或跌”市场的交易员
type UpdownSeriesTrader struct {
	lock sync.RWMutex

	opts TraderOptions

	client   polymarket.ClientInterface
	series   UpdownSeries
	strategy Strategy

	done       chan struct{}
	statusChan chan Status

	curMarket *polymarket.Market
}

var _ Trader = (*UpdownSeriesTrader)(nil)

// Start 开始交易
func (trader *UpdownSeriesTrader) Start(ctx context.Context) error {
	// 获取第一个市场
	curMarket, err := trader.client.GetMarketBySlug(ctx, trader.series.ActiveMarketSlugForTime(time.Now()))
	if err != nil {
		return fmt.Errorf("get current market for series %q error: %w", trader.series.Slug(), err)
	}

	trader.lock.Lock()
	defer trader.lock.Unlock()

	trader.curMarket = curMarket
	trader.done = make(chan struct{})
	trader.statusChan = make(chan Status, 32)

	go trader.runLoop(ctx)

	return nil
}

// marketChannelReceiveLoop 市场
func (trader *UpdownSeriesTrader) runLoop(ctx context.Context) {
	logger := logr.FromContextOrDiscard(ctx)
	ticker := time.NewTicker(time.Second)
	defer func() {
		close(trader.statusChan)
		ticker.Stop()
		close(trader.done)
	}()

	trader.lock.RLock()
	yesTokenID, noTokenID, err := trader.curMarket.GetCLOBTokenIDs()
	if err != nil {
		logger.Error(err, fmt.Sprintf("get sub assets ids for market %q error", trader.curMarket.Slug))
		trader.lock.RUnlock()
		return
	}
	curMarket := Market{
		Slug:        trader.curMarket.Slug,
		ConditionID: trader.curMarket.ConditionID,
		YesTokenID:  yesTokenID,
		NoTokenID:   noTokenID,
	}
	status := Status{
		CurrentMarket: curMarket,
		WatchingMarkets: map[string]Market{
			curMarket.ConditionID: curMarket,
		},
		Prices: MarketPrices{
			Yes: AssetPrices{
				BestBid: decimal.New(51, -2),
				BestAsk: decimal.New(50, -2),
				Last:    decimal.New(50, -2),
			},
			No: AssetPrices{
				BestBid: decimal.New(50, -2),
				BestAsk: decimal.New(49, -2),
				Last:    decimal.New(49, -2),
			},
		},
		ResolutionSource: ResolutionSource{
			URL: trader.curMarket.ResolutionSource,
		},
	}
	trader.lock.RUnlock()

	// 开始监听市场
	logger.Info(fmt.Sprintf("watching market: %s (%s)", status.CurrentMarket.Slug, status.CurrentMarket.ConditionID))
	marketWatcher := polymarket.NewMarketWatcher(
		trader.client,
		[]string{status.CurrentMarket.YesTokenID, status.CurrentMarket.NoTokenID},
	)
	marketWatcher.Start(ctx)
	defer func() { _ = marketWatcher.Close() }()

	// 开始监听 rtds
	var rtdsWatcherChan <-chan *polymarket.RTDSEvent
	status.ResolutionSource.Symbol = trader.series.ResolutionSourceSymbol()
	if status.ResolutionSource.Symbol != "" {
		rtdsWatcher := polymarket.NewRTDSWatcher(trader.client, []polymarket.RTDSSubscription{
			trader.series.ResolutionSourceSubscription(),
		})
		rtdsWatcher.Start(ctx)
		defer func() { _ = rtdsWatcher.Close() }()

		rtdsWatcherChan = rtdsWatcher.Channel()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-rtdsWatcherChan:
			if !ok {
				err := fmt.Errorf("rtds watcher channel closed")
				logger.Error(err, err.Error())
				return
			}

			// 处理 RTDS 事件
			if ok := trader.handleRTDSEvent(ctx, &status, event); !ok {
				continue
			}

		case event, ok := <-marketWatcher.Channel():
			if !ok {
				err := fmt.Errorf("market watcher channel closed")
				logger.Error(err, err.Error())
				return
			}

			// 处理市场事件
			if ok := trader.handleMarketChannelEvent(ctx, &status, marketWatcher, event); !ok {
				continue
			}

		case <-ticker.C:
			trader.handleTicketEvent(ctx, &status, marketWatcher)
		}

		// 模拟撮合订单
		if trader.opts.DryRun {
			trader.simulateMatchingOrders(ctx, &status)
		}

		// 执行策略
		actions, err := trader.strategy.Execute(ctx, status)
		if err != nil {
			logger.Error(err, "execute strategy error")
		}
		// 按照策略执行结果行动
		trader.handleActions(ctx, &status, actions)

		// 再尝试模拟撮合新订单
		if trader.opts.DryRun {
			trader.simulateMatchingOrders(ctx, &status)
		}

		// 发送当前状态
		select {
		case trader.statusChan <- status:
		default:
		}
	}
}

// handleRTDSEvent 处理 RTDS 事件
func (trader *UpdownSeriesTrader) handleRTDSEvent(
	ctx context.Context,
	status *Status,
	event *polymarket.RTDSEvent,
) bool {
	logger := logr.FromContextOrDiscard(ctx)
	if event.Payload.Symbol != status.ResolutionSource.Symbol {
		logger.V(1).Info(fmt.Sprintf(
			"ignore unknown symbol price change, symbol: %s",
			event.Payload.Symbol,
		))
		return false
	}
	status.ResolutionSource.Value = decimal.NewFromFloat(event.Payload.Value)
	return true
}

// handleMarketChannelEvent 处理市场事件
func (trader *UpdownSeriesTrader) handleMarketChannelEvent(
	ctx context.Context,
	status *Status,
	watcher *polymarket.MarketWatcher,
	event *polymarket.CLOBEvent,
) bool {
	changed := false

	// 处理市场事件
	switch event.EventType {
	case polymarket.EventPriceChange:
		// 下单/取消导致出价或要价变化
		eventData := event.PriceChange
		if eventData.Market == status.CurrentMarket.ConditionID {
			for _, change := range eventData.PriceChanges {
				switch change.AssetID {
				case status.CurrentMarket.YesTokenID:
					status.Prices.Yes.BestAsk = change.BestAsk
					status.Prices.Yes.BestBid = change.BestBid
				case status.CurrentMarket.NoTokenID:
					status.Prices.No.BestAsk = change.BestAsk
					status.Prices.No.BestBid = change.BestBid
				}
			}
			changed = true
		}

	case polymarket.EventLastTradePrice:
		// 成交导致最近成交价变化
		eventData := event.LastTradePrice

		// 更新当前市场价格
		if eventData.Market == status.CurrentMarket.ConditionID {
			switch eventData.AssetID {
			case status.CurrentMarket.YesTokenID:
				status.Prices.Yes.Last = eventData.Price
			case status.CurrentMarket.NoTokenID:
				status.Prices.No.Last = eventData.Price
			}
			changed = true
		}

		// 更新持仓价值
		if holding := status.Holding[eventData.AssetID]; holding != nil {
			holding.Price = eventData.Price
			holding.Value = holding.Quantity.Mul(holding.Price).Round(6)
			changed = true
		}

	case polymarket.EventMarketResolved:
		// 市场判定
		eventData := event.MarketResolved

		// 更新订阅
		market, ok := status.WatchingMarkets[eventData.Market]
		if ok {
			delete(status.WatchingMarkets, market.ConditionID)
			watcher.Unsubscribe(ctx, market.YesTokenID, market.NoTokenID)
			changed = true
		}

		// 结算持仓
		winningAssetID := eventData.WinningAssetId
		for _, assetID := range eventData.AssetIDs {
			if assetID != winningAssetID {
				if holding := status.Holding[assetID]; holding != nil {
					changed = true
					delete(status.Holding, assetID)
				}
			}
		}
		if holding := status.Holding[winningAssetID]; holding != nil {
			changed = true
			delete(status.Holding, winningAssetID)
			status.Cash = status.Cash.Add(holding.Quantity)
		}

	default:
		return false
	}

	return changed
}

// handleTicketEvent 处理时钟事件
func (trader *UpdownSeriesTrader) handleTicketEvent(
	ctx context.Context,
	status *Status,
	watcher *polymarket.MarketWatcher,
) {
	// 轮转市场
	trader.rotateMarket(ctx, status, watcher)
}

// rotateMarket 轮转市场
func (trader *UpdownSeriesTrader) rotateMarket(
	ctx context.Context,
	status *Status,
	watcher *polymarket.MarketWatcher,
) bool {
	logger := logr.FromContextOrDiscard(ctx)

	newMarketSlug := trader.series.ActiveMarketSlugForTime(time.Now())
	if newMarketSlug == status.CurrentMarket.Slug {
		return false
	}

	newMarket, err := trader.client.GetMarketBySlug(ctx, newMarketSlug)
	if err != nil {
		logger.Error(err, fmt.Sprintf("get market %q error", newMarketSlug))
		return false
	}

	yesTokenID, noTokenID, err := newMarket.GetCLOBTokenIDs()
	if err != nil {
		logger.Error(err, fmt.Sprintf("get sub assets ids for market %q error", newMarket.Slug))
		return false
	}

	m := Market{
		Slug:        newMarket.Slug,
		ConditionID: newMarket.ConditionID,
		YesTokenID:  yesTokenID,
		NoTokenID:   noTokenID,
	}

	trader.lock.Lock()
	status.CurrentMarket = m
	status.WatchingMarkets[newMarket.ConditionID] = m
	trader.curMarket = newMarket
	trader.lock.Unlock()
	watcher.Subscribe(ctx, yesTokenID, noTokenID)

	return true
}

// handleActions 处理行动
func (trader *UpdownSeriesTrader) handleActions(ctx context.Context, status *Status, actions []Action) {
	logger := logr.FromContextOrDiscard(ctx)

	for _, action := range actions {
		if !trader.opts.DryRun {
			// TODO: 非 dry run 模式需要真实调用 PolyMarket 创建或取消订单，再记录结果
		}

		switch {
		case action.CreateOrder != nil:
			// 创建订单
			if status.PendingOrders == nil {
				status.PendingOrders = make(map[string]Order)
			}
			orderID := uuid.New().String() // TODO: 非 dry run 模式下应根据 PolyMarket 返回结果获取订单 ID
			tokenID := status.CurrentMarket.YesTokenID
			if action.CreateOrder.TokenType == No {
				tokenID = status.CurrentMarket.NoTokenID
			}
			o := Order{
				ID:         orderID,
				TokenType:  action.CreateOrder.TokenType,
				TokenID:    tokenID,
				MarketID:   status.CurrentMarket.ConditionID,
				MarketSlug: status.CurrentMarket.Slug,
				Side:       action.CreateOrder.Side,
				Type:       action.CreateOrder.Type,
				Price:      action.CreateOrder.Price,
				Quantity:   action.CreateOrder.Quantity,
				Amount:     action.CreateOrder.Amount,
				Expiration: action.CreateOrder.Expiration,
				CreatedAt:  time.Now(),
				State:      OrderPending,
			}

			if o.Side == Sell {
				// 扣库存
				holding, ok := status.Holding[o.TokenID]
				if !ok {
					logger.Info(fmt.Sprintf("WARN there are no %q for trading", o.TokenID))
					continue
				}
				if holding.TradableQuantity.LessThan(o.Quantity) {
					logger.Info(fmt.Sprintf("WARN there are not enough %q available for trading", o.TokenID))
					continue
				}
				holding.TradableQuantity = holding.TradableQuantity.Sub(o.Quantity)
			}
			status.PendingOrders[orderID] = o

		case action.CancelOrder != nil:
			// 取消订单
			if ok := status.CancelOrder(action.CancelOrder.ID, OrderCancelled); !ok {
				logger.Info(fmt.Sprintf("WARN order %q not found, can not be cancelled", action.CancelOrder.ID))
			}
		}
	}
}

// simulateMatchingOrders 模拟撮合订单
//
// 不考虑对手方订单量，仅看最近成交价、最佳出价、最佳要价是否满足订单
func (trader *UpdownSeriesTrader) simulateMatchingOrders(ctx context.Context, status *Status) {
	logger := logr.FromContextOrDiscard(ctx)

	now := time.Now()

	for id, order := range status.PendingOrders {
		// 取消非当前市场订单
		if order.MarketSlug != status.CurrentMarket.Slug {
			if ok := status.CancelOrder(id, OrderCancelled); !ok {
				logger.Info(fmt.Sprintf("WARN order %q not found, can not be cancelled", id))
			}
			continue
		}

		// 取消到期的 GTD 订单
		if order.Type == GTD && order.Expiration.Before(now) {
			if ok := status.CancelOrder(id, OrderCancelled); !ok {
				logger.Info(fmt.Sprintf("WARN order %q not found, can not be cancelled", id))
			}
			continue
		}

		curPrices := status.Prices.Yes
		if order.TokenType == No {
			curPrices = status.Prices.No
		}

		switch order.Type {
		case FOK, FAK:
			// 取消不满足条件的市价单
			if (order.Side == Buy && order.Price.LessThan(curPrices.BestAsk)) ||
				(order.Side == Sell && order.Price.GreaterThan(curPrices.BestBid)) {
				if ok := status.CancelOrder(id, OrderFailed); !ok {
					logger.Info(fmt.Sprintf("WARN order %q not found, can not be cancelled", id))
				}
				continue
			}

			// 剩余的直接以最佳出价/要价成交
			filledPrice := curPrices.BestAsk
			if order.Side == Sell {
				filledPrice = curPrices.BestBid
			}
			if filledPrice.IsZero() {
				continue
			}
			status.FillOrder(id, filledPrice, order.Amount.DivRound(filledPrice, 6))

		case GTC, GTD:
			filledPrice := curPrices.Last

			switch order.Side {
			case Buy:
				if curPrices.BestAsk.LessThan(filledPrice) {
					filledPrice = curPrices.BestAsk
				}
				if order.Price.GreaterThanOrEqual(filledPrice) {
					// 成交
					status.FillOrder(id, filledPrice, order.Quantity.Sub(order.FilledQuantity))
				}
			case Sell:
				if curPrices.BestBid.GreaterThan(filledPrice) {
					filledPrice = curPrices.BestBid
				}
				if order.Price.LessThanOrEqual(filledPrice) {
					// 成交
					status.FillOrder(id, filledPrice, order.Quantity.Sub(order.FilledQuantity))
				}
			}
		}
	}
}

// Market 返回交易员正在交易的市场信息
func (trader *UpdownSeriesTrader) Market() polymarket.Market {
	trader.lock.RLock()
	defer trader.lock.RUnlock()

	if trader.curMarket == nil {
		return polymarket.Market{}
	}
	return *trader.curMarket
}

// Channel 获取监听状态的 chan
func (trader *UpdownSeriesTrader) Channel() <-chan Status {
	return trader.statusChan
}

// Done 返回结束通知 chan
func (trader *UpdownSeriesTrader) Done() <-chan struct{} {
	return trader.done
}

// UpdownSeries 涨或跌系列
type UpdownSeries interface {
	// Slug 返回系列 slug
	Slug() string
	// ActiveMarketSlugForTime 获取指定时间活跃的市场 slug
	ActiveMarketSlugForTime(t time.Time) string
	// ResolutionSourceSymbol 判定来源资产代号
	ResolutionSourceSymbol() string
	// ResolutionSourceSubscription 判定来源资产 RTDS 订阅
	ResolutionSourceSubscription() polymarket.RTDSSubscription
}

// Updown5m15m 5 分钟或 15 分钟涨跌系列
type Updown5m15m struct {
	AssetName string
	Interval  time.Duration
}

// Slug 返回系列 slug
func (s Updown5m15m) Slug() string {
	return fmt.Sprintf("%s-updown-%dm", s.AssetName, s.Interval/time.Minute)
}

// ActiveMarketSlugForTime 获取指定时间活跃的市场 slug
func (s Updown5m15m) ActiveMarketSlugForTime(t time.Time) string {
	return s.Slug() + "-" + strconv.FormatInt(t.Unix()/int64(s.Interval/time.Second)*int64(s.Interval/time.Second), 10)
}

// ResolutionSourceSymbol 判定来源资产代号
func (s Updown5m15m) ResolutionSourceSymbol() string {
	switch s.AssetName {
	case "btc", "eth":
		return s.AssetName + "/usd"
	}
	// TODO: 其它暂不支持
	return ""
}

// ResolutionSourceSubscription 判定来源资产 RTDS 订阅
func (s Updown5m15m) ResolutionSourceSubscription() polymarket.RTDSSubscription {
	// TODO: 目前只确认了 btc 和 eth 是这样订阅
	return polymarket.RTDSSubscription{
		Topic:   "crypto_prices_chainlink",
		Type:    "*",
		Filters: fmt.Sprintf(`{"symbol":"%s/usd"}`, s.AssetName),
	}
}

var (
	// BTCUpdown5m btc-updown-5m
	BTCUpdown5m = Updown5m15m{AssetName: "btc", Interval: 5 * time.Minute}
	// BTCUpdown15m btc-updown-15m
	BTCUpdown15m = Updown5m15m{AssetName: "btc", Interval: 15 * time.Minute}
	// ETHUpdown5m eth-updown-5m
	ETHUpdown5m = Updown5m15m{AssetName: "eth", Interval: 5 * time.Minute}
	// ETHUpdown15m eth-updown-15m
	ETHUpdown15m = Updown5m15m{AssetName: "eth", Interval: 15 * time.Minute}

	// AllUpdownSeries 所有涨或跌系列
	AllUpdownSeries = map[string]UpdownSeries{
		BTCUpdown5m.Slug():  BTCUpdown5m,
		BTCUpdown15m.Slug(): BTCUpdown15m,
		ETHUpdown5m.Slug():  ETHUpdown5m,
		ETHUpdown15m.Slug(): ETHUpdown15m,
	}
)
