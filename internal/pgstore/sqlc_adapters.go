package pgstore

import (
	"encoding/json"
	"slices"
	"time"

	"brale-core/internal/pgstore/queries"
	"brale-core/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
)

func mapGateEvent(row queries.GateEvent) store.GateEventRecord {
	return store.GateEventRecord{
		ID:                 uint(row.ID),
		SnapshotID:         uint(row.SnapshotID),
		RoundID:            textValue(row.RoundID),
		Symbol:             row.Symbol,
		Timestamp:          row.Timestamp,
		GlobalTradeable:    row.GlobalTradeable,
		DecisionAction:     textValue(row.DecisionAction),
		Grade:              int(row.Grade),
		GateReason:         textValue(row.GateReason),
		Direction:          textValue(row.Direction),
		ProviderRefsJSON:   json.RawMessage(cloneJSONBytes(row.ProviderRefsJson)),
		RuleHitJSON:        json.RawMessage(cloneJSONBytes(row.RuleHitJson)),
		DerivedJSON:        json.RawMessage(cloneJSONBytes(row.DerivedJson)),
		Fingerprint:        textValue(row.Fingerprint),
		SystemConfigHash:   textValue(row.SystemConfigHash),
		StrategyConfigHash: textValue(row.StrategyConfigHash),
		SourceVersion:      textValue(row.SourceVersion),
		CreatedAt:          timestamptzValue(row.CreatedAt),
	}
}

func mapLLMRound(row queries.LlmRound) store.LLMRoundRecord {
	return store.LLMRoundRecord{
		ID:             row.ID,
		SnapshotID:     uint(row.SnapshotID),
		Symbol:         row.Symbol,
		RoundType:      row.RoundType,
		StartedAt:      timestamptzValue(row.StartedAt),
		FinishedAt:     timestamptzValue(row.FinishedAt),
		TotalLatencyMS: int4Value(row.TotalLatencyMs),
		TotalTokenIn:   int4Value(row.TotalTokenIn),
		TotalTokenOut:  int4Value(row.TotalTokenOut),
		CallCount:      int(row.CallCount),
		Outcome:        textValue(row.Outcome),
		PromptVersion:  row.PromptVersion,
		Error:          textValue(row.Error),
		AgentCount:     int(row.AgentCount),
		ProviderCount:  int(row.ProviderCount),
		GateAction:     textValue(row.GateAction),
		RequestID:      textValue(row.RequestID),
		CreatedAt:      timestamptzValue(row.CreatedAt),
	}
}

func mapPromptRegistry(row queries.PromptRegistry) store.PromptRegistryEntry {
	return store.PromptRegistryEntry{
		ID:           uint(row.ID),
		Role:         row.Role,
		Stage:        row.Stage,
		Locale:       row.Locale,
		Version:      row.Version,
		SystemPrompt: row.SystemPrompt,
		Description:  textValue(row.Description),
		Active:       row.Active,
		CreatedAt:    timestamptzValue(row.CreatedAt),
		UpdatedAt:    timestamptzValue(row.UpdatedAt),
	}
}

func textValue(v pgtype.Text) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func int4Value(v pgtype.Int4) int {
	if !v.Valid {
		return 0
	}
	return int(v.Int32)
}

func timestamptzValue(v pgtype.Timestamptz) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return v.Time.UTC()
}

func cloneJSONBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return slices.Clone(raw)
}
