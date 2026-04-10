## MODIFIED Requirements

### Requirement: Market 结构体
Market 结构体 SHALL 包含市场 slug、条件 ID、Yes 代币 ID、No 代币 ID 和市场结束时间（EndDate）。

#### Scenario: 构建交易状态时包含市场结束时间
- **WHEN** UpdownSeriesTrader 构建或更新 trading.Market
- **THEN** EndDate 字段 SHALL 被设置为对应 polymarket.Market 的 EndDate 值
