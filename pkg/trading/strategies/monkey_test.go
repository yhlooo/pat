package strategies

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/yhlooo/pat/pkg/trading"
)

func BenchmarkMonkeyExecute(b *testing.B) {
	ctx := context.Background()

	status := trading.Status{
		CurrentMarket: trading.Market{
			EndDate: time.Now().Add(5 * time.Minute),
		},
		Prices: trading.MarketPrices{
			Yes: trading.AssetPrices{
				Last: decimal.NewFromFloat(0.5), BestBid: decimal.NewFromFloat(0.49), BestAsk: decimal.NewFromFloat(0.51),
			},
			No: trading.AssetPrices{
				Last: decimal.NewFromFloat(0.5), BestBid: decimal.NewFromFloat(0.49), BestAsk: decimal.NewFromFloat(0.51),
			},
		},
		PendingOrders: map[string]trading.Order{
			"order1": {ID: "order1", Side: trading.Buy, Price: decimal.NewFromFloat(0.5), CreatedAt: time.Now()},
		},
		Holding: map[string]*trading.Asset{
			"asset1": {
				ID: "asset1", Type: trading.Yes, TradableQuantity: decimal.NewFromFloat(10), Price: decimal.NewFromFloat(0.5),
			},
		},
	}

	b.Run("IntervalSkip", func(b *testing.B) {
		m := NewMonkey()
		m.lastTrade = time.Now() // 设置为当前时间，间隔检查会直接跳过
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, _ = m.Execute(ctx, status)
		}
	})

	b.Run("TriggerTrade", func(b *testing.B) {
		m := NewMonkey()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			m.lastTrade = time.Time{} // 重置以绕过间隔控制
			b.StartTimer()
			_, _, _ = m.Execute(ctx, status)
		}
	})
}
