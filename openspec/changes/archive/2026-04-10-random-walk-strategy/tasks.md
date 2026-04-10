## 1. 市场信息增强

- [x] 1.1 在 `trading.Market` 结构体中新增 `EndDate time.Time` 字段
- [x] 1.2 在 `UpdownSeriesTrader.runLoop` 构建 `Market` 时设置 `EndDate` 为 `trader.curMarket.EndDate`
- [x] 1.3 在 `UpdownSeriesTrader.rotateMarket` 构建新 `Market` 时设置 `EndDate` 为 `newMarket.EndDate`

## 2. 随机游走策略实现

- [x] 2.1 创建 `pkg/trading/strategies/random_walk.go`，定义 `RandomWalk` 结构体和 `NewRandomWalk()` 构造函数
- [x] 2.2 实现辅助函数 `normalCDF(x float64) float64`（基于 `math.Erf`）
- [x] 2.3 实现 `Execute` 方法的核心逻辑：市场结束前保护、数据校验、波动率估算、概率计算
- [x] 2.4 实现交易决策逻辑：偏差检测、买卖操作选择（优先卖出持仓）、滑点保护设置

## 3. 测试

- [x] 3.1 为 `normalCDF` 函数编写单元测试
- [x] 3.2 为 `RandomWalk.Execute` 编写单元测试，覆盖关键场景（首次执行、偏差触发交易、间隔控制等）

## 4. 验证

- [x] 4.1 运行 `go fmt ./...`、`go vet ./...`、`go test ./...` 确认通过
