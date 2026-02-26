// 本文件主要内容：提供 LLM JSON 输出清洗方法。
package llmclean

import "strings"

func StripCodeFences(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	firstLineEnd := strings.Index(trimmed, "\n")
	if firstLineEnd == -1 {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
	}
	body := trimmed[firstLineEnd+1:]
	if end := strings.LastIndex(body, "```"); end >= 0 {
		body = body[:end]
	}
	return strings.TrimSpace(body)
}

func CleanJSON(raw string) string {
	s := StripCodeFences(raw)
	if s == "" {
		return s
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		s = s[start : end+1]
	}
	return strings.TrimSpace(s)
}
