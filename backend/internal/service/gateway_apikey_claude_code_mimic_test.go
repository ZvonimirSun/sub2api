//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	require.False(t, newAPIKeyCCMimicAccount(nil).ShouldMimicClaudeCode())
	require.True(t, newAPIKeyCCMimicAccount(map[string]any{"anthropic_apikey_claude_code_mimic": true}).IsAnthropicAPIKeyClaudeCodeMimicEnabled())
	require.True(t, newAPIKeyCCMimicAccount(map[string]any{"anthropic_apikey_claude_code_mimic": true}).ShouldMimicClaudeCode())

	// 自动透传开启时保持旧逻辑：模拟开关不生效
	require.False(t, newAPIKeyCCMimicAccount(map[string]any{
		"anthropic_apikey_claude_code_mimic": true,
		"anthropic_passthrough":              true,
	}).IsAnthropicAPIKeyClaudeCodeMimicEnabled())

	oauth := newAPIKeyCCMimicAccount(map[string]any{"anthropic_apikey_claude_code_mimic": true})
	oauth.Type = AccountTypeOAuth
	require.False(t, oauth.IsAnthropicAPIKeyClaudeCodeMimicEnabled())
	require.True(t, oauth.ShouldMimicClaudeCode())
}

func TestBuildUpstreamRequest_APIKeyClaudeCodeMimic_ReusesOAuthMimicBranch(t *testing.T) {
	c := newAPIKeyCCMimicContext("magpie/1.0")
	c.Request.Header.Set("Anthropic-Beta", "structured-outputs-2025-11-13,custom-unknown-beta")
	c.Request.Header.Set("Accept-Language", "zh-CN")
	account := newAPIKeyCCMimicAccount(map[string]any{"anthropic_apikey_claude_code_mimic": true})
	rawBody := []byte(`{"model":"claude-sonnet-4-6","system":"You are Codex, an agent.","messages":[{"role":"user","content":"hello"}],"stream":true}`)

	svc := &GatewayService{cfg: &config.Config{}}
	body := svc.applyClaudeCodeOAuthMimicryToBody(context.Background(), c, account, rawBody, "You are Codex, an agent.", "claude-sonnet-4-6")
	req, _, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "sk-ant-xxx", "apikey", "claude-sonnet-4-6", true, true)
	require.NoError(t, err)

	// headers：复用 OAuth mimic 分支，强制 CLI 头且跳过客户端白名单头透传，但认证仍走 API Key
	require.Equal(t, claude.DefaultUserAgent(), getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, "cli", getHeaderRaw(req.Header, "x-app"))
	require.Equal(t, "stream", getHeaderRaw(req.Header, "x-stainless-helper-method"))
	require.NotEmpty(t, getHeaderRaw(req.Header, "x-client-request-id"))
	require.Empty(t, getHeaderRaw(req.Header, "accept-language"))
	require.Empty(t, getHeaderRaw(req.Header, "authorization"))
	require.Equal(t, "sk-ant-xxx", getHeaderRaw(req.Header, "x-api-key"))

	beta := getHeaderRaw(req.Header, "anthropic-beta")
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaClaudeCode))
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaOAuth))
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaContextManagement))
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaStructuredOutputs))
	require.False(t, anthropicBetaTokensContains(beta, "custom-unknown-beta"))

	// body：system 被重写为 3-block Claude Code 结构，原 Codex system 迁移到 messages 首部
	out := readUpstreamBodyForTest(t, req)
	system := gjson.GetBytes(out, "system").Array()
	require.Len(t, system, 3)
	require.True(t, strings.HasPrefix(system[0].Get("text").String(), "x-anthropic-billing-header: cc_version="))
	require.Equal(t, claudeCodeSystemPrompt, system[1].Get("text").String())
	require.True(t, system[2].Get("cache_control").Exists())

	msgs := gjson.GetBytes(out, "messages").Array()
	require.Len(t, msgs, 3)
	require.Equal(t, "[System Instructions]\nYou are Codex, an agent.", msgs[0].Get("content.0.text").String())
	require.Equal(t, "assistant", msgs[1].Get("role").String())
	require.Equal(t, "hello", msgs[2].Get("content").String())
	require.True(t, gjson.GetBytes(out, "tools").IsArray())
}

func TestBuildCountTokensRequest_APIKeyClaudeCodeMimic_ReusesOAuthMimicBranch(t *testing.T) {
	c := newAPIKeyCCMimicContext("curl/8.0")
	account := newAPIKeyCCMimicAccount(map[string]any{"anthropic_apikey_claude_code_mimic": true})
	body := []byte(`{"model":"claude-sonnet-4-6","system":"client system","messages":[{"role":"user","content":"hello"}]}`)

	svc := &GatewayService{cfg: &config.Config{}}
	req, _, err := svc.buildCountTokensRequest(context.Background(), c, account, body, "sk-ant-xxx", "apikey", "claude-sonnet-4-6", true)
	require.NoError(t, err)

	require.Equal(t, claude.DefaultUserAgent(), getHeaderRaw(req.Header, "User-Agent"))
	require.Equal(t, "cli", getHeaderRaw(req.Header, "x-app"))
	beta := getHeaderRaw(req.Header, "anthropic-beta")
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaClaudeCode))
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaOAuth))
	require.True(t, anthropicBetaTokensContains(beta, claude.BetaTokenCounting))
}
