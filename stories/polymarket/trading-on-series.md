# 支持在 series 上进行策略交易

## 背景

此前策略交易系统已经支持了在指定市场上进行交易，现在需要新增对几个特定的 series 进行交易，比如 btc-updown-15m ，它是一个循环事件：

```
series: btc-updown-15m
- event: btc-updown-15m-1774712700
  - market: btc-updown-15m-1774712700
- event: btc-updown-15m-1774713600
  - market: btc-updown-15m-1774713600
- event: btc-updown-15m-1774714500
  - market: btc-updown-15m-1774713600
- ...
```

每间隔 15 分钟一轮 00:00-00:15 、 00:15-00:30 、 00:30-00:45 、 ... 以此类推

还有与之类似的 btc-updown-5m （每间隔 5 分钟一轮）

每小时一轮的会稍微复杂点，但也是有规律的，比如 btc-up-or-down-hourly 和 btc-up-or-down-daily ：

```
series: btc-up-or-down-hourly
- event: bitcoin-up-or-down-march-28-2026-10pm-et
  - market: bitcoin-up-or-down-march-28-2026-10pm-et
- event: bitcoin-up-or-down-march-28-2026-11pm-et
  - market: bitcoin-up-or-down-march-28-2026-11pm-et
- event: bitcoin-up-or-down-march-29-2026-12am-et
  - market: bitcoin-up-or-down-march-29-2026-12am-et
- ...
```

```
series: btc-up-or-down-daily
- event: bitcoin-up-or-down-on-march-28-2026
  - market: bitcoin-up-or-down-on-march-28-2026
- event: bitcoin-up-or-down-on-march-29-2026
  - market: bitcoin-up-or-down-on-march-29-2026
- event: bitcoin-up-or-down-on-march-30-2026
  - market: bitcoin-up-or-down-on-march-30-2026
- ...
```

除了 btc 还有 eth-updown-5m 、 eth-updown-15m 、 ...

## 用户用法

通过 `nfa tools polymarket trade --series btc-updown-15m` 可以指定在这种 series 上交易

## 行为

执行器应该自动进入当前时间对应市场进行交易，市场时间区间结束后自动进入下一个市场进行交易，永续执行下去，直到用户 ctrl+c 手动终止。

进入哪些市场应该根据当前时间和市场格式自动推算，不需要查询 series 有哪些市场，因为这种 series 的市场非常多，查询效率较低。

每一轮结束时持仓应该结算并清空，赌赢的每一份价值 1 美元，赌输的价值归零。现金总额在轮与轮之间继承。

## 当前需要支持的 series

- `btc-updown-5m` (示例 market `btc-updown-5m-1774751100` 时间戳是开始时间）
- `btc-updown-15m` (示例 market `btc-updown-15m-1774712700` 时间戳是开始时间)
- `btc-up-or-down-hourly` (示例 market `bitcoin-up-or-down-march-28-2026-10pm-et`)
- `btc-up-or-down-daily` (示例 market `bitcoin-up-or-down-on-march-29-2026`)
- `eth-updown-5m` (示例 market `eth-updown-5m-1774751100` 时间戳是开始时间）
- `eth-updown-15m` (示例 market `eth-updown-5m-1774750500` 时间戳是开始时间）
- `eth-up-or-down-hourly` (示例 market `ethereum-up-or-down-march-28-2026-10pm-et`)
- `eth-up-or-down-daily` (示例 market `ethereum-up-or-down-on-march-29-2026`)
