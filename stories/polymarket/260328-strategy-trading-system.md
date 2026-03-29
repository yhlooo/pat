# 策略交易系统

该系统主要分成三个部分

- 策略: 一个逻辑单元，在任何时刻基于输入的市场和持仓相关信息决策买入、卖出、撤单还是不行动
- 策略执行器: 在指定的市场上应用“策略”进行交易，支持实盘交易和模拟交易（ dry-run ）
- UI: 展示策略执行情况

## 策略

“策略”是一个逻辑单元，在任何时刻基于输入的市场和持仓相关信息决策买入、卖出、撤单还是不行动。

所有策略都需要遵循相同的接口，具体如下：

```go
// Strategy 交易策略
type Strategy interface {
	Execute(ctx context.Context, in StrategyExecuteInput) (*StrategyExecuteResult, error)
}

// StrategyExecuteInput 策略执行输入
type StrategyExecuteInput struct {
	Market Market //市场元信息（ id 、 slug 、描述等）
	Holding Hoding // 持仓信息，持有多少 yes/no 、平均价格
	Orders []Order // 该策略所下的每笔订单信息， ID 、 下单时间、下单价格、数量、出价类型（限价/市价）、状态（成交、未成交、已取消）
	ServerTime time.Time // PolyMarket 服务器时间（如有）
	YesBidPrices []Value // yes 的买价趋势（时序数据）
	NoBidPrices []Value // no 的买价趋势
    YesAskPrices []Value // yes 的卖价趋势
    NoAskPrices []Value // no 的卖价趋势
	TrackingPrices []Value // 跟踪底层标的资产价格趋势，比如比特币（如有）
    TrackingTargetPrice float64 // 跟踪底层标的资产目标价格，比如比特币（如有）
}

// Value 趋势值
type Value struct {
	Time time.Time
	Value float64
}

// StrategyExecuteResult 策略执行结果
type StrategyExecuteResult struct {
	Order []Order // 策略决定下单，买/卖，yes/no ，数量、出价类型（限价/市价）、出价
	CancelOrders []string // 策略决策取消订单，需要取消的订单 ID
	
	// 如果 Order 和 CancelOrders 都为空就表示什么都不做
}
```

还有一些要点：

- 策略必须是无状态且无副作用的，可以在任何时候调用
- 策略输出的订单买入数量最少应价值 1 美元（卖出没有限制）， 1 美元也应作为策略的基础交易量级，策略执行器可以按照配置对策略输出的数量按比例放大

## 策略执行器

策略执行器的功能是在指定的市场上应用“策略”进行交易。

基本运行模式是通过 WebSocket 监听指定市场信息、和市场关联底层资产（如比特币）的价格信息，在这些信息发生任何变化时调用“策略”判断是否需要采取行动，如果需要则采取行动

在以下情形应执行一次策略

- 监听的市场价格、市场关联的底层资产价格发生变化
- 每间隔一个指定的时间，该时间可配置（比如 5s ）

执行策略后根据策略输出进行下单或撤单。支持根据配置进行真实操作或模拟操作：

- 真实操作：在市场上通过 API 真实下单，通过市场真实结算订单
- 模拟操作：（ dry-run ）仅在执行器内部记录该订单，并在价格触及订单时模拟成交，记录盈亏等信息

无论是真实操作还是模拟操作，执行器都应该能实时输出资产总价值、持仓、持仓成本等信息

策略执行器中应记录现金、 yes 、 no 持仓信息：

- 现金初始值为 0 ，可以是负数
- 买入 yes/no 需要消耗对应的现金并增加 yes/no 持仓
- 卖出 yes/no 持仓需减少 yes/no 持仓并增加现金， yes/no 持仓数量不可以是负数
- 资产总价值 = 现金 + yes 持仓数量 * yes 当前卖价 + no 持仓数量 * no 当前卖价。所以策略开始时资产总价值初始是 0 ，随着交易可以是正数或负数，负数表明亏钱了，正数表示挣钱了。

交易倍数：

- 策略输出结果以尽可能小的量进行下单，但是执行器可以根据配置对策略输出的订单数量乘以固定倍数（如 5 倍、 10 倍）以匹配总资金规模
- 输入给策略的已下订单数量、持仓数量等都是翻倍前的原始数量，以提供给策略一个一致的信息。也就是翻倍操作对策略是透明的。

## UI

UI 应实时显示以下信息：

- 市场标题、 slug 、 描述等元信息
- 当前 yes/no 的最新 bid/ask 价格
- 跟踪的底层资产价格（如有）
- 当前根据策略操作买入的持仓/现金数量，持仓价值，持仓平均成本，总资产价值
- 交易记录列表。每一项包含交易时间、资产类型yes/no、报价（如有）、成交价、数量（乘以交易倍数后的实际数量）
