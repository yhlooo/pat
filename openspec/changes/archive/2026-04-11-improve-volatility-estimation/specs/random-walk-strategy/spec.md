## MODIFIED Requirements

### Requirement: 波动率估算
策略 SHALL 通过缓存近 1 分钟的 (时间戳, 价格) 观测点来估算波动率。每次执行时记录当前时间和价格，并清理 1 分钟前的过期数据。波动率 σ 使用 realized variance 公式估算：`σ = √( (1/m) * Σ( (Sᵢ - Sᵢ₋₁)² / (tᵢ - tᵢ₋₁) ) )`，其中 m 为增量数量。

#### Scenario: 首次执行
- **WHEN** 策略首次执行，缓存中无观测点
- **THEN** 记录当前时间和价格到缓存，不进行交易，meta 中 PYes 为空

#### Scenario: 缓存中仅有 1 个观测点
- **WHEN** 缓存中仅记录了 1 个观测点（含本次添加后）
- **THEN** 不进行交易，meta 中 PYes 为空

#### Scenario: 缓存中有足够观测点
- **WHEN** 缓存中有至少 2 个观测点
- **THEN** 使用 realized variance 公式计算 σ，继续后续概率计算

#### Scenario: 价格未变化
- **WHEN** 所有观测点价格相同，导致 σ = 0
- **THEN** 不进行交易，meta 中 PYes 为空

#### Scenario: 过期数据清理
- **WHEN** 每次 Execute 调用时
- **THEN** 缓存中早于当前时间 1 分钟的观测点 SHALL 被移除
