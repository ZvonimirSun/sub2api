package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type modelTraceTestRepo struct {
	AccountRepository
	account   *Account
	reads     int
	onReload  func()
	updates   map[string]any
	updateErr error
	accounts  []Account
	listCalls int
}

func (r *modelTraceTestRepo) List(_ context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	r.listCalls++
	start := params.Offset()
	if start >= len(r.accounts) {
		return nil, nil, nil
	}
	end := min(start+params.Limit(), len(r.accounts))
	return r.accounts[start:end], &pagination.PaginationResult{Pages: (len(r.accounts) + params.Limit() - 1) / params.Limit()}, nil
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
		t.Run(map[bool]string{false: "edited", true: "removed"}[removed], func(t *testing.T) {
			cfg := &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"test": {}}}
			repo := &modelTraceTestRepo{account: &Account{ID: 1, Extra: map[string]any{ModelTraceExtraKey: cfg, "other": "old"}}}
			repo.onReload = func() {
				cfg.Models["other"] = &ModelTraceModelConfig{}
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
			require.Contains(t, saved.Models, "other")
			require.NotNil(t, saved.Models["test"].Latest)
		})
	}
}

func TestModelTraceProbeReportsSaveFailure(t *testing.T) {
	repo := &modelTraceTestRepo{account: &Account{ID: 1, Extra: map[string]any{ModelTraceExtraKey: &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"test": {}}}}}, updateErr: errors.New("write failed")}
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
}

type modelTraceStoredSettings struct {
	SettingRepository
	raw string
	err error
}

func (s *modelTraceStoredSettings) GetValue(context.Context, string) (string, error) {
	return s.raw, s.err
}

func TestModelTraceDefaultDisabled(t *testing.T) {
	for _, settings := range []SettingRepository{
		nil,
		&modelTraceStoredSettings{},
		&modelTraceStoredSettings{raw: "invalid"},
		&modelTraceStoredSettings{err: errors.New("unavailable")},
		&modelTraceStoredSettings{raw: `{"probe_count":3}`},
	} {
		repo := &modelTraceTestRepo{}
		svc := NewModelTraceService(repo, settings, nil, nil)
		require.False(t, svc.GetGlobalConfig(context.Background()).Enabled)
		svc.RunScheduledCycle(context.Background())
		require.Zero(t, repo.listCalls)
	}
	svc := NewModelTraceService(nil, &modelTraceStoredSettings{raw: `{"enabled":true}`}, nil, nil)
	require.True(t, svc.GetGlobalConfig(context.Background()).Enabled)
}

func TestModelTraceListsOnlyAddedAccountsAcrossPages(t *testing.T) {
	accounts := make([]Account, 1002)
	for i := range accounts {
		accounts[i].ID = int64(i + 1)
	}
	accounts[1].Extra = map[string]any{ModelTraceExtraKey: &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"first": {}}}}
	accounts[1001].Extra = map[string]any{ModelTraceExtraKey: &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"last": {}}}}
	accounts[2].Extra = map[string]any{ModelTraceExtraKey: nil}
	repo := &modelTraceTestRepo{accounts: accounts}
	svc := NewModelTraceService(repo, nil, nil, nil)
	items, err := svc.ListMonitors(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, int64(1002), items[1].AccountID)
	require.Equal(t, 2, repo.listCalls)
}

func TestModelTraceRemovingMonitorDoesNotRecreateIt(t *testing.T) {
	cfg := &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"test": {}}}
	repo := &modelTraceTestRepo{account: &Account{ID: 1, Extra: map[string]any{ModelTraceExtraKey: cfg}}}
	svc := NewModelTraceService(repo, &modelTraceTestSettings{}, nil, nil)
	repo.onReload = func() { repo.account.Extra[ModelTraceExtraKey] = nil }
	_, err := svc.ProbeModel(context.Background(), 1, "test")
	require.NoError(t, err)
	require.Nil(t, repo.updates)
	_, err = svc.ProbeModel(context.Background(), 1, "test")
	require.ErrorContains(t, err, "not configured")
	require.NoError(t, svc.RemoveAccountMonitor(context.Background(), 1))
	require.Equal(t, map[string]any{ModelTraceExtraKey: nil}, repo.updates)
}
