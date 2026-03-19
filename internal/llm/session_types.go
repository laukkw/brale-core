package llm

import (
	"fmt"
	"strings"
)

const roundLaneKeyDelimiter = "|"

type RoundID string

func NewRoundID(raw string) (RoundID, error) {
	value, err := normalizeSessionField("round_id", raw)
	if err != nil {
		return "", err
	}
	return RoundID(value), nil
}

func (id RoundID) String() string {
	return string(id)
}

type LLMFlow string

const (
	LLMFlowFlat       LLMFlow = "flat"
	LLMFlowInPosition LLMFlow = "in_position"
)

func NewLLMFlow(raw string) (LLMFlow, error) {
	value, err := normalizeSessionField("flow", raw)
	if err != nil {
		return "", err
	}
	value = strings.ToLower(value)
	switch LLMFlow(value) {
	case LLMFlowFlat, LLMFlowInPosition:
		return LLMFlow(value), nil
	default:
		return "", fmt.Errorf("flow is invalid: %s", value)
	}
}

func (flow LLMFlow) String() string {
	return string(flow)
}

type LLMStage string

const (
	LLMStageIndicator LLMStage = "indicator"
	LLMStageStructure LLMStage = "structure"
	LLMStageMechanics LLMStage = "mechanics"
)

func NewLLMStage(raw string) (LLMStage, error) {
	value, err := normalizeSessionField("stage", raw)
	if err != nil {
		return "", err
	}
	value = strings.ToLower(value)
	switch LLMStage(value) {
	case LLMStageIndicator, LLMStageStructure, LLMStageMechanics:
		return LLMStage(value), nil
	default:
		return "", fmt.Errorf("stage is invalid: %s", value)
	}
}

func (stage LLMStage) String() string {
	return string(stage)
}

type SessionMode string

const (
	SessionModeSession   SessionMode = "session"
	SessionModeStateless SessionMode = "stateless"
)

func NewSessionMode(raw string) (SessionMode, error) {
	value, err := normalizeSessionField("mode", raw)
	if err != nil {
		return "", err
	}
	value = strings.ToLower(value)
	switch SessionMode(value) {
	case SessionModeSession, SessionModeStateless:
		return SessionMode(value), nil
	default:
		return "", fmt.Errorf("mode is invalid: %s", value)
	}
}

func (mode SessionMode) String() string {
	return string(mode)
}

type RoundLaneKey string

func NewRoundLaneKey(roundID RoundID, symbol string, flow LLMFlow, stage LLMStage) (RoundLaneKey, error) {
	rid, err := NewRoundID(roundID.String())
	if err != nil {
		return "", err
	}
	sym, err := normalizeSessionField("symbol", symbol)
	if err != nil {
		return "", err
	}
	parsedFlow, err := NewLLMFlow(flow.String())
	if err != nil {
		return "", err
	}
	parsedStage, err := NewLLMStage(stage.String())
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%s%s%s%s%s%s%s", rid, roundLaneKeyDelimiter, sym, roundLaneKeyDelimiter, parsedFlow, roundLaneKeyDelimiter, parsedStage)
	return RoundLaneKey(key), nil
}

func (key RoundLaneKey) String() string {
	return string(key)
}

func normalizeSessionField(fieldName, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required", fieldName)
	}
	if strings.Contains(value, roundLaneKeyDelimiter) {
		return "", fmt.Errorf("%s must not contain %q", fieldName, roundLaneKeyDelimiter)
	}
	return value, nil
}
