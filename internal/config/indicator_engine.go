package config

import (
	"fmt"
	"strings"
)

const (
	IndicatorEngineTalib     = "talib"
	IndicatorEngineReference = "reference"
)

func NormalizeIndicatorEngine(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "", IndicatorEngineTalib:
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
	case IndicatorEngineTalib, IndicatorEngineReference:
		return nil
	default:
		return fmt.Errorf("must be one of [reference talib]")
	}
}
