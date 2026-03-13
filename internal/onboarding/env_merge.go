package onboarding

import "strings"

func (g *Generator) buildEnvContent(req Request) (string, error) {
	rendered, err := executeTemplate(mustTemplate("env.tmpl", nil), envContextFromRequest(req))
	if err != nil {
		return "", err
	}
	existing, exists := g.readRepoFile(".env")
	if !exists || strings.TrimSpace(existing) == "" {
		if !strings.HasSuffix(rendered, "\n") {
			rendered += "\n"
		}
		return rendered, nil
	}

	target := parseEnvAssignments(rendered)
	if len(target) == 0 {
		return existing, nil
	}
	order := parseEnvOrder(rendered)

	lines := strings.Split(existing, "\n")
	seen := map[string]struct{}{}
	for i := range lines {
		key, prefix, ok := parseEnvLine(lines[i])
		if !ok {
			continue
		}
		newVal, has := target[key]
		if !has {
			continue
		}
		lines[i] = prefix + key + "=" + newVal
		seen[key] = struct{}{}
	}
	missing := make([]string, 0)
	for _, key := range order {
		if _, ok := seen[key]; ok {
			continue
		}
		if val, has := target[key]; has {
			missing = append(missing, key+"="+val)
		}
	}
	if len(missing) > 0 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, missing...)
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out, nil
}

func parseEnvAssignments(content string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		val := strings.TrimSpace(trimmed[eq+1:])
		if key == "" {
			continue
		}
		out[key] = val
	}
	return out
}

func parseEnvOrder(content string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		if key == "" {
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

func parseEnvLine(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	prefix := ""
	work := trimmed
	if strings.HasPrefix(work, "export ") {
		prefix = "export "
		work = strings.TrimSpace(strings.TrimPrefix(work, "export "))
	}
	eq := strings.Index(work, "=")
	if eq < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(work[:eq])
	if key == "" {
		return "", "", false
	}
	return key, prefix, true
}
