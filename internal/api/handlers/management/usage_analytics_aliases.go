package management

import (
	"sort"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/usageledger"
)

type usageAnalyticsAliasFallback struct {
	provider      string
	upstreamModel string
	aliases       map[string]string
}

func (h *Handler) usageAnalyticsModelAliases() []usageledger.ModelAliasRule {
	entries := h.openAICompatibilityWithAuthIndex()
	if len(entries) == 0 {
		return nil
	}

	rules := make([]usageledger.ModelAliasRule, 0)
	seen := make(map[string]struct{})
	addRule := func(rule usageledger.ModelAliasRule) {
		rule.Provider = strings.TrimSpace(rule.Provider)
		rule.AuthIndex = strings.TrimSpace(rule.AuthIndex)
		rule.UpstreamModel = strings.TrimSpace(rule.UpstreamModel)
		rule.Alias = strings.TrimSpace(rule.Alias)
		if rule.Provider == "" || rule.UpstreamModel == "" || rule.Alias == "" || strings.EqualFold(rule.UpstreamModel, rule.Alias) {
			return
		}
		key := strings.ToLower(rule.Provider) + "\x00" + strings.ToLower(rule.AuthIndex) + "\x00" + strings.ToLower(rule.UpstreamModel) + "\x00" + strings.ToLower(rule.Alias)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		rules = append(rules, rule)
	}

	fallbacks := make(map[string]*usageAnalyticsAliasFallback)
	for _, entry := range entries {
		provider := "openai-compatible-" + strings.ToLower(strings.TrimSpace(entry.Name))
		authIndexes := usageAnalyticsCompatibilityAuthIndexes(entry)
		for _, model := range entry.Models {
			upstreamModel := strings.TrimSpace(model.Name)
			alias := strings.TrimSpace(model.Alias)
			if upstreamModel == "" || alias == "" || strings.EqualFold(upstreamModel, alias) {
				continue
			}

			fallbackKey := strings.ToLower(provider) + "\x00" + strings.ToLower(upstreamModel)
			fallback := fallbacks[fallbackKey]
			if fallback == nil {
				fallback = &usageAnalyticsAliasFallback{
					provider:      provider,
					upstreamModel: upstreamModel,
					aliases:       make(map[string]string),
				}
				fallbacks[fallbackKey] = fallback
			}
			fallback.aliases[strings.ToLower(alias)] = alias

			for _, authIndex := range authIndexes {
				addRule(usageledger.ModelAliasRule{
					Provider:      provider,
					AuthIndex:     authIndex,
					UpstreamModel: upstreamModel,
					Alias:         alias,
				})
			}
		}
	}

	for _, fallback := range fallbacks {
		if len(fallback.aliases) != 1 {
			continue
		}
		for _, alias := range fallback.aliases {
			addRule(usageledger.ModelAliasRule{
				Provider:      fallback.provider,
				UpstreamModel: fallback.upstreamModel,
				Alias:         alias,
			})
		}
	}

	sort.Slice(rules, func(i, j int) bool {
		left, right := rules[i], rules[j]
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		if left.AuthIndex != right.AuthIndex {
			return left.AuthIndex < right.AuthIndex
		}
		if left.UpstreamModel != right.UpstreamModel {
			return left.UpstreamModel < right.UpstreamModel
		}
		return left.Alias < right.Alias
	})
	return rules
}

func usageAnalyticsCompatibilityAuthIndexes(entry openAICompatibilityWithAuthIndex) []string {
	indexes := make([]string, 0, len(entry.APIKeyEntries)+1)
	seen := make(map[string]struct{})
	add := func(index string) {
		index = strings.TrimSpace(index)
		if index == "" {
			return
		}
		if _, ok := seen[index]; ok {
			return
		}
		seen[index] = struct{}{}
		indexes = append(indexes, index)
	}
	if len(entry.APIKeyEntries) == 0 {
		add(entry.AuthIndex)
		return indexes
	}
	for _, apiKeyEntry := range entry.APIKeyEntries {
		add(apiKeyEntry.AuthIndex)
	}
	return indexes
}
