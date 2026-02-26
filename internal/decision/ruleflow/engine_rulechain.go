package ruleflow

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"brale-core/internal/pkg/logging"

	"github.com/rulego/rulego"
	ruletypes "github.com/rulego/rulego/api/types"
	"go.uber.org/zap"
)

func (e *Engine) loadEngine(ruleChainPath string) (ruletypes.RuleEngine, error) {
	abs := ruleChainPath
	if !strings.HasPrefix(ruleChainPath, "/") {
		if resolved := resolveRuleChainPath(ruleChainPath); resolved != "" {
			abs = resolved
		}
	}
	e.mu.RLock()
	if engine, ok := e.cache[abs]; ok {
		e.mu.RUnlock()
		return engine, nil
	}
	e.mu.RUnlock()

	content, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	logRuleChainHeader(abs, content)
	content, engineID, err := normalizeRuleChain(content)
	if err != nil {
		return nil, err
	}
	logRuleChainSummary(abs, content)
	if err := e.EnsureRegistered(); err != nil {
		return nil, err
	}
	if e.pool == nil {
		e.pool = rulego.NewRuleGo()
	}
	if engineID != "" {
		e.pool.Del(engineID)
	}
	debugCfg := rulego.NewConfig(
		ruletypes.WithOnDebug(func(chainId, flowType, nodeId string, msg ruletypes.RuleMsg, relationType string, err error) {
			logging.L().Named("ruleflow").Debug("ruleflow debug",
				zap.String("chain_id", chainId),
				zap.String("flow", flowType),
				zap.String("node_id", nodeId),
				zap.String("relation", relationType),
				zap.Error(err),
			)
		}),
	)
	engine, err := e.pool.New(engineID, content, rulego.WithConfig(debugCfg))

	if err != nil {
		return nil, err
	}
	if engine == nil {
		return nil, fmt.Errorf("ruleflow engine nil")
	}
	logRuleChainDSL(engine)
	e.mu.Lock()
	e.cache[abs] = engine
	e.mu.Unlock()
	return engine, nil
}

func normalizeRuleChain(content []byte) ([]byte, string, error) {
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, "", err
	}
	if meta, ok := raw["metadata"]; ok {
		engineID := normalizeRuleChainID(raw)
		if engineID == "" {
			return content, "", nil
		}
		payload := map[string]any{
			"ruleChain": raw["ruleChain"],
			"metadata":  meta,
		}
		ensureRuleChainDebug(payload["ruleChain"])
		out, err := json.Marshal(payload)
		return out, engineID, err
	}
	ruleChain, ok := raw["ruleChain"].(map[string]any)
	if !ok {
		return content, "", nil
	}
	nodes, _ := ruleChain["nodes"].([]any)
	connections, _ := ruleChain["connections"].([]any)
	metadata := map[string]any{
		"nodes":       nodes,
		"connections": connections,
	}
	payload := map[string]any{
		"ruleChain": ruleChain,
		"metadata":  metadata,
	}
	ensureRuleChainDebug(ruleChain)
	engineID := normalizeRuleChainID(map[string]any{"ruleChain": ruleChain})
	out, err := json.Marshal(payload)
	return out, engineID, err
}

func ensureRuleChainDebug(ruleChain any) {
	rc, ok := ruleChain.(map[string]any)
	if !ok {
		return
	}
	rc["debugMode"] = true
}

func normalizeRuleChainID(raw map[string]any) string {
	ruleChain, ok := raw["ruleChain"].(map[string]any)
	if !ok {
		return ""
	}
	if id, ok := ruleChain["id"].(string); ok && strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	name := ""
	if val, ok := ruleChain["name"].(string); ok {
		name = strings.TrimSpace(val)
	}
	hash := sha256.Sum256([]byte(name))
	id := fmt.Sprintf("brale-%x", hash[:6])
	ruleChain["id"] = id
	return id
}

func logRuleChainHeader(path string, content []byte) {
	if len(content) == 0 {
		return
	}
	hash := sha256.Sum256(content)
	header := string(content)
	if len(header) > 200 {
		header = header[:200]
	}
	logging.L().Named("ruleflow").Info("rule chain raw",
		zap.String("path", path),
		zap.String("sha256", fmt.Sprintf("%x", hash[:])),
		zap.String("head", header),
	)
}

func logRuleChainDSL(engine ruletypes.RuleEngine) {
	if engine == nil {
		return
	}
	dsl := engine.DSL()
	head := dsl
	if len(head) > 200 {
		head = head[:200]
	}
	full := string(dsl)
	logging.L().Named("ruleflow").Info("rule chain dsl",
		zap.String("head", string(head)),
	)
	if strings.Contains(full, "temperature") {
		snippet := full
		if len(snippet) > 800 {
			snippet = snippet[:800]
		}
		logging.L().Named("ruleflow").Info("rule chain dsl contains temperature",
			zap.String("snippet", snippet),
		)
	}
}

func logRuleChainSummary(path string, content []byte) {
	var raw map[string]any
	if err := json.Unmarshal(content, &raw); err != nil {
		return
	}
	rc, _ := raw["ruleChain"].(map[string]any)
	if rc != nil {
		logging.L().Named("ruleflow").Info("rule chain info",
			zap.String("path", path),
			zap.String("id", toString(rc["id"])),
			zap.String("name", toString(rc["name"])),
		)
	}
	meta, _ := raw["metadata"].(map[string]any)
	nodes, _ := meta["nodes"].([]any)
	if len(nodes) == 0 {
		return
	}
	ids := make([]string, 0, len(nodes))
	expressions := make([]string, 0)
	for _, node := range nodes {
		item, ok := node.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := item["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
		if typ, ok := item["type"].(string); ok && typ == "switch" {
			if cfg, ok := item["configuration"].(map[string]any); ok {
				if expr, ok := cfg["expression"].(string); ok && expr != "" {
					expressions = append(expressions, expr)
				}
			}
		}
		if typ, ok := item["type"].(string); ok && (typ == "switch" || typ == "filters") {
			if cfg, ok := item["configuration"].(map[string]any); ok {
				if cases, ok := cfg["cases"].([]any); ok {
					for _, c := range cases {
						if cm, ok := c.(map[string]any); ok {
							if expr, ok := cm["case"].(string); ok && expr != "" {
								expressions = append(expressions, expr)
							}
						}
					}
				}
			}
		}
	}
	logging.L().Named("ruleflow").Info("rule chain summary",
		zap.String("path", path),
		zap.Int("nodes", len(nodes)),
		zap.Strings("node_ids", ids),
		zap.Strings("expressions", expressions),
	)
}

func resolveRuleChainPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	cleanPath := filepath.Clean(trimmed)
	if filepath.IsAbs(cleanPath) {
		if _, err := os.Stat(cleanPath); err == nil {
			return cleanPath
		}
	}
	candidates := []string{}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(wd, cleanPath)),
			filepath.Clean(filepath.Join(wd, "..", cleanPath)),
			filepath.Clean(filepath.Join(wd, "..", "..", cleanPath)),
		)
	}
	base := filepath.Base(cleanPath)
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(wd, "configs", "rules", base)),
			filepath.Clean(filepath.Join(wd, "..", "configs", "rules", base)),
			filepath.Clean(filepath.Join(wd, "..", "..", "configs", "rules", base)),
		)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Clean(filepath.Join(exeDir, cleanPath)),
			filepath.Clean(filepath.Join(exeDir, "..", cleanPath)),
			filepath.Clean(filepath.Join(exeDir, "configs", "rules", base)),
			filepath.Clean(filepath.Join(exeDir, "..", "configs", "rules", base)),
		)
		if resolved, resolveErr := filepath.EvalSymlinks(exeDir); resolveErr == nil {
			candidates = append(candidates,
				filepath.Clean(filepath.Join(resolved, cleanPath)),
				filepath.Clean(filepath.Join(resolved, "..", cleanPath)),
				filepath.Clean(filepath.Join(resolved, "configs", "rules", base)),
				filepath.Clean(filepath.Join(resolved, "..", "configs", "rules", base)),
			)
		}
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return cleanPath
}
