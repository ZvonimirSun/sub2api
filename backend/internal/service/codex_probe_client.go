package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/proxy"
)

const (
	DefaultCodexProbeTimeout = 25 * time.Second
	DefaultCodexVersion      = "0.154.0"
)

// CodexProbeResult 包含一次探针请求的执行结果。
type CodexProbeResult struct {
	TurnState         string `json:"turn_state"`
	CflbCookie        string `json:"cflb_cookie,omitempty"`
	OailbCookie       string `json:"oailb_cookie,omitempty"`
	ResponseModel     string `json:"response_model"`
	StatusCode        int    `json:"status_code"`
	LatencyMs         int64  `json:"latency_ms"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
	ErrorMessage      string `json:"error_message,omitempty"`
	ProxyUsed         string `json:"proxy_used,omitempty"`
}

// CodexProbeClient 负责构造与发送针对 ChatGPT Codex 的 HTTP/2 极速截断探针请求。
type CodexProbeClient struct{}

func NewCodexProbeClient() *CodexProbeClient {
	return &CodexProbeClient{}
}

// Probe 发送一次 codex-tui 规范的探测请求。
func (c *CodexProbeClient) Probe(ctx context.Context, account *Account, model string, proxyURL string) (*CodexProbeResult, error) {
	if account == nil {
		return nil, fmt.Errorf("account is nil")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is empty")
	}

	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token == "" {
		token = strings.TrimSpace(account.GetCredential("token"))
	}
	if token == "" {
		return nil, fmt.Errorf("account has no oauth access token")
	}

	chatGPTAccountID := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	baseURL := account.GetOpenAIOAuthBaseURL()
	targetURL := strings.TrimRight(baseURL, "/") + "/responses"

	// 准备指纹与客户端版本
	version := DefaultCodexVersion
	if v := strings.TrimSpace(account.GetExtraString("codex_client_version")); v != "" {
		version = v
	}

	var installID string
	if seed, ok := codexFingerprintSeed(account.Extra); ok {
		installID = resolveConvergedInstallationID(account, seed)
	} else if devID := account.GetOpenAIDeviceID(); devID != "" {
		installID = devID
	} else {
		installID = uuid.NewString()
	}

	// 探针载荷（与 codex-tui 完全一致）
	payload := map[string]any{
		"model": model,
		"input": []map[string]any{
			{
				"role": "user",
				"content": []map[string]any{
					{"type": "input_text", "text": "hi"},
				},
			},
		},
		"stream":       true,
		"store":        false,
		"instructions": "You are a coding assistant.",
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal probe payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create probe request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("OpenAI-Beta", "responses=experimental")
	if chatGPTAccountID != "" {
		req.Header.Set("ChatGPT-Account-ID", chatGPTAccountID)
	}
	req.Header.Set("Originator", "codex-tui")
	req.Header.Set("User-Agent", fmt.Sprintf("codex-tui/%s (Ubuntu 22.4.0; x86_64) xterm-256color", version))
	req.Header.Set("Version", version)
	req.Header.Set("X-Codex-Installation-ID", installID)

	// 仅移除上游负载均衡 Cookie (__cflb 与 __oailb)，保留其他可能存在的 Cookie
	stripProbeLBCookies(req)

	if !account.IsCustomOpenAIOAuthBaseURL() {
		req.Host = "chatgpt.com"
	}

	httpClient, maskedProxy, err := c.resolveHTTPClient(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("resolve probe client: %w", err)
	}

	start := time.Now()
	resp, err := httpClient.Do(req)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return &CodexProbeResult{
			StatusCode:   0,
			LatencyMs:    latency,
			ErrorMessage: err.Error(),
			ProxyUsed:    maskedProxy,
		}, err
	}
	// 关键性能优化：读取完 Header 立即关闭 Body 截断上游 SSE 生成流
	defer func() { _ = resp.Body.Close() }()

	// 提取上游响应的负载均衡 Cookie (__cflb 与 __oailb)
	var cflb, oailb string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "__cflb" && cookie.Value != "" {
			cflb = cookie.Value
		} else if cookie.Name == "__oailb" && cookie.Value != "" {
			oailb = cookie.Value
		}
	}
	if cflb == "" || oailb == "" {
		for _, raw := range resp.Header["Set-Cookie"] {
			parts := strings.Split(raw, ";")
			if len(parts) > 0 {
				pair := strings.TrimSpace(parts[0])
				if eq := strings.IndexByte(pair, '='); eq > 0 {
					name := strings.TrimSpace(pair[:eq])
					val := strings.TrimSpace(pair[eq+1:])
					if name == "__cflb" && cflb == "" {
						cflb = val
					} else if name == "__oailb" && oailb == "" {
						oailb = val
					}
				}
			}
		}
	}

	result := &CodexProbeResult{
		StatusCode:    resp.StatusCode,
		LatencyMs:     latency,
		TurnState:     strings.TrimSpace(resp.Header.Get("x-codex-turn-state")),
		CflbCookie:    cflb,
		OailbCookie:   oailb,
		ResponseModel: strings.TrimSpace(resp.Header.Get("openai-model")),
		ProxyUsed:     maskedProxy,
	}

	if resp.StatusCode >= 400 {
		if retryAfterStr := resp.Header.Get("Retry-After"); retryAfterStr != "" {
			if sec, parseErr := strconv.Atoi(strings.TrimSpace(retryAfterStr)); parseErr == nil && sec > 0 {
				result.RetryAfterSeconds = sec
			}
		}
		// 读取最多 2KB 错误响应以供诊断
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		result.ErrorMessage = strings.TrimSpace(string(errBody))
		if result.ErrorMessage == "" {
			result.ErrorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}

	return result, nil
}

func (c *CodexProbeClient) resolveHTTPClient(proxyURL string) (*http.Client, string, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return nil, "", fmt.Errorf("probe proxy is required")
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil || parsed.Host == "" {
		return nil, "", fmt.Errorf("invalid probe proxy %q: host cannot be empty", proxyURL)
	}

	masked := maskProxyURL(parsed)

	transport := &http.Transport{
		ForceAttemptHTTP2:     true,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}

	switch strings.ToLower(parsed.Scheme) {
	case "socks5", "socks5h":
		var auth *proxy.Auth
		if parsed.User != nil {
			auth = &proxy.Auth{
				User:     parsed.User.Username(),
				Password: "",
			}
			if pass, ok := parsed.User.Password(); ok {
				auth.Password = pass
			}
		}
		dialer, dialErr := proxy.SOCKS5("tcp", parsed.Host, auth, proxy.Direct)
		if dialErr != nil {
			return nil, "", fmt.Errorf("create socks5 dialer: %w", dialErr)
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	default:
		transport.Proxy = http.ProxyURL(parsed)
	}

	return &http.Client{
		Timeout:   DefaultCodexProbeTimeout,
		Transport: transport,
	}, masked, nil
}

func maskProxyURL(u *url.URL) string {
	if u == nil {
		return "direct"
	}
	if u.User != nil {
		return fmt.Sprintf("%s://***@%s", u.Scheme, u.Host)
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

// stripProbeLBCookies 仅从请求头 Cookie 中剥离 __cflb 与 __oailb 负载均衡 Cookie，保留其他可能存在的 Cookie。
func stripProbeLBCookies(req *http.Request) {
	if req == nil {
		return
	}
	raw := req.Header.Get("Cookie")
	if raw == "" {
		return
	}
	parts := strings.Split(raw, ";")
	var kept []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		eq := strings.IndexByte(trimmed, '=')
		if eq > 0 {
			name := strings.TrimSpace(trimmed[:eq])
			if name == "__cflb" || name == "__oailb" {
				continue
			}
		}
		kept = append(kept, trimmed)
	}
	if len(kept) == 0 {
		req.Header.Del("Cookie")
	} else {
		req.Header.Set("Cookie", strings.Join(kept, "; "))
	}
}
