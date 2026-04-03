package decision

import "brale-core/internal/execution"

func cloneLLMRiskTrace(trace *execution.LLMRiskTrace) *execution.LLMRiskTrace {
	if trace == nil {
		return nil
	}
	cloned := *trace
	return &cloned
}

func llmRiskTraceMap(trace *execution.LLMRiskTrace) map[string]any {
	if trace == nil {
		return nil
	}
	out := map[string]any{}
	if trace.Stage != "" {
		out["stage"] = trace.Stage
	}
	if trace.Flow != "" {
		out["flow"] = trace.Flow
	}
	if trace.SystemPrompt != "" {
		out["system_prompt"] = trace.SystemPrompt
	}
	if trace.UserPrompt != "" {
		out["user_prompt"] = trace.UserPrompt
	}
	if trace.RawOutput != "" {
		out["raw_output"] = trace.RawOutput
	}
	if trace.ParsedOutput != nil {
		out["parsed_output"] = trace.ParsedOutput
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
