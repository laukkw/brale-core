package llmapp

import "encoding/json"

func normalizeMechanicsInput(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	delete(payload, "timestamp")
	delete(payload, "fear_greed_next_update_sec")
	normalized, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return normalized
}
