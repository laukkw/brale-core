# 字段翻译参考手册

本文档列出 Brale-Core 决策通知中所有经过中文翻译的字段，帮助用户理解决策卡片中每个指标的含义。

---

## 一、决策动作（Gate Action）

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| ALLOW | 允许 | Gate 通过，允许开仓 | 三路共识通过，可以执行交易 |
| WAIT | 观望 | 暂不操作 | 当前条件不满足，建议等待 |
| VETO | 否决 | Gate 否决，禁止开仓 | 信号不符合风控条件 |
| EXIT | 平仓 | 建议平仓 | 持仓管理建议退出 |
| TIGHTEN | 收紧风控（意图） | 进入收紧流程 | 风险增加，进入收紧评估。**注意：TIGHTEN 只表示意图，不代表已收紧**。子状态：已执行（executed）/ 满足条件待执行（eligible）/ 收紧受阻（blocked）/ 未触发（not_triggered）/ 未评估 |
| KEEP | 继续持仓 | 维持当前持仓不变 | 监控判定无需收紧，继续持仓 |

## 二、方向（Direction）

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| long | 多头 | 看涨方向 |
| short | 空头 | 看跌方向 |
| conflict | 信号冲突 | 多空信号矛盾 |
| none | 无方向 | 无法判定方向 |

## 三、指标状态字段

### 3.1 EMA 排列（ema_stack）

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| bull | 多头排列 | 快线 > 中线 > 慢线 | 趋势看涨，做多信号增强 |
| bear | 空头排列 | 快线 < 中线 < 慢线 | 趋势看跌，做空信号增强 |
| mixed | 信号混杂/分歧 | EMA 交叉缠绕 | 趋势不明确，建议观望 |

### 3.2 价格 vs EMA（price_vs_ema_fast / mid / slow）

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| above | 上方 | 价格位于该 EMA 上方 |
| below | 下方 | 价格位于该 EMA 下方 |
| near | 附近 | 价格紧贴该 EMA |

### 3.2a EMA 距离（ema_distance_fast_atr / ema_distance_mid_atr）

| 字段名 | 中文名 | 含义 |
|---------|------|------|
| ema_distance_fast_atr | 快线EMA距离(ATR) | 价格与快线EMA的距离，以ATR为单位 |
| ema_distance_mid_atr | 中线EMA距离(ATR) | 价格与中线EMA的距离，以ATR为单位 |

数值越大表示价格偏离均线越远，可能出现回归均值的走势。

### 3.3 RSI 斜率（rsi_slope_state）

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| rising | 上升 | RSI 斜率为正 | 动能正在增强 |
| falling | 下降 | RSI 斜率为负 | 动能正在衰减 |
| flat | 走平/减弱 | RSI 斜率接近零 | 动能维持或转向不明确 |

### 3.4 STC 状态（stc_state）

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| rising | 上升 | STC 指标向上 | 短期趋势偏多 |
| falling | 下降 | STC 指标向下 | 短期趋势偏空 |
| flat | 走平/减弱 | STC 指标走平 | 趋势不明 |

### 3.5 OBV 斜率（obv_slope_state）

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| up | 上行 | 成交量净流入增加 | 量能支持上涨 |
| down | 下行 | 成交量净流出增加 | 量能支持下跌 |
| flat | 走平/减弱 | 成交量净值持平 | 量能无明显方向 |

### 3.6 布林带区间（bb_zone）

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| below_lower | 低于下轨 | 价格突破布林带下轨，可能超卖 |
| near_lower | 靠近下轨 | 价格接近下轨 |
| mid | 中轨区间 | 价格在布林带中部 |
| near_upper | 靠近上轨 | 价格接近上轨 |
| above_upper | 突破上轨 | 价格突破布林带上轨，可能超买 |

### 3.7 布林带宽度（bb_width_state）

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| squeeze | 挤压收窄 | 布林带收窄，波动率极低 | 即将出现大幅波动（方向待定） |
| normal | 正常 | 波动率处于常规范围 | 正常交易环境 |
| wide | 宽幅 | 布林带展开，波动率高 | 注意止损距离 |

### 3.8 震荡指数（chop_regime）

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| trending | 趋势行情 | CHOP 指数低，市场有方向性 | 趋势策略有效 |
| choppy | 震荡行情 | CHOP 指数高，市场横盘 | 建议观望，不追趋势 |
| transition | 过渡阶段 | CHOP 指数处于中间值 | 可能正在转换行情类型 |

### 3.9 ATR 扩张（atr_expand_state）

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| expanding | 波动/动能扩张 | ATR 增大，波动率上升 |
| contracting | 波动/动能收敛 | ATR 缩小，波动率下降 |
| flat | 走平/减弱 | ATR 平稳 |

### 3.10 随机RSI区间（stoch_rsi_zone）

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| oversold | 超卖 | 随机 RSI 低于超卖线 |
| overbought | 超买 | 随机 RSI 高于超买线 |

### 3.11 指标事件（events）

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| price_cross_ema_fast_up | 价格上穿快线EMA | 价格突破快速均线向上 |
| price_cross_ema_fast_down | 价格下穿快线EMA | 价格跌破快速均线 |
| price_cross_ema_mid_up | 价格上穿中线EMA | 价格突破中期均线向上 |
| price_cross_ema_mid_down | 价格下穿中线EMA | 价格跌破中期均线 |
| ema_stack_bull_flip | EMA转为多头排列 | 均线系统翻多 |
| ema_stack_bear_flip | EMA转为空头排列 | 均线系统翻空 |
| aroon_strong_bullish | 阿隆指标强势看多 | 阿隆指标发出强烈看涨信号 |
| aroon_strong_bearish | 阿隆指标强势看空 | 阿隆指标发出强烈看跌信号 |

### 3.11a TD Sequential 动态事件

TD Sequential 事件由代码动态生成（indicator_state.go:458-462），格式为 `td_{buy|sell}_setup_{N}`，其中 N 为 8–13。

| 事件模式 | 中文翻译 | 含义 |
|---------|---------|------|
| td_buy_setup_8 | TD买入序列8 | TD Sequential 买入序列第 8 根 K 线 |
| td_buy_setup_9 | TD买入序列9 | TD Sequential 买入序列完成（经典信号） |
| td_buy_setup_10..13 | TD买入序列10..13 | 买入序列延伸，信号减弱 |
| td_sell_setup_8 | TD卖出序列8 | TD Sequential 卖出序列第 8 根 K 线 |
| td_sell_setup_9 | TD卖出序列9 | TD Sequential 卖出序列完成（经典信号） |
| td_sell_setup_10..13 | TD卖出序列10..13 | 卖出序列延伸，信号减弱 |

### 3.12 跨周期汇总（cross_tf_summary）

| 字段名 | 中文名 | 含义 | 对决策的意义 |
|--------|--------|------|------------|
| alignment | 指标一致性 | 多周期指标是否方向一致 | aligned=信号可靠，mixed/conflict=信号矛盾 |
| higher_tf_agreement | 高周期一致性 | 更高周期是否同意当前方向 | true=高周期支持，false=高周期反对 |
| lower_tf_agreement | 低周期一致性 | 更低周期是否同意当前方向 | true=低周期配合，false=低周期不配合 |
| decision_tf_bias | 决策周期偏向 | 决策周期的主方向 | long/short/none |
| conflict_count | 冲突计数 | 跨周期冲突信号数量 | 越高越不可靠 |

---

## 四、市场机制状态字段

### 4.1 持仓量状态（oi_state）

| 字段名 | 中文名 | 含义 |
|--------|--------|------|
| change_state | 变化状态 | OI 近期变化方向（rising/falling/flat） |
| oi_change_pct | OI变化率 | OI 百分比变化 |
| price_change_pct | 价格变化率 | 同期价格百分比变化 |
| oi_price_relation | OI-价格关系 | OI 与价格的联动关系 |

**OI-价格关系值：**

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| price_up_oi_up | 价格上涨/OI上升 | 价格和持仓量同步上升 | 多头趋势健康，新资金入场 |
| price_up_oi_down | 价格上涨/OI下降 | 价格上涨但持仓量下降 | 空头平仓推动上涨，动能可能衰竭 |
| price_down_oi_up | 价格下跌/OI上升 | 价格下跌但持仓量上升 | 空头积极建仓，下跌趋势可能加速 |
| price_down_oi_down | 价格下跌/OI下降 | 价格和持仓量同步下降 | 多头平仓，市场去杠杆 |

### 4.2 资金费率状态（funding_state）

| 字段名 | 中文名 | 含义 |
|--------|--------|------|
| bias | 偏向 | 资金费率方向（long/short/neutral） |
| heat | 资金费率热度 | 资金费率强度 |
| rate | 费率 | 实际资金费率数值 |

**热度值：**

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| neutral | 中性/无明显倾向 | 资金费率正常 | 无额外风险 |
| hot | 过热 | 资金费率异常偏高 | 可能出现反转，持仓成本高 |

### 4.3 拥挤度状态（crowding_state）

| 字段名 | 中文名 | 含义 |
|--------|--------|------|
| bias | 偏向 | 市场多空拥挤方向 |
| ls_ratio | 多空比 | 大户多空持仓比率 |
| taker_ratio | 主动买卖比 | 主动买入/卖出量比率 |
| reversal_risk | 反转风险 | 拥挤导致反转的风险等级（low/medium/high） |

**拥挤方向：**

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| long_crowded | 多头拥挤 | 多头持仓过度集中 | 逆向做多风险高，可能出现多头踩踏 |
| short_crowded | 空头拥挤 | 空头持仓过度集中 | 逆向做空风险高，可能出现空头挤压 |
| balanced | 多空均衡 | 多空持仓均衡 | 无额外拥挤风险 |

### 4.4 清算状态（liquidation_state）

| 字段名 | 中文名 | 含义 |
|--------|--------|------|
| stress | 清算压力 | 当前清算压力等级 |

**压力值：**

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| low | 低 | 清算压力低 | 安全 |
| elevated | 偏高 | 清算压力偏高 | 需要关注，可能影响价格 |
| high | 高 | 清算压力很高 | 高风险，可能引发连锁清算 |

### 4.5 市场情绪（sentiment_state）

| 字段名 | 中文名 | 含义 |
|--------|--------|------|
| fear_greed | 恐贪指数 | 恐惧与贪婪指数状态 |
| top_trader_bias | 大户偏向 | 头部交易者的持仓倾向 |

**恐贪指数值：**

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| fear | 恐惧 | 市场恐惧情绪主导 | 可能是底部机会，但也可能继续下跌 |
| neutral | 中性/无明显倾向 | 市场情绪中立 | 无额外情绪信号 |
| greed | 贪婪 | 市场贪婪情绪主导 | 上涨动力强但需警惕过热 |
| extreme_greed | 极度贪婪 | 市场极度贪婪 | 高度警惕回调风险 |

**情绪标签（tag）：**

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| Strong Long | 强烈看多 | 综合情绪强烈偏多 |
| Long Bias | 偏多 | 综合情绪略偏多 |
| Neutral | 中性/无明显倾向 | 综合情绪中立 |
| Short Bias | 偏空 | 综合情绪略偏空 |
| Strong Short | 强烈看空 | 综合情绪强烈偏空 |

### 4.6 机制冲突（mechanics_conflict）

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| crowding_long_but_liq_stress_high | 多头拥挤但清算压力高 | 多头拥挤 + 清算压力偏高 | 做多风险极高，可能引发多头踩踏 |
| crowding_short_but_liq_stress_high | 空头拥挤但清算压力高 | 空头拥挤 + 清算压力偏高 | 做空风险极高，可能引发空头挤压 |
| funding_long_but_oi_falling | 资金费率偏多但OI下降 | 资金费率看多 + 持仓量下降 | 多方信心不足，小心陷阱 |
| funding_short_but_oi_rising | 资金费率偏空但OI上升 | 资金费率看空 + 持仓量上升 | 空方激进建仓，可能反转 |

---

## 五、趋势结构字段

### 5.1 全局上下文（global_context）

| 字段名 | 中文名 | 含义 |
|--------|--------|------|
| slope_state | 斜率状态 | 趋势斜率方向（rising/falling/flat） |
| trend_slope | 趋势斜率 | 趋势斜率数值 |
| vol_ratio | 成交量比率 | 当前成交量与均值的比率 |

### 5.2 结构突破事件（break_events）

| 字段名 | 中文名 | 含义 |
|--------|--------|------|
| level_price | 关键价位 | 突破的价格水平 |

**突破类型：**

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| break_up | 向上突破 | 价格向上突破结构 | 多头信号 |
| break_down | 向下突破 | 价格向下突破结构 | 空头信号 |
| bos_up | 向上结构突破(BOS) | Break of Structure 向上 | 趋势延续信号 |
| bos_down | 向下结构突破(BOS) | Break of Structure 向下 | 趋势延续信号 |
| choch_up | 向上结构转变(CHoCH) | Change of Character 向上 | 趋势反转信号 |
| choch_down | 向下结构转变(CHoCH) | Change of Character 向下 | 趋势反转信号 |

### 5.3 SMC 结构（smc）

| 字段名 | 中文名 | 含义 |
|--------|--------|------|
| order_block | 订单块(Order Block) | 机构大单区域 |
| fvg | 公允价值缺口(FVG) | Fair Value Gap，价格跳空区域 |

**类型值：**

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| bullish | 看多 | 看多型订单块/FVG |
| bearish | 看空 | 看空型订单块/FVG |
| none | 无 | 未检测到 |

### 5.4 SuperTrend 指标

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| bullish | 看多 | SuperTrend 方向向上 | 支持做多 |
| bearish | 看空 | SuperTrend 方向向下 | 支持做空 |

### 5.5 Aroon 指标

| 原字段值 | 中文 | 含义 | 对决策的意义 |
|---------|------|------|------------|
| strong_up | 强势上行 | Aroon Up 高位 | 多头趋势明确 |
| strong_down | 强势下行 | Aroon Down 高位 | 空头趋势明确 |
| crossover | 交叉 | Aroon 线交叉 | 可能的趋势切换 |

---

## 六、Agent 输出字段

以下字段出现在 Agent 分析结果（indicator / mechanics / structure）中：

### 6.1 指标综合（Indicator Agent）

| 字段名 | 中文名 | 含义 | 取值示例 |
|--------|--------|------|---------|
| expansion | 扩张状态 | 波动率方向 | expanding / contracting |
| alignment | 指标一致性 | 跨指标方向一致度 | aligned / mixed / divergent |
| noise | 噪音水平 | 指标信号噪音 | low / medium / high |
| momentum_detail | 动能细节 | EMA delta 明细文本 | ema_fast delta_pct 0.19 positive... |
| conflict_detail | 冲突细节 | 指标冲突说明 | 未观察到明显冲突 |
| movement_score | 方向分数 | 综合方向评分 | -1.0 ~ 1.0 |
| movement_confidence | 方向置信度 | 评分可信度 | 0.0 ~ 1.0 |

### 6.2 市场机制（Mechanics Agent）

| 字段名 | 中文名 | 含义 | 取值示例 |
|--------|--------|------|---------|
| leverage_state | 杠杆状态 | 市场杠杆水平 | stable / increasing / overheated |
| crowding | 拥挤度 | 多空拥挤状态 | balanced / long_crowded / short_crowded |
| risk_level | 风险等级 | 综合风险评级 | low / medium / high |
| open_interest_context | 持仓量背景 | OI 变化自然语言描述 | OI increased slightly in 15m... |
| anomaly_detail | 异常说明 | 市场异常检测 | fear_greed=8 versus long crowding |
| movement_score | 方向分数 | 综合方向评分 | -1.0 ~ 1.0 |
| movement_confidence | 方向置信度 | 评分可信度 | 0.0 ~ 1.0 |

### 6.3 结构分析（Structure Agent）

| 字段名 | 中文名 | 含义 | 取值示例 |
|--------|--------|------|---------|
| regime | 结构状态 | 市场结构类型 | trend_up / trend_down / range / mixed |
| last_break | 最近结构变化 | 最新突破事件 | break_up / bos_down / choch_up |
| quality | 结构质量 | 结构可靠度 | clean / messy / unclear |
| pattern | 主导形态 | 识别的图表形态 | double_top / flag / triangle_asc |
| volume_action | 量能表现 | 成交量表现 | 自然语言描述 |
| candle_reaction | K线反应 | 价格对关键位的反应 | 自然语言描述 |
| movement_score | 方向分数 | 综合方向评分 | -1.0 ~ 1.0 |
| movement_confidence | 方向置信度 | 评分可信度 | 0.0 ~ 1.0 |

---

## 七、图表形态识别

| 原字段值 | 中文 | 含义 | 类型 |
|---------|------|------|------|
| double_top | 双顶形态 | 经典反转形态 | 反转 |
| double_bottom | 双底形态 | 经典反转形态 | 反转 |
| head_shoulders | 头肩形态 | 顶部反转形态 | 反转 |
| inv_head_shoulders | 反头肩形态 | 底部反转形态 | 反转 |
| triangle_sym | 对称三角形 | 整理形态 | 整理 |
| triangle_asc | 上升三角形 | 看涨整理形态 | 整理 |
| triangle_desc | 下降三角形 | 看跌整理形态 | 整理 |
| wedge_rising | 上升楔形 | 看跌反转形态 | 反转 |
| wedge_falling | 下降楔形 | 看涨反转形态 | 反转 |
| flag | 旗形整理 | 趋势延续形态 | 延续 |
| pennant | 三角旗形 | 趋势延续形态 | 延续 |
| channel_up | 上行通道 | 趋势通道 | 趋势 |
| channel_down | 下行通道 | 趋势通道 | 趋势 |

### 结构状态值

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| breakout_confirmed | 突破确认 | 突破事件已被量能确认 |
| support_retest | 回踩确认 | 支撑位成功回踩 |
| fakeout_rejection | 假突破回落 | 突破被否定，价格回落 |
| structure_broken | 结构失效 | 原有结构已被破坏 |
| clean | 结构清晰 | 结构明确可辨 |
| messy | 结构杂乱 | 结构复杂难辨 |

---

## 八、通用状态值

| 原字段值 | 中文 | 适用场景 |
|---------|------|---------|
| rising | 上升 | 斜率/变化方向 |
| falling | 下降 | 斜率/变化方向 |
| flat | 走平/减弱 | 斜率/变化方向 |
| low | 低 | 风险/压力等级 |
| medium | 中 | 风险/压力等级 |
| high | 高 | 风险/压力等级 |
| critical | 危急(极高风险) | 风险/压力等级 |
| aligned | 指标一致 | 多周期一致性 |
| divergent | 指标分歧/不一致 | 多周期一致性 |
| conflict | 冲突/分歧 | 信号冲突 |
| unknown | 无法判断 | 数据缺失 |
| stable | 稳定 | 杠杆/波动状态 |
| expanding | 波动/动能扩张 | 波动率方向 |
| contracting | 波动/动能收敛 | 波动率方向 |
| moderate | 温和 | 斜率幅度（趋势斜率介于平缓和陡峭之间） |
| steep | 陡峭 | 斜率幅度（趋势斜率较大，动能强劲） |

---

## 九、Gate 原因码

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| PASS_STRONG | 强通过 | 高置信通过 |
| PASS_WEAK | 弱通过 | 低置信通过 |
| CONSENSUS_NOT_PASSED | 三路共识未通过 | 指标/结构/机制未达成一致 |
| DIRECTION_UNCLEAR | 方向不明确 | 无法确定多空方向 |
| STRUCT_INVALID | 结构无效 | 结构数据无效 |
| STRUCT_NO_BIAS | 结构无方向 | 结构无法给出方向偏好 |
| STRUCT_BREAK | 结构失效 | 趋势结构被破坏 |
| STRUCT_HARD_INVALIDATION | 结构硬失效 | 趋势结构严重破坏 |
| STRUCT_THREAT | 结构受威胁 | 结构面临破坏风险 |
| MECH_RISK | 清算风险过高 | 市场机制风险超标 |
| LIQUIDATION_CASCADE | 连锁清算风险 | 可能出现连环清算 |
| MOMENTUM_WEAK | 动能走弱 | 动能指标不支持 |
| INDICATOR_NOISE | 指标噪音 | 指标信号混乱 |
| INDICATOR_MIXED | 指标分歧 | 多个指标信号矛盾 |
| QUALITY_TOO_LOW | 建仓质量不足 | 综合评分不足 |
| EDGE_TOO_LOW | 执行价值不足 | 风险收益比不佳 |
| ENTRY_COOLDOWN_ACTIVE | 开仓冷却中 | 冷却期未结束 |
| GATE_MISSING | Gate 事件缺失 | 缺少 Gate 评估数据 |
| BINDING_MISSING | 策略绑定缺失 | 符号未绑定策略 |
| ENABLED_MISSING | 启用配置缺失 | 符号未启用决策 |
| DATA_MISSING | 数据缺失 | 所需数据不完整 |

---

## 十、Sieve 决策码

Sieve 在 Gate 允许后进一步细化决策：

| 原字段值 | 中文 | 含义 |
|---------|------|------|
| SIEVE_MATCH | Sieve 命中 | Sieve 规则匹配 |
| SIEVE_DEFAULT | Sieve 默认 | 无规则匹配，使用默认 |
| FUEL_HIGH | 燃料充分/高置信 | 高动能 + 高置信度 |
| FUEL_LOW | 燃料充分/低置信 | 高动能 + 低置信度 |
| FUEL_HIGH_ALIGN | 燃料充分/高置信/同向拥挤 | 高动能 + 高置信 + 拥挤同向 |
| FUEL_LOW_ALIGN | 燃料充分/低置信/同向拥挤 | 高动能 + 低置信 + 拥挤同向 |
| NEUTRAL_HIGH | 中性/高置信 | 中性动能 + 高置信度 |
| NEUTRAL_LOW | 中性/低置信 | 中性动能 + 低置信度 |
| NEUTRAL_HIGH_ALIGN | 中性/高置信/同向拥挤 | 中性 + 高置信 + 拥挤同向 |
| NEUTRAL_LOW_ALIGN | 中性/低置信/同向拥挤 | 中性 + 低置信 + 拥挤同向 |
| CROWD_ALIGN_HIGH | 同向拥挤/高置信 | 拥挤与方向一致 + 高置信 |
| CROWD_ALIGN_LOW | 同向拥挤/低置信 | 拥挤与方向一致 + 低置信 |
| CROWD_ALIGN_LOW_BLOCK | 同向拥挤/低置信/拦截 | 同上 + 触发拦截 |
| CROWD_COUNTER_HIGH | 反向拥挤/高置信 | 拥挤与方向相反 + 高置信 |
| CROWD_COUNTER_LOW | 反向拥挤/低置信 | 拥挤与方向相反 + 低置信 |
| CROWD_LONG_ALIGN_HIGH | 多头同向拥挤/高置信 | 多头拥挤同向 + 高置信 |
| CROWD_LONG_ALIGN_LOW | 多头同向拥挤/低置信 | 多头拥挤同向 + 低置信 |
| CROWD_SHORT_ALIGN_HIGH | 空头同向拥挤/高置信 | 空头拥挤同向 + 高置信 |
| CROWD_SHORT_ALIGN_LOW | 空头同向拥挤/低置信 | 空头拥挤同向 + 低置信 |
| CROWD_LONG_COUNTER_HIGH | 多头反向拥挤/高置信 | 多头拥挤反向 + 高置信 |
| CROWD_LONG_COUNTER_LOW | 多头反向拥挤/低置信 | 多头拥挤反向 + 低置信 |
| CROWD_SHORT_COUNTER_HIGH | 空头反向拥挤/高置信 | 空头拥挤反向 + 高置信 |
| CROWD_SHORT_COUNTER_LOW | 空头反向拥挤/低置信 | 空头拥挤反向 + 低置信 |
| BLOCK_CROWD_LONG_ALIGN | 同向拥挤(多)/拦截 | 多头同向拥挤触发拦截 |
| BLOCK_CROWD_SHORT_ALIGN | 同向拥挤(空)/拦截 | 空头同向拥挤触发拦截 |
| BLOCK_LIQ_CASCADE | 链式风险/拦截 | 清算级联风险触发拦截 |
| ALLOW_TREND_BREAKOUT_FUEL | 趋势突破/燃料充分 | 突破 + 足够动能 |
| ALLOW_TREND_BREAKOUT_NEUTRAL | 趋势突破/中性 | 突破 + 中性动能 |
| ALLOW_PULLBACK_FUEL | 回踩确认/燃料充分 | 回踩入场 + 足够动能 |
| ALLOW_PULLBACK_NEUTRAL | 回踩确认/中性 | 回踩入场 + 中性动能 |
| ALLOW_DIV_REV_HIGH | 背离反转/高置信 | 背离反转信号 + 高置信 |

---

> 本文档以 `internal/decision/decisionfmt/formatter_translate.go` Go 源码为参考依据，手动维护。所有业务术语翻译以该文件为唯一来源。`webui/og-card-demo/render.mjs` 已剥离翻译职责，仅负责排版渲染。如有字段变更，请同步更新 Go 翻译文件与本文档。
