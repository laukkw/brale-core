package decisionutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"brale-core/internal/llm"
	"brale-core/internal/pkg/llmclean"
)

type runnerTestPayload struct {
	OK bool `json:"ok"`
}

type staticProvider struct {
	call func(context.Context, string, string) (string, error)
}

func (p staticProvider) Call(ctx context.Context, system, user string) (string, error) {
	return p.call(ctx, system, user)
}

type staticStructuredProvider struct {
	staticProvider
	callStructured func(context.Context, string, string, *llm.JSONSchema) (string, error)
}

func (p staticStructuredProvider) CallStructured(ctx context.Context, system, user string, schema *llm.JSONSchema) (string, error) {
	return p.callStructured(ctx, system, user, schema)
}

func TestRunAndParseWithoutRetryReturnsParseError(t *testing.T) {
	provider := staticProvider{
		call: func(context.Context, string, string) (string, error) {
			return "not-json", nil
		},
	}

	decode := func(raw string) (runnerTestPayload, error) {
		var out runnerTestPayload
		err := json.Unmarshal([]byte(raw), &out)
		return out, err
	}

	_, err := RunAndParse(context.Background(), func(string) (llm.Provider, error) {
		return provider, nil
	}, "stage", "sys", "user", decode, nil)
	if err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestRunAndParseWithRetryReturnsRetriedResult(t *testing.T) {
	var calls int
	var parseErrors int
	provider := staticProvider{
		call: func(_ context.Context, _, _ string) (string, error) {
			calls++
			if calls == 1 {
				return "not-json", nil
			}
			return `{"ok":true}`, nil
		},
	}

	decode := func(raw string) (runnerTestPayload, error) {
		var out runnerTestPayload
		err := json.Unmarshal([]byte(raw), &out)
		return out, err
	}

	got, err := RunAndParse(context.Background(), func(string) (llm.Provider, error) {
		return provider, nil
	}, "stage", "sys", "user", decode, func(string, error) {
		parseErrors++
	}, WithRetryOnParseFail(DefaultRetryPrompt))
	if err != nil {
		t.Fatalf("RunAndParse() error = %v", err)
	}
	if !got.OK {
		t.Fatalf("ok=%v want true", got.OK)
	}
	if calls != 2 {
		t.Fatalf("calls=%d want 2", calls)
	}
	if parseErrors != 1 {
		t.Fatalf("parseErrors=%d want 1", parseErrors)
	}
}

func TestRunAndParseStructuredProviderFallsBackToDecode(t *testing.T) {
	provider := staticStructuredProvider{
		staticProvider: staticProvider{
			call: func(context.Context, string, string) (string, error) {
				return "```json\n{\"ok\":true}\n```", nil
			},
		},
		callStructured: func(ctx context.Context, system, user string, schema *llm.JSONSchema) (string, error) {
			if schema == nil {
				return "", errors.New("schema is required")
			}
			return "```json\n{\"ok\":true}\n```", nil
		},
	}

	decode := func(raw string) (runnerTestPayload, error) {
		var out runnerTestPayload
		err := json.Unmarshal([]byte(llmclean.StripCodeFences(raw)), &out)
		return out, err
	}

	got, err := RunAndParse(context.Background(), func(string) (llm.Provider, error) {
		return provider, nil
	}, "stage", "sys", "user", decode, nil)
	if err != nil {
		t.Fatalf("RunAndParse() error = %v", err)
	}
	if !got.OK {
		t.Fatalf("ok=%v want true", got.OK)
	}
}

func TestRunAndParseStructuredProviderUsesStageDecode(t *testing.T) {
	provider := staticStructuredProvider{
		callStructured: func(ctx context.Context, system, user string, schema *llm.JSONSchema) (string, error) {
			return `{"ok":true,"extra":true}`, nil
		},
	}

	decode := func(raw string) (runnerTestPayload, error) {
		var out runnerTestPayload
		dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
		dec.DisallowUnknownFields()
		err := dec.Decode(&out)
		return out, err
	}

	_, err := RunAndParse(context.Background(), func(string) (llm.Provider, error) {
		return provider, nil
	}, "stage", "sys", "user", decode, nil)
	if err == nil {
		t.Fatalf("expected strict decode error")
	}
}
