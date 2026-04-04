package trading

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/yhlooo/pat/pkg/polymarket"
)

// TraderOptions 交易员运行选项
type TraderOptions struct {
	DryRun bool
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
	// 当前交易的市场 slug
	MarketSlug string
	// 当前市场价格信息
	Prices MarketPrices
	// 判定来源
	ResolutionSource ResolutionSource
}

// MarketPrices 市场价格信息
type MarketPrices struct {
	// Yes 价格
	Yes AssetPrices
	// No 价格
	No AssetPrices
}

// ResolutionSource 判定来源
type ResolutionSource struct {
	URL   string
	Value decimal.Decimal
}

// AssetPrices 资产价格信息
type AssetPrices struct {
	// 买一价
	BestBid decimal.Decimal
	// 卖一价
	BestAsk decimal.Decimal
	// 最后成交价
	Last decimal.Decimal
}
