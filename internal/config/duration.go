package config

import (
	"strings"
	"time"
)

func ParseDurationOrDefault(raw string, fallback time.Duration) time.Duration {
	text := strings.TrimSpace(raw)
	if text == "" {
		return fallback
	}
	value, err := time.ParseDuration(text)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
