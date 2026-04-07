package trading

import (
	"context"
	"time"

	"github.com/shopspring/decimal"

	"github.com/yhlooo/pat/pkg/polymarket"
)

// TraderOptions 交易员运行选项
type TraderOptions struct {
	// 干执行，模拟交易和结算，不真正提交订单到 PolyMarket
	DryRun bool `json:"dryRun,omitempty"`
	// 缩放比例，等比例放大策略输出的买入、卖出数量，以适配资金量
	Scale int `json:"scale,omitempty"`
}

// Trader 交易员
type Trader interface {
	// Start 开始交易
	Start(ctx context.Context) error
	// Market 返回交易员正在交易的市场信息
	Market() polymarket.Market
	// Channel 获取监听状态的 chan
	Channel() <-chan Status
	// Done 返回结束通知 chan
	Done() <-chan struct{}
}

// Status 交易状态
type Status struct {
	// 当前正在交易的市场基础信息
	CurrentMarket Market `json:"currentMarket"`
	// 还未结算的还在监听的市场（键为市场 ConditionID ）
	WatchingMarkets map[string]Market `json:"watchingMarkets"`
	// 当前市场价格信息
	Prices MarketPrices `json:"prices"`
	// 判定来源
	ResolutionSource ResolutionSource `json:"resolutionSource"`
	// 相对现金量
	Cash decimal.Decimal `json:"cash"`
	// 未成交的订单（键为订单 ID ）
	PendingOrders map[string]Order `json:"pendingOrders,omitempty"`
	// 已取消或成交的订单（键为订单 ID ）
	CompletedOrders map[string]Order `json:"completedOrders,omitempty"`
	// 持仓（键为资产 ID ）
	Holding map[string]*Asset `json:"holding,omitempty"`
}

// CancelOrder 记录取消订单
func (s *Status) CancelOrder(id string, state OrderState) bool {
	order, ok := s.PendingOrders[id]
	if !ok {
		return false
	}

	// 删除挂起订单
	delete(s.PendingOrders, id)

	// 记录已取消订单
	order.ResolvedAt = time.Now()
	order.State = state
	if s.CompletedOrders == nil {
		s.CompletedOrders = make(map[string]Order)
	}
	s.CompletedOrders[order.ID] = order

	// 卖单恢复库存
	if order.Side == Sell {
		if _, ok := s.Holding[order.TokenID]; ok {
			qty := s.Holding[order.TokenID].TradableQuantity.Add(order.Quantity)
			s.Holding[order.TokenID].TradableQuantity = qty
		}
	}

	return true
}

// FillOrder 成交订单
func (s *Status) FillOrder(id string, price decimal.Decimal, qty decimal.Decimal) bool {
	order, ok := s.PendingOrders[id]
	if !ok {
		return false
	}

	// 数量超过订单未成交数量
	if qty.GreaterThan(order.Quantity.Sub(order.FilledQuantity)) {
		qty = order.Quantity.Sub(order.FilledQuantity)
	}

	// 记录成交数量
	amount := qty.Mul(price).Round(6)
	order.FilledQuantity = order.FilledQuantity.Add(qty)
	order.FilledAmount = order.FilledAmount.Add(amount)
	order.FilledPrice = order.FilledAmount.DivRound(order.FilledQuantity, 6)

	// 更新库存
	if s.Holding == nil {
		s.Holding = make(map[string]*Asset)
	}
	if s.Holding[order.TokenID] == nil {
		s.Holding[order.TokenID] = &Asset{
			ID:         order.TokenID,
			Type:       order.TokenType,
			MarketID:   order.MarketID,
			MarketSlug: order.MarketSlug,
			Price:      price,
		}
	}
	holding := s.Holding[order.TokenID]
	if order.Side == Buy {
		holding.Quantity = holding.Quantity.Add(qty)
		holding.TradableQuantity = holding.TradableQuantity.Add(qty)
		s.Cash = s.Cash.Sub(amount)
	} else {
		holding.Quantity = holding.Quantity.Sub(qty)
		s.Cash = s.Cash.Add(amount)
	}
	holding.Value = holding.Quantity.Mul(holding.Price).Round(6)
	if holding.Quantity.IsZero() {
		delete(s.Holding, order.TokenID)
	}

	// 更新订单记录
	if order.FilledQuantity.GreaterThanOrEqual(order.Quantity) {
		// 关闭完成的订单
		order.ResolvedAt = time.Now()
		order.State = OrderFilled
		if s.CompletedOrders == nil {
			s.CompletedOrders = make(map[string]Order)
		}
		s.CompletedOrders[id] = order
		delete(s.PendingOrders, id)
	} else {
		// 写回订单信息
		s.PendingOrders[id] = order
	}

	return true
}

// Market 市场信息
type Market struct {
	// 市场 slug
	Slug string `json:"slug"`
	// 条件 ID
	ConditionID string `json:"conditionID"`
	// Yes 代币 ID
	YesTokenID string `json:"yesTokenID"`
	// No 代币 ID
	NoTokenID string `json:"noTokenID"`
}

// MarketPrices 市场价格信息
type MarketPrices struct {
	// Yes 价格
	Yes AssetPrices `json:"yes"`
	// No 价格
	No AssetPrices `json:"no"`
}

// ResolutionSource 判定来源
type ResolutionSource struct {
	URL    string          `json:"url"`
	Value  decimal.Decimal `json:"value"`
	Symbol string          `json:"symbol,omitempty"`
}

// AssetPrices 资产价格信息
type AssetPrices struct {
	// 买一价
	BestBid decimal.Decimal `json:"bestBid"`
	// 卖一价
	BestAsk decimal.Decimal `json:"bestAsk"`
	// 最后成交价
	Last decimal.Decimal `json:"last"`
}

// Order 订单
type Order struct {
	// 订单 ID
	ID string `json:"id"`
	// 代币类型
	TokenType TokenType `json:"tokenType"`
	// 代币 ID
	TokenID string `json:"tokenID"`
	// 所属市场 ConditionID
	MarketID string `json:"marketID"`
	// 所属市场 slug
	MarketSlug string `json:"marketSlug"`
	// 方向
	Side OrderSide `json:"side"`
	// 订单类型
	Type OrderType `json:"type"`
	// 价格
	// 对于限价单表示出价或要价，对于市价单表示可接受的最差价格（滑点保护）
	Price decimal.Decimal `json:"price"`
	// 数量
	// 市价单买入时应指定 Amount ，此时 Quantity 无意义
	Quantity decimal.Decimal `json:"quantity,omitempty"`
	// 金额
	Amount decimal.Decimal `json:"amount,omitempty"`
	// 订单过期时间（仅对 GTD 有意义）
	Expiration time.Time `json:"expiration,omitempty"`

	// 成交数量
	FilledQuantity decimal.Decimal `json:"filledQuantity,omitempty"`
	// 成交价格
	FilledPrice decimal.Decimal `json:"filledPrice,omitempty"`
	// 已成交金额
	FilledAmount decimal.Decimal `json:"filledAmount,omitempty"`

	// 创建时间
	CreatedAt time.Time `json:"createdAt"`
	// 解决时间
	ResolvedAt time.Time `json:"resolvedAt,omitempty"`
	// 订单状态
	State OrderState `json:"state"`
}

// TokenType 交易的代币类型
type TokenType string

const (
	Yes TokenType = "Yes"
	No  TokenType = "No"
)

// OrderSide 订单方向
type OrderSide string

const (
	Buy  OrderSide = "Buy"
	Sell OrderSide = "Sell"
)

// OrderType 订单类型
type OrderType string

const (
	// GTC Good-Til-Cancelled — 在账本上保留,直到成交或取消
	GTC OrderType = "GTC"
	// GTD Good-Til-Date — 活跃至指定的到期时间
	GTD OrderType = "GTD"
	// FOK Fill-Or-Kill — 必须立即完全成交,否则取消
	FOK OrderType = "FOK"
	// FAK Fill-And-Kill — 立即成交可用部分,取消剩余部分
	FAK OrderType = "FAK"
)

type OrderState string

const (
	OrderPending   OrderState = "Pending"
	OrderCancelled OrderState = "Cancelled"
	OrderFilled    OrderState = "Filled"
	OrderFailed    OrderState = "Failed"
)

// Asset 资产
type Asset struct {
	// 资产 ID
	ID string `json:"id"`
	// 资产所属代币类型
	Type TokenType `json:"type"`
	// 所属市场 ConditionID
	MarketID string `json:"marketID"`
	// 所属市场 slug
	MarketSlug string `json:"marketSlug"`
	// 数量
	Quantity decimal.Decimal `json:"quantity"`
	// 可交易数量
	TradableQuantity decimal.Decimal `json:"availableQuantity"`
	// 价格
	Price decimal.Decimal `json:"price"`
	// 总价值
	Value decimal.Decimal `json:"value"`
}
