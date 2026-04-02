package decisionfmt

import "testing"

func TestTranslateDecisionActionTableDriven(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "allow", want: "允许"},
		{in: " TIGHTEN ", want: "收紧风控"},
		{in: "", want: ""},
		{in: "custom", want: "custom"},
	}
	for _, tc := range tests {
		if got := translateDecisionAction(tc.in); got != tc.want {
			t.Fatalf("translateDecisionAction(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslateGateReasonSpecialCases(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "PASS_STRONG", want: "强通过"},
		{in: "AGENT_ERROR:model timeout", want: "Agent 阶段异常：model timeout"},
		{in: "PROVIDER_ERROR:data stale", want: "Provider 阶段异常：data stale"},
		{in: "GATE_ERROR:consensus", want: "Gate 阶段异常：consensus"},
		{in: "weird_reason", want: "weird_reason(英文)"},
	}
	for _, tc := range tests {
		if got := translateGateReason(tc.in); got != tc.want {
			t.Fatalf("translateGateReason(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslateSieveReasonCodeFallsBackToGateReason(t *testing.T) {
	if got := translateSieveReasonCode("CROWD_ALIGN_LOW"); got != "同向拥挤/低置信" {
		t.Fatalf("translateSieveReasonCode mapped=%q", got)
	}
	if got := translateSieveReasonCode("PASS_WEAK"); got != "弱通过" {
		t.Fatalf("translateSieveReasonCode fallback=%q", got)
	}
}

func TestTranslateTermFallbacks(t *testing.T) {
	if got := translateTerm("trend_surge"); got != "trend_surge(趋势加速)" {
		t.Fatalf("translateTerm mapped=%q", got)
	}
	if got := translateTerm("中文"); got != "中文" {
		t.Fatalf("translateTerm han=%q", got)
	}
	if got := translateTerm("unknown_token"); got != "unknown_token(英文)" {
		t.Fatalf("translateTerm unknown=%q", got)
	}
}

func TestTranslateLLMKeyAndProviderRole(t *testing.T) {
	if got := translateLLMKey("confidence"); got != "置信度" {
		t.Fatalf("translateLLMKey=%q", got)
	}
	if got := translateLLMKey("custom_key"); got != "custom_key" {
		t.Fatalf("translateLLMKey custom=%q", got)
	}
	if got := providerRoleLabel("mechanics"); got != "市场机制" {
		t.Fatalf("providerRoleLabel=%q", got)
	}
	if got := providerRoleLabel(" custom "); got != "custom" {
		t.Fatalf("providerRoleLabel custom=%q", got)
	}
}
