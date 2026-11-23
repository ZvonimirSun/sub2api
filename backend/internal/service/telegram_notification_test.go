package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type telegramTestTransport func(*http.Request) (*http.Response, error)

func (f telegramTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTelegramDelivery(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, tc := range []struct {
		body        string
		wantErr     bool
		wantContain string
	}{
		{`{"ok":true}`, false, ""},
		{`{"ok":false,"description":"Bad Request: chat not found"}`, true, "Bad Request: chat not found"},
		{`{"ok":false,"description":"token 123:secret rejected"}`, true, "HTTP 200"},
		{`invalid`, true, "invalid response"},
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
		err := sendTelegramAlert(context.Background(), OpsTelegramAlertConfig{Enabled: true, BotToken: "123:secret", ChatID: "-100"}, &OpsAlertRule{}, &OpsAlertEvent{})
		if (err != nil) != tc.wantErr {
			t.Fatalf("body=%s err=%v", tc.body, err)
		}
		if err != nil {
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("token leaked in error: %v", err)
			}
			if tc.wantContain != "" && !strings.Contains(err.Error(), tc.wantContain) {
				t.Fatalf("expected error containing %q, got %v", tc.wantContain, err)
			}
		}
	}
	http.DefaultTransport = telegramTestTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("URL contains 123:secret") })
	err := sendTelegramAlert(context.Background(), OpsTelegramAlertConfig{Enabled: true, BotToken: "123:secret", ChatID: "-100"}, nil, nil)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestTelegramCustomAPIBaseURL(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })

	var capturedURL string
	http.DefaultTransport = telegramTestTransport(func(r *http.Request) (*http.Response, error) {
		capturedURL = r.URL.String()
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})

	err := sendTelegramAlert(context.Background(), OpsTelegramAlertConfig{
		Enabled:    true,
		BotToken:   "123:secret",
		ChatID:     "-100",
		APIBaseURL: "https://tg-proxy.example.com/",
	}, nil, nil)
	if err != nil {
		t.Fatalf("expected custom api_base_url to succeed: %v", err)
	}
	if capturedURL != "https://tg-proxy.example.com/bot123:secret/sendMessage" {
		t.Fatalf("unexpected endpoint URL: %s", capturedURL)
	}

	if err := sendTelegramAlert(context.Background(), OpsTelegramAlertConfig{
		Enabled:    true,
		BotToken:   "123:secret",
		ChatID:     "-100",
		APIBaseURL: "http://insecure.example.com",
	}, nil, nil); err == nil {
		t.Fatal("expected non-https api_base_url to fail")
	}
}

type telegramSettingsTestRepo struct {
	SettingRepository
	values map[string]string
}

func (r *telegramSettingsTestRepo) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (r *telegramSettingsTestRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func TestTelegramDeliveryTargets(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, tc := range []struct {
		name     string
		threadID int64
		status   int
		response string
		wantErr  bool
	}{
		{"direct", 0, 200, `{"ok":true}`, false},
		{"topic", 42, 200, `{"ok":true}`, false},
		{"bad topic", 99, 400, `{"ok":false,"description":"Bad Request: message thread not found"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			http.DefaultTransport = telegramTestTransport(func(r *http.Request) (*http.Response, error) {
				requests++
				if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
					t.Fatalf("unexpected API call: %s", r.URL.Path)
				}
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if tc.threadID == 0 {
					if _, present := payload["message_thread_id"]; present {
						t.Fatal("direct messages must omit message_thread_id")
					}
				} else if payload["message_thread_id"] != float64(tc.threadID) {
					t.Fatalf("unexpected thread id: %v", payload)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})
			cfg := OpsTelegramAlertConfig{Enabled: true, BotToken: "123:secret", ChatID: "-100", MessageThreadID: tc.threadID}
			err := sendTelegramAlert(context.Background(), cfg, nil, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if requests != 1 {
				t.Fatalf("expected one send without topic creation or retry, got %d requests", requests)
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

func TestTelegramThreadIDSettings(t *testing.T) {
	for _, threadID := range []int64{0, 42, -1} {
		cfg := defaultOpsEmailNotificationConfig()
		cfg.Alert.Telegram.MessageThreadID = threadID
		if err := validateOpsEmailNotificationConfig(cfg); (err != nil) != (threadID < 0) {
			t.Fatalf("thread id %d: %v", threadID, err)
		}
	}
}

func TestTelegramTestNotificationUsesSavedTopic(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	repo := &telegramSettingsTestRepo{values: map[string]string{
		SettingKeyOpsEmailNotificationConfig: `{"alert":{"telegram":{"bot_token":"123:secret","chat_id":"-100","message_thread_id":42}}}`,
	}}
	http.DefaultTransport = telegramTestTransport(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["message_thread_id"] != float64(42) || payload["chat_id"] != "-100" {
			t.Fatalf("unexpected saved destination: %v", payload)
		}
		text, ok := payload["text"].(string)
		if !ok || !strings.Contains(text, "Telegram 测试通知") {
			t.Fatalf("unexpected test message: %v", payload)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})
	svc := &OpsService{settingRepo: repo}
	if err := svc.SendTelegramTestNotification(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repo.values) != 1 {
		t.Fatal("sending must not persist topic state")
	}
}

func TestMaybeSendAlertEmailTelegramOnlyReturnsTrue(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	sentCount := 0
	http.DefaultTransport = telegramTestTransport(func(*http.Request) (*http.Response, error) {
		sentCount++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})

	repo := &telegramSettingsTestRepo{values: map[string]string{
		SettingKeyOpsEmailNotificationConfig: `{"alert":{"enabled":false,"rate_limit_per_hour":1,"telegram":{"enabled":true,"bot_token":"123:secret","chat_id":"-100"}}}`,
	}}
	evaluator := NewOpsAlertEvaluatorService(&OpsService{settingRepo: repo}, nil, nil, nil, nil, nil)
	rule := &OpsAlertRule{ID: 1, Name: "High Error Rate", Severity: "P0", MetricType: "error_rate", Operator: ">", Threshold: 5, NotifyEmail: true}
	event := &OpsAlertEvent{ID: 0, Status: OpsAlertStatusFiring, FiredAt: time.Now().UTC(), Description: "test"}

	if ok := evaluator.maybeSendAlertEmail(context.Background(), nil, rule, event); !ok {
		t.Fatal("expected telegram-only alert delivery to return true")
	}
	if sentCount != 1 {
		t.Fatalf("expected 1 telegram delivery, got %d", sentCount)
	}
	// Second call within the same hour must be blocked by telegramLimiter, leaving emailLimiter untouched.
	if ok := evaluator.maybeSendAlertEmail(context.Background(), nil, rule, event); ok {
		t.Fatal("expected second telegram alert to be rate limited")
	}
	if !evaluator.emailLimiter.Allow(time.Now().UTC()) {
		t.Fatal("expected email limiter to remain independent from telegram limiter")
	}
}
