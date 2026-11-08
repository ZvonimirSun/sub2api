package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Anthropic API Key 账号「模拟 Claude Code 客户端」。
//
// 背景：部分上游（中转/自建网关）只放行 Claude Code 客户端流量。管理后台「测试连接」
// 始终以 Claude Code 客户端特征发起请求，因此能通过；而真实转发对 API Key 账号只做
// 白名单头透传，非 Claude Code 客户端（curl / SDK / 第三方工具）会被上游拦截。
//
// 本功能只模拟「Claude Code 客户端」本身，与 OAuth 模拟刻意解耦，不复用 OAuth 专属逻辑：
//   - 不加 oauth-2025-04-20 beta；不使用 OAuth 指纹缓存 / account_uuid / 会话 ID 伪装；
//   - 不迁移客户端 system 到 messages、不做工具名混淆、不改写 messages 缓存断点、
//     不注入 billing attribution block。
//
// 具体行为（与「测试连接」请求特征对齐）：
//   - 请求头：强制 Claude CLI 的 User-Agent / X-Stainless-* / X-App 等；
//   - anthropic-beta：APIKeyBetaHeader（含 claude-code-20250219，不含 oauth）合并客户端 beta；
//   - system：前置 Claude Code 标识块（不带 cache_control，不改动客户端原 system 内容）；
//   - metadata.user_id：缺失或格式非法时补一个 Claude Code 格式的值（会话级稳定）。
//
// 生效条件：Anthropic + API Key 账号 + extra.anthropic_apikey_claude_code_mimic=true，
// 且未开启自动透传（anthropic_passthrough），且请求不是来自真实 Claude Code 客户端
// （真实客户端自带完整特征，保持原透传行为）。
const extraKeyAnthropicAPIKeyClaudeCodeMimic = "anthropic_apikey_claude_code_mimic"

// IsAnthropicAPIKeyClaudeCodeMimicEnabled 返回 Anthropic API Key 账号是否启用「模拟 Claude Code 客户端」。
// 字段：accounts.extra.anthropic_apikey_claude_code_mimic。
// 自动透传（anthropic_passthrough）开启时恒为 false：透传路径保持旧逻辑不变。
func (a *Account) IsAnthropicAPIKeyClaudeCodeMimicEnabled() bool {
	if a == nil || a.Platform != PlatformAnthropic || a.Type != AccountTypeAPIKey || a.Extra == nil {
		return false
	}
	if a.IsAnthropicAPIKeyPassthroughEnabled() || a.IsBedrock() {
		return false
	}
	enabled, ok := a.Extra[extraKeyAnthropicAPIKeyClaudeCodeMimic].(bool)
	return ok && enabled
}

// isRealClaudeCodeClientRequest 复用 Forward 中的真实 Claude Code 客户端判定：
// UA 为 claude-cli/* 且 metadata.user_id 格式合法；或携带 metadata.user_id 且
// system 中有 billing attribution block（经 new-api 等网关转发的真实 CC 流量）。
func isRealClaudeCodeClientRequest(ctx context.Context, c *gin.Context, body []byte) bool {
	if ctx != nil && IsClaudeCodeClient(ctx) {
		return true
	}
	userAgent := ""
	if c != nil && c.Request != nil {
		userAgent = c.GetHeader("User-Agent")
	}
	metadataUserID := gjson.GetBytes(body, "metadata.user_id").String()
	if isClaudeCodeClient(userAgent, metadataUserID) {
		return true
	}
	return metadataUserID != "" && systemHasBillingAttributionBlock(body)
}

// shouldMimicClaudeCodeForAPIKey 判断本次 API Key 请求是否需要模拟 Claude Code 客户端。
func shouldMimicClaudeCodeForAPIKey(ctx context.Context, c *gin.Context, account *Account, tokenType string, body []byte) bool {
	if tokenType == "oauth" || !account.IsAnthropicAPIKeyClaudeCodeMimicEnabled() {
		return false
	}
	return !isRealClaudeCodeClientRequest(ctx, c, body)
}

// applyAPIKeyClaudeCodeMimicBody 对请求体做 Claude Code 客户端特征补齐。
// withMetadata=false 用于 count_tokens（该端点不接受 metadata）。
func applyAPIKeyClaudeCodeMimicBody(body []byte, account *Account, c *gin.Context, mimicUserAgent string, withMetadata bool) []byte {
	if len(body) == 0 {
		return body
	}
	body = prependClaudeCodeSystemBlockForAPIKey(body)
	if withMetadata {
		body = ensureAPIKeyClaudeCodeMetadataUserID(body, account, c, mimicUserAgent)
	}
	return body
}

// prependClaudeCodeSystemBlockForAPIKey 在 system 最前面插入 Claude Code 标识块。
// 与 OAuth 路径不同：不迁移/改写客户端原 system 内容，也不给插入块加 cache_control，
// 避免占用客户端的缓存断点名额、改变客户端缓存前缀之外的结构。
func prependClaudeCodeSystemBlockForAPIKey(body []byte) []byte {
	system := gjson.GetBytes(body, "system")
	if system.Exists() && systemResultHasClaudeCodePrompt(system) {
		return body
	}

	ccBlock, err := marshalAnthropicSystemTextBlock(claudeCodeSystemPrompt, false)
	if err != nil {
		return body
	}

	items := [][]byte{ccBlock}
	switch {
	case !system.Exists() || system.Type == gjson.Null:
		// 仅 Claude Code 标识块
	case system.Type == gjson.String:
		if strings.TrimSpace(system.String()) != "" {
			textBlock, buildErr := marshalAnthropicSystemTextBlock(system.String(), false)
			if buildErr != nil {
				return body
			}
			items = append(items, textBlock)
		}
	case system.IsArray():
		for _, item := range system.Array() {
			items = append(items, []byte(item.Raw))
		}
	default:
		// 非法 system 类型：不改动，交给上游报错
		return body
	}

	out, ok := setJSONRawBytes(body, "system", buildJSONArrayRaw(items))
	if !ok {
		return body
	}
	return out
}

func systemResultHasClaudeCodePrompt(system gjson.Result) bool {
	if system.Type == gjson.String {
		return hasClaudeCodePrefix(system.String())
	}
	if !system.IsArray() {
		return false
	}
	found := false
	system.ForEach(func(_, item gjson.Result) bool {
		text := item.Get("text")
		if text.Type == gjson.String && hasClaudeCodePrefix(text.String()) {
			found = true
			return false
		}
		return true
	})
	return found
}

// ensureAPIKeyClaudeCodeMetadataUserID 在 metadata.user_id 缺失或格式非法时补一个
// Claude Code 格式的值。device_id 按账号派生（稳定），session_id 用会话级稳定种子派生
// （账号 + 客户端区分因子 + 首条 user 文本），与真实 CLI 进程级稳定的 session 近似。
// 格式（legacy / JSON）随模拟 UA 的版本号选择，保证与出站 User-Agent 一致。
func ensureAPIKeyClaudeCodeMetadataUserID(body []byte, account *Account, c *gin.Context, mimicUserAgent string) []byte {
	if account == nil {
		return body
	}
	if existing := gjson.GetBytes(body, "metadata.user_id").String(); existing != "" && ParseMetadataUserID(existing) != nil {
		return body
	}

	deviceSum := sha256.Sum256([]byte("sub2api:apikey-claude-code-device:" + strconv.FormatInt(account.ID, 10)))
	deviceID := hex.EncodeToString(deviceSum[:])

	discriminator := ""
	if c != nil && c.Request != nil {
		discriminator = c.ClientIP() + ":" + NormalizeSessionUserAgent(c.GetHeader("User-Agent"))
	}
	sessionID := generateSessionUUID(buildStableSessionSeed(account.ID, discriminator, extractFirstUserText(body)))

	userID := FormatMetadataUserID(deviceID, "", sessionID, ExtractCLIVersion(mimicUserAgent))
	out, err := sjson.SetBytes(body, "metadata.user_id", userID)
	if err != nil {
		return body
	}
	return out
}

// computeAPIKeyClaudeCodeMimicBeta 计算 API Key 模拟 Claude Code 时的 anthropic-beta：
// APIKeyBetaHeader（claude-code + interleaved-thinking + fine-grained-tool-streaming）
// 合并客户端 beta；始终剔除 oauth beta（API Key 不能携带），并应用 beta 策略过滤。
func computeAPIKeyClaudeCodeMimicBeta(clientHeaders http.Header, extraRequired []string, drop map[string]struct{}) string {
	required := append(strings.Split(claude.APIKeyBetaHeader, ","), extraRequired...)
	clientBeta := ""
	if clientHeaders != nil {
		clientBeta = getHeaderRaw(clientHeaders, "anthropic-beta")
	}
	finalDrop := make(map[string]struct{}, len(drop)+1)
	for k := range drop {
		finalDrop[k] = struct{}{}
	}
	finalDrop[claude.BetaOAuth] = struct{}{}
	return mergeAnthropicBetaDropping(required, clientBeta, finalDrop)
}

// applyAPIKeyClaudeCodeMimicHeaders 强制写入 Claude Code CLI 请求头，并同步
// X-Claude-Code-Session-Id 为 metadata.user_id 中的 session_id。
func applyAPIKeyClaudeCodeMimicHeaders(req *http.Request, body []byte, isStream bool, mimicUserAgent string) {
	if req == nil {
		return
	}
	applyClaudeCodeMimicHeaders(req, isStream, mimicUserAgent)
	if uid := gjson.GetBytes(body, "metadata.user_id").String(); uid != "" {
		if parsed := ParseMetadataUserID(uid); parsed != nil {
			setHeaderRaw(req.Header, "X-Claude-Code-Session-Id", parsed.SessionID)
		}
	}
}
