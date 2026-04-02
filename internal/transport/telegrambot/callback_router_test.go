package telegrambot

import "strings"

import "testing"

func TestCallbackPrefixesRemainStable(t *testing.T) {
	if got := strings.TrimPrefix(cbObservePrefix+"BTCUSDT", cbObservePrefix); got != "BTCUSDT" {
		t.Fatalf("observe prefix trim=%q", got)
	}
	if got := strings.TrimPrefix(cbLatestPrefix+"ETHUSDT", cbLatestPrefix); got != "ETHUSDT" {
		t.Fatalf("latest prefix trim=%q", got)
	}
}
