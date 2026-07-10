package usageledger

import "strings"

// ModelAliasRule maps an upstream model to the configured model alias.
type ModelAliasRule struct {
	Provider      string
	AuthIndex     string
	UpstreamModel string
	Alias         string
}

func resolveAnalyticsModel(event Event, rules []ModelAliasRule) string {
	if alias := strings.TrimSpace(event.ModelAlias); alias != "" {
		return alias
	}

	provider := strings.TrimSpace(event.Provider)
	authIndex := strings.TrimSpace(event.AuthIndex)
	upstreamModel := strings.TrimSpace(event.Model)
	if upstreamModel == "" {
		return ""
	}

	exactAliases := make([]string, 0, 1)
	for _, rule := range rules {
		if !isModelAliasRule(rule) || !strings.EqualFold(strings.TrimSpace(rule.Provider), provider) || !strings.EqualFold(strings.TrimSpace(rule.UpstreamModel), upstreamModel) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(rule.AuthIndex), authIndex) {
			exactAliases = appendUniqueModelAlias(exactAliases, strings.TrimSpace(rule.Alias))
		}
	}
	if len(exactAliases) == 1 {
		return exactAliases[0]
	}
	if len(exactAliases) > 1 {
		return upstreamModel
	}

	providerAliases := make([]string, 0, 1)
	for _, rule := range rules {
		if isModelAliasRule(rule) && strings.EqualFold(strings.TrimSpace(rule.Provider), provider) && strings.EqualFold(strings.TrimSpace(rule.UpstreamModel), upstreamModel) {
			providerAliases = appendUniqueModelAlias(providerAliases, strings.TrimSpace(rule.Alias))
		}
	}
	if len(providerAliases) == 1 {
		return providerAliases[0]
	}
	return upstreamModel
}

func isModelAliasRule(rule ModelAliasRule) bool {
	upstreamModel := strings.TrimSpace(rule.UpstreamModel)
	alias := strings.TrimSpace(rule.Alias)
	return strings.TrimSpace(rule.Provider) != "" && upstreamModel != "" && alias != "" && !strings.EqualFold(upstreamModel, alias)
}

func appendUniqueModelAlias(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}
