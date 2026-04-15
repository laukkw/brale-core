package config

import (
	"fmt"
	"strings"
)

const (
	// IndicatorEngineTA is the default pure-Go engine (no CGO).
	IndicatorEngineTA = "ta"
	// IndicatorEngineTalib wraps go-talib (CGO); retained for parity diff.
	IndicatorEngineTalib = "talib"
	// IndicatorEngineReference is the legacy pure-Go reference engine.
	IndicatorEngineReference = "reference"
)

func NormalizeIndicatorEngine(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "", IndicatorEngineTA:
		return IndicatorEngineTA
	case IndicatorEngineTalib:
		return IndicatorEngineTalib
	case IndicatorEngineReference:
		return IndicatorEngineReference
	default:
		return strings.ToLower(strings.TrimSpace(engine))
	}
}

func NormalizeOptionalIndicatorEngine(engine string) string {
	trimmed := strings.TrimSpace(engine)
	if trimmed == "" {
		return ""
	}
	return NormalizeIndicatorEngine(trimmed)
}

func ValidateIndicatorEngine(engine string) error {
	switch NormalizeIndicatorEngine(engine) {
	case IndicatorEngineTA, IndicatorEngineTalib, IndicatorEngineReference:
		return nil
	default:
		return fmt.Errorf("must be one of [ta reference talib]")
	}
}
