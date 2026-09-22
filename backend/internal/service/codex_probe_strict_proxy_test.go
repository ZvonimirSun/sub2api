package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockCodexAccountRepo struct {
	AccountRepository
	accounts map[int64]*Account
}

func (m *mockCodexAccountRepo) GetByID(ctx context.Context, id int64) (*Account, error) {
	if acc, ok := m.accounts[id]; ok {
		return acc, nil
	}
	return nil, nil
}

func (m *mockCodexAccountRepo) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	var list []Account
	for _, a := range m.accounts {
		if a.Platform == platform {
			list = append(list, *a)
		}
	}
	return list, nil
}

func (m *mockCodexAccountRepo) UpdateExtra(ctx context.Context, id int64, extra map[string]any) error {
	if acc, ok := m.accounts[id]; ok {
		acc.Extra = extra
	}
	return nil
}

type mockCodexSettingRepo struct {
	SettingRepository
	values map[string]string
}

func (m *mockCodexSettingRepo) GetValue(ctx context.Context, key string) (string, error) {
	if val, ok := m.values[key]; ok {
		return val, nil
	}
	return "", nil
}

func TestCodexProbeClient_StrictProxyRequirement(t *testing.T) {
	client := NewCodexProbeClient()
	account := &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "test-token",
		},
	}

	// 1. 空代理地址必须被拒绝
	_, err := client.Probe(context.Background(), account, "gpt-5.4-orion", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "probe proxy is required")

	// 2. 纯空格代理地址必须被拒绝
	_, err = client.Probe(context.Background(), account, "gpt-5.4-orion", "   ")
	require.Error(t, err)
	require.Contains(t, err.Error(), "probe proxy is required")

	// 3. 无 Host 的非法代理必须被拒绝
	_, err = client.Probe(context.Background(), account, "gpt-5.4-orion", "http://")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid probe proxy")
}

func TestCodexTurnStateService_ProbeRequiresConfiguredProxyPool(t *testing.T) {
	ctx := context.Background()

	// 账号设置了自身代理 account.Proxy，但未配置系统探针专用代理池
	accRepo := &mockCodexAccountRepo{
		accounts: map[int64]*Account{
			1: {
				ID:       1,
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Status:   StatusActive,
				Credentials: map[string]any{
					"access_token": "test-token",
				},
				Extra: map[string]any{
					CodexTurnStateMonitoredModelsExtraKey: map[string]any{
						"gpt-5.4-orion": map[string]any{
							"enabled": true,
						},
					},
				},
			},
		},
	}

	// 探针代理池未配置（空）
	settingRepo := &mockCodexSettingRepo{
		values: map[string]string{},
	}

	svc := NewCodexTurnStateService(accRepo, settingRepo, nil)

	// 1. 验证 GetState 的 Proxy 计数为 0
	overview, err := svc.GetState(ctx)
	require.NoError(t, err)
	require.Equal(t, 0, overview.StaticProxyCount)
	require.Equal(t, 0, overview.DynamicProxyCount)

	// 2. 探针调用必须被拦截并返回错误
	res, probeErr := svc.ProbeAccountModel(ctx, 1, "gpt-5.4-orion", true)
	require.Nil(t, res)
	require.Error(t, probeErr)
	require.Contains(t, probeErr.Error(), "no probe proxies configured in pool")
}

func TestCodexTurnStateService_GetState_CountsProxiesCorrectly(t *testing.T) {
	ctx := context.Background()
	accRepo := &mockCodexAccountRepo{
		accounts: map[int64]*Account{},
	}
	settingRepo := &mockCodexSettingRepo{
		values: map[string]string{
			SettingKeyCodexTurnStateProxies: `{"static_proxies":["http://1.1.1.1:8080","http://2.2.2.2:8080"],"dynamic_proxies":["http://dyn.example.com:8080"]}`,
		},
	}

	svc := NewCodexTurnStateService(accRepo, settingRepo, nil)
	overview, err := svc.GetState(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, overview.StaticProxyCount)
	require.Equal(t, 1, overview.DynamicProxyCount)
}

func TestApplyPinnedCodexTurnState_IgnoresUnmonitoredOrExpiredResidualStates(t *testing.T) {
	// 场景 1：数据库中有历史残留的 state，但该账号没有配置 monitored_models
	past := time.Now().Add(-24 * time.Hour)
	accUnmonitored := &Account{
		ID:       10,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Extra: map[string]any{
			PinnedCodexTurnStatesExtraKey: map[string]any{
				"gpt-6-astra": map[string]any{
					"state":      "legacy-state-value",
					"expires_at": past.Format(time.RFC3339),
				},
			},
		},
	}

	headers := make(http.Header)
	applied := applyPinnedCodexTurnState(headers, accUnmonitored, "gpt-6-astra")
	require.False(t, applied, "未配置监控槽位时，历史残留数据绝对不能被应用")
	require.Empty(t, headers.Get(openAICodexTurnStateHeader))

	// 场景 2：配置了监控槽位但 enabled = false（已暂停）
	accPaused := &Account{
		ID:       11,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Extra: map[string]any{
			CodexTurnStateMonitoredModelsExtraKey: map[string]any{
				"gpt-6-astra": map[string]any{
					"enabled": false,
				},
			},
			PinnedCodexTurnStatesExtraKey: map[string]any{
				"gpt-6-astra": map[string]any{
					"state": "some-state",
				},
			},
		},
	}
	headers = make(http.Header)
	applied = applyPinnedCodexTurnState(headers, accPaused, "gpt-6-astra")
	require.False(t, applied, "槽位已暂停时，不能被应用")
	require.Empty(t, headers.Get(openAICodexTurnStateHeader))

	// 场景 3：配置了监控且 enabled = true，但 state 的 expires_at 已过期
	accExpired := &Account{
		ID:       12,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Extra: map[string]any{
			CodexTurnStateMonitoredModelsExtraKey: map[string]any{
				"gpt-6-astra": map[string]any{
					"enabled": true,
				},
			},
			PinnedCodexTurnStatesExtraKey: map[string]any{
				"gpt-6-astra": map[string]any{
					"state":      "some-state",
					"expires_at": past.Format(time.RFC3339),
				},
			},
		},
	}
	headers = make(http.Header)
	applied = applyPinnedCodexTurnState(headers, accExpired, "gpt-6-astra")
	require.False(t, applied, "State 已过期时，不能被应用")
	require.Empty(t, headers.Get(openAICodexTurnStateHeader))
}
