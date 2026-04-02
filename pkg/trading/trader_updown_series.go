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

	curAssetIDs, err := curMarket.GetCLOBTokenIDs()
	if err != nil {
		logger.Error(err, fmt.Sprintf("get sub assets ids for market %q error", curMarket.Slug))
		return
	}

	// 开始监听市场
	logger.Info(fmt.Sprintf("watching market: %s (%s)", curMarket.Slug, curMarket.ConditionID))
	marketWatcher := polymarket.NewMarketWatcher(trader.client, curAssetIDs[:])
	marketWatcher.Start(ctx)
	defer func() { _ = marketWatcher.Close() }()

	status := Status{
		MarketSlug: curMarket.Slug,
		Prices: MarketPrices{
			Outcome1: AssetPrices{
				BestBid: decimal.New(51, -2),
				BestAsk: decimal.New(50, -2),
				Last:    decimal.New(50, -2),
			},
			Outcome2: AssetPrices{
				BestBid: decimal.New(50, -2),
				BestAsk: decimal.New(49, -2),
				Last:    decimal.New(49, -2),
			},
		},
	}

	for {
		select {
		case <-ctx.Done():
			return
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
					case curAssetIDs[0]:
						status.Prices.Outcome1.BestAsk = change.BestAsk
						status.Prices.Outcome1.BestBid = change.BestBid
					case curAssetIDs[1]:
						status.Prices.Outcome2.BestAsk = change.BestAsk
						status.Prices.Outcome2.BestBid = change.BestBid
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
				case curAssetIDs[0]:
					status.Prices.Outcome1.Last = eventData.Price
				case curAssetIDs[1]:
					status.Prices.Outcome2.Last = eventData.Price
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
				resolvedAssetIDs, _ := market.GetCLOBTokenIDs()
				marketWatcher.Unsubscribe(ctx, resolvedAssetIDs[:]...)

			default:
				continue
			}

			// 发送当前状态
			select {
			case trader.statusChan <- status:
			default:
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
			trader.lock.Unlock()
			curAssetIDs, err = curMarket.GetCLOBTokenIDs()
			if err != nil {
				logger.Error(err, fmt.Sprintf("get sub assets ids for market %q error", curMarket.Slug))
				return
			}
			marketWatcher.Subscribe(ctx, curAssetIDs[:]...)
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
	return s.Slug() + "-" + strconv.FormatInt(t.Round(s.Interval).Unix(), 10)
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
)
