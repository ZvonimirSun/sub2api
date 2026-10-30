package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var telegramTopicMu sync.Mutex
var telegramTopicCache sync.Map // hashed chat/token key -> int64 thread id

var errTelegramTopicMissing = errors.New("telegram topic no longer exists")

// sendTelegramAlert delivers an ops alert through the Telegram Bot API.
// It intentionally uses the standard library so deployments do not need an
// additional Telegram SDK dependency.
func sendTelegramAlert(ctx context.Context, cfg OpsTelegramAlertConfig, rule *OpsAlertRule, event *OpsAlertEvent, repos ...SettingRepository) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.BotToken) == "" || strings.TrimSpace(cfg.ChatID) == "" {
		return fmt.Errorf("telegram bot token and chat id are required")
	}
	token := strings.TrimSpace(cfg.BotToken)
	var repo SettingRepository
	if len(repos) > 0 {
		repo = repos[0]
	}
	// Serialize topic lookup/creation; the evaluator already has a distributed leader lock.
	if strings.EqualFold(cfg.ChatMode, "topic") {
		telegramTopicMu.Lock()
		defer telegramTopicMu.Unlock()
	}
	payload := map[string]any{
		"chat_id":    strings.TrimSpace(cfg.ChatID),
		"text":       buildOpsAlertTelegramBody(rule, event),
		"parse_mode": "MarkdownV2",
	}
	if strings.EqualFold(cfg.ChatMode, "topic") {
		threadID, err := telegramTopicThread(ctx, token, cfg.ChatID, repo, false)
		if err != nil {
			return err
		}
		payload["message_thread_id"] = threadID
	}
	if err := telegramPost(ctx, token, "sendMessage", payload, nil); errors.Is(err, errTelegramTopicMissing) && strings.EqualFold(cfg.ChatMode, "topic") {
		// The topic may have been deleted; forget it, create a replacement, and retry once.
		telegramTopicCache.Delete(telegramTopicKey(token, cfg.ChatID))
		threadID, createErr := telegramTopicThread(ctx, token, cfg.ChatID, repo, true)
		if createErr != nil {
			return createErr
		}
		payload["message_thread_id"] = threadID
		return telegramPost(ctx, token, "sendMessage", payload, nil)
	} else if err != nil {
		return err
	}
	return nil
}

func telegramPost(ctx context.Context, token, method string, payload map[string]any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	token = strings.TrimSpace(token)
	for _, r := range token {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != ':' && r != '_' && r != '-' {
			return fmt.Errorf("invalid telegram bot token")
		}
	}
	url := "https://api.telegram.org/bot" + token + "/" + method
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("could not create telegram request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		// net/http errors include the request URL, which contains the bot token.
		return fmt.Errorf("telegram request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	var result struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
		ErrorCode   int             `json:"error_code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return fmt.Errorf("telegram returned an invalid response (HTTP %d)", resp.StatusCode)
	}
	if !result.OK || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		description := strings.ToLower(result.Description)
		if method == "sendMessage" && (result.ErrorCode == 400 || resp.StatusCode == 400) &&
			(strings.Contains(description, "message thread not found") || strings.Contains(description, "topic_id_invalid")) {
			return errTelegramTopicMissing
		}
		return fmt.Errorf("telegram rejected notification or returned an invalid response")
	}
	if out != nil {
		return json.Unmarshal(result.Result, out)
	}
	return nil
}

func telegramTopicKey(token, chat string) string {
	return fmt.Sprintf("ops_telegram_topic:%x", sha256.Sum256([]byte(token+"\x00"+strings.TrimSpace(chat))))
}

func telegramTopicThread(ctx context.Context, token, chat string, repo SettingRepository, recreate bool) (int64, error) {
	key := telegramTopicKey(token, chat)
	if !recreate {
		if value, ok := telegramTopicCache.Load(key); repo == nil && ok {
			if id, ok := value.(int64); ok {
				return id, nil
			}
		}
		if repo != nil {
			raw, err := repo.GetValue(ctx, key)
			if err != nil && !errors.Is(err, ErrSettingNotFound) {
				return 0, err
			}
			var id int64
			if err == nil && json.Unmarshal([]byte(raw), &id) == nil && id > 0 {
				telegramTopicCache.Store(key, id)
				return id, nil
			}
		}
	}
	var me struct {
		FirstName string `json:"first_name"`
	}
	if err := telegramPost(ctx, token, "getMe", map[string]any{}, &me); err != nil {
		return 0, err
	}
	name := strings.TrimSpace(me.FirstName)
	if name == "" {
		return 0, fmt.Errorf("telegram returned an empty bot name")
	}
	var result struct {
		MessageThreadID int64 `json:"message_thread_id"`
	}
	if err := telegramPost(ctx, token, "createForumTopic", map[string]any{"chat_id": strings.TrimSpace(chat), "name": name}, &result); err != nil {
		return 0, err
	}
	if result.MessageThreadID <= 0 {
		return 0, fmt.Errorf("telegram returned invalid topic id")
	}
	telegramTopicCache.Store(key, result.MessageThreadID)
	if repo != nil {
		raw, _ := json.Marshal(result.MessageThreadID)
		if err := repo.Set(ctx, key, string(raw)); err != nil {
			return 0, err
		}
	}
	return result.MessageThreadID, nil
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
