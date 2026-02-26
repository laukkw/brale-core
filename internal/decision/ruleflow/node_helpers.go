package ruleflow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"brale-core/internal/decision/decisionutil"

	"github.com/rulego/rulego/api/types"
)

func readRoot(msg types.RuleMsg) (map[string]any, error) {
	data, err := msg.GetJsonData()
	if err != nil {
		return nil, err
	}
	root, ok := data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("ruleflow input invalid")
	}
	return root, nil
}

func respondRuleMsgJSON(ctx types.RuleContext, msg types.RuleMsg, root map[string]any) bool {
	payload, err := json.Marshal(root)
	if err != nil {
		ctx.TellFailure(msg, err)
		return false
	}
	msg.DataType = types.JSON
	msg.SetData(string(payload))
	ctx.TellSuccess(msg)
	return true
}

func toMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	if out, ok := v.(map[string]any); ok {
		return out
	}
	return map[string]any{}
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toBool(v any) bool {
	return decisionutil.ToBool(v)
}

func toInt(v any) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case float32:
		return int(val)
	case string:
		if strings.TrimSpace(val) == "" {
			return 0
		}
		parsed, err := strconv.Atoi(val)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func toFloat(v any) float64 {
	return decisionutil.ToFloat(v)
}

func toFloatSlice(raw []any) []float64 {
	out := make([]float64, 0, len(raw))
	for _, item := range raw {
		out = append(out, toFloat(item))
	}
	return out
}

func resolvePrevHighLow(trend map[string]any) (float64, float64) {
	recentRaw, ok := trend["recent_candles"].([]any)
	if !ok || len(recentRaw) == 0 {
		return 0, 0
	}
	last := toMap(recentRaw[len(recentRaw)-1])
	return toFloat(last["h"]), toFloat(last["l"])
}
