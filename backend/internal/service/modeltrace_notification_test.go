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
		want     bool
	}{
		{"first probe", nil, &ModelTraceRecord{DetectedModel: "m1"}, false},
		{"same model", &ModelTraceRecord{DetectedModel: "m1"}, &ModelTraceRecord{DetectedModel: "m1"}, false},
		{"previous failed", &ModelTraceRecord{DetectedModel: "m1", Error: "err"}, &ModelTraceRecord{DetectedModel: "m2"}, false},
		{"current failed", &ModelTraceRecord{DetectedModel: "m1"}, &ModelTraceRecord{DetectedModel: "m2", Error: "err"}, false},
		{"unknown ignored", &ModelTraceRecord{DetectedModel: "Unknown"}, &ModelTraceRecord{DetectedModel: "m2"}, false},
		{"changed", &ModelTraceRecord{DetectedModel: "m1"}, &ModelTraceRecord{DetectedModel: "m2"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, modelTraceFingerprintChanged(tc.previous, tc.current))
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

func (r *eventAlertRepoStub) UpdateAlertEventEmailSent(context.Context, int64, bool) error {
	return nil
}

func TestOpsAlertEventRuleDispatch(t *testing.T) {
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	sends := 0
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		sends++
		var payload map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
		require.Contains(t, payload["text"], "测试账号")
		require.Contains(t, payload["text"], "oldmodel")
		require.Contains(t, payload["text"], "newmodel")
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
		opsService:      &OpsService{settingRepo: settings},
		opsRepo:         repo,
		emailLimiter:    newSlidingWindowLimiter(0, time.Hour),
		telegramLimiter: newSlidingWindowLimiter(0, time.Hour),
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
