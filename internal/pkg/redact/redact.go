package redact

import (
	"regexp"
	"strings"
)

var telegramBotTokenPattern = regexp.MustCompile(`/bot[0-9]+:[A-Za-z0-9_-]+`)

// Secrets removes known secrets and common token-bearing URL fragments from text
// before the value is persisted to logs or returned as an error.
func Secrets(text string, knownSecrets ...string) string {
	out := text
	for _, secret := range knownSecrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		out = strings.ReplaceAll(out, secret, "<redacted>")
	}
	out = telegramBotTokenPattern.ReplaceAllString(out, "/bot<redacted>")
	return out
}
