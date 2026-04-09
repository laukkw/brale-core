package llmctl

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/llm"
)

func TestBuildProbeTargetsReturnsThreeStagesInOrder(t *testing.T) {
	sys := config.SystemConfig{
		LLMModels: map[string]config.LLMModelConfig{
			"model-indicator": {Endpoint: "https://indicator.example/v1", APIKey: "k1"},
			"model-structure": {Endpoint: "https://structure.example/v1", APIKey: "k2"},
			"model-mechanics": {Endpoint: "https://mechanics.example/v1", APIKey: "k3"},
		},
	}
	sym := config.SymbolConfig{
		LLM: config.SymbolLLMConfig{
			Agent: config.LLMRoleSet{
				Indicator: config.LLMRoleConfig{Model: "model-indicator"},
				Structure: config.LLMRoleConfig{Model: "model-structure"},
				Mechanics: config.LLMRoleConfig{Model: "model-mechanics"},
			},
		},
	}

	targets, err := BuildProbeTargets(sys, sym, "")
	if err != nil {
		t.Fatalf("BuildProbeTargets() error = %v", err)
	}
	if len(targets) != 3 {
		t.Fatalf("targets=%d want 3", len(targets))
	}
	if targets[0].Stage != "indicator" || targets[1].Stage != "structure" || targets[2].Stage != "mechanics" {
		t.Fatalf("unexpected stage order: %#v", targets)
	}
}

func TestBuildProbeTargetsFiltersStage(t *testing.T) {
	sys := config.SystemConfig{
		LLMModels: map[string]config.LLMModelConfig{
			"model-structure": {Endpoint: "https://structure.example/v1", APIKey: "k2"},
		},
	}
	sym := config.SymbolConfig{
		LLM: config.SymbolLLMConfig{
			Agent: config.LLMRoleSet{
				Structure: config.LLMRoleConfig{Model: "model-structure"},
			},
		},
	}

	targets, err := BuildProbeTargets(sys, sym, "structure")
	if err != nil {
		t.Fatalf("BuildProbeTargets() error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets=%d want 1", len(targets))
	}
	if targets[0].Stage != "structure" {
		t.Fatalf("stage=%q want structure", targets[0].Stage)
	}
}

func TestProbeStructuredSupportWithClientSuccess(t *testing.T) {
	client := &llm.OpenAIClient{
		Endpoint:         "https://llm.example/v1",
		Model:            "m",
		APIKey:           "k",
		Timeout:          time.Second,
		StructuredOutput: true,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`), nil
		})},
	}

	if err := ProbeStructuredSupportWithClient(context.Background(), client); err != nil {
		t.Fatalf("ProbeStructuredSupportWithClient() error = %v", err)
	}
}

func TestProbeStructuredSupportWithClientRejectsInvalidPayload(t *testing.T) {
	client := &llm.OpenAIClient{
		Endpoint:         "https://llm.example/v1",
		Model:            "m",
		APIKey:           "k",
		Timeout:          time.Second,
		StructuredOutput: true,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return jsonResponse(`{"choices":[{"message":{"content":"not-json"}}]}`), nil
		})},
	}

	if err := ProbeStructuredSupportWithClient(context.Background(), client); err == nil {
		t.Fatalf("expected error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
