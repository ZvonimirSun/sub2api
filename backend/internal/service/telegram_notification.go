package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultTelegramAPIBaseURL = "https://api.telegram.org"

// telegramHTTPClient reuses the default transport connection pool (and allows
// tests to substitute http.DefaultTransport) while blocking redirects.
var telegramHTTPClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func normalizeTelegramAPIBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	trimmed = strings.TrimRight(trimmed, "/")
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("alert.telegram.api_base_url must be a valid https:// URL without query or fragment")
	}
	return trimmed, nil
}

// sendTelegramAlert delivers an ops alert through the Telegram Bot API.
// It intentionally uses the standard library so deployments do not need an
// additional Telegram SDK dependency.
func sendTelegramAlert(ctx context.Context, cfg OpsTelegramAlertConfig, rule *OpsAlertRule, event *OpsAlertEvent) error {
	if !cfg.Enabled {
		return nil
	}
	token := strings.TrimSpace(cfg.BotToken)
	chatID := strings.TrimSpace(cfg.ChatID)
	if token == "" || chatID == "" {
		return fmt.Errorf("telegram bot token and chat id are required")
	}
	if cfg.MessageThreadID < 0 {
		return fmt.Errorf("telegram message thread id must be >= 0")
	}
	for _, r := range token {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != ':' && r != '_' && r != '-' {
			return fmt.Errorf("invalid telegram bot token")
		}
	}
	baseURL, err := normalizeTelegramAPIBaseURL(cfg.APIBaseURL)
	if err != nil {
		return err
	}
	if baseURL == "" {
		baseURL = defaultTelegramAPIBaseURL
	}

	payload := map[string]any{
		"chat_id":    chatID,
		"text":       buildOpsAlertTelegramBody(rule, event),
		"parse_mode": "MarkdownV2",
	}
	if cfg.MessageThreadID > 0 {
		payload["message_thread_id"] = cfg.MessageThreadID
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	endpoint := baseURL + "/bot" + token + "/sendMessage"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("could not create telegram request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := telegramHTTPClient.Do(req)
	if err != nil {
		// net/http errors include the request URL, which contains the bot token.
		return fmt.Errorf("telegram request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return fmt.Errorf("telegram returned an invalid response (HTTP %d)", resp.StatusCode)
	}
	if !result.OK || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		desc := strings.TrimSpace(result.Description)
		if desc != "" && !strings.Contains(desc, token) {
			return fmt.Errorf("telegram rejected notification (HTTP %d): %s", resp.StatusCode, desc)
		}
		return fmt.Errorf("telegram rejected notification (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func buildOpsAlertTelegramBody(rule *OpsAlertRule, event *OpsAlertEvent) string {
	if rule == nil || event == nil {
		return "*运维告警*"
	}
	value, threshold := "-", fmt.Sprintf("%.2f", rule.Threshold)
	if event.MetricValue != nil {
		value = fmt.Sprintf("%.2f", *event.MetricValue)
	}
	if event.ThresholdValue != nil {
		threshold = fmt.Sprintf("%.2f", *event.ThresholdValue)
	}
	return fmt.Sprintf("*运维告警 %s*\n*规则：* %s\n*状态：* %s\n*指标：* %s %s %s\n*触发时间：* %s\n*描述：* %s",
		telegramMarkdownV2Escape(rule.Severity), telegramMarkdownV2Escape(rule.Name), telegramMarkdownV2Escape(event.Status),
		telegramMarkdownV2Escape(rule.MetricType), telegramMarkdownV2Escape(rule.Operator), telegramMarkdownV2Escape(fmt.Sprintf("%s (threshold %s)", value, threshold)),
		telegramMarkdownV2Escape(event.FiredAt.Format(time.RFC3339)), telegramMarkdownV2Escape(event.Description))
}

func telegramMarkdownV2Escape(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!', '\\':
			_ = b.WriteByte('\\')
		}
		_, _ = b.WriteRune(r)
	}
	return b.String()
}
