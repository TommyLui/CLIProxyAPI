package management

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
)

func performProviderNameRequest(
	t *testing.T,
	method string,
	target string,
	body string,
	handler func(*gin.Context),
) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	handler(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s status = %d, want %d; body=%s", method, target, rec.Code, http.StatusOK, rec.Body.String())
	}
	return rec
}

func TestProviderKeyNamesRoundTripThroughManagementLists(t *testing.T) {
	t.Parallel()

	h := &Handler{
		cfg:            &config.Config{},
		configFilePath: writeTestConfigFile(t),
	}

	performProviderNameRequest(
		t,
		http.MethodPut,
		"/v0/management/gemini-api-key",
		`[{"name":"Gemini 主账号","api-key":"gemini-key","base-url":"https://gemini.example"}]`,
		h.PutGeminiKeys,
	)
	if got := h.cfg.GeminiKey[0].Name; got != "Gemini 主账号" {
		t.Fatalf("gemini name = %q, want %q", got, "Gemini 主账号")
	}
	performProviderNameRequest(
		t,
		http.MethodPatch,
		"/v0/management/gemini-api-key",
		`{"index":0,"value":{"name":"Gemini 备用"}}`,
		h.PatchGeminiKey,
	)
	if got := h.cfg.GeminiKey[0].Name; got != "Gemini 备用" {
		t.Fatalf("patched gemini name = %q, want %q", got, "Gemini 备用")
	}

	performProviderNameRequest(
		t,
		http.MethodPut,
		"/v0/management/claude-api-key",
		`[{"name":"Claude 主账号","api-key":"claude-key","base-url":"https://claude.example"}]`,
		h.PutClaudeKeys,
	)
	if got := h.cfg.ClaudeKey[0].Name; got != "Claude 主账号" {
		t.Fatalf("claude name = %q, want %q", got, "Claude 主账号")
	}
	performProviderNameRequest(
		t,
		http.MethodPatch,
		"/v0/management/claude-api-key",
		`{"index":0,"value":{"name":"Claude 备用"}}`,
		h.PatchClaudeKey,
	)
	if got := h.cfg.ClaudeKey[0].Name; got != "Claude 备用" {
		t.Fatalf("patched claude name = %q, want %q", got, "Claude 备用")
	}

	performProviderNameRequest(
		t,
		http.MethodPut,
		"/v0/management/codex-api-key",
		`[{"name":"Codex 主账号","api-key":"codex-key","base-url":"https://codex.example/v1"}]`,
		h.PutCodexKeys,
	)
	if got := h.cfg.CodexKey[0].Name; got != "Codex 主账号" {
		t.Fatalf("codex name = %q, want %q", got, "Codex 主账号")
	}
	performProviderNameRequest(
		t,
		http.MethodPatch,
		"/v0/management/codex-api-key",
		`{"index":0,"value":{"name":"Codex 备用"}}`,
		h.PatchCodexKey,
	)
	if got := h.cfg.CodexKey[0].Name; got != "Codex 备用" {
		t.Fatalf("patched codex name = %q, want %q", got, "Codex 备用")
	}

	performProviderNameRequest(
		t,
		http.MethodPut,
		"/v0/management/vertex-api-key",
		`[{"name":"Vertex 主账号","api-key":"vertex-key","base-url":"https://vertex.example"}]`,
		h.PutVertexCompatKeys,
	)
	if got := h.cfg.VertexCompatAPIKey[0].Name; got != "Vertex 主账号" {
		t.Fatalf("vertex name = %q, want %q", got, "Vertex 主账号")
	}
	performProviderNameRequest(
		t,
		http.MethodPatch,
		"/v0/management/vertex-api-key",
		`{"index":0,"value":{"name":"Vertex 备用"}}`,
		h.PatchVertexCompatKey,
	)
	if got := h.cfg.VertexCompatAPIKey[0].Name; got != "Vertex 备用" {
		t.Fatalf("patched vertex name = %q, want %q", got, "Vertex 备用")
	}
}
