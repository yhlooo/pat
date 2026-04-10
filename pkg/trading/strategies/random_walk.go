package strategies

import (
	"context"
	"math"
	"time"

	"github.com/shopspring/decimal"

	"github.com/yhlooo/pat/pkg/trading"
)

// NewRandomWalk 创建随机游走策略
func NewRandomWalk() *RandomWalk {
	return &RandomWalk{}
}

// RandomWalk 随机游走策略
//
// 基于随机游走数学模型计算涨跌概率，与市场价格比较进行套利交易
type RandomWalk struct {
	lastTradeTime time.Time       // 上次交易时间
	lastExecTime  time.Time       // 上次执行时间
	lastPrice     decimal.Decimal // 上次执行时的资产价格
}

var _ trading.Strategy = (*RandomWalk)(nil)

// Execute 执行策略
func (s *RandomWalk) Execute(_ context.Context, status trading.Status) ([]trading.Action, map[string]interface{}, error) {
	now := time.Now()
	meta := make(map[string]interface{})

	// 市场结束前 30 秒内不交易
	remainingTime := status.CurrentMarket.EndDate.Sub(now)
	if remainingTime <= 30*time.Second {
		return nil, meta, nil
	}

	// 获取必要数据
	S0 := status.ResolutionSource.Value      // 当前资产价格
	K := status.ResolutionSource.TargetValue // 市场基准价格
	yesPrice := status.Prices.Yes.Last       // Yes 市场价格

	// 数据缺失时不交易
	if S0.IsZero() || K.IsZero() || yesPrice.IsZero() {
		return nil, meta, nil
	}
	// 排除极端值
	if yesPrice.LessThan(decimal.NewFromFloat(0.04)) || yesPrice.GreaterThan(decimal.NewFromFloat(0.96)) {
		return nil, meta, nil
	}

	// 计算剩余秒数
	n := float64(int(remainingTime.Seconds()))

	// 估算波动率
	if s.lastExecTime.IsZero() {
		// 首次执行，记录当前状态，不交易
		s.lastExecTime = now
		s.lastPrice = S0
		return nil, meta, nil
	}

	timeDiff := now.Sub(s.lastExecTime).Seconds()
	if timeDiff <= 0 {
		return nil, meta, nil
	}

	sigma := S0.Sub(s.lastPrice).Abs().Div(decimal.NewFromFloat(timeDiff))
	if sigma.IsZero() {
		// 价格未变化，更新记录但不交易
		s.lastExecTime = now
		s.lastPrice = S0
		return nil, meta, nil
	}

	// 计算概率 P(Yes) = 1 - Φ((K - S0) / (σ * √n))
	x := K.Sub(S0).Div(sigma.Mul(decimal.NewFromFloat(math.Sqrt(n)))).InexactFloat64()
	pYes := 1.0 - normalCDF(x)
	meta["PYes"] = pYes

	// 更新执行记录
	s.lastExecTime = now
	s.lastPrice = S0

	// 交易间隔控制
	if !s.lastTradeTime.IsZero() && now.Sub(s.lastTradeTime) < 30*time.Second {
		return nil, meta, nil
	}

	// 计算偏差
	deviation := decimal.NewFromFloat(pYes).Sub(yesPrice)

	// 偏差阈值
	threshold := decimal.NewFromFloat(0.2)

	// 滑点保护常量
	slippage := decimal.NewFromFloat(0.2)

	var actions []trading.Action

	switch {
	case deviation.GreaterThan(threshold):
		// Yes 被低估，应买入 Yes 或卖出 No
		noHolding := s.findHolding(status, trading.No)
		if noHolding != nil && noHolding.TradableQuantity.GreaterThan(decimal.Zero) {
			// 有 No 持仓，优先卖出
			actions = append(actions, trading.Action{CreateOrder: &trading.CreateOrder{
				TokenType: trading.No,
				Side:      trading.Sell,
				Type:      trading.FOK,
				Price:     status.Prices.No.BestBid.Sub(slippage),
				Quantity:  noHolding.TradableQuantity,
			}})
		} else {
			// 无 No 持仓，买入 Yes
			actions = append(actions, trading.Action{CreateOrder: &trading.CreateOrder{
				TokenType: trading.Yes,
				Side:      trading.Buy,
				Type:      trading.FOK,
				Price:     status.Prices.Yes.BestAsk.Add(slippage),
				Amount:    decimal.NewFromInt(1),
			}})
		}

	case deviation.LessThan(threshold.Neg()):
		// Yes 被高估，应卖出 Yes 或买入 No
		yesHolding := s.findHolding(status, trading.Yes)
		if yesHolding != nil && yesHolding.TradableQuantity.GreaterThan(decimal.Zero) {
			// 有 Yes 持仓，优先卖出
			actions = append(actions, trading.Action{CreateOrder: &trading.CreateOrder{
				TokenType: trading.Yes,
				Side:      trading.Sell,
				Type:      trading.FOK,
				Price:     status.Prices.Yes.BestBid.Sub(slippage),
				Quantity:  yesHolding.TradableQuantity,
			}})
		} else {
			// 无 Yes 持仓，买入 No
			actions = append(actions, trading.Action{CreateOrder: &trading.CreateOrder{
				TokenType: trading.No,
				Side:      trading.Buy,
				Type:      trading.FOK,
				Price:     status.Prices.No.BestAsk.Add(slippage),
				Amount:    decimal.NewFromInt(1),
			}})
		}
	}

	if len(actions) > 0 {
		s.lastTradeTime = now
	}

	return actions, meta, nil
}

// findHolding 查找指定类型的持仓
func (s *RandomWalk) findHolding(status trading.Status, tokenType trading.TokenType) *trading.Asset {
	for _, holding := range status.Holding {
		if holding.Type == tokenType {
			return holding
		}
	}
	return nil
}

// normalCDF 标准正态分布累积分布函数
//
// Φ(x) = 0.5 * (1 + erf(x / √2))
func normalCDF(x float64) float64 {
	return 0.5 * (1.0 + math.Erf(x/math.Sqrt2))
}
