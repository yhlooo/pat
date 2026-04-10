## ADDED Requirements

### Requirement: 概率计算
策略 SHALL 使用正态近似计算 Yes(Up) 概率 P(Yes)。公式为 `P(Yes) = 1 - Φ((K - S₀) / (σ√n))`，其中：
- S₀ = `status.ResolutionSource.Value`（当前资产价格）
- K = `status.ResolutionSource.TargetValue`（市场基准价格）
- n = 剩余秒数（市场结束时间 - 当前时间）
- σ = 估算的每秒波动幅度

#### Scenario: 当前价格高于基准价格
- **WHEN** S₀ > K 且 σ > 0 且 n > 0
- **THEN** P(Yes) > 0.5

#### Scenario: 当前价格低于基准价格
- **WHEN** S₀ < K 且 σ > 0 且 n > 0
- **THEN** P(Yes) < 0.5

#### Scenario: 价格数据缺失
- **WHEN** S₀、K 或 `status.Prices.Yes.Last` 任一为零值
- **THEN** 不采取任何交易操作，meta 中 PYes 为空

### Requirement: 波动率估算
策略 SHALL 通过记录每次执行时的资产价格和时间来估算波动率。σ = |当前价格 - 上次记录价格| / 距上次记录的秒数。

#### Scenario: 首次执行
- **WHEN** 策略首次执行，无历史价格记录
- **THEN** 记录当前时间和价格，不进行交易，meta 中 PYes 为空

#### Scenario: 价格未变化
- **WHEN** 当前价格与上次记录价格相同，导致 σ = 0
- **THEN** 更新记录但不进行交易，meta 中 PYes 为空

### Requirement: 交易间隔控制
策略 SHALL 确保两次交易之间至少间隔 30 秒。

#### Scenario: 距上次交易不足 30 秒
- **WHEN** 当前时间距上次交易时间 < 30 秒
- **THEN** 不执行交易（但仍计算概率并返回 meta）

### Requirement: 市场结束前保护
策略 SHALL 在市场结束前 30 秒内不执行任何交易。

#### Scenario: 市场即将结束
- **WHEN** 距市场结束时间 ≤ 30 秒
- **THEN** 不执行交易，meta 中 PYes 为空

### Requirement: 交易决策
策略 SHALL 比较 P(Yes) 与 `status.Prices.Yes.Last` 的偏差来决定交易操作：
- 偏差 > 0.2：Yes 被低估，应买入 Yes 或卖出 No（有 No 持仓时优先卖出 No）
- 偏差 < -0.2：Yes 被高估，应卖出 Yes 或买入 No（有 Yes 持仓时优先卖出 Yes）

#### Scenario: Yes 被低估且有 No 持仓
- **WHEN** P(Yes) - Price(Yes) > 0.2 且持有 No 资产
- **THEN** 以 FOK 市价单卖出全部 No 持仓，滑点保护为 BestBid(No) - 0.2

#### Scenario: Yes 被低估且无 No 持仓
- **WHEN** P(Yes) - Price(Yes) > 0.2 且不持有 No 资产
- **THEN** 以 FOK 市价单买入 1 USD 的 Yes，滑点保护为 BestAsk(Yes) + 0.2

#### Scenario: Yes 被高估且有 Yes 持仓
- **WHEN** P(Yes) - Price(Yes) < -0.2 且持有 Yes 资产
- **THEN** 以 FOK 市价单卖出全部 Yes 持仓，滑点保护为 BestBid(Yes) - 0.2

#### Scenario: Yes 被高估且无 Yes 持仓
- **WHEN** P(Yes) - Price(Yes) < -0.2 且不持有 Yes 资产
- **THEN** 以 FOK 市价单买入 1 USD 的 No，滑点保护为 BestAsk(No) + 0.2

#### Scenario: 偏差在阈值内
- **WHEN** |P(Yes) - Price(Yes)| ≤ 0.2
- **THEN** 不执行交易

### Requirement: 元数据返回
策略 SHALL 在每次执行的 meta 中返回计算得出的 PYes 值。

#### Scenario: 成功计算概率
- **WHEN** 概率计算完成
- **THEN** meta 中包含 "PYes" 键，值为计算得出的概率

### Requirement: 滑点保护
所有 FOK 订单 SHALL 设置滑点保护：买单为 BestAsk + 0.2，卖单为 BestBid - 0.2。

#### Scenario: 买单滑点保护
- **WHEN** 下买单
- **THEN** Price 字段设置为对应资产的 BestAsk + 0.2

#### Scenario: 卖单滑点保护
- **WHEN** 下卖单
- **THEN** Price 字段设置为对应资产的 BestBid - 0.2
