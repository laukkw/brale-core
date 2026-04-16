package onboarding

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/creachadair/tomledit"
)

// tomlQuoted wraps a string value in TOML-compatible double quotes.
func tomlQuoted(v string) string {
	return strconv.Quote(v)
}

type tomlUpdate struct {
	Path  []string
	Value string
}

func applyTomlUpdates(content string, updates []tomlUpdate) (string, error) {
	out := content
	for _, update := range updates {
		if len(update.Path) == 0 {
			return "", fmt.Errorf("toml path is required")
		}
		next, err := applyTomlUpdate(out, update.Path, update.Value)
		if err != nil {
			return "", err
		}
		out = next
	}
	return out, nil
}

func RewriteSymbolConfig(base, targetSymbol string) (string, error) {
	out, err := applyTomlUpdates(base, []tomlUpdate{
		{Path: []string{"symbol"}, Value: tomlQuoted(targetSymbol)},
	})
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func RewriteStrategyConfig(base, targetSymbol string) (string, error) {
	out, err := applyTomlUpdates(base, []tomlUpdate{
		{Path: []string{"symbol"}, Value: tomlQuoted(targetSymbol)},
		{Path: []string{"id"}, Value: tomlQuoted("default-" + strings.ToLower(targetSymbol))},
	})
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func applyTomlUpdate(content string, path []string, value string) (string, error) {
	doc, err := tomledit.Parse(strings.NewReader(content))
	if err != nil {
		return "", err
	}
	entries := doc.Find(path...)
	if len(entries) == 0 {
		section := ""
		if len(path) > 1 {
			section = strings.Join(path[:len(path)-1], ".")
		}
		return insertTomlKey(content, section, path[len(path)-1], value)
	}
	lines := strings.Split(content, "\n")
	for _, entry := range entries {
		if entry == nil || !entry.IsMapping() || entry.KeyValue == nil {
			continue
		}
		lineNo := entry.KeyValue.Line
		if lineNo < 1 || lineNo > len(lines) {
			continue
		}
		lines[lineNo-1] = replaceTomlAssignment(lines[lineNo-1], value)
	}
	return strings.Join(lines, "\n"), nil
}

func applyObserveModeOnSieveRows(content string) (string, error) {
	doc, err := tomledit.Parse(strings.NewReader(content))
	if err != nil {
		return "", err
	}
	lines := strings.Split(content, "\n")
	gateEntries := doc.Find("risk_management", "sieve", "rows", "gate_action")
	for _, entry := range gateEntries {
		if entry == nil || !entry.IsMapping() || entry.KeyValue == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(entry.KeyValue.Value.String()), "\"VETO\"") {
			continue
		}
		lineNo := entry.KeyValue.Line
		if lineNo < 1 || lineNo > len(lines) {
			continue
		}
		lines[lineNo-1] = replaceTomlAssignment(lines[lineNo-1], tomlQuoted("WAIT"))
	}
	sizeEntries := doc.Find("risk_management", "sieve", "rows", "size_factor")
	for _, entry := range sizeEntries {
		if entry == nil || !entry.IsMapping() || entry.KeyValue == nil {
			continue
		}
		lineNo := entry.KeyValue.Line
		if lineNo < 1 || lineNo > len(lines) {
			continue
		}
		lines[lineNo-1] = replaceTomlAssignment(lines[lineNo-1], "0.0")
	}
	return strings.Join(lines, "\n"), nil
}

func insertTomlKey(content string, section string, key string, value string) (string, error) {
	lines := strings.Split(content, "\n")

	insert := fmt.Sprintf("%s = %s", key, value)
	_, end, hasSection := tomlSectionBounds(lines, section)
	if !hasSection {
		if section == "" {
			lines = append(lines, insert)
			return strings.Join(lines, "\n"), nil
		}
		lines = append(lines, "", "["+section+"]", insert)
		return strings.Join(lines, "\n"), nil
	}
	if section == "" {
		lines = append(lines[:end], append([]string{insert}, lines[end:]...)...)
		return strings.Join(lines, "\n"), nil
	}
	lines = append(lines[:end], append([]string{insert}, lines[end:]...)...)
	return strings.Join(lines, "\n"), nil
}

func tomlSectionBounds(lines []string, section string) (int, int, bool) {
	if section == "" {
		end := len(lines)
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if _, ok := parseTomlSection(trimmed); ok {
				end = i
				break
			}
		}
		return 0, end, true
	}
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		sec, ok := parseTomlSection(trimmed)
		if !ok {
			continue
		}
		if sec != section {
			continue
		}
		start = i + 1
		break
	}
	if start < 0 {
		return -1, -1, false
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if _, ok := parseTomlSection(trimmed); ok {
			end = i
			break
		}
	}
	return start, end, true
}

func parseTomlSection(trimmedLine string) (string, bool) {
	if strings.HasPrefix(trimmedLine, "[[") && strings.HasSuffix(trimmedLine, "]]") {
		return strings.TrimSpace(trimmedLine[2 : len(trimmedLine)-2]), true
	}
	if strings.HasPrefix(trimmedLine, "[") && strings.HasSuffix(trimmedLine, "]") {
		return strings.TrimSpace(trimmedLine[1 : len(trimmedLine)-1]), true
	}
	return "", false
}

func replaceTomlAssignment(line string, value string) string {
	idx := strings.Index(line, "=")
	if idx < 0 {
		return line
	}
	left := strings.TrimRight(line[:idx], " \t")
	right := line[idx+1:]
	comment := ""
	if c := strings.Index(right, "#"); c >= 0 {
		comment = strings.TrimSpace(right[c:])
	}
	out := left + " = " + value
	if comment != "" {
		out += " " + comment
	}
	return out
}
