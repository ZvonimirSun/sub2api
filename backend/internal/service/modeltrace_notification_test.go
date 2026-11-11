package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestModelTraceFingerprintChanged(t *testing.T) {
	for _, tc := range []struct {
		name     string
		previous *ModelTraceRecord
		current  *ModelTraceRecord
		changed  bool
	}{
		{"first result", nil, &ModelTraceRecord{DetectedModel: "model-b"}, false},
		{"unchanged", &ModelTraceRecord{DetectedModel: "model-a"}, &ModelTraceRecord{DetectedModel: "model-a"}, false},
		{"changed", &ModelTraceRecord{DetectedModel: "model-a"}, &ModelTraceRecord{DetectedModel: "model-b"}, true},
		{"whitespace", &ModelTraceRecord{DetectedModel: " model-a "}, &ModelTraceRecord{DetectedModel: "model-a"}, false},
		{"failed current", &ModelTraceRecord{DetectedModel: "model-a"}, &ModelTraceRecord{DetectedModel: "Unknown", Error: "timeout"}, false},
		{"failed previous", &ModelTraceRecord{DetectedModel: "Unknown", Error: "timeout"}, &ModelTraceRecord{DetectedModel: "model-b"}, false},
		{"unknown current", &ModelTraceRecord{DetectedModel: "model-a"}, &ModelTraceRecord{DetectedModel: "Unknown"}, false},
		{"unknown previous", &ModelTraceRecord{DetectedModel: "Unknown"}, &ModelTraceRecord{DetectedModel: "model-b"}, false},
		{"empty current", &ModelTraceRecord{DetectedModel: "model-a"}, &ModelTraceRecord{}, false},
		{"empty previous", &ModelTraceRecord{}, &ModelTraceRecord{DetectedModel: "model-b"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.changed, modelTraceFingerprintChanged(tc.previous, tc.current))
		})
	}
}

func TestModelTraceChangeNotification(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	sends := 0
	http.DefaultTransport = telegramTestTransport(func(r *http.Request) (*http.Response, error) {
		sends++
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		text, ok := payload["text"].(string)
		require.True(t, ok)
		for _, value := range []string{"P0", "测试账号", "42", "上次指纹模型：oldmodel", "本次指纹模型：newmodel", "请求模型：requested"} {
			require.Contains(t, text, value)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`))}, nil
	})
	settings := &telegramSettingsTestRepo{values: map[string]string{
		SettingKeyOpsEmailNotificationConfig: `{"alert":{"rate_limit_per_hour":1,"telegram":{"enabled":true,"bot_token":"123:secret","chat_id":"-100"}}}`,
		SettingKeyOpsAlertRuntimeSettings:    `{}`,
	}}
	evaluator := &OpsAlertEvaluatorService{
		opsService:   &OpsService{settingRepo: settings},
		emailLimiter: newSlidingWindowLimiter(0, time.Hour),
	}
	svc := NewModelTraceService(nil, settings, nil, nil)
	svc.alertEvaluator = evaluator
	account := &Account{ID: 42, Name: "测试账号"}
	svc.notifyModelTraceChange(context.Background(), account, "requested", "oldmodel", "newmodel")
	require.Equal(t, 1, sends)
	// Fingerprint alerts share the evaluator's budget, including other ops alerts.
	svc.notifyModelTraceChange(context.Background(), account, "requested", "oldmodel", "newmodel")
	require.Equal(t, 1, sends)
	require.False(t, evaluator.emailLimiter.Allow(time.Now().UTC()))

	for _, tc := range []struct {
		name    string
		config  string
		runtime string
	}{
		{"disabled", `{"alert":{"telegram":{"enabled":false}}}`, `{}`},
		{"severity", `{"alert":{"min_severity":"critical","telegram":{"enabled":true,"bot_token":"123:secret","chat_id":"-100"}}}`, `{}`},
		{"silenced", `{"alert":{"telegram":{"enabled":true,"bot_token":"123:secret","chat_id":"-100"}}}`, `{"silencing":{"enabled":true,"global_until_rfc3339":"2999-01-01T00:00:00Z"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings.values[SettingKeyOpsEmailNotificationConfig] = tc.config
			settings.values[SettingKeyOpsAlertRuntimeSettings] = tc.runtime
			evaluator.emailLimiter = newSlidingWindowLimiter(0, time.Hour)
			svc.notifyModelTraceChange(context.Background(), account, "requested", "oldmodel", "newmodel")
			require.Equal(t, 1, sends)
		})
	}
}
