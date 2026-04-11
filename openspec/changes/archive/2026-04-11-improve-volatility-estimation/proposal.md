## Why

当前随机游走策略的波动率 σ 仅基于相邻两次执行的价格差计算（`|Δprice| / Δtime`），只有 1 个样本点，噪声极大，容易因为瞬时波动产生错误的交易信号。使用近 1 分钟的价格序列估算波动率可以显著提高估计的统计可靠性。

## What Changes

- 将波动率估算方式从"两点瞬时变化率"改为基于近 1 分钟价格序列的 realized variance 估计
- 引入价格缓存结构，存储近 1 分钟的 (时间戳, 价格) 观测点
- 每次执行时自动清理过期数据，防止长时间运行时的内存泄漏
- 至少需要 2 个数据点才能估算波动率，否则不交易

## Capabilities

### New Capabilities

（无新增能力）

### Modified Capabilities

- `random-walk-strategy`: 波动率估算的需求从"两点瞬时变化率"改为"基于近 1 分钟价格序列的 realized variance 估计"

## Impact

- `pkg/trading/strategies/random_walk.go`：核心改动文件，重构波动率计算逻辑
- `pkg/trading/strategies/random_walk_test.go`：更新相关单元测试
- 无 API 变更，Strategy 接口不变
- 无依赖变更
