package newsoverlay

import (
	"net/url"
	"strings"
	"unicode"

	"brale-core/internal/config"
	"brale-core/internal/news/gdelt"
)

var defaultRelevantTitleKeywords = []string{
	"bitcoin",
	"btc",
	"ethereum",
	"eth",
	"solana",
	"sol",
	"xrp",
	"crypto",
	"cryptocurrency",
	"digital asset",
	"stablecoin",
	"exchange",
	"on chain",
	"onchain",
	"funding rate",
	"open interest",
	"liquidation",
	"sec",
	"cftc",
	"federal reserve",
	"fomc",
	"treasury",
	"etf",
}

type symbolScope struct {
	enabled      bool
	titleMatches []string
}

type articleFilter struct {
	maxItemsPerDomain   int
	blockedDomains      map[string]struct{}
	blockedTitleKeyword []string
}

func filterWindowArticles(in map[string][]gdelt.Article, windows []string, cfg config.NewsOverlayConfig) map[string][]gdelt.Article {
	return filterWindowArticlesWithScope(in, windows, cfg, symbolScope{})
}

func filterWindowArticlesForSymbols(in map[string][]gdelt.Article, windows []string, cfg config.NewsOverlayConfig, symbols []string) map[string][]gdelt.Article {
	return filterWindowArticlesWithScope(in, windows, cfg, scopeFromSymbols(symbols))
}

func filterWindowArticlesWithScope(in map[string][]gdelt.Article, windows []string, cfg config.NewsOverlayConfig, scope symbolScope) map[string][]gdelt.Article {
	out := make(map[string][]gdelt.Article, len(windows))
	if len(windows) == 0 {
		return out
	}
	filter := newArticleFilter(cfg)
	seen := make(map[string]struct{}, 128)
	domainCount := make(map[string]int, 64)

	for _, window := range windows {
		items := in[window]
		if len(items) == 0 {
			out[window] = nil
			continue
		}
		filtered := make([]gdelt.Article, 0, len(items))
		for _, item := range items {
			titleNorm := normalizeTitle(item.Title)
			if titleNorm == "" {
				continue
			}
			domain := normalizeDomain(item.Domain)
			if domain == "" {
				domain = normalizeDomainFromURL(item.URL)
			}
			if filter.isBlockedDomain(domain) {
				continue
			}
			if filter.isBlockedTitle(titleNorm) {
				continue
			}
			if !isRelevantTitleWithScope(titleNorm, scope) {
				continue
			}
			key := titleNorm + "|" + domain
			if domain == "" {
				key = titleNorm + "|" + strings.ToLower(strings.TrimSpace(item.URL))
			}
			if _, ok := seen[key]; ok {
				continue
			}
			if domain != "" && filter.maxItemsPerDomain > 0 && domainCount[domain] >= filter.maxItemsPerDomain {
				continue
			}
			seen[key] = struct{}{}
			if domain != "" {
				domainCount[domain]++
			}
			copyItem := item
			if strings.TrimSpace(copyItem.Domain) == "" {
				copyItem.Domain = domain
			}
			filtered = append(filtered, copyItem)
		}
		out[window] = filtered
	}
	return out
}

func newArticleFilter(cfg config.NewsOverlayConfig) articleFilter {
	blockedDomains := make(map[string]struct{}, len(cfg.BlockedDomains))
	for _, item := range cfg.BlockedDomains {
		domain := normalizeDomain(item)
		if domain == "" {
			continue
		}
		blockedDomains[domain] = struct{}{}
	}
	blockedKeywords := make([]string, 0, len(cfg.BlockedTitleKeywords))
	for _, item := range cfg.BlockedTitleKeywords {
		normalized := normalizeTitle(item)
		if normalized == "" {
			continue
		}
		blockedKeywords = append(blockedKeywords, normalized)
	}
	maxItemsPerDomain := cfg.MaxItemsPerDomain
	if maxItemsPerDomain <= 0 {
		maxItemsPerDomain = 4
	}
	return articleFilter{
		maxItemsPerDomain:   maxItemsPerDomain,
		blockedDomains:      blockedDomains,
		blockedTitleKeyword: blockedKeywords,
	}
}

func (f articleFilter) isBlockedDomain(domain string) bool {
	if domain == "" || len(f.blockedDomains) == 0 {
		return false
	}
	_, ok := f.blockedDomains[domain]
	return ok
}

func (f articleFilter) isBlockedTitle(titleNormalized string) bool {
	if titleNormalized == "" || len(f.blockedTitleKeyword) == 0 {
		return false
	}
	for _, keyword := range f.blockedTitleKeyword {
		if strings.Contains(titleNormalized, keyword) {
			return true
		}
	}
	return false
}

func isRelevantTitleWithScope(titleNormalized string, scope symbolScope) bool {
	if titleNormalized == "" {
		return false
	}
	if scope.enabled && len(scope.titleMatches) > 0 {
		return containsAnyPhrase(titleNormalized, scope.titleMatches)
	}
	for _, keyword := range defaultRelevantTitleKeywords {
		if strings.Contains(titleNormalized, keyword) {
			return true
		}
	}
	return false
}

func containsAnyPhrase(titleNormalized string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(titleNormalized, keyword) {
			return true
		}
	}
	return false
}

func scopeFromSymbols(symbols []string) symbolScope {
	scope := symbolScope{enabled: true}
	terms := []string{"bitcoin", "btc", "ethereum", "eth"}
	seen := map[string]struct{}{
		"bitcoin":  {},
		"btc":      {},
		"ethereum": {},
		"eth":      {},
	}
	for _, raw := range symbols {
		base := extractBaseAsset(raw)
		if base == "" {
			continue
		}
		for _, item := range assetQueryTerms(base) {
			norm := normalizeTitle(item)
			if norm == "" {
				continue
			}
			if _, ok := seen[norm]; ok {
				continue
			}
			seen[norm] = struct{}{}
			terms = append(terms, norm)
		}
	}
	scope.titleMatches = terms
	return scope
}

func normalizeDomain(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.TrimPrefix(raw, "http://")
	raw = strings.TrimPrefix(raw, "https://")
	raw = strings.TrimPrefix(raw, "www.")
	if idx := strings.Index(raw, "/"); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(raw)
}

func normalizeDomainFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return normalizeDomain(parsed.Hostname())
}

func normalizeTitle(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	lastSpace := false
	for _, r := range raw {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func minEffectiveItemsByWindow(cfg config.NewsOverlayConfig) map[string]int {
	min1h := cfg.MinEffectiveItems1H
	if min1h <= 0 {
		min1h = cfg.MinItems1H
	}
	if min1h <= 0 {
		min1h = 2
	}
	min4h := cfg.MinEffectiveItems4H
	if min4h <= 0 {
		min4h = cfg.MinItems4H
	}
	if min4h <= 0 {
		min4h = 3
	}
	return map[string]int{"1h": min1h, "4h": min4h}
}

func hasEnoughEffectiveItems(itemsCount map[string]int, mins map[string]int, windows []string) bool {
	for _, window := range windows {
		if itemsCount[window] < mins[window] {
			return false
		}
	}
	return true
}
