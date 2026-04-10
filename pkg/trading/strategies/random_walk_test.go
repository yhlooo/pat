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
		s := NewRandomWalk()
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
		s := NewRandomWalk()
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
		s := NewRandomWalk()

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
		s := NewRandomWalk()
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行：记录状态
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		s.Execute(ctx, status)

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
		s := NewRandomWalk()
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		s.Execute(ctx, status)

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
		s := NewRandomWalk()
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		s.Execute(ctx, status)

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
		s := NewRandomWalk()
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		s.Execute(ctx, status)

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
		s := NewRandomWalk()
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		s.Execute(ctx, status)

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
		s := NewRandomWalk()
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		s.Execute(ctx, status)

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
		s := NewRandomWalk()
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		s.Execute(ctx, status)

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
		s := NewRandomWalk()
		endDate := time.Now().Add(5 * time.Minute)

		// 第一次执行
		status := makeStatus(endDate, 100.0, 100.0, 0.5)
		s.Execute(ctx, status)

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
}
