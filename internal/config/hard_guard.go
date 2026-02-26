package config

// HardGuardConfig 定义硬保护默认阈值（当前为内置常量，不走外部配置文件）。
type HardGuardConfig struct {
	// LongRSIExtreme: 多头持仓时，RSI 高于该值触发极端超买保护。
	LongRSIExtreme float64
	// ShortRSIExtreme: 空头持仓时，RSI 低于该值触发极端超卖保护。
	ShortRSIExtreme float64
	// FiveMinAdverseMovePct: 5 分钟不利方向涨跌幅阈值（百分比）。
	FiveMinAdverseMovePct float64
}

func DefaultHardGuardConfig() HardGuardConfig {
	return HardGuardConfig{
		LongRSIExtreme:        85.0,
		ShortRSIExtreme:       15.0,
		FiveMinAdverseMovePct: 3.0,
	}
}
