package newsoverlay

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"brale-core/internal/config"
	"brale-core/internal/news/gdelt"
	"brale-core/internal/pkg/llmclean"
	"brale-core/internal/pkg/logging"

	"go.uber.org/zap"
)

const gdeltRequestGap = 30 * time.Second

var hotTitleKeywords = []string{
	"liquidation",
	"hack",
	"exploit",
	"etf",
	"sec",
	"cftc",
	"federal reserve",
	"fomc",
	"funding rate",
	"open interest",
}

type LLMCaller interface {
	Call(ctx context.Context, system, user string) (string, error)
}

type NewsPromptBuilder interface {
	AgentNewsOverlayPrompt(inputJSON string) (string, string, error)
}

type ErrorNotifier interface {
	SendError(ctx context.Context, message string) error
}

type Updater struct {
	Config        config.NewsOverlayConfig
	FocusSymbols  []string
	Store         *Store
	GDELT         *gdelt.Client
	LLM           LLMCaller
	PromptBuilder NewsPromptBuilder
	Notifier      ErrorNotifier
	Logger        *zap.Logger
}

type llmDecision struct {
	EntryMultiplierLong  float64                       `json:"entry_multiplier_long"`
	EntryMultiplierShort float64                       `json:"entry_multiplier_short"`
	TightenScoreBySide   map[string]map[string]float64 `json:"tighten_score_by_side"`
	Evidence             []EvidenceItem                `json:"evidence"`
}

type llmInput struct {
	UpdatedAt string                  `json:"updated_at"`
	Windows   map[string][]llmArticle `json:"windows"`
}

type llmArticle struct {
	Title  string `json:"title"`
	URL    string `json:"url"`
	Domain string `json:"domain,omitempty"`
	SeenAt string `json:"seen_at,omitempty"`
}

func (u *Updater) RunOnce(ctx context.Context) error {
	if u == nil {
		return fmt.Errorf("news overlay updater is nil")
	}
	if !u.Config.Enabled {
		return nil
	}
	if u.Store == nil {
		return fmt.Errorf("news overlay store is required")
	}
	if u.GDELT == nil {
		return fmt.Errorf("gdelt client is required")
	}
	if u.LLM == nil {
		return fmt.Errorf("llm caller is required")
	}
	if u.PromptBuilder == nil {
		return fmt.Errorf("prompt builder is required")
	}
	queries := resolveQueriesForSymbols(u.Config, u.FocusSymbols)
	if len(queries) == 0 {
		return fmt.Errorf("news overlay query is required")
	}
	queryLabel := strings.Join(queries, " || ")
	now := time.Now().UTC()
	windows := []string{"1h", "4h"}
	windowArticles := make(map[string][]gdelt.Article, len(windows))
	rawItemsCount := make(map[string]int, len(windows))
	lastRequestAt := time.Time{}
	for _, window := range windows {
		articles, err := u.fetchWindowArticles(ctx, window, queries, &lastRequestAt)
		if err != nil {
			return fmt.Errorf("gdelt fetch %s failed: %w", window, err)
		}
		windowArticles[window] = articles
		rawItemsCount[window] = len(articles)
	}
	newsRaw := make([]NewsItemRaw, 0, rawItemsCount["1h"]+rawItemsCount["4h"])
	for _, window := range windows {
		for _, article := range windowArticles[window] {
			newsRaw = append(newsRaw, NewsItemRaw{
				Window:       window,
				Title:        article.Title,
				URL:          article.URL,
				Domain:       article.Domain,
				SeenAt:       article.SeenAt.UTC(),
				SignalSource: "doc",
			})
		}
	}

	filteredByWindow := filterWindowArticlesForSymbols(windowArticles, windows, u.Config, u.FocusSymbols)
	newsUsed := flattenWindowArticles(filteredByWindow, windows)
	effectiveItemsCount := make(map[string]int, len(windows))
	for _, window := range windows {
		effectiveItemsCount[window] = len(filteredByWindow[window])
	}
	minEffectiveItems := minEffectiveItemsByWindow(u.Config)
	if !hasEnoughEffectiveItems(effectiveItemsCount, minEffectiveItems, windows) {
		decision := neutralDecision()
		overlay := OverlayResult{
			UpdatedAt:            now,
			EntryMultiplierLong:  clampMultiplier(decision.EntryMultiplierLong),
			EntryMultiplierShort: clampMultiplier(decision.EntryMultiplierShort),
			TightenScoreBySide:   normalizeScores(decision.TightenScoreBySide),
			ItemsCount:           effectiveItemsCount,
			Evidence:             normalizeEvidence(decision.Evidence),
		}
		snapshot := Snapshot{
			UpdatedAt:      now,
			Overlay:        overlay,
			NewsItemsRaw:   newsRaw,
			NewsItemsUsed:  newsUsed,
			LLMDecisionRaw: marshalDecision(decision),
		}
		u.Store.Save(snapshot)
		u.notifyDataInsufficient(ctx, queryLabel, effectiveItemsCount, minEffectiveItems)
		u.logUpdate(ctx, overlay)
		return nil
	}

	userInput := llmInput{
		UpdatedAt: now.Format(time.RFC3339),
		Windows:   make(map[string][]llmArticle, len(windows)),
	}
	for _, window := range windows {
		articles := filteredByWindow[window]
		payloadItems := make([]llmArticle, 0, len(articles))
		for _, article := range articles {
			item := llmArticle{
				Title:  article.Title,
				URL:    article.URL,
				Domain: article.Domain,
			}
			if !article.SeenAt.IsZero() {
				item.SeenAt = article.SeenAt.UTC().Format(time.RFC3339)
			}
			payloadItems = append(payloadItems, item)
		}
		userInput.Windows[window] = payloadItems
	}
	inputRaw, err := json.Marshal(userInput)
	if err != nil {
		return err
	}
	systemPrompt, userPrompt, err := u.PromptBuilder.AgentNewsOverlayPrompt(string(inputRaw))
	if err != nil {
		return err
	}
	llmRaw, err := u.LLM.Call(ctx, systemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("news overlay llm failed: %w", err)
	}
	decision, err := parseLLMDecision(llmRaw)
	if err != nil {
		return fmt.Errorf("news overlay llm parse failed: %w", err)
	}
	overlay := OverlayResult{
		UpdatedAt:            now,
		EntryMultiplierLong:  clampMultiplier(decision.EntryMultiplierLong),
		EntryMultiplierShort: clampMultiplier(decision.EntryMultiplierShort),
		TightenScoreBySide:   normalizeScores(decision.TightenScoreBySide),
		ItemsCount:           effectiveItemsCount,
		Evidence:             normalizeEvidence(decision.Evidence),
	}
	snapshot := Snapshot{
		UpdatedAt:      now,
		Overlay:        overlay,
		NewsItemsRaw:   newsRaw,
		NewsItemsUsed:  newsUsed,
		LLMDecisionRaw: strings.TrimSpace(llmRaw),
	}
	u.Store.Save(snapshot)
	u.notifyDataInsufficient(ctx, queryLabel, effectiveItemsCount, minEffectiveItems)
	u.logUpdate(ctx, overlay)
	return nil
}

func (u *Updater) fetchWindowArticles(ctx context.Context, window string, queries []string, lastRequestAt *time.Time) ([]gdelt.Article, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	merged := make([]gdelt.Article, 0, len(queries)*u.Config.MaxRecords)
	for idx, query := range queries {
		if err := waitForNextGDELTRequest(ctx, lastRequestAt); err != nil {
			return nil, err
		}
		articles, err := u.GDELT.FetchArticles(ctx, query, window, u.Config.MaxRecords)
		if err != nil {
			return nil, fmt.Errorf("query[%d] failed: %w", idx, err)
		}
		*lastRequestAt = time.Now()
		merged = append(merged, articles...)
	}
	return dedupeArticlesWithinWindow(merged), nil
}

func waitForNextGDELTRequest(ctx context.Context, lastRequestAt *time.Time) error {
	if lastRequestAt == nil || lastRequestAt.IsZero() {
		return nil
	}
	wait := gdeltRequestGap - time.Since(*lastRequestAt)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func dedupeArticlesWithinWindow(in []gdelt.Article) []gdelt.Article {
	if len(in) == 0 {
		return nil
	}
	out := make([]gdelt.Article, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		title := normalizeTitle(item.Title)
		if title == "" {
			continue
		}
		domain := normalizeDomain(item.Domain)
		if domain == "" {
			domain = normalizeDomainFromURL(item.URL)
		}
		key := title + "|" + domain
		if domain == "" {
			key = title + "|" + strings.ToLower(strings.TrimSpace(item.URL))
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if strings.TrimSpace(item.Domain) == "" {
			item.Domain = domain
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti := out[i].SeenAt
		tj := out[j].SeenAt
		switch {
		case !ti.IsZero() && !tj.IsZero() && !ti.Equal(tj):
			return ti.After(tj)
		case !ti.IsZero() && tj.IsZero():
			return true
		case ti.IsZero() && !tj.IsZero():
			return false
		}
		hi := articleHeatScore(out[i].Title)
		hj := articleHeatScore(out[j].Title)
		if hi != hj {
			return hi > hj
		}
		return strings.ToLower(strings.TrimSpace(out[i].Title)) < strings.ToLower(strings.TrimSpace(out[j].Title))
	})
	return out
}

func flattenWindowArticles(in map[string][]gdelt.Article, windows []string) []NewsItemRaw {
	if len(in) == 0 || len(windows) == 0 {
		return nil
	}
	out := make([]NewsItemRaw, 0)
	for _, window := range windows {
		items := in[window]
		for _, item := range items {
			out = append(out, NewsItemRaw{
				Window:       window,
				Title:        item.Title,
				URL:          item.URL,
				Domain:       item.Domain,
				SeenAt:       item.SeenAt.UTC(),
				SignalSource: "doc",
			})
		}
	}
	return out
}

func (u *Updater) logUpdate(ctx context.Context, overlay OverlayResult) {
	logger := u.Logger
	if logger == nil {
		logger = logging.FromContext(ctx).Named("news-overlay")
	}
	logger.Info("news overlay updated",
		zap.Time("updated_at", overlay.UpdatedAt),
		zap.Int("items_1h", overlay.ItemsCount["1h"]),
		zap.Int("items_4h", overlay.ItemsCount["4h"]),
		zap.Float64("entry_multiplier_long", overlay.EntryMultiplierLong),
		zap.Float64("entry_multiplier_short", overlay.EntryMultiplierShort),
	)
}

func (u *Updater) notifyDataInsufficient(ctx context.Context, query string, items map[string]int, mins map[string]int) {
	if u.Notifier == nil {
		return
	}
	for _, window := range []string{"1h", "4h"} {
		count := items[window]
		minItems := mins[window]
		if count >= minItems {
			continue
		}
		message := fmt.Sprintf("[NEWS_OVERLAY][WARN] data insufficient\n- window: %s\n- effective_items_count: %d\n- min_effective_items: %d\n- sourcelang: english\n- maxrecords: %d\n- query: %s\n- action: skip_news_overlay_llm",
			window,
			count,
			minItems,
			u.Config.MaxRecords,
			query,
		)
		_ = u.Notifier.SendError(ctx, message)
	}
}

func neutralDecision() llmDecision {
	return llmDecision{
		EntryMultiplierLong:  1.0,
		EntryMultiplierShort: 1.0,
		TightenScoreBySide: map[string]map[string]float64{
			"long":  {"1h": 0, "4h": 0},
			"short": {"1h": 0, "4h": 0},
		},
		Evidence: nil,
	}
}

func marshalDecision(decision llmDecision) string {
	raw, err := json.Marshal(decision)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func parseLLMDecision(raw string) (llmDecision, error) {
	raw = llmclean.CleanJSON(raw)
	if strings.TrimSpace(raw) == "" {
		return llmDecision{}, fmt.Errorf("empty llm decision")
	}
	var decision llmDecision
	if err := json.Unmarshal([]byte(raw), &decision); err != nil {
		return llmDecision{}, err
	}
	return decision, nil
}

func normalizeQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	lower := strings.ToLower(query)
	if strings.Contains(lower, "sourcelang:") {
		return query
	}
	return query + " sourcelang:english"
}

func normalizeQueries(cfg config.NewsOverlayConfig) []string {
	out := make([]string, 0, len(cfg.Queries)+1)
	seen := make(map[string]struct{}, len(cfg.Queries)+1)
	appendQuery := func(raw string) {
		query := normalizeQuery(raw)
		if query == "" {
			return
		}
		key := strings.ToLower(query)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, query)
	}
	for _, query := range cfg.Queries {
		appendQuery(query)
	}
	if len(out) > 0 {
		return out
	}
	appendQuery(cfg.Query)
	return out
}

func resolveQueriesForSymbols(cfg config.NewsOverlayConfig, symbols []string) []string {
	querySets := [][]string{{"bitcoin", "BTC"}, {"ethereum", "ETH"}}
	seenBase := map[string]struct{}{"BTC": {}, "ETH": {}}
	for _, raw := range symbols {
		base := extractBaseAsset(raw)
		if base == "" {
			continue
		}
		if _, ok := seenBase[base]; ok {
			continue
		}
		seenBase[base] = struct{}{}
		querySets = append(querySets, assetQueryTerms(base))
	}
	queries := make([]string, 0, len(querySets))
	for _, terms := range querySets {
		if len(terms) == 0 {
			continue
		}
		parts := make([]string, 0, len(terms))
		for _, term := range terms {
			term = strings.TrimSpace(term)
			if term == "" {
				continue
			}
			parts = append(parts, term)
		}
		if len(parts) == 0 {
			continue
		}
		queries = append(queries, "("+strings.Join(parts, " OR ")+") sourcelang:english")
	}
	if len(queries) > 0 {
		merged := cfg
		merged.Query = ""
		merged.Queries = queries
		return normalizeQueries(merged)
	}
	return normalizeQueries(cfg)
}

func extractBaseAsset(raw string) string {
	symbol := strings.ToUpper(strings.TrimSpace(raw))
	if symbol == "" {
		return ""
	}
	if idx := strings.Index(symbol, "/"); idx > 0 {
		symbol = symbol[:idx]
	}
	if idx := strings.Index(symbol, "-"); idx > 0 {
		symbol = symbol[:idx]
	}
	if idx := strings.Index(symbol, "_"); idx > 0 {
		symbol = symbol[:idx]
	}
	quotes := []string{"USDT", "USDC", "BUSD", "USD", "PERP"}
	for _, quote := range quotes {
		if strings.HasSuffix(symbol, quote) && len(symbol) > len(quote) {
			base := strings.TrimSuffix(symbol, quote)
			if base != "" {
				return base
			}
		}
	}
	return symbol
}

func assetQueryTerms(base string) []string {
	base = strings.ToUpper(strings.TrimSpace(base))
	switch base {
	case "BTC":
		return []string{"bitcoin", "BTC"}
	case "ETH":
		return []string{"ethereum", "ETH"}
	case "XAG":
		return []string{"XAG", "silver"}
	case "XAU":
		return []string{"XAU", "gold"}
	default:
		return []string{base}
	}
}

func articleHeatScore(title string) int {
	titleNorm := normalizeTitle(title)
	if titleNorm == "" {
		return 0
	}
	score := 0
	for _, keyword := range hotTitleKeywords {
		if strings.Contains(titleNorm, keyword) {
			score++
		}
	}
	return score
}

func normalizeScores(in map[string]map[string]float64) map[string]map[string]float64 {
	out := map[string]map[string]float64{
		"long":  {"1h": 0, "4h": 0},
		"short": {"1h": 0, "4h": 0},
	}
	for _, side := range []string{"long", "short"} {
		windowMap, ok := in[side]
		if !ok {
			continue
		}
		for _, window := range []string{"1h", "4h"} {
			if val, ok := windowMap[window]; ok {
				out[side][window] = clampScore(val)
			}
		}
	}
	return out
}

func normalizeEvidence(in []EvidenceItem) []EvidenceItem {
	if len(in) == 0 {
		return nil
	}
	out := make([]EvidenceItem, 0, len(in))
	for _, item := range in {
		window := strings.TrimSpace(item.Window)
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		if window != "1h" && window != "4h" {
			window = "1h"
		}
		out = append(out, EvidenceItem{Window: window, Title: title})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Window == out[j].Window {
			return out[i].Title < out[j].Title
		}
		return out[i].Window < out[j].Window
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func clampMultiplier(v float64) float64 {
	if v < 0.2 {
		return 0.2
	}
	if v > 1.5 {
		return 1.5
	}
	return v
}

func clampScore(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
