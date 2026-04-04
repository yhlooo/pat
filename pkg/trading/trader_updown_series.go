package trading

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/shopspring/decimal"

	"github.com/yhlooo/pat/pkg/polymarket"
)

// NewUpdownSeriesTrader 创建涨或跌系列交易员
func NewUpdownSeriesTrader(
	series UpdownSeries,
	client polymarket.ClientInterface,
	opts TraderOptions,
) *UpdownSeriesTrader {
	return &UpdownSeriesTrader{
		opts:   opts,
		client: client,
		series: series,
	}
}

// UpdownSeriesTrader “某资产涨或跌”市场的交易员
type UpdownSeriesTrader struct {
	lock sync.RWMutex

	opts TraderOptions

	client     polymarket.ClientInterface
	series     UpdownSeries
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
	curMarket := trader.curMarket
	trader.lock.RUnlock()

	// 未判定的市场
	// key 是市场 condition id
	notResolvedMarkets := map[string]*polymarket.Market{
		curMarket.ConditionID: curMarket,
	}

	yesAssetID, noAssetID, err := curMarket.GetCLOBTokenIDs()
	if err != nil {
		logger.Error(err, fmt.Sprintf("get sub assets ids for market %q error", curMarket.Slug))
		return
	}

	status := Status{
		MarketSlug: curMarket.Slug,
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
			URL: curMarket.ResolutionSource,
		},
	}

	// 开始监听市场
	logger.Info(fmt.Sprintf("watching market: %s (%s)", curMarket.Slug, curMarket.ConditionID))
	marketWatcher := polymarket.NewMarketWatcher(trader.client, []string{yesAssetID, noAssetID})
	marketWatcher.Start(ctx)
	defer func() { _ = marketWatcher.Close() }()

	// 开始监听 rtds
	var rtdsWatcherChan <-chan *polymarket.RTDSEvent
	resolutionSourceSymbol := trader.series.ResolutionSourceSymbol()
	if resolutionSourceSymbol != "" {
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

			if event.Payload.Symbol != resolutionSourceSymbol {
				logger.V(1).Info(fmt.Sprintf(
					"ignore unknown symbol price change, symbol: %s",
					event.Payload.Symbol,
				))
				continue
			}
			status.ResolutionSource.Value = decimal.NewFromFloat(event.Payload.Value)

		case event, ok := <-marketWatcher.Channel():
			if !ok {
				err := fmt.Errorf("market watcher channel closed")
				logger.Error(err, err.Error())
				return
			}

			// 处理市场事件
			switch event.EventType {
			case polymarket.EventPriceChange:
				// 下单/取消导致出价或要价变化
				eventData := event.PriceChange
				if eventData.Market != curMarket.ConditionID {
					logger.V(1).Info(fmt.Sprintf(
						"ignore not current market price change, market: %s",
						eventData.Market,
					))
					continue
				}
				for _, change := range eventData.PriceChanges {
					switch change.AssetID {
					case yesAssetID:
						status.Prices.Yes.BestAsk = change.BestAsk
						status.Prices.Yes.BestBid = change.BestBid
					case noAssetID:
						status.Prices.No.BestAsk = change.BestAsk
						status.Prices.No.BestBid = change.BestBid
					}
				}

			case polymarket.EventLastTradePrice:
				// 成交导致最近成交价变化
				eventData := event.LastTradePrice
				if eventData.Market != curMarket.ConditionID {
					logger.V(1).Info(fmt.Sprintf(
						"ignore not current market last trade price, market: %s",
						eventData.Market,
					))
					continue
				}
				switch eventData.AssetID {
				case yesAssetID:
					status.Prices.Yes.Last = eventData.Price
				case noAssetID:
					status.Prices.No.Last = eventData.Price
				}

			case polymarket.EventMarketResolved:
				// 市场判定
				eventData := event.MarketResolved
				market, ok := notResolvedMarkets[eventData.Market]
				if !ok {
					logger.V(1).Info(fmt.Sprintf(
						"ignore not watching market resolved, market: %s",
						eventData.Market,
					))
					continue
				}
				// TODO: 结算持仓
				delete(notResolvedMarkets, market.ConditionID)
				resolvedYes, resolvedNo, _ := market.GetCLOBTokenIDs()
				marketWatcher.Unsubscribe(ctx, resolvedYes, resolvedNo)

			default:
				continue
			}

		case <-ticker.C:
			// 检查是否需要订阅新市场
			newMarketSlug := trader.series.ActiveMarketSlugForTime(time.Now())
			if newMarketSlug == curMarket.Slug {
				continue
			}
			newMarket, err := trader.client.GetMarketBySlug(ctx, newMarketSlug)
			if err != nil {
				logger.Error(err, fmt.Sprintf("get market %q error", newMarketSlug))
				continue
			}

			trader.lock.Lock()
			notResolvedMarkets[newMarket.ConditionID] = newMarket
			curMarket = newMarket
			trader.curMarket = curMarket
			trader.lock.Unlock()
			status.MarketSlug = newMarket.Slug
			yesAssetID, noAssetID, err = curMarket.GetCLOBTokenIDs()
			if err != nil {
				logger.Error(err, fmt.Sprintf("get sub assets ids for market %q error", curMarket.Slug))
				return
			}
			marketWatcher.Subscribe(ctx, yesAssetID, noAssetID)
		}

		// 发送当前状态
		select {
		case trader.statusChan <- status:
		default:
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
