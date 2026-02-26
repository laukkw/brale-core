// 本文件主要内容：定义压缩后的特征 JSON 结构。
package features

type IndicatorJSON struct {
	Symbol   string
	Interval string
	RawJSON  []byte
}

type TrendJSON struct {
	Symbol   string
	Interval string
	RawJSON  []byte
}

type MechanicsSnapshot struct {
	Symbol  string
	RawJSON []byte
}

type CompressionResult struct {
	Indicators map[string]map[string]IndicatorJSON
	Trends     map[string]map[string]TrendJSON
	Mechanics  map[string]MechanicsSnapshot
}

type FeatureError struct {
	Symbol string
	Stage  string
	Err    error
}
