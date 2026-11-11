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

type eventAlertRepoStub struct {
	OpsRepository
	rules   []*OpsAlertRule
	latest  *OpsAlertEvent
	created *OpsAlertEvent
}

func (r *eventAlertRepoStub) ListAlertRules(context.Context) ([]*OpsAlertRule, error) {
	return r.rules, nil
}

func (r *eventAlertRepoStub) GetLatestAlertEvent(context.Context, int64) (*OpsAlertEvent, error) {
	return r.latest, nil
}

func (r *eventAlertRepoStub) CreateAlertEvent(_ context.Context, event *OpsAlertEvent) (*OpsAlertEvent, error) {
	created := *event
	created.ID = 12
	r.created = &created
	return &created, nil
}

func TestOpsAlertEventRuleDispatch(t *testing.T) {
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
		SettingKeyOpsEmailNotificationConfig: `{"alert":{"min_severity":"critical","rate_limit_per_hour":1,"telegram":{"enabled":true,"bot_token":"123:secret","chat_id":"-100"}}}`,
		SettingKeyOpsAlertRuntimeSettings:    `{}`,
	}}
	repo := &eventAlertRepoStub{rules: []*OpsAlertRule{{
		ID: 7, Name: "模型指纹发生变化", Enabled: true, Severity: "P0",
		MetricType: opsAlertEventMetricType(OpsAlertEventModelTraceFingerprint), CooldownMinutes: 5, NotifyEmail: true,
	}}}
	evaluator := &OpsAlertEvaluatorService{
		opsService:   &OpsService{settingRepo: settings},
		opsRepo:      repo,
		emailLimiter: newSlidingWindowLimiter(0, time.Hour),
	}
	event := &OpsAlertEvent{
		Description: "账号：测试账号（ID: 42）\n请求模型：requested\n上次指纹模型：oldmodel\n本次指纹模型：newmodel",
		FiredAt:     time.Now().UTC(),
	}
	evaluator.TriggerEvent(context.Background(), OpsAlertEventModelTraceFingerprint, event)
	require.Equal(t, 1, sends)
	require.Equal(t, int64(7), repo.created.RuleID)
	require.Equal(t, "P0", repo.created.Severity)
	require.Equal(t, OpsAlertStatusFiring, repo.created.Status)

	// Event rules persist events and apply their configured cooldown.
	repo.latest = repo.created
	evaluator.TriggerEvent(context.Background(), OpsAlertEventModelTraceFingerprint, event)
	require.Equal(t, 1, sends)
}
