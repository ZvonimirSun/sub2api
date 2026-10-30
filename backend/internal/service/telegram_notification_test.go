package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type telegramTestTransport func(*http.Request) (*http.Response, error)

func (f telegramTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTelegramDelivery(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, tc := range []struct {
		body    string
		wantErr bool
	}{
		{`{"ok":true}`, false}, {`{"ok":false}`, true}, {`invalid`, true},
	} {
		http.DefaultTransport = telegramTestTransport(func(r *http.Request) (*http.Response, error) {
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["parse_mode"] != "MarkdownV2" || payload["message_thread_id"] != nil {
				t.Fatalf("unexpected payload: %v", payload)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
		})
		err := sendTelegramAlert(context.Background(), OpsTelegramAlertConfig{Enabled: true, BotToken: "123:secret", ChatID: "-100", ChatMode: "direct"}, &OpsAlertRule{}, &OpsAlertEvent{})
		if (err != nil) != tc.wantErr {
			t.Fatalf("body=%s err=%v", tc.body, err)
		}
	}
	http.DefaultTransport = telegramTestTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("URL contains 123:secret") })
	err := sendTelegramAlert(context.Background(), OpsTelegramAlertConfig{Enabled: true, BotToken: "123:secret", ChatID: "-100"}, nil, nil)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe error: %v", err)
	}
}

type telegramTopicTestRepo struct {
	SettingRepository
	values map[string]string
}

func (r *telegramTopicTestRepo) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (r *telegramTopicTestRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func TestTelegramTopicLifecycle(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, failure := range []string{"", "deleted", "rate_limit", "network"} {
		t.Run(failure, func(t *testing.T) {
			cfg := OpsTelegramAlertConfig{Enabled: true, BotToken: "123:secret", ChatID: "-100", ChatMode: "topic"}
			key := telegramTopicKey(cfg.BotToken, cfg.ChatID)
			telegramTopicCache.Delete(key)
			t.Cleanup(func() { telegramTopicCache.Delete(key) })
			repo := &telegramTopicTestRepo{values: map[string]string{}}
			creates, sends := 0, 0
			failNext := ""
			http.DefaultTransport = telegramTestTransport(func(r *http.Request) (*http.Response, error) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				body, status := `{"ok":true}`, 200
				switch {
				case strings.HasSuffix(r.URL.Path, "/getMe"):
					body = `{"ok":true,"result":{"first_name":"运维机器人","username":"ops_bot"}}`
				case strings.HasSuffix(r.URL.Path, "/createForumTopic"):
					creates++
					if payload["name"] != "运维机器人" {
						t.Fatalf("unexpected topic name: %v", payload)
					}
					body = fmt.Sprintf(`{"ok":true,"result":{"message_thread_id":%d}}`, creates+40)
				case strings.HasSuffix(r.URL.Path, "/sendMessage"):
					sends++
					if payload["message_thread_id"] != float64(creates+40) {
						t.Fatalf("unexpected thread: %v", payload)
					}
					switch failNext {
					case "deleted":
						body, status = `{"ok":false,"error_code":400,"description":"Bad Request: message thread not found"}`, 400
					case "rate_limit":
						body, status = `{"ok":false,"error_code":429}`, 429
					case "network":
						failNext = ""
						return nil, errors.New("network failed")
					}
					failNext = ""
				default:
					t.Fatalf("unexpected method: %s", r.URL.Path)
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			if err := sendTelegramAlert(context.Background(), cfg, nil, nil, repo); err != nil {
				t.Fatal(err)
			}
			// Simulate a restart: the stored thread must be reused without creating a new one.
			telegramTopicCache.Delete(key)
			failNext = failure
			err := sendTelegramAlert(context.Background(), cfg, nil, nil, repo)
			if (err != nil) != (failure == "rate_limit" || failure == "network") {
				t.Fatalf("unexpected error: %v", err)
			}
			wantCreates, wantSends := 1, 2
			if failure == "deleted" {
				wantCreates, wantSends = 2, 3
			}
			if creates != wantCreates || sends != wantSends {
				t.Fatalf("creates=%d sends=%d", creates, sends)
			}
		})
	}
}

func TestTelegramPublicConfigAndPartialUpdate(t *testing.T) {
	cfg := &OpsEmailNotificationConfig{Alert: OpsEmailAlertConfig{Telegram: OpsTelegramAlertConfig{BotToken: "secret"}}}
	data, err := json.Marshal(cfg.PublicConfig())
	if err != nil || strings.Contains(string(data), "secret") || !strings.Contains(string(data), `"bot_token_configured":true`) {
		t.Fatalf("unsafe public config: %s %v", data, err)
	}
	var req OpsEmailNotificationConfigUpdateRequest
	if err := json.Unmarshal([]byte(`{"alert":{"enabled":true}}`), &req); err != nil {
		t.Fatal(err)
	}
	if !req.Alert.Enabled || req.Alert.Telegram != nil {
		t.Fatal("omitted telegram must be distinguishable")
	}
	if got := telegramMarkdownV2Escape("a_[x].!"); got != `a\_\[x\]\.\!` {
		t.Fatalf("unexpected escaping: %s", got)
	}
}

func TestTelegramChatModeSettings(t *testing.T) {
	if got := defaultOpsEmailNotificationConfig().Alert.Telegram.ChatMode; got != "direct" {
		t.Fatalf("unexpected default mode: %s", got)
	}
	for _, mode := range []string{"", "direct", "topic", "invalid"} {
		cfg := defaultOpsEmailNotificationConfig()
		cfg.Alert.Telegram.ChatMode = mode
		err := validateOpsEmailNotificationConfig(cfg)
		if (err != nil) != (mode == "invalid") {
			t.Fatalf("mode %q: %v", mode, err)
		}
		normalizeOpsEmailNotificationConfig(cfg)
		if mode == "" && cfg.Alert.Telegram.ChatMode != "direct" {
			t.Fatal("legacy settings must default to direct")
		}
	}
}
