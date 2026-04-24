package config

import "strings"

const (
	PromptLocaleZH = "zh"
	PromptLocaleEN = "en"
)

func NormalizePromptLocale(locale string) string {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "", PromptLocaleZH:
		return PromptLocaleZH
	case PromptLocaleEN:
		return PromptLocaleEN
	default:
		return strings.ToLower(strings.TrimSpace(locale))
	}
}

func IsSupportedPromptLocale(locale string) bool {
	switch NormalizePromptLocale(locale) {
	case PromptLocaleZH, PromptLocaleEN:
		return true
	default:
		return false
	}
}
