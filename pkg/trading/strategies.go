package trading

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Strategy 策略
type Strategy interface {
	// Execute 执行策略
	Execute(ctx context.Context, statue Status) ([]Action, error)
}

// Action 操作
type Action struct {
	CreateOrder *CreateOrder `json:"createOrder,omitempty"`
	CancelOrder *CancelOrder `json:"cancelOrder,omitempty"`
}

// CreateOrder 创建订单
type CreateOrder struct {
	// 交易资产类型
	TokenType TokenType `json:"tokenType"`
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
}

// CancelOrder 取消订单
type CancelOrder struct {
	ID string
}

// DiscardStrategy 不执行任何操作的策略
var DiscardStrategy = Strategy(discardStrategy{})

// discardStrategy 不执行任何操作的策略
type discardStrategy struct{}

// Execute 执行策略
func (discardStrategy) Execute(_ context.Context, _ Status) ([]Action, error) {
	return nil, nil
}
