# cinar/indicator v2 指标替换实施方案

> 日期: 2026-04-10
> 版本: v4.1 (补充代码级细节)
> 范围: 引入 `SuperTrend`、引入 `STC`、直接移除 `MACD`
> 依赖: `github.com/cinar/indicator/v2`

---

## 一、先说结论

这轮不再做“`STC` 和 `MACD` 并存一段时间再决定”的保守方案，直接做取舍。

最终目标很明确：

1. `Structure` 层新增 `SuperTrend`
2. `Risk` 摘要能看到 `SuperTrend`，但它**不参与** `nearest_below_entry / nearest_above_entry` 的锚点竞争
3. `Indicator` 层新增 `STC`
4. `MACD` 整条链路直接删除，不再保留参数、输出、默认值、校验项
5. `RSI` 保留，不能动
6. `MFI`、`Donchian`、`VWAP` 本轮不做

这个方案的核心思路不是“多塞几个指标”，而是把现在真正缺的两类信息补上：

- `SuperTrend` 补的是结构状态和 trailing 参考
- `STC` 补的是趋势建立和衰退的相位信息

而 `MACD` 在你当前这个系统里，本质上只是给 LLM 多提供一组 EMA 派生的动量描述。长期和 `STC` 一起留着，信息价值不高，输入冗余反而更真实。

---

## 二、已经确认过的前提

### 2.1 `go-talib` 不是 CGO 绑定

现在仓库里用的是 `github.com/markcheno/go-talib`，它是纯 Go 实现，不是 TA-Lib 的 C 绑定。

所以这轮引入 `cinar/indicator` 的理由只有一个：

**它提供了你现在真要用的 `SuperTrend` 和 `STC`。**

不是为了“摆脱 CGO”，也不是为了“统一技术指标库”。

### 2.2 这轮会影响哪里

这轮不是“只改压缩器，不动其他地方”的小补丁。

真实影响面是这条链路：

```text
Klines
  -> Indicator Compress
  -> Trend Compress
  -> Risk Prompt Anchors
  -> Agent Prompt
  -> Provider Prompt
  -> InPosition Prompt
  -> Risk Prompt
```

不会动的部分也要说清楚：

- 不改 consensus 算法
- 不改 gate / sieve 规则引擎
- 不改 FSM
- 不改执行层
- 不做 channel streaming
- 不做 MCP tool use
- 不做 cinar 的 strategy / backtest / repository 接入

### 2.3 本轮的取舍

这轮的取舍不是“指标越多越好”，而是：

- 保留有硬依赖的信号
- 补缺失的信号
- 删除重复的信号

所以本轮保留 / 新增 / 删除如下：

| 类别 | 项目 | 处理 |
|------|------|------|
| 保留 | EMA | 保留 |
| 保留 | RSI | 保留 |
| 保留 | ATR | 保留 |
| 保留 | OBV | 保留 |
| 新增 | SuperTrend | 新增 |
| 新增 | STC | 新增 |
| 删除 | MACD | 直接删除 |
| 暂缓 | MFI | 暂缓 |
| 暂缓 | Donchian | 暂缓 |
| 暂缓 | VWAP | 暂缓 |

---

## 三、为什么 `RSI` 不能删，为什么 `MACD` 可以删

### 3.1 `RSI` 不能删

`RSI` 不是“只是给 Indicator Agent 看一眼”的字段。

它已经进入了两条别的链路：

1. `Hard Guard` 直接依赖 `RSI` 极值退出
2. `Structure` 压缩会把 `RSI` 塞进 `structure_points` 和 `recent_candles`

这意味着删 `RSI` 不叫“换指标”，那叫“重写风控和结构解释”。

本轮不做这件事。

### 3.2 `MACD` 可以删

`MACD` 和 `STC` 在你现在这个系统里的角色非常接近，都是：

- 让 LLM 理解趋势有没有建立
- 让 LLM 理解动量有没有衰退
- 让 LLM 理解相位有没有切换

区别是：

- `MACD` 更传统
- `STC` 对切换更快
- 你现在不是在写硬编码策略，而是在给 LLM 提供压缩后的判断证据

在这个语境里，`STC` 比 `MACD` 更适合留下来。

所以这轮不再把 replay 当成“删不删 `MACD` 的决策工具”，而是把它降级成**上线前的回归烟测**。  
也就是说：

- 设计决策已经做了，`STC` 替换 `MACD`
- replay 只负责帮你看有没有异常漂移
- replay 不再负责决定“到底删不删”

---

## 四、本轮不做的东西，为什么不做

### 4.1 `MFI`

先不做。

原因不是它没用，而是你当前这套输入语义还不稳。

如果继续沿用“取 bars 最多的 interval”这种实现，在等长 interval 下会退化成 Go map 迭代顺序，结果不稳定。这不是小瑕疵，是会把同一份输入在不同运行里算出不同 `MFI` 的问题。

这类问题一旦进入 LLM 输入，会被放大成解释漂移。

所以 `MFI` 暂缓，恢复条件只有三种：

1. 按每个 interval 独立输出，做成 `mfi_by_interval`
2. 明确绑定到最短周期
3. 只在单一周期币种启用

### 4.2 `Donchian`

先不做。

现在结构层已经有：

- `range_high`
- `range_low`
- swing levels
- EMA levels
- Bollinger upper/lower

在这个基础上再塞 `Donchian`，大概率只是重复，不会明显增加信息量。

除非以后明确决定：

- 用 `Donchian` 替掉 `range_high / range_low`
- 或者单独把它定义成 breakout boundary

否则不值得进这轮。

### 4.3 `VWAP`

标准 session VWAP 不做。

当前数据模型是固定窗口快照，不是带交易时段语义的日内流。
这不适合直接上 session VWAP。

以后真要做，也优先：

1. `anchored VWAP`
2. `rolling VWAP`
3. 最后才是 session VWAP

---

## 五、实施原则

这轮实现必须守住四个原则。

### 5.1 一个指标只服务一个明确目标

- `SuperTrend` 服务 `Structure + Risk`
- `STC` 服务 `Indicator`

不要把 `SuperTrend` 拿去抢结构锚点，不要把 `STC` 重新做成一套“半个 MACD”。

### 5.2 runtime 和 validation 用同一套 required bars 逻辑

不能出现这种情况：

- runtime 说 bars 足够，可以算
- config validation 说 bars 不够，不让过

或者反过来。

这类漂移最容易在后面埋坑。

所以本轮要把 required bars 逻辑收成一个共享 helper，validation、preset、实现都从这里取值，不再手写公式。

这里先把语义写死，不然后面还是会各写各的：

- `SuperTrendRequiredBars` / `STCRequiredBars` 返回的是 runtime 判定用的最小输入 bars 数，也就是 `len(candles) >= N` 时允许开始计算
- helper 内部已经把上游 `IdlePeriod()` 到首个可用值之间的差额算进去，外层聚合函数不要再顺手 `+1`
- `TrendPresetRequiredBars()` 和 `requiredKlineLimit()` 只负责取 max，不负责再发明第二层 buffer

如果后面确认某个老指标链路确实还需要 safety margin，也只能定义成一个共享常量，从同一个地方取，不能 runtime 一套、validation 一套。

### 5.3 `SuperTrend` 只做独立摘要，不进入 anchor 竞争

这点必须写死。

因为你现在 anchor 的选择逻辑本来就是“最近距离优先”，如果把短周期的 trailing level 混进去，它很容易抢走 swing / order block / range 的位置。

这会直接改风险计划，而且是那种很难从表面看出来的漂移。

### 5.4 tracked 文件全收口，本地未跟踪文件不顺手乱改

这轮会更新 repo 里真正受影响的 tracked 文件，比如：

- `configs/symbols/default.toml`
- `docs/configuration.org`
- `webui/config-onboarding-prototype/index.html`
- `webui/config-onboarding-prototype/main.js`

但不会顺手去改你本地那些未跟踪的 symbol config。

如果那些本地文件里还残留 `macd_*`，由于 mapstructure 对未知字段通常是忽略的，它们不会阻塞程序运行。后面你再手动清理就行。

---

## 六、完整改动设计

这一节按文件拆开写，尽量写到“打开文件就能开始改”的程度。

### 6.1 `go.mod`

新增依赖：

```go
require github.com/cinar/indicator/v2 <version>
```

这里不做别的指标库替换，也不移除 `go-talib`。

`go-talib` 还要继续承担：

- `EMA`
- `RSI`
- `ATR`
- `BBands`

本轮只让 `cinar` 提供：

- `SuperTrend`
- `STC`

Step 1 先做一件很小但很重要的事：把依赖接进来以后，先本地确认一次上游真实 API 名字和 `IdlePeriod()` 行为，再把后面的 helper 和调用点写死。这样可以避免文档里的构造函数名和真实版本不一致，后面整条链一起返工。

### 6.2 新增共享 required-bars helper

新建文件：`internal/config/indicator_requirements.go`

```go
package config

import (
	ctrend "github.com/cinar/indicator/v2/trend"
	cvol "github.com/cinar/indicator/v2/volatility"
)

const (
	DefaultSTCKPeriod = 10
	DefaultSTCDPeriod = 3
)

// SuperTrendRequiredBars 返回 runtime 允许开始计算 SuperTrend 的最小输入 bar 数。
// 直接调用上游库的 IdlePeriod(), 不手写近似公式。
func SuperTrendRequiredBars(period int, multiplier float64) int {
	st := cvol.NewSuperTrendWithPeriod[float64](period, multiplier)
	return st.IdlePeriod() + 1
}

// STCRequiredBars 返回 runtime 允许开始计算 STC 的最小输入 bar 数。
func STCRequiredBars(fast, slow int) int {
	stc := ctrend.NewStcWithPeriod[float64](fast, slow, DefaultSTCKPeriod, DefaultSTCDPeriod)
	return stc.IdlePeriod() + 1
}
```

注意: 这个文件会引入 `cinar/indicator/v2`, 使得 `config` 包依赖它。如果你不想让 config 包直接依赖外部指标库, 可以把这两个函数放到 `internal/decision/features/` 下。但考虑到 `validation_symbol.go` 和 `trend_presets.go` 都在 config 包, 放 config 里调用链更短。

这里再写死一条：这两个 helper 返回的就是 runtime 判定阈值，调用方不要再额外 `+1`。如果后面还要留 safety margin，只能在一个共享常量里加，不能各处自己补。

### 6.3 `internal/config/trend_presets.go`

当前 `TrendPreset` 结构体 (trend_presets.go:11) 有 15 个字段。增加三个：

```go
type TrendPreset struct {
	// ... 现有 15 个字段保持不动 ...
	SuperTrendPeriod     int
	SuperTrendMultiplier float64
	SkipSuperTrend       bool
}
```

`DefaultTrendPreset()` (trend_presets.go:36) 补默认值：

```go
SuperTrendPeriod:     14,
SuperTrendMultiplier: 2.5,
```

`presetRequiredBars()` (trend_presets.go:180) 当前是：

```go
func presetRequiredBars(preset TrendPreset) int {
	return maxInt(
		preset.RSIPeriod,
		preset.ATRPeriod,
		preset.EMA20Period,
		preset.EMA50Period,
		preset.EMA200Period,
		preset.VolumeMAPeriod,
		preset.RecentCandles,
		preset.FractalSpan*2+1,
	)
}
```

改成：

```go
func presetRequiredBars(preset TrendPreset) int {
	stRequired := 0
	if !preset.SkipSuperTrend && preset.SuperTrendPeriod > 0 {
		stRequired = SuperTrendRequiredBars(preset.SuperTrendPeriod, preset.SuperTrendMultiplier)
	}
	return maxInt(
		preset.RSIPeriod,
		preset.ATRPeriod,
		preset.EMA20Period,
		preset.EMA50Period,
		preset.EMA200Period,
		preset.VolumeMAPeriod,
		preset.RecentCandles,
		preset.FractalSpan*2+1,
		stRequired,
	)
}
```

`trendPresetByRole()` (trend_presets.go:132) 里各 role 分支不需要单独设 SuperTrend 参数, 让它们继承 `DefaultTrendPreset()` 的值就行。

另外把 `TrendPresetRequiredBars()` 一起改掉，只做 max 聚合，不再额外 `+1`。否则 runtime 用 `n >= requiredBars`，validation 用 `requiredBars+1`，还是会多出一档。

### 6.4 `internal/decision/features/trend_compress.go`

这是 `SuperTrend` 的主战场。

#### 6.4.1 新增结构体

新增：

```go
type TrendSuperTrendSnapshot struct {
	Interval    string  `json:"interval"`
	State       string  `json:"state"`
	Level       float64 `json:"level"`
	DistancePct float64 `json:"distance_pct"`
}
```

这里特意把 `Interval` 放进结构体里，不要省。

原因很简单：

- `Risk` 摘要最后只会保留一个 `supertrend`
- 如果不带 `interval`，后面的 LLM 看到了 level，却不知道这是 `1h` 还是 `4h`

#### 6.4.2 `TrendCompressedInput` 增字段

```go
SuperTrend *TrendSuperTrendSnapshot `json:"supertrend,omitempty"`
```

#### 6.4.3 `TrendCompressOptions` 增字段

```go
SuperTrendPeriod     int
SuperTrendMultiplier float64
SkipSuperTrend       bool
```

`DefaultTrendCompressOptions()` 补默认值。

`normalizeTrendCompressOptions()` 补零值回填。

#### 6.4.4 计算位置

放在 `BuildTrendCompressedInput` (trend_compress.go:189) 里:
- `gc` 构建完成之后 (约 265 行)
- `structurePoints := selectStructurePoints(...)` 之前 (约 267 行)

当前 `BuildTrendCompressedInput` 的签名是:

```go
func BuildTrendCompressedInput(symbol, interval string, candles []snapshot.Candle, opts TrendCompressOptions) (TrendCompressedInput, error)
```

`interval` 参数已经在函数签名里, 直接传给 `buildSuperTrendSnapshot`。

代码形态：

```go
var superTrend *TrendSuperTrendSnapshot
if !opts.SkipSuperTrend {
	requiredBars := config.SuperTrendRequiredBars(opts.SuperTrendPeriod, opts.SuperTrendMultiplier)
	if n >= requiredBars {
		st := cvol.NewSuperTrendWithPeriod[float64](opts.SuperTrendPeriod, opts.SuperTrendMultiplier)
		stResult := helper.ChanToSlice(st.Compute(
			helper.SliceToChan(highs),
			helper.SliceToChan(lows),
			helper.SliceToChan(closes),
		))
		superTrend = buildSuperTrendSnapshot(interval, stResult, closes)
	}
}
```

注意: `highs`, `lows`, `closes` 在函数开头 (约 197-203 行) 已经从 candles 提取好了, 直接复用。

最后在 return 的 `TrendCompressedInput` (约 287 行) 里加 `SuperTrend: superTrend`。

#### 6.4.5 `buildSuperTrendSnapshot`

建议签名：

```go
func buildSuperTrendSnapshot(interval string, stSeries, closes []float64) *TrendSuperTrendSnapshot
```

判定规则也写死，别留给实现者自己猜：

- 从尾部往前扫，找最后一个 `stSeries[i]` 和 `closes[i]` 都有效的配对位置
- `level = stSeries[i]`
- `close = closes[i]`
- `state` 直接按同一根 bar 的相对位置定：`close >= level` 记为 `UP`，否则记为 `DOWN`
- `distance_pct = abs(close-level) / close * 100`
- 如果最后找不到有效配对，直接返回 `nil`

返回值：

```go
&TrendSuperTrendSnapshot{
	Interval:    interval,
	State:       "UP" / "DOWN",
	Level:       roundFloat(level, 4),
	DistancePct: roundFloat(distPct, 4),
}
```

这里不要先对 `stSeries` 做 `sanitizeSeries()` 再和 `closes` 配对，因为那会把序列头部的无效值裁掉，索引关系虽然大多数时候还能碰巧对上，但语义上不稳。直接在 helper 里按原始索引回扫最安全。

#### 6.4.6 import

新增：

```go
"github.com/cinar/indicator/v2/helper"
cvol "github.com/cinar/indicator/v2/volatility"
```

### 6.5 `internal/runtime/runtime_builder_compression.go`

`toTrendOptionsFromPreset()` 增加三项映射：

```go
SuperTrendPeriod:     preset.SuperTrendPeriod,
SuperTrendMultiplier: preset.SuperTrendMultiplier,
SkipSuperTrend:       preset.SkipSuperTrend,
```

`toIndicatorOptions()` 改动更大，因为这里要接 `STC`，同时删掉 `MACD`。

目标状态：

```go
opts := decision.IndicatorCompressOptions{
	EMAFast:   cfg.EMAFast,
	EMAMid:    cfg.EMAMid,
	EMASlow:   cfg.EMASlow,
	RSIPeriod: cfg.RSIPeriod,
	ATRPeriod: cfg.ATRPeriod,
	STCFast:   cfg.STCFast,
	STCSlow:   cfg.STCSlow,
	LastN:     cfg.LastN,
	Pretty:    cfg.Pretty,
	SkipSTC:   cfg.SkipSTC,
}
```

`!indicatorEnabled` 分支只需要：

```go
opts.SkipEMA = true
opts.SkipRSI = true
opts.SkipSTC = true
```

不再有 `SkipMACD`。

### 6.6 `internal/decision/risk_prompt_anchors.go`

这里是本轮最容易改错的地方。

最终要求只有两条：

1. `supertrend` 要进摘要
2. 它不能进 anchor 竞争

#### 6.6.1 不动 candidates 逻辑

`buildStructureAnchorSummary` (risk_prompt_anchors.go:27) 里的 for 循环当前往 candidates 里 append 了:
- EMA (ema20/50/200)
- swing levels (last_swing_high/low)
- `candidatesFromStructure` (fractal_high/low, band_upper/lower, range_high/low 等)
- `candidatesFromOrderBlock` (order_block_upper/lower)

这些全部照旧。**不要把 SuperTrend append 进 candidates。**

`selectNearestAnchor` (risk_prompt_anchors.go:229) 的逻辑是纯最近距离优先 + 同距离偏短周期, 如果 SuperTrend trailing level 进去, 它在短周期上几乎总是最近的, 会抢掉 swing/order block 的位置。

#### 6.6.2 只补顶层摘要

在 for 循环之后, `summary` 构建之前 (约 94 行), 加:

```go
if st := pickShortestIntervalSuperTrend(byInterval, keys); st != nil {
	summary["supertrend"] = map[string]any{
		"interval":     st.Interval,
		"state":        st.State,
		"level":        st.Level,
		"distance_pct": st.DistancePct,
	}
}
```

新增 helper:

```go
func pickShortestIntervalSuperTrend(byInterval map[string]features.TrendJSON, keys []string) *features.TrendSuperTrendSnapshot {
	// keys 已经按短→长排序 (由 decisionutil.SortedTrendKeys 保证)
	for _, key := range keys {
		block, err := parseTrendCompressedInput(byInterval[key].RawJSON)
		if err != nil {
			continue
		}
		if block.SuperTrend != nil {
			return block.SuperTrend
		}
	}
	return nil
}
```

这样 risk prompt 最终能看到:

```json
{
  "ema_by_interval": { ... },
  "last_swing_by_interval": { ... },
  "nearest_below_entry": { "source": "fractal_low", ... },
  "nearest_above_entry": { "source": "ema200", ... },
  "supertrend": {
    "interval": "1h",
    "state": "UP",
    "level": 3220.45,
    "distance_pct": 0.76
  }
}
```

`nearest_below_entry` / `nearest_above_entry` 仍然由 swing/EMA/bollinger/range/order_block 决定。

### 6.7 `internal/config/prompts.go`

这轮不是只改 `Agent` prompt。要一起改五层, 下面给出每层的具体文案。

当前 prompts.go 里没有任何 MACD 相关文案 (已 grep 确认), 所以不需要删 MACD 文案, 只需要加新的。

#### 6.7.1 `defaultAgentStructurePrompt` (prompts.go:68)

在 "重要约束（防止行动泄漏）" 之前 (约 103 行), 插入:

```go
"新增输入字段说明：\n" +
"- supertrend: 输出当前趋势状态 state(UP/DOWN) 和动态 trailing level。" +
"state 翻转是 regime change 的强信号, 应显著影响 regime 和 movement_score 判断。" +
"level 可作为支撑/阻力的辅助参考, 但不替代 structure_points 和 key_levels。\n" +
"\n"
```

#### 6.7.2 `defaultProviderStructurePrompt` (prompts.go:106)

在 "signal_tag 需要综合结构状态给出" 之后 (约 116 行), 补:

```go
"- 若输入包含 supertrend, 当 supertrend.state 与 regime 判断一致时, 可增强 clear_structure 置信; " +
"当 supertrend.state 与原方向矛盾时, integrity 应偏保守。\n"
```

#### 6.7.3 `defaultInPosStructurePrompt` (prompts.go:119)

在 "当结构信号与仓位状态存在冲突时" 之前 (约 125 行), 补:

```go
"- 若输入包含 supertrend 且发生 state 翻转 (如 UP→DOWN), 或价格跌破/突破 supertrend.level, " +
"应作为 threat_level 升级和 tighten 倾向的辅助证据。\n"
```

#### 6.7.4 `defaultAgentIndicatorPrompt` (prompts.go:11)

在 "重要约束" 之前 (约 42 行), 插入:

```go
"新增输入字段说明：\n" +
"- stc: Schaff Trend Cycle (0-100), 比 MACD 更快的趋势相位指标。" +
"state=rising 表示趋势正在建立, state=falling 表示趋势正在衰退, state=flat 表示相位不明确。" +
"可作为 expansion、alignment、movement_score 方向判断的 early confirmation。\n" +
"\n"
```

#### 6.7.5 `defaultProviderIndicatorPrompt` (prompts.go:45)

在 "signal_tag 需要综合整体判断给出" 之后 (约 56 行), 补:

```go
"- 若输入包含 stc, 当 stc 明显上拐(rising)且与其他指标方向一致, 可增强 momentum_expansion 判断; " +
"stc falling 且多指标矛盾时 signal_tag 应偏向 momentum_weak 或 noise。\n"
```

#### 6.7.6 `defaultInPosIndicatorPrompt` (prompts.go:59)

在 "不要因为轻微波动就直接输出 exit" 之前 (约 64 行), 补:

```go
"- 若输入包含 stc, 可作为 divergence_detected 和 momentum_sustaining 的辅助证据: " +
"stc 由 rising 转 falling 时应增加 divergence 置信。\n"
```

#### 6.7.7 `defaultRiskFlatInitPrompt` (prompts.go:182)

在约束中 (约 202 行 "结构锚点摘要中的 nearest_below_entry" 之后), 补:

```go
"- 若结构锚点摘要包含 supertrend, 它是独立的 trailing 参考, 不是 nearest_below_entry / nearest_above_entry 的替代。" +
"supertrend.level 可辅助判断止损是否合理, 但最终止损仍应基于结构锚点。\n"
```

#### 6.7.8 `defaultRiskTightenUpdatePrompt` (prompts.go:214)

在约束中 (约 230 行), 补:

```go
"- 若结构锚点摘要包含 supertrend, 其 level 可作为 trailing stop 收紧的参考锚点, " +
"但不应替代基于 swing / EMA / order block 的止损逻辑。\n"
```

### 6.8 `internal/config/types.go`

当前 `IndicatorConfig` (types.go:124):

```go
type IndicatorConfig struct {
	EMAFast    int  `mapstructure:"ema_fast"`
	EMAMid     int  `mapstructure:"ema_mid"`
	EMASlow    int  `mapstructure:"ema_slow"`
	RSIPeriod  int  `mapstructure:"rsi_period"`
	ATRPeriod  int  `mapstructure:"atr_period"`
	MACDFast   int  `mapstructure:"macd_fast"`    // 删
	MACDSlow   int  `mapstructure:"macd_slow"`    // 删
	MACDSignal int  `mapstructure:"macd_signal"`  // 删
	LastN      int  `mapstructure:"last_n"`
	Pretty     bool `mapstructure:"pretty"`
}
```

改成:

```go
type IndicatorConfig struct {
	EMAFast   int  `mapstructure:"ema_fast"`
	EMAMid    int  `mapstructure:"ema_mid"`
	EMASlow   int  `mapstructure:"ema_slow"`
	RSIPeriod int  `mapstructure:"rsi_period"`
	ATRPeriod int  `mapstructure:"atr_period"`
	STCFast   int  `mapstructure:"stc_fast"`
	STCSlow   int  `mapstructure:"stc_slow"`
	SkipSTC   bool `mapstructure:"skip_stc"`
	LastN     int  `mapstructure:"last_n"`
	Pretty    bool `mapstructure:"pretty"`
}
```

不保留 MACD 兼容字段。旧 TOML 里残留的 `macd_*` 字段会被 mapstructure 忽略, 不会报错。

### 6.9 `internal/config/defaults.go`

当前 `DefaultSymbolConfig()` (defaults.go:9) 的 `Indicators` 块 (defaults.go:26-36):

```go
Indicators: IndicatorConfig{
	EMAFast:    21,
	EMAMid:     50,
	EMASlow:    200,
	RSIPeriod:  14,
	ATRPeriod:  14,
	MACDFast:   12,   // 删
	MACDSlow:   26,   // 删
	MACDSignal: 9,    // 删
	LastN:      5,
},
```

改成:

```go
Indicators: IndicatorConfig{
	EMAFast:   21,
	EMAMid:    50,
	EMASlow:   200,
	RSIPeriod: 14,
	ATRPeriod: 14,
	STCFast:   23,
	STCSlow:   50,
	LastN:     5,
},
```

### 6.10 `internal/config/validation_symbol.go`

这里要改两处。

#### 6.10.1 删除 MACD 校验

`validateIndicatorConfig` (validation_symbol.go:62) 当前:

```go
func validateIndicatorConfig(cfg IndicatorConfig) error {
	if cfg.EMAFast <= 0 || cfg.EMAMid <= 0 || cfg.EMASlow <= 0 {
		return validationErrorf("indicators.ema_fast/ema_mid/ema_slow must be > 0")
	}
	if cfg.RSIPeriod <= 0 {
		return validationErrorf("indicators.rsi_period must be > 0")
	}
	if cfg.ATRPeriod <= 0 {
		return validationErrorf("indicators.atr_period must be > 0")
	}
	if cfg.MACDFast <= 0 || cfg.MACDSlow <= 0 || cfg.MACDSignal <= 0 {  // 删掉这整个 if
		return validationErrorf("indicators.macd_fast/macd_slow/macd_signal must be > 0")
	}
	if cfg.LastN <= 0 {
		return validationErrorf("indicators.last_n must be > 0")
	}
	return nil
}
```

删掉 MACD 校验块, 加 STC 校验:

```go
func validateIndicatorConfig(cfg IndicatorConfig) error {
	if cfg.EMAFast <= 0 || cfg.EMAMid <= 0 || cfg.EMASlow <= 0 {
		return validationErrorf("indicators.ema_fast/ema_mid/ema_slow must be > 0")
	}
	if cfg.RSIPeriod <= 0 {
		return validationErrorf("indicators.rsi_period must be > 0")
	}
	if cfg.ATRPeriod <= 0 {
		return validationErrorf("indicators.atr_period must be > 0")
	}
	if !cfg.SkipSTC && (cfg.STCFast <= 0 || cfg.STCSlow <= 0) {
		return validationErrorf("indicators.stc_fast/stc_slow must be > 0 when STC is enabled")
	}
	if cfg.LastN <= 0 {
		return validationErrorf("indicators.last_n must be > 0")
	}
	return nil
}
```

#### 6.10.2 `requiredKlineLimit` 改造

当前 (validation_symbol.go:175):

```go
func requiredKlineLimit(cfg SymbolConfig) int {
	trendRequired := TrendPresetRequiredBars(cfg.Intervals)
	required := maxInt(
		cfg.Indicators.EMAFast,
		cfg.Indicators.EMAMid,
		cfg.Indicators.EMASlow,
		cfg.Indicators.RSIPeriod,
		cfg.Indicators.ATRPeriod,
		cfg.Indicators.MACDFast,    // 删
		cfg.Indicators.MACDSlow,    // 删
		cfg.Indicators.MACDSignal,  // 删
		trendRequired,
	)
	return max(1, required)
}
```

改成:

```go
func requiredKlineLimit(cfg SymbolConfig) int {
	trendRequired := TrendPresetRequiredBars(cfg.Intervals)
	stcRequired := 0
	if !cfg.Indicators.SkipSTC {
		stcRequired = STCRequiredBars(cfg.Indicators.STCFast, cfg.Indicators.STCSlow)
	}
	required := maxInt(
		cfg.Indicators.EMAFast,
		cfg.Indicators.EMAMid,
		cfg.Indicators.EMASlow,
		cfg.Indicators.RSIPeriod,
		cfg.Indicators.ATRPeriod,
		stcRequired,
		trendRequired,
	)
	return max(1, required)
}
```

注意: `trendRequired` 已经通过 `presetRequiredBars` → `SuperTrendRequiredBars` 包含了 SuperTrend 的 warmup, 所以这里不需要再单独加 SuperTrend 项。只需要加 STC。

### 6.11 `internal/decision/features/indicator_compress.go`

这是 `STC` 上线、`MACD` 下线的主文件。当前 554 行。

#### 6.11.1 删除 `MACD` 全部痕迹

删除以下内容:

| 行号 | 内容 | 说明 |
|------|------|------|
| 68:data struct | `MACD *macdSnapshot` | indicatorData 字段 |
| 85-93 | `type macdSnapshot struct { ... }` | 整个类型定义 |
| 110-113 | `type seriesSnapshot struct { ... }` | 只被 MACD histogram 用, 删后检查是否有其他引用 |
| 17-31:Options | `MACDFast/MACDSlow/MACDSignal/SkipMACD` | IndicatorCompressOptions 字段 |
| 33-45:Defaults | `MACDFast:12, MACDSlow:26, MACDSignal:9` | DefaultIndicatorCompressOptions 字段 |
| 212-221 | `if !opts.SkipMACD && maxPeriod > 0 { ... }` | buildIndicatorData 中 MACD 计算分支 |
| 256-268:normalize | `opts.MACDFast/MACDSlow/MACDSignal` | normalizeIndicatorCompressOptions 中 MACD 回填 |
| 292-322 | `func buildMACDSnapshot(...) *macdSnapshot { ... }` | 整个函数 |
| 390-420 | `func macdSignal(...) string { ... }` | 整个函数 |
| 422-427 | `func minInt(a, b int) int { ... }` | 检查是否只被 macdSignal 使用, 若是则删 |

`seriesSnapshot` 的引用检查: 它只在 `macdSnapshot.Histogram` 里用。`buildMACDSnapshot` 里构造。删 MACD 后, 如果没有其他引用, 一起删。

#### 6.11.2 新增 `STC`

新增类型 (放在 `obvSnapshot` 之后):

```go
type stcSnapshot struct {
	Current float64   `json:"current"`
	LastN   []float64 `json:"last_n,omitempty"`
	State   string    `json:"state,omitempty"` // "rising" / "falling" / "flat"
}
```

`indicatorData` 新增字段 (放在 OBV 之后):

```go
STC *stcSnapshot `json:"stc,omitempty"`
```

`IndicatorCompressOptions` 新增 (替换删掉的 MACD 字段位置):

```go
STCFast int
STCSlow int
SkipSTC bool
```

`DefaultIndicatorCompressOptions()` 补:

```go
STCFast: 23,
STCSlow: 50,
```

`normalizeIndicatorCompressOptions()` 补:

```go
if opts.STCFast <= 0 {
	opts.STCFast = def.STCFast
}
if opts.STCSlow <= 0 {
	opts.STCSlow = def.STCSlow
}
```

#### 6.11.3 计算逻辑

放在 `buildOBVSnapshot` 之后 (原来 MACD 分支被删后, OBV 是最后一个指标):

```go
if !opts.SkipSTC {
	requiredBars := config.STCRequiredBars(opts.STCFast, opts.STCSlow)
	if len(closes) >= requiredBars {
		stcInd := ctrend.NewStcWithPeriod[float64](
			opts.STCFast,
			opts.STCSlow,
			config.DefaultSTCKPeriod,
			config.DefaultSTCDPeriod,
		)
		stcResult := helper.ChanToSlice(stcInd.Compute(helper.SliceToChan(closes)))
		if s := buildSTCSnapshot(sanitizeSeries(stcResult), opts.LastN); s != nil {
			data.STC = s
		}
	}
}
```

`buildSTCSnapshot`:

```go
func buildSTCSnapshot(series []float64, tail int) *stcSnapshot {
	const stcStateDelta = 2.0

	if len(series) == 0 {
		return nil
	}
	cur := series[len(series)-1]
	s := &stcSnapshot{
		Current: roundFloat(cur, 4),
		LastN:   roundSeriesTail(series, tail),
	}
	if len(s.LastN) >= 2 {
		prev := s.LastN[len(s.LastN)-2]
		switch {
		case cur-prev > stcStateDelta:
			s.State = "rising"
		case prev-cur > stcStateDelta:
			s.State = "falling"
		default:
			s.State = "flat"
		}
	}
	return s
}
```

`10/3` 和 `2.0` 都别散落在多个文件里。前者收成共享常量，后者至少在函数内收成命名常量，不要留裸数字。

#### 6.11.4 import

删除: 无 (talib 仍被 EMA/RSI/ATR 使用)。

新增:

```go
"brale-core/internal/config"
"github.com/cinar/indicator/v2/helper"
ctrend "github.com/cinar/indicator/v2/trend"
```

### 6.12 `configs/symbols/default.toml`

当前 `[indicators]` 块 (default.toml:30-50):

```toml
[indicators]
ema_fast = 21
ema_mid = 50
ema_slow = 200
rsi_period = 14
atr_period = 14
macd_fast = 12      # 删
macd_slow = 26      # 删
macd_signal = 9     # 删
last_n = 5
pretty = false
```

改成:

```toml
[indicators]
# EMA 快线周期。
ema_fast = 21
# EMA 中线周期。
ema_mid = 50
# EMA 慢线周期。
ema_slow = 200
# RSI 计算周期。
rsi_period = 14
# ATR 计算周期。
atr_period = 14
# STC 快线周期 (Schaff Trend Cycle, 替代 MACD)。
stc_fast = 23
# STC 慢线周期。
stc_slow = 50
# 指标序列取最近 N 根窗口用于 LLM 总结。
last_n = 5
# 是否格式化输出 JSON（会增加体积）。
pretty = false
```

### 6.13 `docs/configuration.org`

configuration.org:35 当前:

```text
- 技术指标窗口参数（EMA / RSI / ATR / MACD）
```

改成:

```text
- 技术指标窗口参数（EMA / RSI / ATR / STC）
```

如果文件里其他地方有 MACD 的具体例子, 一并改成 STC。

### 6.14 `internal/decision/features/trend_compress_test.go`

新增测试至少覆盖：

1. 足够 bars 时能产出 `supertrend`
2. `state` 只能是 `UP` / `DOWN`
3. `level > 0`
4. `bar 数 < required bars` 时省略
5. `bar 数 == required bars` 时刚好产出
6. 非默认参数下 required bars 随参数变化
7. 横盘不产出 `NaN`

### 6.15 `internal/decision/features/indicator_compress_test.go`

新增测试至少覆盖：

1. 足够 bars 时产出 `stc`
2. `stc.current` 在 `[0,100]`
3. `state` 只能是 `rising/falling/flat`
4. `macd` 字段不存在
5. `bar 数 < required bars` 时省略
6. `bar 数 == required bars` 时刚好产出
7. 非默认参数下 required bars 随参数变化
8. 横盘不产出 `NaN`

### 6.16 `internal/decision/risk_prompt_anchors_test.go`

新增测试覆盖：

1. `summary` 顶层含 `supertrend`
2. `supertrend` 带 `interval`
3. `nearest_below_entry / nearest_above_entry` 的 `source` 不会是 `supertrend`
4. 没有 `supertrend` 的 block 不报错

### 6.17 `internal/config/validation_symbol_test.go`

新增测试覆盖：

1. `ValidateSymbolConfig()` 不再要求 `macd_*`
2. `requiredKlineLimit()` 会把 `STC` warmup 算进去
3. `skip_stc=true` 时 `requiredKlineLimit()` 不再被 `STC` 拉高

---

## 七、实现顺序

推荐按下面顺序改，不要乱序。

### Step 1: 先接依赖和 required-bars helper

先做：

- `go.mod`
- `internal/config/indicator_requirements.go`

因为后面很多地方都要用这个 helper。依赖接进来以后，先本地确认一次 `cinar` 的真实构造函数名和 `IdlePeriod()` 行为，再继续往下改。

### Step 2: 再做 `SuperTrend`

按顺序改：

1. `trend_presets.go`
2. `trend_compress.go`
3. `runtime_builder_compression.go`
4. `risk_prompt_anchors.go`
5. `prompts.go`
6. `trend_compress_test.go`
7. `risk_prompt_anchors_test.go`

这一阶段结束时，系统应该已经能：

- 在 `Trend` 输出里看到 `supertrend`
- 在 risk summary 顶层看到 `supertrend`
- 但 anchor 竞争逻辑不变

### Step 3: 再做 `STC` 替换 `MACD`

按顺序改：

1. `types.go`
2. `defaults.go`
3. `validation_symbol.go`
4. `runtime_builder_compression.go`
5. `indicator_compress.go`
6. `prompts.go`
7. `configs/symbols/default.toml`
8. `docs/configuration.org`
9. `webui/config-onboarding-prototype/index.html`
10. `webui/config-onboarding-prototype/main.js`
11. `indicator_compress_test.go`
12. `validation_symbol_test.go`

这一阶段结束时，系统应该已经：

- 没有 `MACD`
- 有 `STC`
- 模板配置里没有 `macd_*`

### Step 4: 最后做回归烟测

这里不是为了决定删不删 `MACD`，而是为了确认没有明显异常。

检查四件事：

1. `Indicator` prompt 输入能被正常消费
2. `movement_score` 没有离谱漂移
3. `gate action` 没有系统性翻转
4. risk plan 没有出现明显异常收紧或异常放宽

---

## 八、验证方式

### 8.1 单元测试

最少跑这些：

```bash
go test ./internal/decision/features/...
go test ./internal/decision/...
go test ./internal/config/...
```

### 8.2 烟测重点

重点不是“测试都绿了就完事”，而是看三类输出：

1. `Indicator` JSON 里没有 `macd`，有 `stc`
2. `Trend` JSON 里有 `supertrend`
3. `structure anchor summary` 里有顶层 `supertrend`，但 `nearest_*` 还是原来的结构来源

### 8.3 replay 烟测

如果有历史快照或日志回放条件，额外做一轮：

1. 抽 20~50 个历史决策周期
2. 跑新方案
3. 看：
   - `movement_score` 是否明显偏移
   - `movement_confidence` 是否明显塌陷
   - `ALLOW/WAIT/VETO` 是否出现大面积反转
   - risk plan 是否出现大量异常紧止损

这一步不是 gate，只是 sanity check。

---

## 九、回滚方案

这轮不涉及数据库 schema，不涉及持久化迁移，所以回滚很简单。

如果发现新链路明显不对，可以按下面顺序回退：

1. 去掉 `SuperTrend`
2. 恢复 `MACD`
3. 去掉 `STC`
4. 恢复模板配置和文档
5. 保留 required-bars helper，不一定要删

因为这轮改动主要集中在压缩和 prompt 层，回滚成本低，不会碰数据面。

---

## 十、这轮明确不清理的东西

虽然前面已经发现了死路径，但这轮先不顺手做：

1. `MechanicsCompressOptions.Sentiment`
2. `MechanicsCompressedInput.Liquidations`

原因不是它们没问题，而是这轮的主题不是清理 mechanics 死代码。

本轮的目标很单纯：

- `SuperTrend` 上去
- `STC` 上去
- `MACD` 下来

别在同一轮把问题摊太大。

---

## 十一、最终交付状态

这轮结束后，仓库应该是这个状态：

- `Indicator` 输入包含：`EMA / RSI / ATR / OBV / STC`
- `Trend` 输入包含：`EMA / RSI / ATR / Bollinger / structure / SuperTrend`
- `MACD` 不再存在于压缩器、配置、模板、校验和文档里
- `supertrend` 能被风险层看见，但不会破坏当前结构锚点选择
- required bars 的判断只有一个真值源，不再 runtime 一套、validation 一套

这就是本轮应该交付的完整结果。
