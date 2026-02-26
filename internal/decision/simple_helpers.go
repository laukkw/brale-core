package decision

// 本文件主要内容：提供压缩数据的简化读取工具。
import (
	"brale-core/internal/decision/decisionutil"
	"brale-core/internal/decision/features"
)

type simpleIndicator = decisionutil.SimpleIndicator

func PickIndicator(data features.CompressionResult, symbol string) (simpleIndicator, error) {
	return decisionutil.PickIndicator(data, symbol)
}

func PickIndicatorJSON(data features.CompressionResult, symbol string) (features.IndicatorJSON, bool) {
	return decisionutil.PickIndicatorJSON(data, symbol)
}

func PickIndicatorJSONByInterval(data features.CompressionResult, symbol, interval string) (features.IndicatorJSON, bool) {
	return decisionutil.PickIndicatorJSONByInterval(data, symbol, interval)
}

func PickTrendJSON(data features.CompressionResult, symbol string) (features.TrendJSON, bool) {
	return decisionutil.PickTrendJSON(data, symbol)
}

func PickTrendJSONByInterval(data features.CompressionResult, symbol, interval string) (features.TrendJSON, bool) {
	return decisionutil.PickTrendJSONByInterval(data, symbol, interval)
}

func PickMechanicsJSON(data features.CompressionResult, symbol string) (features.MechanicsSnapshot, bool) {
	return decisionutil.PickMechanicsJSON(data, symbol)
}
