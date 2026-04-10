## Why

当前交易系统已有完整的基础设施（市场监听、订单管理、策略执行框架），但只有随机交易的猴子策略用于测试。需要实现一个基于随机游走数学模型的交易策略，利用 BTC/ETH 价格在短时间尺度上的随机波动特性，计算涨跌概率并与市场价格比较，在价格偏离概率较大时进行套利交易。

## What Changes

- 在 `trading.Market` 结构体中新增 `EndDate` 字段，使策略能获取市场结束时间
- 在 `UpdownSeriesTrader` 构造 `trading.Market` 时从 `polymarket.Market` 传入 `EndDate`
- 新增 `RandomWalk` 策略实现（`pkg/trading/strategies/random_walk.go`），包含：
  - 基于正态近似的概率计算（Φ 函数）
  - 波动率估算（从相邻两次执行的价差推算）
  - 概率与市场价格偏差检测（阈值 0.2）
  - 交易操作决策（买入/卖出 Yes/No，优先卖出已有持仓）
  - 交易间隔控制（至少 30s）和市场结束前保护（最后 30s 不交易）

## Capabilities

### New Capabilities
- `random-walk-strategy`: 随机游走交易策略，基于数学模型计算涨跌概率并与市场价格比较进行套利

### Modified Capabilities
- `trading-status`: 交易状态中的市场信息增加市场结束时间字段

## Impact

- `pkg/trading/traders.go`: `Market` 结构体新增 `EndDate` 字段
- `pkg/trading/trader_updown_series.go`: 构建 `Market` 时设置 `EndDate`
- `pkg/trading/strategies/random_walk.go`: 新增策略实现文件
