// 本文件主要内容：基于结构枚举推导市场方向。
package marketdir

type Regime string

type LastBreak string

type Direction string

const (
	RegimeTrendUp   Regime = "trend_up"
	RegimeTrendDown Regime = "trend_down"
	RegimeRange     Regime = "range"
	RegimeMixed     Regime = "mixed"
	RegimeUnclear   Regime = "unclear"
)

const (
	LastBreakBosUp     LastBreak = "bos_up"
	LastBreakBosDown   LastBreak = "bos_down"
	LastBreakChochUp   LastBreak = "choch_up"
	LastBreakChochDown LastBreak = "choch_down"
	LastBreakNone      LastBreak = "none"
	LastBreakUnknown   LastBreak = "unknown"
)

const (
	DirectionLong  Direction = "long"
	DirectionShort Direction = "short"
	DirectionNone  Direction = "none"
)

func Derive(regime Regime, lastBreak LastBreak) Direction {
	switch regime {
	case RegimeTrendUp:
		switch lastBreak {
		case LastBreakBosUp, LastBreakChochUp:
			return DirectionLong
		case LastBreakNone, LastBreakUnknown:
			return DirectionNone
		}
	case RegimeTrendDown:
		switch lastBreak {
		case LastBreakBosDown, LastBreakChochDown:
			return DirectionShort
		case LastBreakNone, LastBreakUnknown:
			return DirectionNone
		}
	case RegimeRange:
		return DirectionNone
	}
	return DirectionNone
}
