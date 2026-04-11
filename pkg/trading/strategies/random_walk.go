package strategies

import (
	"context"
	"math"
	"time"

	"github.com/shopspring/decimal"

	"github.com/yhlooo/pat/pkg/trading"
)

// ModelType 随机游走模型类型
type ModelType int

const (
	// Arithmetic 算术随机游走（绝对波动）
	Arithmetic ModelType = iota
	// Geometric 几何随机游走（相对波动）
	Geometric
)

// NewRandomWalk 创建随机游走策略
func NewRandomWalk(model ModelType) *RandomWalk {
	return &RandomWalk{model: model}
}

// pricePoint 价格观测点
type pricePoint struct {
	time  time.Time
	price decimal.Decimal
}

// RandomWalk 随机游走策略
//
// 基于随机游走数学模型计算涨跌概率，与市场价格比较进行套利交易。
// 支持算术随机游走（绝对波动）和几何随机游走（相对波动）两种模型。
type RandomWalk struct {
	model         ModelType    // 随机游走模型类型
	lastTradeTime time.Time    // 上次交易时间
	priceHistory  []pricePoint // 近期价格观测点（仅保留近 1 分钟）
}

var _ trading.Strategy = (*RandomWalk)(nil)

// Execute 执行策略
func (s *RandomWalk) Execute(_ context.Context, status trading.Status) ([]trading.Action, map[string]interface{}, error) {
	// 数据延迟超过 2s 不交易
	if status.MarketChannelDelay > 2*time.Second || status.RTDSDelay > 2*time.Second {
		return nil, nil, nil
	}

	// 获取必要数据
	S0 := status.ResolutionSource.Value      // 当前资产价格
	K := status.ResolutionSource.TargetValue // 市场基准价格
	yesPrice := status.Prices.Yes.Last       // Yes 市场价格

	// 数据缺失时不交易
	if S0.IsZero() || K.IsZero() || yesPrice.IsZero() {
		return nil, nil, nil
	}

	now := time.Now()

	// 市场结束前 10 秒内不交易
	remainingTime := status.CurrentMarket.EndDate.Sub(now)
	if remainingTime <= 10*time.Second {
		return nil, nil, nil
	}

	// 几何模式下价格必须为正值（ln 要求 S > 0）
	if s.model == Geometric && (S0.LessThanOrEqual(decimal.Zero) || K.LessThanOrEqual(decimal.Zero)) {
		return nil, nil, nil
	}
	// 排除极端值
	if yesPrice.LessThan(decimal.NewFromFloat(0.04)) || yesPrice.GreaterThan(decimal.NewFromFloat(0.96)) {
		return nil, nil, nil
	}

	// 计算剩余秒数
	n := float64(int(remainingTime.Seconds()))

	// 记录当前观测点
	s.priceHistory = append(s.priceHistory, pricePoint{time: now, price: S0})

	// 清理过期数据（仅保留近 1 分钟）
	cutoff := now.Add(-time.Minute)
	i := 0
	for i < len(s.priceHistory) && s.priceHistory[i].time.Before(cutoff) {
		i++
	}
	s.priceHistory = s.priceHistory[i:]

	// 至少需要 2 个观测点才能估算波动率
	if len(s.priceHistory) < 2 {
		return nil, nil, nil
	}

	// 使用 realized variance 估算波动率
	// Arithmetic: σ = √( (1/m) * Σ( (Sᵢ - Sᵢ₋₁)² / (tᵢ - tᵢ₋₁) ) )
	// Geometric:  σ = √( (1/m) * Σ( (ln(Sᵢ/Sᵢ₋₁))² / (tᵢ - tᵢ₋₁) ) )
	var sumSquaredReturns float64
	validPairs := 0
	for j := 1; j < len(s.priceHistory); j++ {
		dt := s.priceHistory[j].time.Sub(s.priceHistory[j-1].time).Seconds()
		if dt <= 0 {
			continue
		}
		var ds float64
		if s.model == Geometric {
			// 几何模式：对数收益率，跳过非正值观测点
			prevPrice := s.priceHistory[j-1].price
			currPrice := s.priceHistory[j].price
			if !prevPrice.IsPositive() || !currPrice.IsPositive() {
				continue
			}
			ds = math.Log(currPrice.Div(prevPrice).InexactFloat64())
		} else {
			// 算术模式：绝对价格差
			ds = s.priceHistory[j].price.Sub(s.priceHistory[j-1].price).InexactFloat64()
		}
		sumSquaredReturns += ds * ds / dt
		validPairs++
	}

	if validPairs == 0 {
		return nil, nil, nil
	}

	sigma := math.Sqrt(sumSquaredReturns / float64(validPairs))
	if sigma == 0 {
		return nil, nil, nil
	}

	// 预估波动幅度 = σ * √n （剩余时间内的预期价格波动标准差）
	estimatedAmplitude := sigma * math.Sqrt(n)

	// 计算概率 P(Yes) = 1 - Φ((transform(K) - transform(S0)) / (σ * √n))
	// Arithmetic: transform(S) = S（恒等变换）
	// Geometric:  transform(S) = ln(S)（对数变换）
	var s0Transformed, kTransformed float64
	if s.model == Geometric {
		s0Transformed = math.Log(S0.InexactFloat64())
		kTransformed = math.Log(K.InexactFloat64())
	} else {
		s0Transformed = S0.InexactFloat64()
		kTransformed = K.InexactFloat64()
	}
	x := (kTransformed - s0Transformed) / estimatedAmplitude
	pYes := 1.0 - normalCDF(x)

	meta := map[string]interface{}{
		"EstimatedAmplitude": estimatedAmplitude,
		"PYes":               pYes,
	}

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
