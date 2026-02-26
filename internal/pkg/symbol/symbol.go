package symbol

import "strings"

func Normalize(symbol string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(symbol))
	if trimmed == "" {
		return ""
	}
	if strings.ContainsAny(trimmed, "/:") {
		trimmed = FromFreqtradePair(trimmed)
		if trimmed == "" {
			return ""
		}
	}
	if hasKnownQuoteSuffix(trimmed) {
		return trimmed
	}
	return trimmed + "USDT"
}

func FromFreqtradePair(pair string) string {
	s := strings.ToUpper(strings.TrimSpace(pair))
	if s == "" {
		return ""
	}
	if strings.Contains(s, ":") {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) > 0 {
			s = strings.TrimSpace(parts[0])
		}
	}
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0] + parts[1]
		}
		return strings.ReplaceAll(s, "/", "")
	}
	return s
}

func ToFreqtradePair(symbol string) string {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if s == "" {
		return ""
	}
	if strings.Contains(s, "/") {
		if strings.Contains(s, ":") {
			return s
		}
		if strings.HasSuffix(s, "/USDT") {
			return s + ":USDT"
		}
		return s
	}
	if strings.HasSuffix(s, "USDT") && len(s) > 4 {
		return strings.TrimSuffix(s, "USDT") + "/USDT:USDT"
	}
	return s
}

func hasKnownQuoteSuffix(symbol string) bool {
	return strings.HasSuffix(symbol, "USDT") ||
		strings.HasSuffix(symbol, "USDC") ||
		strings.HasSuffix(symbol, "BUSD") ||
		strings.HasSuffix(symbol, "USD")
}
