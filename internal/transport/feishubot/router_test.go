package feishubot

import "testing"

func TestParseCommandRoute(t *testing.T) {
	tests := []struct {
		in     string
		action string
		arg    string
	}{
		{in: "monitor", action: "monitor"},
		{in: "/history", action: "trades"},
		{in: "observe btc", action: "observe", arg: "btc"},
		{in: "schedule on", action: "schedule_on"},
		{in: "decision eth", action: "latest", arg: "eth"},
	}
	for _, tc := range tests {
		route := parseCommandRoute(tc.in)
		if route.Action != tc.action || route.Arg != tc.arg {
			t.Fatalf("parseCommandRoute(%q)=(%q,%q) want (%q,%q)", tc.in, route.Action, route.Arg, tc.action, tc.arg)
		}
	}
}
