## 1. 数据结构与缓存管理

- [x] 1.1 定义 `pricePoint` 结构体（`time time.Time`, `price decimal.Decimal`），替换 `RandomWalk` 中的 `lastPrice` 和 `lastExecTime` 字段为 `priceHistory []pricePoint`
- [x] 1.2 在 Execute 方法开头添加缓存清理逻辑：移除早于当前时间 1 分钟的观测点

## 2. 波动率计算重构

- [x] 2.1 在 Execute 方法中，将当前 (时间, 价格) 记录到缓存
- [x] 2.2 实现 realized variance 计算：`σ = √( (1/m) * Σ( (Sᵢ - Sᵢ₋₁)² / (tᵢ - tᵢ₋₁) ) )`
- [x] 2.3 处理边界情况：缓存中不足 2 个观测点时不交易，σ = 0 时不交易

## 3. 单元测试更新

- [x] 3.1 更新"首次执行应不交易"测试用例以适配新的缓存逻辑
- [x] 3.2 更新"价格未变化应不交易"测试用例（需构造多个相同价格的缓存点）
- [x] 3.3 添加测试：缓存中仅有 1 个点时不交易
- [x] 3.4 添加测试：过期数据被正确清理
- [x] 3.5 添加测试：多个观测点时 realized variance 计算正确

## 4. 代码质量检查

- [x] 4.1 执行 `go fmt ./...`、`go vet ./...`、`go test ./...` 确认通过
