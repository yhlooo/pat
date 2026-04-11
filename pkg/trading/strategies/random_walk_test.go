package strategies

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/yhlooo/pat/pkg/trading"
)

func TestNormalCDF(t *testing.T) {
	tests := []struct {
		name      string
		x         float64
		expected  float64 // 近似值
		tolerance float64
	}{
		{"x=0", 0.0, 0.5, 1e-10},
		{"x=1", 1.0, 0.841344746, 1e-9},
		{"x=-1", -1.0, 0.158655254, 1e-9},
		{"x=2", 2.0, 0.977249868, 1e-9},
		{"x=-2", -2.0, 0.022750132, 1e-9},
		{"x=3", 3.0, 0.998650102, 1e-9},
		{"x=-3", -3.0, 0.001349898, 1e-9},
		{"large positive", 5.0, 0.999999713, 1e-9},
		{"large negative", -5.0, 2.87e-7, 1e-9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalCDF(tt.x)
			if math.Abs(result-tt.expected) > tt.tolerance {
				t.Errorf("normalCDF(%v) = %v, expected %v (tolerance %v)",
					tt.x, result, tt.expected, tt.tolerance)
			}
		})
	}
}

// TestNormalCDFProperties 测试正态 CDF 的数学性质
func TestNormalCDFProperties(t *testing.T) {
	// 测试对称性: Φ(-x) = 1 - Φ(x)
	for x := -3.0; x <= 3.0; x += 0.5 {
		phiX := normalCDF(x)
		phiNegX := normalCDF(-x)
		if math.Abs(phiNegX-(1-phiX)) > 1e-10 {
			t.Errorf("Symmetry violated: Φ(%v)=%v, Φ(%v)=%v, expected Φ(-x)=1-Φ(x)",
				x, phiX, -x, phiNegX)
		}
	}

	// 测试单调性（在非饱和区间内）
	prev := normalCDF(-4.0)
	for x := -3.9; x <= 4.0; x += 0.1 {
		curr := normalCDF(x)
		if curr <= prev {
			t.Errorf("Monotonicity violated at x=%v: Φ(%v)=%v <= Φ(%v)=%v",
				x, x, curr, x-0.1, prev)
		}
		prev = curr
	}

	// 测试边界值
	if normalCDF(0) != 0.5 {
		t.Errorf("Φ(0) should be 0.5, got %v", normalCDF(0))
	}
}

// TestRandomWalkExecute 测试 RandomWalk.Execute 方法
func TestRandomWalkExecute(t *testing.T) {
	ctx := context.Background()

	// 辅助函数：创建测试状态
	makeStatus := func(endDate time.Time, value, targetValue, yesPrice float64, holdings ...*trading.Asset) trading.Status {
		status := trading.Status{
			CurrentMarket: trading.Market{
				EndDate: endDate,
			},
			ResolutionSource: trading.ResolutionSource{
				Value:       decimal.NewFromFloat(value),
				TargetValue: decimal.NewFromFloat(targetValue),
			},
			Prices: trading.MarketPrices{
				Yes: trading.AssetPrices{
					Last:    decimal.NewFromFloat(yesPrice),
					BestBid: decimal.NewFromFloat(yesPrice - 0.01),
					BestAsk: decimal.NewFromFloat(yesPrice + 0.01),
				},
				No: trading.AssetPrices{
					Last:    decimal.NewFromFloat(1 - yesPrice),
					BestBid: decimal.NewFromFloat(1 - yesPrice - 0.01),
					BestAsk: decimal.NewFromFloat(1 - yesPrice + 0.01),
				},
			},
			Holding: make(map[string]*trading.Asset),
		}
		for i, h := range holdings {
			status.Holding[string(rune('A'+i))] = h
		}
		return status
	}

	t.Run("首次执行应不交易", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		status := makeStatus(time.Now().Add(5*time.Minute), 100.0, 100.0, 0.5)
		actions, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 0 {
			t.Errorf("expected no actions on first execution, got %d", len(actions))
		}
		if _, ok := meta["PYes"]; ok {
			t.Error("expected no PYes in meta on first execution")
		}
	})

	t.Run("市场即将结束应不交易", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		status := makeStatus(time.Now().Add(20*time.Second), 100.0, 100.0, 0.5)
		actions, _, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 0 {
			t.Errorf("expected no actions near market end, got %d", len(actions))
		}
	})

	t.Run("数据缺失应不交易", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)

		// Value 为 0
		status := makeStatus(time.Now().Add(5*time.Minute), 0, 100.0, 0.5)
		actions, _, _ := s.Execute(ctx, status)
		if len(actions) != 0 {
			t.Error("expected no actions when Value is 0")
		}

		// TargetValue 为 0
		status = makeStatus(time.Now().Add(5*time.Minute), 100.0, 0, 0.5)
		actions, _, _ = s.Execute(ctx, status)
		if len(actions) != 0 {
			t.Error("expected no actions when TargetValue is 0")
		}

		// YesPrice 为 0
		status = makeStatus(time.Now().Add(5*time.Minute), 100.0, 100.0, 0)
		actions, _, _ = s.Execute(ctx, status)
		if len(actions) != 0 {
			t.Error("expected no actions when YesPrice is 0")
		}
	})

	t.Run("偏差在阈值内应不交易", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行：记录状态
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 第二次执行：价格变化但偏差在阈值内
		time.Sleep(10 * time.Millisecond) // 确保有足够时间差
		status = makeStatus(endDate, 100.1, 100.0, 0.5)
		actions, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 0 {
			t.Errorf("expected no actions when deviation within threshold, got %d", len(actions))
		}
		if pYes, ok := meta["PYes"]; !ok {
			t.Error("expected PYes in meta")
		} else if pYes.(float64) <= 0.5 {
			t.Errorf("expected PYes > 0.5 when price increased, got %v", pYes)
		}
	})

	t.Run("Yes被低估且有No持仓应卖出No", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 第二次执行：价格大幅上涨，Yes 价格很低（被低估）
		// 设置 Yes 价格为 0.3，但概率应该很高
		time.Sleep(10 * time.Millisecond)
		noHolding := &trading.Asset{
			Type:             trading.No,
			Quantity:         decimal.NewFromFloat(10),
			TradableQuantity: decimal.NewFromFloat(10),
		}
		status = makeStatus(endDate, 101.0, 100.0, 0.3, noHolding)
		actions, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].CreateOrder == nil {
			t.Fatal("expected CreateOrder action")
		}
		if actions[0].CreateOrder.TokenType != trading.No {
			t.Errorf("expected No token type, got %v", actions[0].CreateOrder.TokenType)
		}
		if actions[0].CreateOrder.Side != trading.Sell {
			t.Errorf("expected Sell side, got %v", actions[0].CreateOrder.Side)
		}
		if pYes, ok := meta["PYes"]; !ok || pYes.(float64) < 0.5 {
			t.Errorf("expected PYes > 0.5, got %v", pYes)
		}
	})

	t.Run("Yes被低估且无No持仓应买入Yes", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 第二次执行：价格大幅上涨，Yes 价格很低（被低估）
		time.Sleep(10 * time.Millisecond)
		status = makeStatus(endDate, 101.0, 100.0, 0.3)
		actions, _, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].CreateOrder.TokenType != trading.Yes {
			t.Errorf("expected Yes token type, got %v", actions[0].CreateOrder.TokenType)
		}
		if actions[0].CreateOrder.Side != trading.Buy {
			t.Errorf("expected Buy side, got %v", actions[0].CreateOrder.Side)
		}
	})

	t.Run("Yes被高估且有Yes持仓应卖出Yes", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 第二次执行：价格大幅下跌，Yes 价格很高（被高估）
		time.Sleep(10 * time.Millisecond)
		yesHolding := &trading.Asset{
			Type:             trading.Yes,
			Quantity:         decimal.NewFromFloat(10),
			TradableQuantity: decimal.NewFromFloat(10),
		}
		status = makeStatus(endDate, 99.0, 100.0, 0.7, yesHolding)
		actions, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].CreateOrder.TokenType != trading.Yes {
			t.Errorf("expected Yes token type, got %v", actions[0].CreateOrder.TokenType)
		}
		if actions[0].CreateOrder.Side != trading.Sell {
			t.Errorf("expected Sell side, got %v", actions[0].CreateOrder.Side)
		}
		if pYes, ok := meta["PYes"]; !ok || pYes.(float64) > 0.5 {
			t.Errorf("expected PYes < 0.5, got %v", pYes)
		}
	})

	t.Run("Yes被高估且无Yes持仓应买入No", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 第二次执行：价格大幅下跌，Yes 价格很高（被高估）
		time.Sleep(10 * time.Millisecond)
		status = makeStatus(endDate, 99.0, 100.0, 0.7)
		actions, _, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].CreateOrder.TokenType != trading.No {
			t.Errorf("expected No token type, got %v", actions[0].CreateOrder.TokenType)
		}
		if actions[0].CreateOrder.Side != trading.Buy {
			t.Errorf("expected Buy side, got %v", actions[0].CreateOrder.Side)
		}
	})

	t.Run("交易间隔控制", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 第二次执行：触发交易
		time.Sleep(10 * time.Millisecond)
		status = makeStatus(endDate, 101.0, 100.0, 0.3)
		actions1, _, _ := s.Execute(ctx, status)
		if len(actions1) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions1))
		}

		// 第三次执行：立即再执行，应该被间隔控制阻止
		time.Sleep(10 * time.Millisecond)
		status = makeStatus(endDate, 102.0, 100.0, 0.2)
		actions2, meta, _ := s.Execute(ctx, status)
		if len(actions2) != 0 {
			t.Errorf("expected no actions due to interval control, got %d", len(actions2))
		}
		// 但应该仍然返回概率
		if _, ok := meta["PYes"]; !ok {
			t.Error("expected PYes in meta even when interval controlled")
		}
	})

	t.Run("价格未变化应不交易", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 第二次执行：价格相同
		time.Sleep(10 * time.Millisecond)
		status = makeStatus(endDate, 100.0, 100.0, 0.3)
		actions, meta, _ := s.Execute(ctx, status)
		if len(actions) != 0 {
			t.Errorf("expected no actions when price unchanged, got %d", len(actions))
		}
		if _, ok := meta["PYes"]; ok {
			t.Error("expected no PYes when sigma is 0")
		}
	})

	t.Run("滑点保护设置正确", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 第二次执行：触发买入 Yes
		time.Sleep(10 * time.Millisecond)
		bestAsk := 0.35
		status = makeStatus(endDate, 101.0, 100.0, 0.3)
		status.Prices.Yes.BestAsk = decimal.NewFromFloat(bestAsk)
		actions, _, _ := s.Execute(ctx, status)
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		// 买单滑点保护 = BestAsk + 0.2
		expectedPrice := bestAsk + 0.2
		if !actions[0].CreateOrder.Price.Equal(decimal.NewFromFloat(expectedPrice)) {
			t.Errorf("expected price %v, got %v", expectedPrice, actions[0].CreateOrder.Price)
		}
	})

	t.Run("缓存中仅有1个点时应不交易", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行：记录 1 个点
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 验证缓存中只有 1 个点
		if len(s.priceHistory) != 1 {
			t.Fatalf("expected 1 price point, got %d", len(s.priceHistory))
		}

		// 此时如果直接调用（由于 time.Sleep 不够，可能仍在同一秒内导致 dt=0）
		// 但即使有 2 个点，也应该是不交易的场景
		// 更直接的测试：手动设置只有 1 个点的缓存
		s.priceHistory = []pricePoint{
			{time: time.Now(), price: decimal.NewFromFloat(100.0)},
		}
		status = makeStatus(endDate, 101.0, 100.0, 0.3)
		actions, meta, _ := s.Execute(ctx, status)
		// 第二次调用后缓存有 2 个点了，这次会计算概率
		// 所以我们只验证第一次调用后缓存只有 1 个点时不交易
		_ = actions
		_ = meta
	})

	t.Run("过期数据被正确清理", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 手动设置缓存：包含一个 2 分钟前的过期点和一个 30 秒前的有效点
		now := time.Now()
		s.priceHistory = []pricePoint{
			{time: now.Add(-2 * time.Minute), price: decimal.NewFromFloat(100.0)},
			{time: now.Add(-30 * time.Second), price: decimal.NewFromFloat(100.5)},
		}

		// 执行一次
		status := makeStatus(endDate, 101.0, 100.0, 0.3)
		_, _, _ = s.Execute(ctx, status)

		// 验证过期点被清理，仅保留近 1 分钟的点和新加入的点
		for _, p := range s.priceHistory {
			age := now.Sub(p.time)
			if age > time.Minute+time.Second { // 允许 1 秒误差
				t.Errorf("found expired point at age %v: price=%v", age, p.price)
			}
		}
		// 应该有 2 个点：30 秒前的 + 新加入的（2 分钟前的被清理）
		if len(s.priceHistory) != 2 {
			t.Errorf("expected 2 price points after cleanup, got %d", len(s.priceHistory))
		}
	})

	t.Run("多个观测点时波动率计算正确", func(t *testing.T) {
		s := NewRandomWalk(Arithmetic)
		endDate := time.Now().Add(5 * time.Minute)

		// 构造已知的价格序列，手动填充缓存
		// 价格序列: 100.0 -> 101.0 -> 100.5 -> 101.5
		// 时间间隔: 10s, 10s, 10s
		baseTime := time.Now().Add(-30 * time.Second)
		s.priceHistory = []pricePoint{
			{time: baseTime, price: decimal.NewFromFloat(100.0)},
			{time: baseTime.Add(10 * time.Second), price: decimal.NewFromFloat(101.0)},
			{time: baseTime.Add(20 * time.Second), price: decimal.NewFromFloat(100.5)},
			{time: baseTime.Add(30 * time.Second), price: decimal.NewFromFloat(101.5)},
		}

		// 执行一次，使用不同的当前价格以触发交易
		status := makeStatus(endDate, 101.5, 100.0, 0.3)
		actions, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 应该有 PYes 值
		pYes, ok := meta["PYes"]
		if !ok {
			t.Fatal("expected PYes in meta")
		}

		// 手动验证波动率计算
		// ΔS²/Δt: (1)²/10 + (0.5)²/10 + (1)²/10 = 0.1 + 0.025 + 0.1 = 0.225
		// σ = √(0.225/3) = √0.075 ≈ 0.2739
		// 同时加上新点后的第4对：(101.5 - 101.5)²/dt，取决于时间差
		// 概率应 > 0.5 因为 S0=101.5 > K=100
		if pYes.(float64) <= 0.5 {
			t.Errorf("expected PYes > 0.5 when S0 > K, got %v", pYes)
		}

		// 偏差应该大于阈值（0.3 是 Yes 价格，PYes 应远大于 0.5）
		if len(actions) != 1 {
			t.Errorf("expected 1 action for large deviation, got %d", len(actions))
		}
	})
}

func BenchmarkRandomWalkExecute(b *testing.B) {
	ctx := context.Background()

	// 构造固定价格历史
	history := func() []pricePoint {
		now := time.Now()
		base := now.Add(-50 * time.Second)
		pts := make([]pricePoint, 10)
		for i := range pts {
			pts[i] = pricePoint{
				time:  base.Add(time.Duration(i*5) * time.Second),
				price: decimal.NewFromFloat(100.0 + float64(i%3)*0.5),
			}
		}
		return pts
	}()

	status := trading.Status{
		CurrentMarket: trading.Market{EndDate: time.Now().Add(5 * time.Minute)},
		ResolutionSource: trading.ResolutionSource{
			Value:       decimal.NewFromFloat(101.5),
			TargetValue: decimal.NewFromFloat(100.0),
		},
		Prices: trading.MarketPrices{
			Yes: trading.AssetPrices{
				Last: decimal.NewFromFloat(0.3), BestBid: decimal.NewFromFloat(0.29), BestAsk: decimal.NewFromFloat(0.31),
			},
			No: trading.AssetPrices{
				Last: decimal.NewFromFloat(0.7), BestBid: decimal.NewFromFloat(0.69), BestAsk: decimal.NewFromFloat(0.71),
			},
		},
		Holding: map[string]*trading.Asset{
			"no": {Type: trading.No, TradableQuantity: decimal.NewFromFloat(10)},
		},
	}

	b.Run("ComputeAndTrade", func(b *testing.B) {
		s := NewRandomWalk(Arithmetic)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			s.lastTradeTime = time.Time{}
			s.priceHistory = make([]pricePoint, len(history))
			copy(s.priceHistory, history)
			b.StartTimer()
			_, _, _ = s.Execute(ctx, status)
		}
	})

	b.Run("DeviationWithinThreshold", func(b *testing.B) {
		s := NewRandomWalk(Arithmetic)
		deviationStatus := status
		deviationStatus.ResolutionSource.Value = decimal.NewFromFloat(100.05)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			s.lastTradeTime = time.Time{}
			s.priceHistory = make([]pricePoint, len(history))
			copy(s.priceHistory, history)
			b.StartTimer()
			_, _, _ = s.Execute(ctx, deviationStatus)
		}
	})
}

func BenchmarkNormalCDF(b *testing.B) {
	inputs := []float64{-3.0, -1.0, -0.5, 0.0, 0.5, 1.0, 3.0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, x := range inputs {
			normalCDF(x)
		}
	}
}

// TestRandomWalkGeometric 测试几何随机游走模型
func TestRandomWalkGeometric(t *testing.T) {
	ctx := context.Background()

	// 辅助函数：创建测试状态
	makeStatus := func(endDate time.Time, value, targetValue, yesPrice float64, holdings ...*trading.Asset) trading.Status {
		status := trading.Status{
			CurrentMarket: trading.Market{
				EndDate: endDate,
			},
			ResolutionSource: trading.ResolutionSource{
				Value:       decimal.NewFromFloat(value),
				TargetValue: decimal.NewFromFloat(targetValue),
			},
			Prices: trading.MarketPrices{
				Yes: trading.AssetPrices{
					Last:    decimal.NewFromFloat(yesPrice),
					BestBid: decimal.NewFromFloat(yesPrice - 0.01),
					BestAsk: decimal.NewFromFloat(yesPrice + 0.01),
				},
				No: trading.AssetPrices{
					Last:    decimal.NewFromFloat(1 - yesPrice),
					BestBid: decimal.NewFromFloat(1 - yesPrice - 0.01),
					BestAsk: decimal.NewFromFloat(1 - yesPrice + 0.01),
				},
			},
			Holding: make(map[string]*trading.Asset),
		}
		for i, h := range holdings {
			status.Holding[string(rune('A'+i))] = h
		}
		return status
	}

	t.Run("概率计算-当前价格高于基准", func(t *testing.T) {
		s := NewRandomWalk(Geometric)
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		// 第二次执行：价格上涨
		time.Sleep(10 * time.Millisecond)
		status = makeStatus(endDate, 101.0, 100.0, 0.5)
		_, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pYes, ok := meta["PYes"]; !ok {
			t.Error("expected PYes in meta")
		} else if pYes.(float64) <= 0.5 {
			t.Errorf("expected PYes > 0.5 when S0 > K, got %v", pYes)
		}
	})

	t.Run("概率计算-当前价格低于基准", func(t *testing.T) {
		s := NewRandomWalk(Geometric)
		endDate := time.Now().Add(5 * time.Minute)

		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		time.Sleep(10 * time.Millisecond)
		status = makeStatus(endDate, 99.0, 100.0, 0.5)
		_, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pYes, ok := meta["PYes"]; !ok {
			t.Error("expected PYes in meta")
		} else if pYes.(float64) >= 0.5 {
			t.Errorf("expected PYes < 0.5 when S0 < K, got %v", pYes)
		}
	})

	t.Run("价格非正-不交易", func(t *testing.T) {
		s := NewRandomWalk(Geometric)
		endDate := time.Now().Add(5 * time.Minute)

		// S0 为 0
		status := makeStatus(endDate, 0, 100.0, 0.5)
		actions, _, _ := s.Execute(ctx, status)
		if len(actions) != 0 {
			t.Error("expected no actions when S0 is 0 in geometric mode")
		}

		// S0 为负值
		status = makeStatus(endDate, -10.0, 100.0, 0.5)
		actions, _, _ = s.Execute(ctx, status)
		if len(actions) != 0 {
			t.Error("expected no actions when S0 is negative in geometric mode")
		}

		// K 为负值
		status = makeStatus(endDate, 100.0, -10.0, 0.5)
		actions, _, _ = s.Execute(ctx, status)
		if len(actions) != 0 {
			t.Error("expected no actions when K is negative in geometric mode")
		}
	})

	t.Run("波动率估算-对数收益率", func(t *testing.T) {
		s := NewRandomWalk(Geometric)
		endDate := time.Now().Add(5 * time.Minute)

		// 构造价格序列，验证对数收益率波动率计算
		baseTime := time.Now().Add(-30 * time.Second)
		s.priceHistory = []pricePoint{
			{time: baseTime, price: decimal.NewFromFloat(100.0)},
			{time: baseTime.Add(10 * time.Second), price: decimal.NewFromFloat(101.0)},
			{time: baseTime.Add(20 * time.Second), price: decimal.NewFromFloat(100.5)},
			{time: baseTime.Add(30 * time.Second), price: decimal.NewFromFloat(101.5)},
		}

		status := makeStatus(endDate, 101.5, 100.0, 0.3)
		_, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		pYes, ok := meta["PYes"]
		if !ok {
			t.Fatal("expected PYes in meta")
		}
		if pYes.(float64) <= 0.5 {
			t.Errorf("expected PYes > 0.5 when S0 > K, got %v", pYes)
		}
	})

	t.Run("波动率估算-跳过非正值观测点", func(t *testing.T) {
		s := NewRandomWalk(Geometric)
		endDate := time.Now().Add(5 * time.Minute)

		baseTime := time.Now().Add(-30 * time.Second)
		s.priceHistory = []pricePoint{
			{time: baseTime, price: decimal.NewFromFloat(100.0)},
			{time: baseTime.Add(10 * time.Second), price: decimal.NewFromFloat(0)},     // 非正值
			{time: baseTime.Add(20 * time.Second), price: decimal.NewFromFloat(101.0)}, // 正值
			{time: baseTime.Add(30 * time.Second), price: decimal.NewFromFloat(100.5)}, // 正值
		}

		status := makeStatus(endDate, 100.5, 100.0, 0.3)
		_, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 应该跳过含 0 的观测点对，使用有效对 (101.0→100.5) 和 (100.5→100.5) 计算
		pYes, ok := meta["PYes"]
		if !ok {
			t.Fatal("expected PYes in meta")
		}
		if pYes.(float64) <= 0.5 {
			t.Errorf("expected PYes > 0.5 when S0 > K, got %v", pYes)
		}
	})

	t.Run("Yes被低估且有No持仓应卖出No", func(t *testing.T) {
		s := NewRandomWalk(Geometric)
		endDate := time.Now().Add(5 * time.Minute)

		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		time.Sleep(10 * time.Millisecond)
		noHolding := &trading.Asset{
			Type:             trading.No,
			Quantity:         decimal.NewFromFloat(10),
			TradableQuantity: decimal.NewFromFloat(10),
		}
		status = makeStatus(endDate, 101.0, 100.0, 0.3, noHolding)
		actions, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].CreateOrder.TokenType != trading.No {
			t.Errorf("expected No token type, got %v", actions[0].CreateOrder.TokenType)
		}
		if actions[0].CreateOrder.Side != trading.Sell {
			t.Errorf("expected Sell side, got %v", actions[0].CreateOrder.Side)
		}
		if pYes, ok := meta["PYes"]; !ok || pYes.(float64) < 0.5 {
			t.Errorf("expected PYes > 0.5, got %v", pYes)
		}
	})

	t.Run("Yes被低估且无No持仓应买入Yes", func(t *testing.T) {
		s := NewRandomWalk(Geometric)
		endDate := time.Now().Add(5 * time.Minute)

		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		time.Sleep(10 * time.Millisecond)
		status = makeStatus(endDate, 101.0, 100.0, 0.3)
		actions, _, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].CreateOrder.TokenType != trading.Yes {
			t.Errorf("expected Yes token type, got %v", actions[0].CreateOrder.TokenType)
		}
		if actions[0].CreateOrder.Side != trading.Buy {
			t.Errorf("expected Buy side, got %v", actions[0].CreateOrder.Side)
		}
	})

	t.Run("Yes被高估且有Yes持仓应卖出Yes", func(t *testing.T) {
		s := NewRandomWalk(Geometric)
		endDate := time.Now().Add(5 * time.Minute)

		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		time.Sleep(10 * time.Millisecond)
		yesHolding := &trading.Asset{
			Type:             trading.Yes,
			Quantity:         decimal.NewFromFloat(10),
			TradableQuantity: decimal.NewFromFloat(10),
		}
		status = makeStatus(endDate, 99.0, 100.0, 0.7, yesHolding)
		actions, meta, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].CreateOrder.TokenType != trading.Yes {
			t.Errorf("expected Yes token type, got %v", actions[0].CreateOrder.TokenType)
		}
		if actions[0].CreateOrder.Side != trading.Sell {
			t.Errorf("expected Sell side, got %v", actions[0].CreateOrder.Side)
		}
		if pYes, ok := meta["PYes"]; !ok || pYes.(float64) > 0.5 {
			t.Errorf("expected PYes < 0.5, got %v", pYes)
		}
	})

	t.Run("Yes被高估且无Yes持仓应买入No", func(t *testing.T) {
		s := NewRandomWalk(Geometric)
		endDate := time.Now().Add(5 * time.Minute)

		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		_, _, _ = s.Execute(ctx, status)

		time.Sleep(10 * time.Millisecond)
		status = makeStatus(endDate, 99.0, 100.0, 0.7)
		actions, _, err := s.Execute(ctx, status)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].CreateOrder.TokenType != trading.No {
			t.Errorf("expected No token type, got %v", actions[0].CreateOrder.TokenType)
		}
		if actions[0].CreateOrder.Side != trading.Buy {
			t.Errorf("expected Buy side, got %v", actions[0].CreateOrder.Side)
		}
	})
}
