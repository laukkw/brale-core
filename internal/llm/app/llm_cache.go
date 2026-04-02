// 本文件主要内容：在内存中缓存 LLM 输出并基于输入指纹做复用。

package llmapp

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type LLMStageCache struct {
	mu   sync.RWMutex
	data map[string]cachedStage
}

type cachedStage struct {
	Fingerprint string
	OutputJSON  []byte
	At          time.Time
}

func NewLLMStageCache() *LLMStageCache {
	return &LLMStageCache{data: map[string]cachedStage{}}
}

func (c *LLMStageCache) Load(symbol, stage string, input []byte) (cachedStage, bool) {
	if c == nil {
		return cachedStage{}, false
	}
	key := cacheKey(symbol, stage)
	fp := hashBytes(input)
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.data[key]
	if !ok || item.Fingerprint != fp {
		return cachedStage{}, false
	}
	return item, true
}

func (c *LLMStageCache) LoadLatest(symbol, stage string) (cachedStage, bool) {
	if c == nil {
		return cachedStage{}, false
	}
	key := cacheKey(symbol, stage)
	c.mu.RLock()
	defer c.mu.RUnlock()
	item, ok := c.data[key]
	if !ok {
		return cachedStage{}, false
	}
	return item, true
}

func (c *LLMStageCache) Store(symbol, stage string, output any, input []byte) {
	if c == nil {
		return
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return
	}
	key := cacheKey(symbol, stage)
	item := cachedStage{
		Fingerprint: hashBytes(input),
		OutputJSON:  raw,
		At:          time.Now().UTC(),
	}
	c.mu.Lock()
	c.data[key] = item
	c.mu.Unlock()
}

func appendLastOutput(user string, cache *LLMStageCache, symbol string, stage string) string {
	if cache == nil {
		return user
	}
	item, ok := cache.LoadLatest(symbol, stage)
	if !ok {
		return user
	}
	if len(item.OutputJSON) == 0 {
		return user
	}
	return fmt.Sprintf(
		"%s\n历史参考输出(UTC时间):%s\n以下内容只可作为历史参考，禁止直接复用其中的结论、reason、signal_tag、monitor_tag、布尔判断、置信度或其他结果。你只能将其用于对比前后变化、辅助评估走向演化，并必须严格依据本轮输入重新完成判断。若历史参考输出与本轮输入不一致，必须以本轮输入为准。\n%s",
		user,
		item.At.Format(time.RFC3339),
		string(item.OutputJSON),
	)
}

func cacheKey(symbol, stage string) string {
	return fmt.Sprintf("%s|%s", symbol, stage)
}

func hashBytes(input []byte) string {
	sum := sha256.Sum256(input)
	return fmt.Sprintf("%x", sum[:])
}
