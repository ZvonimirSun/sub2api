//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newAPIKeyCCMimicAccount(extra map[string]any) *Account {
	return &Account{
		ID: 9101, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "sk-ant-xxx"},
		Extra:       extra,
		Status:      StatusActive, Schedulable: true,
	}
}

func newAPIKeyCCMimicContext(userAgent string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	if userAgent != "" {
		c.Request.Header.Set("User-Agent", userAgent)
	}
	return c
}

func TestIsAnthropicAPIKeyClaudeCodeMimicEnabled(t *testing.T) {
	require.False(t, newAPIKeyCCMimicAccount(nil).IsAnthropicAPIKeyClaudeCodeMimicEnabled())
	require.True(t, newAPIKeyCCMimicAccount(map[string]any{extraKeyAnthropicAPIKeyClaudeCodeMimic: true}).IsAnthropicAPIKeyClaudeCodeMimicEnabled())

	// 自动透传开启时保持旧逻辑：模拟开关不生效
	require.False(t, newAPIKeyCCMimicAccount(map[string]any{
		extraKeyAnthropicAPIKeyClaudeCodeMimic: true,
		"anthropic_passthrough":                true,
	}).IsAnthropicAPIKeyClaudeCodeMimicEnabled())

	oauth := newAPIKeyCCMimicAccount(map[string]any{extraKeyAnthropicAPIKeyClaudeCodeMimic: true})
	oauth.Type = AccountTypeOAuth
	require.False(t, oauth.IsAnthropicAPIKeyClaudeCodeMimicEnabled())
}

func TestBuildUpstreamRequest_APIKeyClaudeCodeMimic_AppliesClientFeatures(t *testing.T) {
	c := newAPIKeyCCMimicContext("curl/8.0")
	c.Request.Header.Set("Anthropic-Beta", "context-management-2025-06-27,oauth-2025-04-20")
	account := newAPIKeyCCMimicAccount(map[string]any{extraKeyAnthropicAPIKeyClaudeCodeMimic: true})
	body := []byte(`{"model":"claude-sonnet-4-6","system":"client system","messages":[{"role":"user","content":"hello"}],"stream":true}`)

	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "sk-ant-xxx", "apikey", "claude-sonnet-4-6", true, false)
	require.NoError(t, err)

	// headers
	require.Equal(t, claude.DefaultUserAgent(), getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, "cli", getHeaderRaw(req.Header, "x-app"))
	require.Equal(t, "stream", getHeaderRaw(req.Header, "x-stainless-helper-method"))
	require.NotEmpty(t, getHeaderRaw(req.Header, "x-client-request-id"))
	require.Empty(t, getHeaderRaw(req.Header, "authorization"), "API Key 账号不应使用 Bearer")

	beta := getHeaderRaw(req.Header, "anthropic-beta")
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaClaudeCode))
	require.True(t, anthropicBetaTokensContains(beta, "context-management-2025-06-27"), "保留客户端 beta")
	require.False(t, anthropicBetaTokensContains(beta, claude.BetaOAuth), "API Key 不能携带 oauth beta")

	// body
	out := readUpstreamBodyForTest(t, req)
	system := gjson.GetBytes(out, "system").Array()
	require.Len(t, system, 2)
	require.Equal(t, claudeCodeSystemPrompt, system[0].Get("text").String())
	require.False(t, system[0].Get("cache_control").Exists())
	require.Equal(t, "client system", system[1].Get("text").String(), "客户端 system 原样保留")

	uid := gjson.GetBytes(out, "metadata.user_id").String()
	parsed := ParseMetadataUserID(uid)
	require.NotNil(t, parsed)
	require.Equal(t, parsed.SessionID, getHeaderRaw(req.Header, "X-Claude-Code-Session-Id"))
	require.NotContains(t, string(out), "x-anthropic-billing-header:", "不注入 OAuth 的 billing block")
}

func TestBuildUpstreamRequest_APIKeyClaudeCodeMimic_StableSessionAcrossTurns(t *testing.T) {
	account := newAPIKeyCCMimicAccount(map[string]any{extraKeyAnthropicAPIKeyClaudeCodeMimic: true})
	svc := &GatewayService{cfg: &config.Config{}}

	build := func(body string) string {
		req, _, err := svc.buildUpstreamRequest(context.Background(), newAPIKeyCCMimicContext("curl/8.0"), account, []byte(body), "sk-ant-xxx", "apikey", "claude-sonnet-4-6", false, false)
		require.NoError(t, err)
		return gjson.GetBytes(readUpstreamBodyForTest(t, req), "metadata.user_id").String()
	}
	first := build(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello"}]}`)
	second := build(`{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"},{"role":"user","content":"more"}]}`)
	require.Equal(t, first, second)
}

func TestBuildUpstreamRequest_APIKeyClaudeCodeMimic_RealClaudeCodeClientUntouched(t *testing.T) {
	c := newAPIKeyCCMimicContext("claude-cli/2.1.200 (external, cli)")
	account := newAPIKeyCCMimicAccount(map[string]any{extraKeyAnthropicAPIKeyClaudeCodeMimic: true})
	uid := FormatMetadataUserID("a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90", "", "11111111-2222-4333-8444-555555555555", "2.1.200")
	body := []byte(`{"model":"claude-sonnet-4-6","system":"client system","metadata":{"user_id":` + jsonQuote(uid) + `},"messages":[]}`)

	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "sk-ant-xxx", "apikey", "claude-sonnet-4-6", false, false)
	require.NoError(t, err)

	out := readUpstreamBodyForTest(t, req)
	require.Equal(t, "client system", gjson.GetBytes(out, "system").String())
	require.Equal(t, uid, gjson.GetBytes(out, "metadata.user_id").String())
	require.Equal(t, "claude-cli/2.1.200 (external, cli)", getHeaderRaw(req.Header, "User-Agent"))
}

func TestBuildUpstreamRequest_APIKeyClaudeCodeMimic_DisabledKeepsLegacyBehavior(t *testing.T) {
	c := newAPIKeyCCMimicContext("curl/8.0")
	body := []byte(`{"model":"claude-sonnet-4-6","system":"client system","messages":[]}`)
	svc := &GatewayService{cfg: &config.Config{}}

	for name, extra := range map[string]map[string]any{
		"off":         nil,
		"passthrough": {extraKeyAnthropicAPIKeyClaudeCodeMimic: true, "anthropic_passthrough": true},
	} {
		t.Run(name, func(t *testing.T) {
			req, _, err := svc.buildUpstreamRequest(context.Background(), c, newAPIKeyCCMimicAccount(extra), body, "sk-ant-xxx", "apikey", "claude-sonnet-4-6", false, false)
			require.NoError(t, err)
			out := readUpstreamBodyForTest(t, req)
			require.Equal(t, "client system", gjson.GetBytes(out, "system").String())
			require.False(t, gjson.GetBytes(out, "metadata.user_id").Exists())
			require.Equal(t, "curl/8.0", getHeaderRaw(req.Header, "User-Agent"))
			require.Empty(t, getHeaderRaw(req.Header, "anthropic-beta"))
		})
	}
}

func TestPrependClaudeCodeSystemBlockForAPIKey_Idempotent(t *testing.T) {
	body := []byte(`{"system":[{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."},{"type":"text","text":"x"}]}`)
	require.Equal(t, string(body), string(prependClaudeCodeSystemBlockForAPIKey(body)))

	noSystem := prependClaudeCodeSystemBlockForAPIKey([]byte(`{"messages":[]}`))
	require.Equal(t, claudeCodeSystemPrompt, gjson.GetBytes(noSystem, "system.0.text").String())
	require.Len(t, gjson.GetBytes(noSystem, "system").Array(), 1)
}

func TestBuildCountTokensRequest_APIKeyClaudeCodeMimic_AppliesHeadersAndSystemWithoutMetadata(t *testing.T) {
	c := newAPIKeyCCMimicContext("curl/8.0")
	account := newAPIKeyCCMimicAccount(map[string]any{extraKeyAnthropicAPIKeyClaudeCodeMimic: true})
	body := []byte(`{"model":"claude-sonnet-4-6","system":"client system","messages":[{"role":"user","content":"hello"}]}`)

	svc := &GatewayService{cfg: &config.Config{}}
	req, outBody, err := svc.buildCountTokensRequest(context.Background(), c, account, body, "sk-ant-xxx", "apikey", "claude-sonnet-4-6", false)
	require.NoError(t, err)

	require.Equal(t, claude.DefaultUserAgent(), getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, "cli", getHeaderRaw(req.Header, "x-app"))
	beta := getHeaderRaw(req.Header, "anthropic-beta")
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaClaudeCode))
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaTokenCounting))
	require.False(t, anthropicBetaTokensContains(beta, claude.BetaOAuth))

	system := gjson.GetBytes(outBody, "system").Array()
	require.Len(t, system, 2)
	require.Equal(t, claudeCodeSystemPrompt, system[0].Get("text").String())
	require.Equal(t, "client system", system[1].Get("text").String())
	require.False(t, gjson.GetBytes(outBody, "metadata").Exists(), "count_tokens 不注入 metadata")
}

func jsonQuote(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b = append(b, '\\')
		}
		b = append(b, string(r)...)
	}
	b = append(b, '"')
	return string(b)
}
