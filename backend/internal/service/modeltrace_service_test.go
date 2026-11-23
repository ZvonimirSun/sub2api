package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	lmfpd "github.com/Ikaleio/lm-detector/sdk/go"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type modelTraceTestRepo struct {
	AccountRepository
	mu        sync.Mutex
	account   *Account
	accounts  []Account
	reads     int
	onReload  func()
	updates   map[string]any
	updateErr error
	listCalls int
}

func (r *modelTraceTestRepo) List(_ context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listCalls++
	start := params.Offset()
	if start >= len(r.accounts) {
		return nil, nil, nil
	}
	end := min(start+params.Limit(), len(r.accounts))
	return r.accounts[start:end], &pagination.PaginationResult{Pages: (len(r.accounts) + params.Limit() - 1) / params.Limit()}, nil
}

func (r *modelTraceTestRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if r.reads == 2 && r.onReload != nil {
		r.onReload()
	}
	if r.account != nil && (r.account.ID == id || len(r.accounts) == 0) {
		copyAcc := *r.account
		return &copyAcc, nil
	}
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			copyAcc := r.accounts[i]
			return &copyAcc, nil
		}
	}
	return nil, fmt.Errorf("account %d not found", id)
}

func (r *modelTraceTestRepo) UpdateExtra(_ context.Context, _ int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = updates
	return r.updateErr
}

type modelTraceTestSettings struct{ SettingRepository }

func (*modelTraceTestSettings) GetValue(context.Context, string) (string, error) {
	return `{"enabled":true,"probe_count":1,"interval_seconds":0,"reasoning_effort":"low","cron_expression":"*/30 * * * *"}`, nil
}

type modelTraceStubScorer struct {
	decision    string
	prediction  string
	probability *float64
	analyzeErr  error
	lastOutputs []lmfpd.Output
}

func (s *modelTraceStubScorer) GenerateChallenges(count int) ([]modelTraceChallenge, error) {
	out := make([]modelTraceChallenge, count)
	for i := range out {
		out[i] = modelTraceChallenge{
			ID:            fmt.Sprintf("stub-%d", i+1),
			ExpectedCount: 300 + i,
			Prompt:        fmt.Sprintf("generate %d numbers", 300+i),
		}
	}
	return out, nil
}

func (s *modelTraceStubScorer) ParseNumbers(text string) []int {
	return lmfpd.ParseNumbers(text)
}

func (s *modelTraceStubScorer) Analyze(_ context.Context, outputs []lmfpd.Output) (*lmfpd.Analysis, error) {
	s.lastOutputs = append([]lmfpd.Output(nil), outputs...)
	if s.analyzeErr != nil {
		return nil, s.analyzeErr
	}
	decision := s.decision
	if decision == "" {
		decision = "scored"
	}
	return &lmfpd.Analysis{
		Decision:    decision,
		Prediction:  s.prediction,
		Probability: s.probability,
	}, nil
}

func makeValidNumberReply(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("%d", (i*7)%350+1)
	}
	return strings.Join(parts, " ")
}

func TestModelTraceProbePreservesConcurrentConfig(t *testing.T) {
	for _, change := range []string{"edited", "removed", "paused"} {
		t.Run(change, func(t *testing.T) {
			cfg := &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"test": {}}}
			repo := &modelTraceTestRepo{account: &Account{ID: 1, Extra: map[string]any{ModelTraceExtraKey: cfg, "other": "old"}}}
			repo.onReload = func() {
				cfg.Models["other"] = &ModelTraceModelConfig{}
				repo.account.Extra["other"] = "new"
				if change == "removed" {
					delete(cfg.Models, "test")
				}
				if change == "paused" {
					enabled := false
					cfg.Enabled = &enabled
				}
			}
			svc := NewModelTraceService(repo, &modelTraceTestSettings{}, nil, nil)
			_, err := svc.ProbeModel(context.Background(), 1, "test")
			require.NoError(t, err)
			if change == "removed" {
				require.Nil(t, repo.updates)
				return
			}
			require.Len(t, repo.updates, 1)
			saved, ok := repo.updates[ModelTraceExtraKey].(*ModelTraceAccountConfig)
			require.True(t, ok)
			require.Contains(t, saved.Models, "other")
			require.NotNil(t, saved.Models["test"].Latest)
			require.NotEmpty(t, saved.Models["test"].Latest.Requests)
			require.NotEmpty(t, saved.Models["test"].Latest.Requests[0].Error)
			if change == "paused" {
				require.False(t, saved.monitoringEnabled(), "saving an in-flight result must preserve the pause")
			}
		})
	}
}

func TestModelTraceProbeReportsSaveFailure(t *testing.T) {
	repo := &modelTraceTestRepo{account: &Account{ID: 1, Extra: map[string]any{ModelTraceExtraKey: &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"test": {}}}}}, updateErr: errors.New("write failed")}
	svc := NewModelTraceService(repo, &modelTraceTestSettings{}, nil, nil)
	_, err := svc.ProbeModel(context.Background(), 1, "test")
	require.ErrorContains(t, err, "write failed")
}

func TestModelTraceSchedulesAndProbeCountLimits(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	now := time.Date(2026, 10, 12, 14, 30, 0, 0, loc)
	require.True(t, modelTraceScheduleDue("*/30 * * * *", now))
	require.False(t, modelTraceScheduleDue("0 * * * *", now))
	require.True(t, modelTraceScheduleDue("0 */2 * * *", now.Add(-30*time.Minute)))
	require.False(t, modelTraceScheduleDue("invalid", now))

	svc := NewModelTraceService(&modelTraceTestRepo{account: &Account{ID: 1}}, &modelTraceTestSettings{}, nil, nil)
	require.ErrorContains(t, svc.UpdateGlobalConfig(context.Background(), ModelTraceGlobalConfig{CronExpression: "invalid"}), "invalid cron")
	require.ErrorContains(t, svc.UpdateGlobalConfig(context.Background(), ModelTraceGlobalConfig{ProbeCount: 4, CronExpression: "*/30 * * * *"}), "probe_count")

	clampedSvc := NewModelTraceService(nil, &modelTraceStoredSettings{raw: `{"enabled":true,"probe_count":10}`}, nil, nil)
	require.Equal(t, modelTraceMaxProbeCount, clampedSvc.GetGlobalConfig(context.Background()).ProbeCount)
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

func TestModelTraceAccountTogglePreservesModelsAndHistory(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	cfg := &ModelTraceAccountConfig{LastRunAt: &now, Models: map[string]*ModelTraceModelConfig{
		"test": {Latest: &ModelTraceRecord{DetectedModel: "fingerprint"}, History: []ModelTraceRecord{{DetectedModel: "previous"}}},
	}}
	repo := &modelTraceTestRepo{account: &Account{ID: 1, Extra: map[string]any{ModelTraceExtraKey: cfg, "other": "untouched"}}}
	svc := NewModelTraceService(repo, nil, nil, nil)
	t.Cleanup(svc.Stop)
	require.True(t, DecodeModelTraceConfig(repo.account.Extra).monitoringEnabled(), "legacy configs stay enabled")

	enabled := false
	require.NoError(t, svc.UpdateAccountConfig(ctx, 1, UpdateAccountModelTraceRequest{Enabled: &enabled}))
	saved, ok := repo.updates[ModelTraceExtraKey].(*ModelTraceAccountConfig)
	require.True(t, ok, "updated config should have the expected type")
	require.False(t, saved.monitoringEnabled())
	require.Equal(t, cfg.Models, saved.Models)
	require.Equal(t, cfg.LastRunAt, saved.LastRunAt)
	require.Len(t, repo.updates, 1, "only the monitoring config is written")
	repo.account.Extra[ModelTraceExtraKey] = saved

	// Editing models must not silently re-enable a paused account.
	require.NoError(t, svc.UpdateAccountConfig(ctx, 1, UpdateAccountModelTraceRequest{Models: []string{"test", "new"}}))
	saved, ok = repo.updates[ModelTraceExtraKey].(*ModelTraceAccountConfig)
	require.True(t, ok, "updated config should have the expected type")
	require.False(t, saved.monitoringEnabled())
	require.Equal(t, cfg.Models["test"], saved.Models["test"])
	repo.account.Extra[ModelTraceExtraKey] = saved

	enabled = true
	require.NoError(t, svc.UpdateAccountConfig(ctx, 1, UpdateAccountModelTraceRequest{Enabled: &enabled}))
	saved, ok = repo.updates[ModelTraceExtraKey].(*ModelTraceAccountConfig)
	require.True(t, ok, "updated config should have the expected type")
	require.True(t, saved.monitoringEnabled())
	require.Len(t, saved.Models, 2)
	repo.account.Extra[ModelTraceExtraKey] = saved

	// An explicit empty list still clears models while an omitted list preserves them.
	require.NoError(t, svc.UpdateAccountConfig(ctx, 1, UpdateAccountModelTraceRequest{Models: []string{}}))
	saved, ok = repo.updates[ModelTraceExtraKey].(*ModelTraceAccountConfig)
	require.True(t, ok, "updated config should have the expected type")
	require.Empty(t, saved.Models)
}

func TestModelTracePausedAccountSkipsAutomaticButAllowsManualProbes(t *testing.T) {
	enabled := false
	cfg := &ModelTraceAccountConfig{Enabled: &enabled, Models: map[string]*ModelTraceModelConfig{"test": {}}}
	account := &Account{ID: 1, Status: StatusActive, Schedulable: true, Extra: map[string]any{ModelTraceExtraKey: cfg}}
	repo := &modelTraceTestRepo{account: account, accounts: []Account{*account}}
	settings := &modelTraceStoredSettings{raw: `{"enabled":true,"probe_count":1,"interval_seconds":0,"cron_expression":"* * * * *"}`}
	svc := NewModelTraceService(repo, settings, nil, nil)
	t.Cleanup(svc.Stop)
	ctx := context.Background()
	items, err := svc.ListMonitors(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.False(t, items[0].Enabled)
	require.True(t, items[0].Schedulable)
	require.Contains(t, items[0].Models, "test")
	svc.RunScheduledCycle(ctx)
	require.Zero(t, repo.reads, "scheduler skips paused accounts")
	// A cycle that was already queued must reload the switch before its next model.
	svc.runAccountCycle(ctx, account, &ModelTraceAccountConfig{Models: cfg.Models})
	require.Equal(t, 1, repo.reads)
	require.Nil(t, repo.updates, "automatic probes do not replace history while paused")
	record, err := svc.ProbeModel(ctx, 1, "test")
	require.NoError(t, err, "manual probes remain available while automatic monitoring is paused")
	require.NotEmpty(t, record.Requests, "manual probe reached the completion path")
	saved, ok := repo.updates[ModelTraceExtraKey].(*ModelTraceAccountConfig)
	require.True(t, ok, "updated config should have the expected type")
	require.False(t, saved.monitoringEnabled(), "manual probing must not enable automatic monitoring")
	require.NotNil(t, saved.Models["test"].Latest)
}

func TestModelTraceSchedulingPausedAnd429SkipAutomaticButAllowManualProbes(t *testing.T) {
	future := time.Now().Add(10 * time.Minute)
	cases := []struct {
		name    string
		account Account
	}{
		{
			name: "scheduling paused",
			account: Account{
				ID:          10,
				Status:      StatusActive,
				Schedulable: false,
				Extra:       map[string]any{ModelTraceExtraKey: &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"gpt-5.4": {}}}},
			},
		},
		{
			name: "account rate limited 429",
			account: Account{
				ID:               11,
				Status:           StatusActive,
				Schedulable:      true,
				RateLimitResetAt: &future,
				Extra:            map[string]any{ModelTraceExtraKey: &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"gpt-5.4": {}}}},
			},
		},
		{
			name: "account overloaded",
			account: Account{
				ID:            12,
				Status:        StatusActive,
				Schedulable:   true,
				OverloadUntil: &future,
				Extra:         map[string]any{ModelTraceExtraKey: &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"gpt-5.4": {}}}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acc := tc.account
			repo := &modelTraceTestRepo{account: &acc, accounts: []Account{acc}}
			settings := &modelTraceStoredSettings{raw: `{"enabled":true,"probe_count":1,"interval_seconds":0,"cron_expression":"* * * * *"}`}
			svc := NewModelTraceService(repo, settings, nil, nil)
			t.Cleanup(svc.Stop)
			prob := 0.91
			svc.scorer = &modelTraceStubScorer{prediction: "gpt-5.4", probability: &prob}
			probeCalls := 0
			svc.probeFn = func(context.Context, *Account, string, string, string) (string, error) {
				probeCalls++
				return makeValidNumberReply(250), nil
			}

			items, err := svc.ListMonitors(context.Background())
			require.NoError(t, err)
			require.Len(t, items, 1)
			require.False(t, items[0].Schedulable)

			// Automatic scheduled cycle must skip unschedulable/429 accounts.
			svc.RunScheduledCycle(context.Background())
			require.Zero(t, probeCalls)
			require.Nil(t, repo.updates)

			// Even if runAccountCycle was already queued, it re-checks latest scheduling state.
			svc.runAccountCycle(context.Background(), &acc, DecodeModelTraceConfig(acc.Extra))
			require.Zero(t, probeCalls)
			require.Nil(t, repo.updates)

			// Manual probe ("立即测试") must bypass scheduling/429 pause.
			rec, err := svc.ProbeModel(context.Background(), acc.ID, "gpt-5.4")
			require.NoError(t, err)
			require.NotNil(t, rec)
			require.Equal(t, 1, probeCalls)
			require.Equal(t, "gpt-5.4", rec.DetectedModel)
			require.InDelta(t, 0.91, rec.Probability, 1e-9)
		})
	}
}

func TestModelTraceScheduledCycleSkipsModelRateLimitedOnly(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(10 * time.Minute)
	cfg := &ModelTraceAccountConfig{
		Models: map[string]*ModelTraceModelConfig{
			"claude-opus-4-6":   {},
			"claude-sonnet-4-6": {},
		},
	}
	acc := &Account{
		ID:          21,
		Platform:    PlatformAnthropic,
		Status:      StatusActive,
		Schedulable: true,
		Extra: map[string]any{
			ModelTraceExtraKey: cfg,
		},
	}
	setAccountModelRateLimitSnapshot(acc, "claude-opus-4-6", future, "rate_limited", now)
	require.False(t, acc.IsSchedulableForModelWithContext(context.Background(), "claude-opus-4-6"))
	require.True(t, acc.IsSchedulableForModelWithContext(context.Background(), "claude-sonnet-4-6"))
	repo := &modelTraceContextRepo{modelTraceTestRepo: modelTraceTestRepo{account: acc, accounts: []Account{*acc}}}
	settings := &modelTraceStoredSettings{raw: `{"enabled":true,"probe_count":1,"interval_seconds":0,"cron_expression":"* * * * *"}`}
	svc := NewModelTraceService(repo, settings, nil, nil)
	t.Cleanup(svc.Stop)
	prob := 0.88
	svc.scorer = &modelTraceStubScorer{prediction: "claude-sonnet-4-6", probability: &prob}

	var probed []string
	svc.probeFn = func(_ context.Context, _ *Account, modelID, _, _ string) (string, error) {
		probed = append(probed, modelID)
		return makeValidNumberReply(250), nil
	}

	svc.RunScheduledCycle(context.Background())
	require.Equal(t, []string{"claude-sonnet-4-6"}, probed)

	saved := DecodeModelTraceConfig(repo.account.Extra)
	require.Nil(t, saved.Models["claude-opus-4-6"].Latest)
	require.NotNil(t, saved.Models["claude-sonnet-4-6"].Latest)
	require.Equal(t, "claude-sonnet-4-6", saved.Models["claude-sonnet-4-6"].Latest.DetectedModel)
}

func TestModelTraceScheduledProbeAbortsOn429WithoutOverwritingHistory(t *testing.T) {
	previous := &ModelTraceRecord{DetectedModel: "gpt-5.4", Probability: 0.95}
	cfg := &ModelTraceAccountConfig{
		Models: map[string]*ModelTraceModelConfig{
			"gpt-5.4": {Latest: previous},
		},
	}
	acc := &Account{
		ID:          22,
		Status:      StatusActive,
		Schedulable: true,
		Extra:       map[string]any{ModelTraceExtraKey: cfg},
	}
	repo := &modelTraceTestRepo{account: acc, accounts: []Account{*acc}}
	settings := &modelTraceStoredSettings{raw: `{"enabled":true,"probe_count":3,"interval_seconds":0,"cron_expression":"* * * * *"}`}
	svc := NewModelTraceService(repo, settings, nil, nil)
	t.Cleanup(svc.Stop)
	svc.scorer = &modelTraceStubScorer{prediction: "gpt-5.4"}

	attempts := 0
	svc.probeFn = func(context.Context, *Account, string, string, string) (string, error) {
		attempts++
		return "", errors.New("API returned 429: too many requests")
	}

	svc.RunScheduledCycle(context.Background())
	require.Equal(t, 1, attempts, "scheduled probe must stop retrying immediately on 429")
	require.Nil(t, repo.updates, "aborting on 429 must not overwrite previous valid fingerprint")
}

func TestModelTraceQueuedManualProbeStillRunsWhenPaused(t *testing.T) {
	cfg := &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"test": {}}}
	repo := &modelTraceTestRepo{account: &Account{ID: 1, Extra: map[string]any{ModelTraceExtraKey: cfg}}}
	svc := NewModelTraceService(repo, &modelTraceTestSettings{}, nil, nil)
	t.Cleanup(svc.Stop)
	unlock, err := svc.acquireAccountLock(context.Background(), 1)
	require.NoError(t, err)
	task, err := svc.SubmitProbe(context.Background(), 1, "test")
	if err != nil {
		unlock()
		t.Fatal(err)
	}
	require.Equal(t, "queued", task.Status)
	enabled := false
	cfg.Enabled = &enabled
	unlock()
	require.Eventually(t, func() bool {
		svc.tasksMu.Lock()
		defer svc.tasksMu.Unlock()
		return svc.tasks[modelTraceTaskKey{1, "test"}].Status == "failed"
	}, time.Second, time.Millisecond)
	svc.tasksMu.Lock()
	finished := svc.tasks[modelTraceTaskKey{1, "test"}]
	svc.tasksMu.Unlock()
	require.Contains(t, finished.Error, "accountTestService is not available", "manual task reached the completion path despite pausing")
	saved, ok := repo.updates[ModelTraceExtraKey].(*ModelTraceAccountConfig)
	require.True(t, ok, "updated config should have the expected type")
	require.False(t, saved.monitoringEnabled())
	require.NotEmpty(t, saved.Models["test"].Latest.Requests)
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
	require.True(t, items[0].Enabled, "legacy configs are exposed as enabled")
	require.True(t, items[1].Enabled)
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

func TestModelTraceAccountLockWaitCanBeCanceled(t *testing.T) {
	svc := NewModelTraceService(nil, nil, nil, nil)
	unlock, err := svc.acquireAccountLock(context.Background(), 31)
	require.NoError(t, err)
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, probeErr := svc.ProbeModel(ctx, 31, "test")
		done <- probeErr
	}()
	select {
	case probeErr := <-done:
		require.ErrorIs(t, probeErr, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("probe continued waiting on the account lock after cancellation")
	}
	// An unrelated account does not wait for account 31.
	otherUnlock, err := svc.acquireAccountLock(context.Background(), 44)
	require.NoError(t, err)
	otherUnlock()
}

type modelTraceCancelSettings struct {
	modelTraceTestSettings
	cancel context.CancelFunc
}

func (s *modelTraceCancelSettings) GetValue(ctx context.Context, key string) (string, error) {
	s.cancel()
	return s.modelTraceTestSettings.GetValue(ctx, key)
}

type modelTraceContextRepo struct {
	modelTraceTestRepo
	readDeadlines []time.Time
}

func (r *modelTraceContextRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	deadline, _ := ctx.Deadline()
	r.mu.Lock()
	r.readDeadlines = append(r.readDeadlines, deadline)
	r.mu.Unlock()
	return r.modelTraceTestRepo.GetByID(ctx, id)
}

func (r *modelTraceContextRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.modelTraceTestRepo.UpdateExtra(ctx, id, updates); err != nil {
		return err
	}
	r.mu.Lock()
	for key, value := range updates {
		r.account.Extra[key] = value
	}
	r.mu.Unlock()
	return nil
}

func TestModelTraceCanceledProbeSavesFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"test": {}}}
	repo := &modelTraceContextRepo{modelTraceTestRepo: modelTraceTestRepo{account: &Account{ID: 31, Extra: map[string]any{ModelTraceExtraKey: cfg}}}}
	svc := NewModelTraceService(repo, &modelTraceCancelSettings{cancel: cancel}, nil, nil)
	record, err := svc.ProbeModel(ctx, 31, "test")
	require.NoError(t, err)
	require.Equal(t, context.Canceled.Error(), record.Error)
	require.Equal(t, "Unknown", record.DetectedModel)
	require.NotNil(t, repo.updates)
	saved := DecodeModelTraceConfig(repo.account.Extra).Models["test"].Latest
	require.NotNil(t, saved)
	require.Equal(t, record.Error, saved.Error)
	require.True(t, record.TestedAt.Equal(saved.TestedAt))
}

func TestModelTraceScheduledModelsHaveNoProbeDeadline(t *testing.T) {
	cfg := &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"first": {}, "second": {}}}
	repo := &modelTraceContextRepo{modelTraceTestRepo: modelTraceTestRepo{account: &Account{ID: 31, Status: StatusActive, Schedulable: true, Extra: map[string]any{ModelTraceExtraKey: cfg}}}}
	svc := NewModelTraceService(repo, &modelTraceTestSettings{}, nil, nil)
	prob := 0.9
	svc.scorer = &modelTraceStubScorer{prediction: "gpt-5.4", probability: &prob}
	svc.probeFn = func(context.Context, *Account, string, string, string) (string, error) {
		return makeValidNumberReply(250), nil
	}
	svc.runAccountCycle(context.Background(), repo.account, cfg)
	saved := DecodeModelTraceConfig(repo.account.Extra)
	require.NotNil(t, saved.Models["first"].Latest)
	require.NotNil(t, saved.Models["second"].Latest)
	require.Len(t, repo.readDeadlines, 6)
	for _, start := range []int{0, 3} {
		require.True(t, repo.readDeadlines[start].IsZero(), "account cycle must not impose a shared model deadline")
		require.True(t, repo.readDeadlines[start+1].IsZero(), "model probe must not impose a local deadline")
		require.WithinDuration(t, time.Now().Add(5*time.Second), repo.readDeadlines[start+2], time.Second)
	}
}

func TestModelTraceSubmittedTaskSurvivesRequestCancellation(t *testing.T) {
	cfg := &ModelTraceAccountConfig{Models: map[string]*ModelTraceModelConfig{"test": {}}}
	account := &Account{ID: 31, Extra: map[string]any{ModelTraceExtraKey: cfg}}
	repo := &modelTraceContextRepo{modelTraceTestRepo: modelTraceTestRepo{account: account, accounts: []Account{*account}}}
	svc := NewModelTraceService(repo, &modelTraceTestSettings{}, nil, nil)
	defer svc.Stop()
	unlock, err := svc.acquireAccountLock(context.Background(), 31)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task, err := svc.SubmitProbe(ctx, 31, "test")
	require.NoError(t, err)
	require.Equal(t, "queued", task.Status)
	duplicate, err := svc.SubmitProbe(ctx, 31, "test")
	require.NoError(t, err)
	require.Equal(t, task.ID, duplicate.ID)
	items, err := svc.ListMonitors(ctx)
	require.NoError(t, err)
	require.Equal(t, task.ID, items[0].Models["test"].Task.ID)
	cancel() // Closing the browser must not cancel the accepted task.
	unlock()
	var finished ModelTraceTask
	require.Eventually(t, func() bool {
		svc.tasksMu.Lock()
		defer svc.tasksMu.Unlock()
		finished = svc.tasks[modelTraceTaskKey{31, "test"}]
		return finished.Status == "failed" || finished.Status == "completed"
	}, time.Second, time.Millisecond)
	// The test intentionally has no upstream service; its failure is not HTTP cancellation.
	require.Equal(t, "failed", finished.Status)
	require.NotEqual(t, context.Canceled.Error(), finished.Error)
	require.NotNil(t, repo.updates)
	require.NotNil(t, DecodeModelTraceConfig(repo.account.Extra).Models["test"].Latest)
}
