package decisionutil

import (
	"encoding/json"
	"fmt"
)

func ParseEnumJSON(data []byte, allowed map[string]struct{}, name string) (string, error) {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return "", err
	}
	if _, ok := allowed[value]; !ok {
		return "", fmt.Errorf("invalid %s: %s", name, value)
	}
	return value, nil
}
