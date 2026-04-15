package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"brale-core/internal/store"
)

type Request struct {
	Symbol string
	Limit  int
}

type EvalCase struct {
	RoundID       string `json:"round_id"`
	Symbol        string `json:"symbol"`
	RoundType     string `json:"round_type"`
	PromptVersion string `json:"prompt_version,omitempty"`
	Outcome       string `json:"outcome,omitempty"`
	Error         string `json:"error,omitempty"`
	LatencyMS     int    `json:"latency_ms"`
	TokenIn       int    `json:"token_in"`
	TokenOut      int    `json:"token_out"`
	CallCount     int    `json:"call_count"`
}

type EvalResult struct {
	RoundID       string  `json:"round_id"`
	Symbol        string  `json:"symbol"`
	RoundType     string  `json:"round_type"`
	PromptVersion string  `json:"prompt_version,omitempty"`
	Outcome       string  `json:"outcome,omitempty"`
	Error         string  `json:"error,omitempty"`
	LatencyMS     int     `json:"latency_ms"`
	TokenIn       int     `json:"token_in"`
	TokenOut      int     `json:"token_out"`
	CallCount     int     `json:"call_count"`
	Score         float64 `json:"score"`
}

type Bucket struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Summary struct {
	TotalRounds       int      `json:"total_rounds"`
	ErrorRounds       int      `json:"error_rounds"`
	AverageScore      float64  `json:"average_score"`
	AverageLatencyMS  float64  `json:"average_latency_ms"`
	AverageTokenIn    float64  `json:"average_token_in"`
	AverageTokenOut   float64  `json:"average_token_out"`
	OutcomeBreakdown  []Bucket `json:"outcome_breakdown,omitempty"`
	PromptBreakdown   []Bucket `json:"prompt_breakdown,omitempty"`
	SymbolBreakdown   []Bucket `json:"symbol_breakdown,omitempty"`
	RoundTypeBreakout []Bucket `json:"round_type_breakdown,omitempty"`
}

type Report struct {
	Request Request      `json:"request"`
	Summary Summary      `json:"summary"`
	Cases   []EvalCase   `json:"cases"`
	Results []EvalResult `json:"results"`
}

type Harness struct {
	Store store.LLMRoundStore
}

func (h Harness) Run(ctx context.Context, req Request) (Report, error) {
	if h.Store == nil {
		return Report{}, fmt.Errorf("llm round store is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	rounds, err := h.Store.ListLLMRounds(ctx, strings.TrimSpace(req.Symbol), limit)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		Request: Request{
			Symbol: strings.TrimSpace(req.Symbol),
			Limit:  limit,
		},
		Cases:   make([]EvalCase, 0, len(rounds)),
		Results: make([]EvalResult, 0, len(rounds)),
	}

	outcomes := map[string]int{}
	prompts := map[string]int{}
	symbols := map[string]int{}
	roundTypes := map[string]int{}

	var totalScore float64
	var totalLatency int
	var totalTokenIn int
	var totalTokenOut int
	var errorRounds int

	for _, rec := range rounds {
		evalCase := EvalCase{
			RoundID:       rec.ID,
			Symbol:        rec.Symbol,
			RoundType:     rec.RoundType,
			PromptVersion: rec.PromptVersion,
			Outcome:       rec.Outcome,
			Error:         rec.Error,
			LatencyMS:     rec.TotalLatencyMS,
			TokenIn:       rec.TotalTokenIn,
			TokenOut:      rec.TotalTokenOut,
			CallCount:     rec.CallCount,
		}
		score := scoreRound(rec)
		report.Cases = append(report.Cases, evalCase)
		report.Results = append(report.Results, EvalResult{
			RoundID:       rec.ID,
			Symbol:        rec.Symbol,
			RoundType:     rec.RoundType,
			PromptVersion: rec.PromptVersion,
			Outcome:       rec.Outcome,
			Error:         rec.Error,
			LatencyMS:     rec.TotalLatencyMS,
			TokenIn:       rec.TotalTokenIn,
			TokenOut:      rec.TotalTokenOut,
			CallCount:     rec.CallCount,
			Score:         score,
		})

		totalScore += score
		totalLatency += rec.TotalLatencyMS
		totalTokenIn += rec.TotalTokenIn
		totalTokenOut += rec.TotalTokenOut
		if strings.TrimSpace(rec.Error) != "" {
			errorRounds++
		}
		outcomes[bucketName(rec.Outcome, "unknown")]++
		prompts[bucketName(rec.PromptVersion, "builtin")]++
		symbols[bucketName(rec.Symbol, "unknown")]++
		roundTypes[bucketName(rec.RoundType, "unknown")]++
	}

	report.Summary = Summary{
		TotalRounds:       len(rounds),
		ErrorRounds:       errorRounds,
		AverageScore:      average(totalScore, len(rounds)),
		AverageLatencyMS:  average(float64(totalLatency), len(rounds)),
		AverageTokenIn:    average(float64(totalTokenIn), len(rounds)),
		AverageTokenOut:   average(float64(totalTokenOut), len(rounds)),
		OutcomeBreakdown:  toBuckets(outcomes),
		PromptBreakdown:   toBuckets(prompts),
		SymbolBreakdown:   toBuckets(symbols),
		RoundTypeBreakout: toBuckets(roundTypes),
	}
	return report, nil
}

func scoreRound(rec store.LLMRoundRecord) float64 {
	if strings.TrimSpace(rec.Error) != "" {
		return 0
	}
	score := 0.4
	if rec.CallCount > 0 {
		score += 0.2
	}
	if rec.TotalLatencyMS > 0 {
		score += 0.2
	}
	if rec.TotalTokenIn > 0 || rec.TotalTokenOut > 0 {
		score += 0.1
	}
	if strings.TrimSpace(rec.PromptVersion) != "" {
		score += 0.1
	}
	if score > 1 {
		return 1
	}
	return score
}

func bucketName(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func average(total float64, count int) float64 {
	if count <= 0 {
		return 0
	}
	return total / float64(count)
}

func toBuckets(values map[string]int) []Bucket {
	if len(values) == 0 {
		return nil
	}
	out := make([]Bucket, 0, len(values))
	for name, count := range values {
		out = append(out, Bucket{Name: name, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Name < out[j].Name
		}
		return out[i].Count > out[j].Count
	})
	return out
}
