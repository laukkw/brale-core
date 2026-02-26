// This file defines supported long/short intervals.
package longshort

import "strings"

var supportedIntervals = map[string]struct{}{
	"5m":  {},
	"15m": {},
	"30m": {},
	"1h":  {},
	"2h":  {},
	"4h":  {},
	"6h":  {},
	"12h": {},
	"1d":  {},
}

func FilterSupported(intervals []string) []string {
	if len(intervals) == 0 {
		return nil
	}
	out := make([]string, 0, len(intervals))
	seen := make(map[string]struct{}, len(intervals))
	for _, iv := range intervals {
		key := strings.ToLower(strings.TrimSpace(iv))
		if key == "" {
			continue
		}
		if _, ok := supportedIntervals[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}
