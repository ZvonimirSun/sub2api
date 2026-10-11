package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type modelTraceTestRepo struct {
	AccountRepository
	account   *Account
	reads     int
	onReload  func()
	updates   map[string]any
	updateErr error
}

func (r *modelTraceTestRepo) GetByID(context.Context, int64) (*Account, error) {
	r.reads++
	if r.reads == 2 && r.onReload != nil {
		r.onReload()
	}
	copy := *r.account
	return &copy, nil
}

func (r *modelTraceTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.updates = updates
	return r.updateErr
}

type modelTraceTestSettings struct{ SettingRepository }

func (*modelTraceTestSettings) GetValue(context.Context, string) (string, error) {
	return `{"enabled":true,"probe_count":1,"interval_seconds":0,"reasoning_effort":"low","cron_expression":"*/30 * * * *"}`, nil
}

func TestModelTraceProbePreservesConcurrentConfig(t *testing.T) {
	for _, removed := range []bool{false, true} {
		t.Run(map[bool]string{false: "paused", true: "removed"}[removed], func(t *testing.T) {
			cfg := &ModelTraceAccountConfig{Enabled: true, Models: map[string]*ModelTraceModelConfig{"test": {}}}
			repo := &modelTraceTestRepo{account: &Account{ID: 1, Extra: map[string]any{ModelTraceExtraKey: cfg, "other": "old"}}}
			repo.onReload = func() {
				cfg.Enabled = false
				cfg.CronExpression = "0 */2 * * *"
				repo.account.Extra["other"] = "new"
				if removed {
					delete(cfg.Models, "test")
				}
			}
			svc := NewModelTraceService(repo, &modelTraceTestSettings{}, nil, nil)
			_, err := svc.ProbeModel(context.Background(), 1, "test")
			require.NoError(t, err)
			if removed {
				require.Nil(t, repo.updates)
				return
			}
			require.Len(t, repo.updates, 1)
			saved, ok := repo.updates[ModelTraceExtraKey].(*ModelTraceAccountConfig)
			require.True(t, ok)
			require.False(t, saved.Enabled)
			require.Equal(t, "0 */2 * * *", saved.CronExpression)
			require.NotNil(t, saved.Models["test"].Latest)
		})
	}
}

func TestModelTraceProbeReportsSaveFailure(t *testing.T) {
	repo := &modelTraceTestRepo{account: &Account{ID: 1}, updateErr: errors.New("write failed")}
	svc := NewModelTraceService(repo, &modelTraceTestSettings{}, nil, nil)
	_, err := svc.ProbeModel(context.Background(), 1, "test")
	require.ErrorContains(t, err, "write failed")
}

func TestModelTraceSchedules(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Date(2026, 10, 12, 14, 30, 0, 0, loc)
	require.True(t, modelTraceScheduleDue("*/30 * * * *", now))
	require.False(t, modelTraceScheduleDue("0 * * * *", now))
	require.True(t, modelTraceScheduleDue("0 */2 * * *", now.Add(-30*time.Minute)))
	require.False(t, modelTraceScheduleDue("invalid", now))
	svc := NewModelTraceService(&modelTraceTestRepo{account: &Account{ID: 1}}, &modelTraceTestSettings{}, nil, nil)
	require.ErrorContains(t, svc.UpdateGlobalConfig(context.Background(), ModelTraceGlobalConfig{CronExpression: "invalid"}), "invalid cron")
	require.ErrorContains(t, svc.UpdateAccountConfig(context.Background(), 1, UpdateAccountModelTraceRequest{CronExpression: "invalid"}), "invalid cron")
}
