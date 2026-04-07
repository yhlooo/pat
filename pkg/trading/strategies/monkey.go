package strategies

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/shopspring/decimal"

	"github.com/yhlooo/pat/pkg/trading"
)

// NewMonkey 创建猴子策略
func NewMonkey() *Monkey {
	return &Monkey{
		rand:     rand.New(rand.NewPCG(uint64(time.Now().Unix()), uint64(time.Now().Unix()))),
		interval: 10 * time.Second,
	}
}

// Monkey 猴子策略
//
// 每间隔一个固定的时间就随机进行一个交易操作
type Monkey struct {
	rand      *rand.Rand
	interval  time.Duration
	lastTrade time.Time
}

var _ trading.Strategy = (*Monkey)(nil)

// Execute 执行策略
func (m *Monkey) Execute(_ context.Context, status trading.Status) ([]trading.Action, error) {
	if time.Since(m.lastTrade) < m.interval {
		return nil, nil
	}
	m.lastTrade = time.Now()

	cancelWeight := len(status.PendingOrders)
	sellWeight := len(status.Holding)
	choice := m.rand.IntN(2 + cancelWeight + sellWeight)

	// 市价买入 $1 Yes
	if choice == 0 {
		return []trading.Action{{CreateOrder: &trading.CreateOrder{
			TokenType: trading.Yes,
			Side:      trading.Buy,
			Type:      trading.FOK,
			Price:     decimal.NewFromFloat(0.99),
			Amount:    decimal.NewFromFloat(1),
		}}}, nil
	}
	// 市价买入 $1 No
	if choice == 1 {
		return []trading.Action{{CreateOrder: &trading.CreateOrder{
			TokenType: trading.No,
			Side:      trading.Buy,
			Type:      trading.FOK,
			Price:     decimal.NewFromFloat(0.99),
			Amount:    decimal.NewFromFloat(1),
		}}}, nil
	}
	// 取消订单 * n
	if choice < 2+cancelWeight {
		return []trading.Action{{CancelOrder: &trading.CancelOrder{
			ID: status.GetPendingOrderList()[choice-2].ID,
		}}}, nil
	}
	// 卖出持仓 * n
	holding := status.GetHoldingList()[choice-2-cancelWeight]
	return []trading.Action{{CreateOrder: &trading.CreateOrder{
		TokenType: holding.Type,
		Side:      trading.Sell,
		Type:      trading.FOK,
		Price:     decimal.NewFromFloat(0.01),
		Quantity:  holding.TradableQuantity,
	}}}, nil
}
