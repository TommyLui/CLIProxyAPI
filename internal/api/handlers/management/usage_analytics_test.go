package management

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/usageledger"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/watcher/synthesizer"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestUsageAnalyticsEndpointReturnsUsageLedgerAnalytics(t *testing.T) {
	store := openManagementUsageStore(t)
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, nil)
	h.SetUsageLedger(store)
	router := usageManagementTestRouter(h)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	if _, err := store.InsertEvent(context.Background(), usageledger.Event{
		RequestID:       "req-analytics",
		Timestamp:       now,
		Provider:        "opencode-go",
		Model:           "kimi-k2.6",
		APIKeyHash:      "key-a",
		AccountRef:      "opencode-go:acct-a",
		ReasoningEffort: "high",
		Tokens:          usageledger.TokenUsage{InputTokens: 20, OutputTokens: 10, TotalTokens: 30},
	}); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	body := map[string]any{
		"from_ms": now.Add(-time.Minute).UnixMilli(),
		"to_ms":   now.Add(time.Minute).UnixMilli(),
		"filters": map[string]any{
			"providers": []string{"opencode-go"},
		},
		"include": map[string]any{
			"summary":       true,
			"model_stats":   true,
			"api_key_stats": true,
			"events_page": map[string]any{
				"limit": 5,
			},
		},
	}
	rec := performUsageManagementJSON(http.MethodPost, "/v0/management/usage-analytics", body, router)
	if rec.Code != http.StatusOK {
		t.Fatalf("analytics status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response usageledger.AnalyticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode analytics: %v", err)
	}
	if response.Summary == nil || response.Summary.TotalCalls != 1 || response.Summary.TotalTokens != 30 {
		t.Fatalf("summary = %#v", response.Summary)
	}
	if response.Events == nil || len(response.Events.Items) != 1 || response.Events.Items[0].RequestID != "req-analytics" {
		t.Fatalf("events = %#v", response.Events)
	}
	if response.Events.Items[0].ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %q, want high", response.Events.Items[0].ReasoningEffort)
	}
}

func TestUsageAnalyticsEndpointResolvesConfiguredModelAlias(t *testing.T) {
	store := openManagementUsageStore(t)
	const provider = "openai-compatible-cf worker"
	const upstream = "@cf/zai-org/glm-5.2"
	const alias = "glm-5.2"
	const baseURL = "https://cf.example/v1"
	const apiKey = "cf-key"

	idGen := synthesizer.NewStableIDGenerator()
	authID, _ := idGen.Next("openai-compatibility:cf worker", apiKey, baseURL, "")
	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{ID: authID, Provider: provider}
	authIndex := auth.EnsureIndex()
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{
		AuthDir: t.TempDir(),
		OpenAICompatibility: []config.OpenAICompatibility{{
			Name:    "cf worker",
			BaseURL: baseURL,
			APIKeyEntries: []config.OpenAICompatibilityAPIKey{{
				APIKey: apiKey,
			}},
			Models: []config.OpenAICompatibilityModel{{
				Name:  upstream,
				Alias: alias,
			}},
		}},
	}, manager)
	h.SetUsageLedger(store)
	router := usageManagementTestRouter(h)

	now := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)
	if err := store.UpsertModelPrice(context.Background(), usageledger.ModelPrice{
		Model:       alias,
		InputPer1M:  10,
		OutputPer1M: 20,
		Source:      "test",
	}); err != nil {
		t.Fatalf("upsert alias price: %v", err)
	}
	if _, err := store.InsertEvent(context.Background(), usageledger.Event{
		RequestID: "req-configured-alias",
		Timestamp: now,
		Provider:  provider,
		Model:     upstream,
		AuthIndex: authIndex,
		Tokens: usageledger.TokenUsage{
			InputTokens:  100,
			OutputTokens: 50,
			TotalTokens:  150,
		},
	}); err != nil {
		t.Fatalf("insert historical event: %v", err)
	}

	rec := performUsageManagementJSON(http.MethodPost, "/v0/management/usage-analytics", map[string]any{
		"from_ms": now.Add(-time.Minute).UnixMilli(),
		"to_ms":   now.Add(time.Minute).UnixMilli(),
		"include": map[string]any{
			"summary":     true,
			"model_stats": true,
			"events_page": map[string]any{"limit": 5},
		},
	}, router)
	if rec.Code != http.StatusOK {
		t.Fatalf("analytics status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response usageledger.AnalyticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode analytics: %v", err)
	}
	if len(response.ModelStats) != 1 || response.ModelStats[0].Model != alias {
		t.Fatalf("model stats = %#v", response.ModelStats)
	}
	if response.ModelStats[0].Cost == nil || math.Abs(*response.ModelStats[0].Cost-0.002) > 0.000000001 {
		t.Fatalf("model stat cost = %#v, want 0.002", response.ModelStats[0].Cost)
	}
	if response.Events == nil || len(response.Events.Items) != 1 {
		t.Fatalf("events = %#v", response.Events)
	}
	event := response.Events.Items[0]
	if event.Model != alias || event.UpstreamModel != upstream {
		t.Fatalf("event model = %q, upstream model = %q", event.Model, event.UpstreamModel)
	}
	if event.EstimatedCostUSD == nil || math.Abs(*event.EstimatedCostUSD-0.002) > 0.000000001 {
		t.Fatalf("event cost = %#v, want 0.002", event.EstimatedCostUSD)
	}
}

func TestUsageAnalyticsModelAliasesUseAuthIndexesAndOnlyUnambiguousFallbacks(t *testing.T) {
	const provider = "openai-compatible-cf worker"
	manager := coreauth.NewManager(nil, nil, nil)
	idGen := synthesizer.NewStableIDGenerator()
	register := func(baseURL, apiKey string) string {
		t.Helper()
		id, _ := idGen.Next("openai-compatibility:cf worker", apiKey, baseURL, "")
		auth := &coreauth.Auth{ID: id, Provider: provider}
		index := auth.EnsureIndex()
		if _, err := manager.Register(context.Background(), auth); err != nil {
			t.Fatalf("register auth: %v", err)
		}
		return index
	}

	indexA := register("https://cf-a.example/v1", "key-a")
	indexB := register("https://cf-b.example/v1", "key-b")
	indexC := register("https://cf-c.example/v1", "key-c")
	h := NewHandlerWithoutConfigFilePath(&config.Config{
		AuthDir: t.TempDir(),
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    " CF Worker ",
				BaseURL: "https://cf-a.example/v1",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{
					APIKey: "key-a",
				}},
				Models: []config.OpenAICompatibilityModel{
					{Name: "unique-upstream", Alias: "unique-alias"},
					{Name: "duplicate-upstream", Alias: "duplicate-alias"},
					{Name: "duplicate-upstream", Alias: "duplicate-alias"},
					{Name: "identity", Alias: "IDENTITY"},
					{Name: "", Alias: "missing-upstream"},
					{Name: "missing-alias", Alias: ""},
				},
			},
			{
				Name:    "cf worker",
				BaseURL: "https://cf-b.example/v1",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{
					APIKey: "key-b",
				}},
				Models: []config.OpenAICompatibilityModel{{Name: "shared-upstream", Alias: "alias-a"}},
			},
			{
				Name:    "cf worker",
				BaseURL: "https://cf-c.example/v1",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{
					APIKey: "key-c",
				}},
				Models: []config.OpenAICompatibilityModel{{Name: "shared-upstream", Alias: "alias-b"}},
			},
		},
	}, manager)

	rules := h.usageAnalyticsModelAliases()
	if !reflect.DeepEqual(rules, h.usageAnalyticsModelAliases()) {
		t.Fatalf("rules are not deterministic: %#v", rules)
	}

	hasRule := func(authIndex, upstream, alias string) bool {
		t.Helper()
		for _, rule := range rules {
			if rule.Provider == provider && rule.AuthIndex == authIndex && rule.UpstreamModel == upstream && rule.Alias == alias {
				return true
			}
		}
		return false
	}
	if !hasRule(indexA, "unique-upstream", "unique-alias") || !hasRule(indexA, "duplicate-upstream", "duplicate-alias") {
		t.Fatalf("missing exact rules for first auth index: %#v", rules)
	}
	if !hasRule(indexB, "shared-upstream", "alias-a") || !hasRule(indexC, "shared-upstream", "alias-b") {
		t.Fatalf("missing exact rules for distinct auth indexes: %#v", rules)
	}
	if !hasRule("", "unique-upstream", "unique-alias") || !hasRule("", "duplicate-upstream", "duplicate-alias") {
		t.Fatalf("missing unambiguous provider fallback: %#v", rules)
	}
	if hasRule("", "shared-upstream", "alias-a") || hasRule("", "shared-upstream", "alias-b") {
		t.Fatalf("ambiguous provider fallback = %#v", rules)
	}
	if hasRule(indexA, "identity", "IDENTITY") || hasRule(indexA, "", "missing-upstream") || hasRule(indexA, "missing-alias", "") {
		t.Fatalf("invalid rules = %#v", rules)
	}

	seen := make(map[usageledger.ModelAliasRule]struct{}, len(rules))
	for i, rule := range rules {
		if _, ok := seen[rule]; ok {
			t.Fatalf("duplicate rule = %#v", rule)
		}
		seen[rule] = struct{}{}
		if i > 0 && rule.Provider+"\x00"+rule.AuthIndex+"\x00"+rule.UpstreamModel+"\x00"+rule.Alias < rules[i-1].Provider+"\x00"+rules[i-1].AuthIndex+"\x00"+rules[i-1].UpstreamModel+"\x00"+rules[i-1].Alias {
			t.Fatalf("rules are not sorted: %#v", rules)
		}
	}
}

func TestUsageAnalyticsEndpointUsesAuthIndexAndDoesNotGuessConflictingFallback(t *testing.T) {
	store := openManagementUsageStore(t)
	const provider = "openai-compatible-cf worker"
	const upstream = "@cf/zai-org/glm-5.2"
	const aliasA = "glm-5.2-a"
	const aliasB = "glm-5.2-b"
	manager := coreauth.NewManager(nil, nil, nil)
	idGen := synthesizer.NewStableIDGenerator()
	register := func(baseURL, apiKey string) string {
		t.Helper()
		id, _ := idGen.Next("openai-compatibility:cf worker", apiKey, baseURL, "")
		auth := &coreauth.Auth{ID: id, Provider: provider}
		index := auth.EnsureIndex()
		if _, err := manager.Register(context.Background(), auth); err != nil {
			t.Fatalf("register auth: %v", err)
		}
		return index
	}

	indexA := register("https://cf-a.example/v1", "key-a")
	indexB := register("https://cf-b.example/v1", "key-b")
	h := NewHandlerWithoutConfigFilePath(&config.Config{
		AuthDir: t.TempDir(),
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "cf worker",
				BaseURL: "https://cf-a.example/v1",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{
					APIKey: "key-a",
				}},
				Models: []config.OpenAICompatibilityModel{{Name: upstream, Alias: aliasA}},
			},
			{
				Name:    "cf worker",
				BaseURL: "https://cf-b.example/v1",
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{
					APIKey: "key-b",
				}},
				Models: []config.OpenAICompatibilityModel{{Name: upstream, Alias: aliasB}},
			},
		},
	}, manager)
	h.SetUsageLedger(store)
	router := usageManagementTestRouter(h)

	if err := store.UpsertModelPrice(context.Background(), usageledger.ModelPrice{Model: aliasA, InputPer1M: 10, OutputPer1M: 20, Source: "test"}); err != nil {
		t.Fatalf("upsert first alias price: %v", err)
	}
	if err := store.UpsertModelPrice(context.Background(), usageledger.ModelPrice{Model: aliasB, InputPer1M: 20, OutputPer1M: 20, Source: "test"}); err != nil {
		t.Fatalf("upsert second alias price: %v", err)
	}

	now := time.Date(2026, 7, 10, 2, 0, 0, 0, time.UTC)
	for _, event := range []usageledger.Event{
		{
			RequestID: "req-auth-a",
			Timestamp: now,
			Provider:  provider,
			Model:     upstream,
			AuthIndex: indexA,
			Tokens:    usageledger.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
		{
			RequestID: "req-auth-b",
			Timestamp: now.Add(time.Second),
			Provider:  provider,
			Model:     upstream,
			AuthIndex: indexB,
			Tokens:    usageledger.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
		{
			RequestID: "req-no-auth-index",
			Timestamp: now.Add(2 * time.Second),
			Provider:  provider,
			Model:     upstream,
			Tokens:    usageledger.TokenUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		},
	} {
		if _, err := store.InsertEvent(context.Background(), event); err != nil {
			t.Fatalf("insert event %s: %v", event.RequestID, err)
		}
	}

	rec := performUsageManagementJSON(http.MethodPost, "/v0/management/usage-analytics", map[string]any{
		"from_ms": now.Add(-time.Minute).UnixMilli(),
		"to_ms":   now.Add(time.Minute).UnixMilli(),
		"include": map[string]any{
			"events_page": map[string]any{"limit": 5},
		},
	}, router)
	if rec.Code != http.StatusOK {
		t.Fatalf("analytics status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response usageledger.AnalyticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode analytics: %v", err)
	}
	if response.Events == nil || len(response.Events.Items) != 3 {
		t.Fatalf("events = %#v", response.Events)
	}
	byRequestID := make(map[string]usageledger.AnalyticsEventRow, len(response.Events.Items))
	for _, event := range response.Events.Items {
		byRequestID[event.RequestID] = event
	}
	assertEvent := func(requestID, model, upstreamModel string, cost *float64) {
		t.Helper()
		event, ok := byRequestID[requestID]
		if !ok || event.Model != model || event.UpstreamModel != upstreamModel {
			t.Fatalf("event %s = %#v", requestID, event)
		}
		if cost == nil {
			if event.EstimatedCostUSD != nil {
				t.Fatalf("event %s cost = %v, want nil", requestID, *event.EstimatedCostUSD)
			}
			return
		}
		if event.EstimatedCostUSD == nil || math.Abs(*event.EstimatedCostUSD-*cost) > 0.000000001 {
			t.Fatalf("event %s cost = %#v, want %v", requestID, event.EstimatedCostUSD, *cost)
		}
	}
	costA, costB := 0.002, 0.003
	assertEvent("req-auth-a", aliasA, upstream, &costA)
	assertEvent("req-auth-b", aliasB, upstream, &costB)
	assertEvent("req-no-auth-index", upstream, "", nil)
}

func TestUsageAnalyticsEndpointEnrichesAPIKeyPreviewFromRuntimeAuth(t *testing.T) {
	store := openManagementUsageStore(t)
	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{
		ID:       "compat-auth",
		Provider: "openai-compatible-opencode-go",
		Attributes: map[string]string{
			coreauth.AttributeAuthKind: "apikey",
			coreauth.AttributeAPIKey:   "sk-api-key-abcdwxyz",
			"usage_source":             "opencode-go:acc-a",
			"base_url":                 "https://opencode.ai/zen/go/v1",
			"compat_name":              "opencode-go",
		},
	}
	authIndex := auth.EnsureIndex()
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)
	h.SetUsageLedger(store)
	router := usageManagementTestRouter(h)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	if _, err := store.InsertEvent(context.Background(), usageledger.Event{
		RequestID: "req-compat-key",
		Timestamp: now,
		Provider:  "openai-compatible-opencode-go",
		Model:     "opencode-gpt-5",
		AuthIndex: authIndex,
		AuthType:  "apikey",
		Tokens:    usageledger.TokenUsage{TotalTokens: 30},
	}); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	body := map[string]any{
		"from_ms": now.Add(-time.Minute).UnixMilli(),
		"to_ms":   now.Add(time.Minute).UnixMilli(),
		"include": map[string]any{
			"api_key_stats": true,
		},
	}
	rec := performUsageManagementJSON(http.MethodPost, "/v0/management/usage-analytics", body, router)
	if rec.Code != http.StatusOK {
		t.Fatalf("analytics status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response usageledger.AnalyticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode analytics: %v", err)
	}
	if len(response.APIKeyStats) != 1 {
		t.Fatalf("api key stats = %#v", response.APIKeyStats)
	}
	if response.APIKeyStats[0].APIKeyPreview != "sk-a***wxyz" {
		t.Fatalf("api key preview = %q", response.APIKeyStats[0].APIKeyPreview)
	}
}

func TestUsageAnalyticsEndpointEnrichesCredentialDisplayNameFromRuntimeAuth(t *testing.T) {
	store := openManagementUsageStore(t)
	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{
		ID:       "codex-auth",
		Provider: "codex",
		FileName: "codex-hidden.json",
		Attributes: map[string]string{
			coreauth.AttributeAuthKind: coreauth.AuthKindOAuth,
		},
		Metadata: map[string]any{
			"email": "codex-user@example.com",
		},
	}
	authIndex := auth.EnsureIndex()
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)
	h.SetUsageLedger(store)
	router := usageManagementTestRouter(h)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	if _, err := store.InsertEvent(context.Background(), usageledger.Event{
		RequestID: "req-codex-oauth",
		Timestamp: now,
		Provider:  "codex",
		Model:     "gpt-5.3-codex-spark",
		AuthIndex: authIndex,
		AuthType:  "oauth",
		Tokens:    usageledger.TokenUsage{TotalTokens: 30},
	}); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	body := map[string]any{
		"from_ms": now.Add(-time.Minute).UnixMilli(),
		"to_ms":   now.Add(time.Minute).UnixMilli(),
		"include": map[string]any{
			"credential_stats": true,
			"events_page": map[string]any{
				"limit": 5,
			},
		},
	}
	rec := performUsageManagementJSON(http.MethodPost, "/v0/management/usage-analytics", body, router)
	if rec.Code != http.StatusOK {
		t.Fatalf("analytics status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response usageledger.AnalyticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode analytics: %v", err)
	}
	if len(response.CredentialStats) != 1 {
		t.Fatalf("credential stats = %#v", response.CredentialStats)
	}
	if response.CredentialStats[0].CredentialDisplayName != "codex-user@example.com" {
		t.Fatalf("credential display name = %q", response.CredentialStats[0].CredentialDisplayName)
	}
	if response.Events == nil || len(response.Events.Items) != 1 {
		t.Fatalf("events = %#v", response.Events)
	}
	if response.Events.Items[0].CredentialDisplayName != "codex-user@example.com" {
		t.Fatalf("event credential display name = %q", response.Events.Items[0].CredentialDisplayName)
	}
}

func TestUsageAnalyticsEndpointReclassifiesLegacyAPIKeyCredentialStats(t *testing.T) {
	store := openManagementUsageStore(t)
	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{
		ID:       "codex-api-key-auth",
		Provider: "codex",
		Label:    "codex-apikey",
		Attributes: map[string]string{
			coreauth.AttributeAuthKind: "apikey",
			coreauth.AttributeAPIKey:   "sk-codex-key-abcdwxyz",
		},
	}
	authIndex := auth.EnsureIndex()
	keyHash := usageledger.HashAPIKey("sk-codex-key-abcdwxyz")
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, manager)
	h.SetUsageLedger(store)
	router := usageManagementTestRouter(h)

	now := time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC)
	if _, err := store.InsertEvent(context.Background(), usageledger.Event{
		RequestID: "req-codex-key-legacy",
		Timestamp: now,
		Provider:  "codex",
		Model:     "gpt-5.5",
		AuthIndex: authIndex,
		Tokens:    usageledger.TokenUsage{TotalTokens: 30},
	}); err != nil {
		t.Fatalf("insert legacy event: %v", err)
	}
	if _, err := store.InsertEvent(context.Background(), usageledger.Event{
		RequestID:         "req-codex-key-current",
		Timestamp:         now.Add(time.Second),
		Provider:          "codex",
		Model:             "gpt-5.5",
		AuthIndex:         authIndex,
		AuthType:          "apikey",
		CredentialKeyHash: keyHash,
		Tokens:            usageledger.TokenUsage{TotalTokens: 10},
	}); err != nil {
		t.Fatalf("insert current event: %v", err)
	}

	body := map[string]any{
		"from_ms": now.Add(-time.Minute).UnixMilli(),
		"to_ms":   now.Add(time.Minute).UnixMilli(),
		"include": map[string]any{
			"api_key_stats":    true,
			"credential_stats": true,
			"events_page": map[string]any{
				"limit": 5,
			},
		},
	}
	rec := performUsageManagementJSON(http.MethodPost, "/v0/management/usage-analytics", body, router)
	if rec.Code != http.StatusOK {
		t.Fatalf("analytics status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	var response usageledger.AnalyticsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode analytics: %v", err)
	}
	if len(response.CredentialStats) != 0 {
		t.Fatalf("credential stats = %#v", response.CredentialStats)
	}
	if len(response.APIKeyStats) != 1 {
		t.Fatalf("api key stats = %#v", response.APIKeyStats)
	}
	stat := response.APIKeyStats[0]
	if stat.APIKeyHash != keyHash || stat.Calls != 2 || stat.TotalTokens != 40 {
		t.Fatalf("api key stat = %#v", stat)
	}
	if stat.APIKeyPreview != "sk-c***wxyz" {
		t.Fatalf("api key preview = %q", stat.APIKeyPreview)
	}
	if response.Events == nil || len(response.Events.Items) != 2 {
		t.Fatalf("events = %#v", response.Events)
	}
	for _, event := range response.Events.Items {
		if event.CredentialDisplayName != "codex-sk-c***wxyz" {
			t.Fatalf("event credential display name = %q", event.CredentialDisplayName)
		}
	}
}
