package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

const (
	SettingKeyCodexTurnStateProxies = "codex_turn_state_proxies"
	MaxProbeHistoryPerSlot          = 50
	DefaultFailureBackoffSeconds    = 300
)

// CodexProbeHistoryItem 记录单次探针调用的结果与耗时。
type CodexProbeHistoryItem struct {
	At                time.Time `json:"at"`
	AccountID         int64     `json:"account_id"`
	Model             string    `json:"model"`
	StatusCode        int       `json:"status_code"`
	LatencyMs         int64     `json:"latency_ms"`
	Success           bool      `json:"success"`
	StateLen          int       `json:"state_len"`
	TargetStateLen    int       `json:"target_state_len"`
	ProxyUsed         string    `json:"proxy_used,omitempty"`
	ErrorMessage      string    `json:"error_message,omitempty"`
	RetryAfterSeconds int       `json:"retry_after_seconds,omitempty"`
}

// CodexSlotStats 统计槽位的调用指标。
type CodexSlotStats struct {
	TotalProbes int64 `json:"total_probes"`
	Successes   int64 `json:"successes"`
	Failures    int64 `json:"failures"`
	LastLatency int64 `json:"last_latency_ms"`
	AvgLatency  int64 `json:"avg_latency_ms"`
}

// CodexModelSlotStatus 描述单个账号下某个模型的监控状态。
type CodexModelSlotStatus struct {
	Model                 string                    `json:"model"`
	Enabled               bool                      `json:"enabled"`
	TargetStateLen        int                       `json:"target_state_len"`
	RefreshAdvanceMinutes int                       `json:"refresh_advance_minutes"`
	CookieRenewalSeconds  int                       `json:"cookie_renewal_seconds"`
	StateInspection       *CodexTurnStateInspection `json:"inspection,omitempty"`
	DegradationStatus     string                    `json:"degradation_status"` // no_record, handled, degraded, unknown
	RecentDegradation     *DegradedAccount          `json:"recent_degradation,omitempty"`
	InCooldown            bool                      `json:"in_cooldown"`
	CooldownUntil         *time.Time                `json:"cooldown_until,omitempty"`
	LastProbe             *CodexProbeHistoryItem    `json:"last_probe,omitempty"`
	Cflb                  string                    `json:"cflb,omitempty"`
	Oailb                 string                    `json:"oailb,omitempty"`
	CookieAgeSeconds      int64                     `json:"cookie_age_seconds,omitempty"`
	CookieRemainingSec    int64                     `json:"cookie_remaining_seconds,omitempty"`
}

// CodexAccountState 包含一个账号及其所有受监控模型的状态列表。
type CodexAccountState struct {
	AccountID   int64                  `json:"account_id"`
	AccountName string                 `json:"account_name"`
	Platform    string                 `json:"platform"`
	Type        string                 `json:"type"`
	Status      string                 `json:"status"`
	Models      []CodexModelSlotStatus `json:"models"`
}

// CodexTurnStateOverview 提供整个监控系统的全景概览。
type CodexTurnStateOverview struct {
	TotalAccounts     int                 `json:"total_accounts"`
	TotalModels       int                 `json:"total_models"`
	HealthyModels     int                 `json:"healthy_models"`
	ExpiringModels    int                 `json:"expiring_models"`
	DegradedModels    int                 `json:"degraded_models"`
	StaticProxyCount  int                 `json:"static_proxy_count"`
	DynamicProxyCount int                 `json:"dynamic_proxy_count"`
	Accounts          []CodexAccountState `json:"accounts"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

// CodexTurnStateProxyConfig 管理探针专用配置（代理池与自动续期参数）。
type CodexTurnStateProxyConfig struct {
	StaticProxies        []string `json:"static_proxies"`
	DynamicProxies       []string `json:"dynamic_proxies"`
	CookieRenewalSeconds int      `json:"cookie_renewal_seconds,omitempty"`
}

// CodexTurnStateService 是 Codex Turn-State 续期与监控的主业务服务。
type CodexTurnStateService struct {
	accountRepo AccountRepository
	settingRepo SettingRepository
	probeClient *CodexProbeClient
	sqlDB       *sql.DB

	mu                sync.RWMutex
	cooldowns         map[string]time.Time               // key: "accountID:model" -> cooldown deadline
	history           map[string][]CodexProbeHistoryItem // key: "accountID:model" -> history items (ring buffer)
	stats             map[string]*CodexSlotStats         // key: "accountID:model" -> statistics
	staticProxyIndex  uint64                             // atomic round-robin index for static proxy pool
	dynamicProxyIndex uint64                             // atomic round-robin index for dynamic proxy pool
}

func NewCodexTurnStateService(
	accountRepo AccountRepository,
	settingRepo SettingRepository,
	sqlDB *sql.DB,
) *CodexTurnStateService {
	return &CodexTurnStateService{
		accountRepo: accountRepo,
		settingRepo: settingRepo,
		probeClient: NewCodexProbeClient(),
		sqlDB:       sqlDB,
		cooldowns:   make(map[string]time.Time),
		history:     make(map[string][]CodexProbeHistoryItem),
		stats:       make(map[string]*CodexSlotStats),
	}
}

func slotKey(accountID int64, model string) string {
	return fmt.Sprintf("%d:%s", accountID, strings.ToLower(strings.TrimSpace(model)))
}

// GetState 汇总所有受监控账号的当前状态、到期倒计时与降级判定。
func (s *CodexTurnStateService) GetState(ctx context.Context) (*CodexTurnStateOverview, error) {
	if s.accountRepo == nil {
		return nil, errors.New("account repository unavailable")
	}

	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	degradedList, _ := s.GetDegraded(ctx, DegradedAccountsDefaultWindow)
	degradedMap := make(map[string]DegradedAccount, len(degradedList))
	for _, d := range degradedList {
		key := slotKey(d.AccountID, d.SentModel)
		if prev, ok := degradedMap[key]; !ok || d.LastSeen.After(prev.LastSeen) {
			degradedMap[key] = d
		}
	}

	now := time.Now().UTC()
	proxyCfg, _ := s.GetProxyConfig(ctx)
	staticCount := 0
	dynamicCount := 0
	if proxyCfg != nil {
		staticCount = len(proxyCfg.StaticProxies)
		dynamicCount = len(proxyCfg.DynamicProxies)
	}

	overview := &CodexTurnStateOverview{
		Accounts:          make([]CodexAccountState, 0),
		UpdatedAt:         now,
		StaticProxyCount:  staticCount,
		DynamicProxyCount: dynamicCount,
	}

	for _, acc := range accounts {
		if !acc.IsOpenAIOAuthLike() || acc.Extra == nil {
			continue
		}
		monitored := acc.GetCodexMonitoredModels()
		if len(monitored) == 0 {
			continue
		}

		pinnedStates := acc.GetPinnedCodexTurnStates()
		accState := CodexAccountState{
			AccountID:   acc.ID,
			AccountName: acc.Name,
			Platform:    acc.Platform,
			Type:        acc.Type,
			Status:      acc.Status,
			Models:      make([]CodexModelSlotStatus, 0, len(monitored)),
		}

		for modelName, mcfg := range monitored {
			key := slotKey(acc.ID, modelName)
			effectiveRenewalSec := mcfg.CookieRenewalSeconds
			if effectiveRenewalSec <= 0 && proxyCfg != nil {
				effectiveRenewalSec = proxyCfg.CookieRenewalSeconds
			}
			if effectiveRenewalSec <= 0 {
				effectiveRenewalSec = DefaultCodexCookieRenewalAge
			}

			slot := CodexModelSlotStatus{
				Model:                 modelName,
				Enabled:               mcfg.Enabled,
				TargetStateLen:        mcfg.TargetStateLen,
				RefreshAdvanceMinutes: mcfg.RefreshAdvanceMinutes,
				CookieRenewalSeconds:  effectiveRenewalSec,
				DegradationStatus:     "no_record",
			}

			// 读取并解析 Turn-State 与 LB Cookie
			var pinnedUpdated time.Time
			if entry, ok := pinnedStates[modelName]; ok && entry.State != "" {
				insp, _ := InspectCodexTurnState(entry.State)
				slot.StateInspection = insp
				slot.Cflb = entry.Cflb
				slot.Oailb = entry.Oailb
				if entry.UpdatedAt != nil {
					pinnedUpdated = *entry.UpdatedAt
					cookieAge := int64(now.Sub(pinnedUpdated).Seconds())
					if cookieAge < 0 {
						cookieAge = 0
					}
					slot.CookieAgeSeconds = cookieAge
					rem := int64(effectiveRenewalSec) - cookieAge
					if rem < 0 {
						rem = 0
					}
					slot.CookieRemainingSec = rem
				}
			}

			// 关联降级审计
			if deg, ok := degradedMap[key]; ok {
				degCopy := deg
				slot.RecentDegradation = &degCopy
				if pinnedUpdated.IsZero() {
					slot.DegradationStatus = "degraded"
				} else if deg.LastSeen.After(pinnedUpdated) {
					slot.DegradationStatus = "degraded"
				} else {
					slot.DegradationStatus = "handled"
				}
			}

			// 检查冷却状态
			s.mu.RLock()
			if dl, ok := s.cooldowns[key]; ok && dl.After(now) {
				slot.InCooldown = true
				dlCopy := dl
				slot.CooldownUntil = &dlCopy
			}
			if hist := s.history[key]; len(hist) > 0 {
				lastCopy := hist[len(hist)-1]
				slot.LastProbe = &lastCopy
			}
			s.mu.RUnlock()

			// 统计指标
			overview.TotalModels++
			if slot.DegradationStatus == "degraded" {
				overview.DegradedModels++
			}
			if slot.StateInspection != nil && slot.StateInspection.Valid && !slot.StateInspection.IsExpired {
				if slot.StateInspection.RemainingSeconds <= mcfg.RefreshAdvanceMinutes*60 {
					overview.ExpiringModels++
				} else {
					overview.HealthyModels++
				}
			}

			accState.Models = append(accState.Models, slot)
		}

		overview.Accounts = append(overview.Accounts, accState)
		overview.TotalAccounts++
	}

	return overview, nil
}

// AddMonitoredModel 为指定账号添加或更新一个模型的监控配置。
func (s *CodexTurnStateService) AddMonitoredModel(ctx context.Context, accountID int64, model string, targetLen int, advanceMinutes int, cookieRenewalSeconds int) error {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return errors.New("model name cannot be empty")
	}
	if targetLen <= 0 {
		targetLen = 292
	}
	if advanceMinutes <= 0 {
		advanceMinutes = 15
	}

	acc, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}
	if acc == nil || !acc.IsOpenAIOAuthLike() {
		return errors.New("account is not an active OpenAI OAuth-like account")
	}

	extra := maps.Clone(acc.Extra)
	if extra == nil {
		extra = make(map[string]any)
	}

	var monitored map[string]any
	if raw, ok := extra[CodexTurnStateMonitoredModelsExtraKey].(map[string]any); ok && raw != nil {
		monitored = maps.Clone(raw)
	} else {
		monitored = make(map[string]any)
	}

	modelMap := map[string]any{
		"enabled":                 true,
		"target_state_len":        targetLen,
		"refresh_advance_minutes": advanceMinutes,
	}
	if cookieRenewalSeconds > 0 {
		modelMap["cookie_renewal_seconds"] = cookieRenewalSeconds
	}
	monitored[model] = modelMap
	extra[CodexTurnStateMonitoredModelsExtraKey] = monitored

	return s.accountRepo.UpdateExtra(ctx, accountID, extra)
}

// RemoveMonitoredModel 移除指定账号下指定模型的监控配置。
func (s *CodexTurnStateService) RemoveMonitoredModel(ctx context.Context, accountID int64, model string) error {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return errors.New("model name cannot be empty")
	}

	acc, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}
	if acc == nil || acc.Extra == nil {
		return nil
	}

	raw, ok := acc.Extra[CodexTurnStateMonitoredModelsExtraKey].(map[string]any)
	if !ok || raw == nil {
		return nil
	}

	extra := maps.Clone(acc.Extra)
	monitored := maps.Clone(raw)
	delete(monitored, model)
	if len(monitored) == 0 {
		delete(extra, CodexTurnStateMonitoredModelsExtraKey)
	} else {
		extra[CodexTurnStateMonitoredModelsExtraKey] = monitored
	}

	return s.accountRepo.UpdateExtra(ctx, accountID, extra)
}

// ToggleMonitoredModel 启用或暂停某个模型的监控。
func (s *CodexTurnStateService) ToggleMonitoredModel(ctx context.Context, accountID int64, model string, enabled bool) error {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return errors.New("model name cannot be empty")
	}

	acc, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}
	if acc == nil || acc.Extra == nil {
		return errors.New("account not found or has no extra configuration")
	}

	raw, ok := acc.Extra[CodexTurnStateMonitoredModelsExtraKey].(map[string]any)
	if !ok || raw == nil {
		return errors.New("account has no monitored models")
	}

	slotRaw, ok := raw[model]
	if !ok {
		return fmt.Errorf("model %s not found in monitored models", model)
	}

	extra := maps.Clone(acc.Extra)
	monitored := maps.Clone(raw)
	slotMap := make(map[string]any)
	if m, ok := slotRaw.(map[string]any); ok {
		slotMap = maps.Clone(m)
	}
	slotMap["enabled"] = enabled
	monitored[model] = slotMap
	extra[CodexTurnStateMonitoredModelsExtraKey] = monitored

	return s.accountRepo.UpdateExtra(ctx, accountID, extra)
}

// ProbeAccountModel 对账号的指定模型执行一次探针发包与状态落库。
func (s *CodexTurnStateService) ProbeAccountModel(ctx context.Context, accountID int64, model string, force bool) (*CodexProbeResult, error) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return nil, errors.New("model is empty")
	}

	acc, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	if acc == nil || !acc.IsOpenAIOAuthLike() {
		return nil, errors.New("account not found or not OpenAI OAuth")
	}

	key := slotKey(accountID, model)
	now := time.Now().UTC()

	s.mu.RLock()
	if !force {
		if dl, ok := s.cooldowns[key]; ok && dl.After(now) {
			s.mu.RUnlock()
			return nil, fmt.Errorf("slot in cooldown until %s", dl.Format(time.RFC3339))
		}
	}
	s.mu.RUnlock()

	// 获取代理配置，执行“静态优先，动态兜底轮询”
	proxyCfg, _ := s.GetProxyConfig(ctx)
	if proxyCfg == nil || (len(proxyCfg.StaticProxies) == 0 && len(proxyCfg.DynamicProxies) == 0) {
		return nil, errors.New("no probe proxies configured in pool")
	}

	var result *CodexProbeResult
	var probeErr error

	// 阶段一：静态代理优先尝试
	staticProxy := s.selectStaticProxy(proxyCfg)
	if staticProxy != "" {
		result, probeErr = s.probeClient.Probe(ctx, acc, model, staticProxy)
	}

	// 阶段二：若未配置静态代理、或静态代理探测未成功（超时/429/报错），且配置了动态网关，则执行动态网关兜底轮询
	isStaticSuccess := result != nil && result.StatusCode == 200 && result.TurnState != ""
	if !isStaticSuccess && len(proxyCfg.DynamicProxies) > 0 {
		dynamicProxy := s.selectDynamicProxy(proxyCfg)
		if dynamicProxy != "" {
			dynResult, dynErr := s.probeClient.Probe(ctx, acc, model, dynamicProxy)
			if dynResult != nil {
				if staticProxy != "" {
					dynResult.ProxyUsed += " (fallback)"
				}
				result = dynResult
				probeErr = dynErr
			}
		}
	}

	if result == nil {
		result = &CodexProbeResult{
			StatusCode:   0,
			ErrorMessage: "probe execution failed",
		}
		if probeErr != nil {
			result.ErrorMessage = probeErr.Error()
		}
	}

	success := result.StatusCode == 200 && result.TurnState != ""

	// 记录历史与统计指标
	s.recordProbeOutcome(accountID, model, result, success)

	if success {
		// 校验解析 State
		insp, inspErr := InspectCodexTurnState(result.TurnState)
		if inspErr == nil && insp != nil && insp.Valid {
			// 将新 State 与 __cflb/__oailb Cookie 合并保存到账号 Extra 中
			if saveErr := s.saveTurnState(ctx, accountID, model, result, insp); saveErr != nil {
				logger.LegacyPrintf("service.codex_turn_state", "failed to persist pinned turn-state: account_id=%d model=%s err=%v", accountID, model, saveErr)
			}
		}
		// 成功时清除退避冷却
		s.mu.Lock()
		delete(s.cooldowns, key)
		s.mu.Unlock()
	} else {
		// 失败时设置退避冷却时间
		cooldownSeconds := DefaultFailureBackoffSeconds
		if result.RetryAfterSeconds > 0 {
			cooldownSeconds = result.RetryAfterSeconds
		}
		s.mu.Lock()
		s.cooldowns[key] = now.Add(time.Duration(cooldownSeconds) * time.Second)
		s.mu.Unlock()
	}

	return result, probeErr
}

func (s *CodexTurnStateService) saveTurnState(ctx context.Context, accountID int64, model string, res *CodexProbeResult, insp *CodexTurnStateInspection) error {
	acc, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || acc == nil {
		return fmt.Errorf("reload account for saving state: %w", err)
	}

	extra := maps.Clone(acc.Extra)
	if extra == nil {
		extra = make(map[string]any)
	}

	var pins map[string]any
	if raw, ok := extra[PinnedCodexTurnStatesExtraKey].(map[string]any); ok && raw != nil {
		pins = maps.Clone(raw)
	} else {
		pins = make(map[string]any)
	}

	now := time.Now().UTC()
	pins[model] = map[string]any{
		"state":      res.TurnState,
		"state_len":  len(res.TurnState),
		"cflb":       res.CflbCookie,
		"oailb":      res.OailbCookie,
		"issued_at":  insp.IssuedAt.Format(time.RFC3339Nano),
		"expires_at": insp.ExpiresAt.Format(time.RFC3339Nano),
		"updated_at": now.Format(time.RFC3339Nano),
	}
	extra[PinnedCodexTurnStatesExtraKey] = pins

	return s.accountRepo.UpdateExtra(ctx, accountID, extra)
}

func (s *CodexTurnStateService) recordProbeOutcome(accountID int64, model string, res *CodexProbeResult, success bool) {
	key := slotKey(accountID, model)
	item := CodexProbeHistoryItem{
		At:                time.Now().UTC(),
		AccountID:         accountID,
		Model:             model,
		StatusCode:        res.StatusCode,
		LatencyMs:         res.LatencyMs,
		Success:           success,
		StateLen:          len(res.TurnState),
		ProxyUsed:         res.ProxyUsed,
		ErrorMessage:      res.ErrorMessage,
		RetryAfterSeconds: res.RetryAfterSeconds,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 环形队列记录最近 50 条历史
	list := s.history[key]
	if len(list) >= MaxProbeHistoryPerSlot {
		list = list[1:]
	}
	s.history[key] = append(list, item)

	// 更新统计
	st, ok := s.stats[key]
	if !ok {
		st = &CodexSlotStats{}
		s.stats[key] = st
	}
	st.TotalProbes++
	if success {
		st.Successes++
	} else {
		st.Failures++
	}
	st.LastLatency = res.LatencyMs
	if st.TotalProbes > 0 {
		st.AvgLatency = (st.AvgLatency*(st.TotalProbes-1) + res.LatencyMs) / st.TotalProbes
	}
}

func (s *CodexTurnStateService) selectStaticProxy(cfg *CodexTurnStateProxyConfig) string {
	if cfg == nil || len(cfg.StaticProxies) == 0 {
		return ""
	}
	idx := atomic.AddUint64(&s.staticProxyIndex, 1) - 1
	return cfg.StaticProxies[idx%uint64(len(cfg.StaticProxies))]
}

func (s *CodexTurnStateService) selectDynamicProxy(cfg *CodexTurnStateProxyConfig) string {
	if cfg == nil || len(cfg.DynamicProxies) == 0 {
		return ""
	}
	idx := atomic.AddUint64(&s.dynamicProxyIndex, 1) - 1
	return cfg.DynamicProxies[idx%uint64(len(cfg.DynamicProxies))]
}

// normalizeProxyURL 标准化单条代理配置，支持标准 URL (http/https/socks5/socks5h) 与代理商导出的 host:port:user:pass、host:port 格式。
func normalizeProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") {
		return ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return ""
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" && scheme != "socks5" && scheme != "socks5h" {
			return ""
		}
		return raw
	}
	// 兼容常见代理商导出的 host:port:user:pass 格式
	parts := strings.Split(raw, ":")
	if len(parts) == 4 {
		host, port, user, pass := parts[0], parts[1], parts[2], parts[3]
		return fmt.Sprintf("http://%s:%s@%s:%s", url.QueryEscape(user), url.QueryEscape(pass), host, port)
	}
	if len(parts) == 2 {
		host, port := parts[0], parts[1]
		return fmt.Sprintf("http://%s:%s", host, port)
	}
	return ""
}

// GetProxyConfig 读取探针专用代理池配置（包含静态优先池与动态网关兜底池）。
func (s *CodexTurnStateService) GetProxyConfig(ctx context.Context) (*CodexTurnStateProxyConfig, error) {
	cfg := &CodexTurnStateProxyConfig{
		StaticProxies:        make([]string, 0),
		DynamicProxies:       make([]string, 0),
		CookieRenewalSeconds: DefaultCodexCookieRenewalAge,
	}
	if s.settingRepo == nil {
		return cfg, nil
	}
	val, err := s.settingRepo.GetValue(ctx, SettingKeyCodexTurnStateProxies)
	if err != nil || strings.TrimSpace(val) == "" {
		return cfg, nil
	}

	// 优先解析包含 static_proxies 与 dynamic_proxies 的 JSON 对象结构
	if err := json.Unmarshal([]byte(val), cfg); err == nil && (len(cfg.StaticProxies) > 0 || len(cfg.DynamicProxies) > 0 || cfg.CookieRenewalSeconds > 0) {
		if cfg.CookieRenewalSeconds <= 0 {
			cfg.CookieRenewalSeconds = DefaultCodexCookieRenewalAge
		}
		return cfg, nil
	}

	// 兼容旧格式：纯数组 []string 解析为 StaticProxies
	var list []string
	if err := json.Unmarshal([]byte(val), &list); err == nil {
		for _, item := range list {
			if norm := normalizeProxyURL(item); norm != "" {
				cfg.StaticProxies = append(cfg.StaticProxies, norm)
			}
		}
		return cfg, nil
	}

	// 兼容旧格式：换行符纯文本
	for _, line := range strings.Split(val, "\n") {
		if norm := normalizeProxyURL(line); norm != "" {
			cfg.StaticProxies = append(cfg.StaticProxies, norm)
		}
	}
	return cfg, nil
}

// SetProxyConfig 保存探针专用代理池配置，支持静态优先池与动态兜底池独立存储。
func (s *CodexTurnStateService) SetProxyConfig(ctx context.Context, cfg *CodexTurnStateProxyConfig) error {
	if s.settingRepo == nil {
		return errors.New("setting repository unavailable")
	}
	cleaned := &CodexTurnStateProxyConfig{
		StaticProxies:        make([]string, 0),
		DynamicProxies:       make([]string, 0),
		CookieRenewalSeconds: DefaultCodexCookieRenewalAge,
	}
	if cfg != nil {
		if cfg.CookieRenewalSeconds > 0 {
			cleaned.CookieRenewalSeconds = cfg.CookieRenewalSeconds
		}
		for _, p := range cfg.StaticProxies {
			if norm := normalizeProxyURL(p); norm != "" {
				cleaned.StaticProxies = append(cleaned.StaticProxies, norm)
			}
		}
		for _, p := range cfg.DynamicProxies {
			if norm := normalizeProxyURL(p); norm != "" {
				cleaned.DynamicProxies = append(cleaned.DynamicProxies, norm)
			}
		}
	}
	data, err := json.Marshal(cleaned)
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, SettingKeyCodexTurnStateProxies, string(data))
}

// GetProxies 为向后兼容返回所有代理的扁平列表。
func (s *CodexTurnStateService) GetProxies(ctx context.Context) ([]string, error) {
	cfg, err := s.GetProxyConfig(ctx)
	if err != nil {
		return nil, err
	}
	combined := append(cfg.StaticProxies, cfg.DynamicProxies...)
	return combined, nil
}

// SetProxies 为向后兼容将数组保存为 StaticProxies。
func (s *CodexTurnStateService) SetProxies(ctx context.Context, proxies []string) error {
	cfg := &CodexTurnStateProxyConfig{
		StaticProxies: proxies,
	}
	return s.SetProxyConfig(ctx, cfg)
}

// GetHistory 获取最近探针执行历史。
func (s *CodexTurnStateService) GetHistory(_ context.Context) map[string][]CodexProbeHistoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]CodexProbeHistoryItem, len(s.history))
	for k, v := range s.history {
		items := make([]CodexProbeHistoryItem, len(v))
		copy(items, v)
		result[k] = items
	}
	return result
}

// GetStats 获取探针槽位统计。
func (s *CodexTurnStateService) GetStats(_ context.Context) map[string]CodexSlotStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]CodexSlotStats, len(s.stats))
	for k, v := range s.stats {
		result[k] = *v
	}
	return result
}

// ClearHistory 清除探测历史记录。若传入特定 accountID 与 model 则清除指定槽位，否则全量清除。
func (s *CodexTurnStateService) ClearHistory(accountID *int64, model string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	model = strings.ToLower(strings.TrimSpace(model))
	if accountID != nil && model != "" {
		key := slotKey(*accountID, model)
		delete(s.history, key)
		delete(s.stats, key)
		return
	}
	if accountID != nil {
		prefix := fmt.Sprintf("%d:", *accountID)
		for k := range s.history {
			if strings.HasPrefix(k, prefix) {
				delete(s.history, k)
				delete(s.stats, k)
			}
		}
		return
	}
	s.history = make(map[string][]CodexProbeHistoryItem)
	s.stats = make(map[string]*CodexSlotStats)
}

// GetDegraded 从 usage_logs 聚合分析近期的模型降级情况。
func (s *CodexTurnStateService) GetDegraded(ctx context.Context, window time.Duration) ([]DegradedAccount, error) {
	if s.sqlDB == nil {
		return nil, nil
	}
	window = ClampDegradedAccountsWindow(window)
	now := time.Now().UTC()
	start := now.Add(-window)

	query := `
WITH mismatches AS MATERIALIZED (
    SELECT
        ul.account_id                                              AS account_id,
        COALESCE(NULLIF(BTRIM(ul.requested_model), ''), ul.model)  AS requested_model,
        COALESCE(NULLIF(BTRIM(ul.upstream_model), ''), ul.model)   AS sent_model,
        BTRIM(ul.upstream_response_model)                          AS response_model,
        COUNT(*)::bigint                                           AS occurrences,
        MIN(ul.created_at)                                         AS first_seen,
        MAX(ul.created_at)                                         AS last_seen,
        ROUND(AVG(ul.first_token_ms)::numeric, 2)::float8          AS ttft_avg_ms
    FROM usage_logs ul
    WHERE ul.upstream_model_mismatch IS TRUE
      AND ul.created_at >= $1
      AND ul.created_at < $2
      AND ul.account_id > 0
      AND NULLIF(BTRIM(ul.upstream_response_model), '') IS NOT NULL
    GROUP BY 1, 2, 3, 4
)
SELECT
    m.account_id,
    a.name,
    m.requested_model,
    m.sent_model,
    m.response_model,
    m.occurrences,
    m.first_seen,
    m.last_seen,
    m.ttft_avg_ms
FROM mismatches m
JOIN accounts a ON a.id = m.account_id
WHERE a.platform = $3
  AND a.deleted_at IS NULL
ORDER BY m.occurrences DESC, m.last_seen DESC, m.account_id ASC
LIMIT 500`

	rows, err := s.sqlDB.QueryContext(ctx, query, start, now, PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []DegradedAccount
	for rows.Next() {
		var row DegradedAccount
		if err := rows.Scan(
			&row.AccountID,
			&row.AccountName,
			&row.RequestedModel,
			&row.SentModel,
			&row.ResponseModel,
			&row.Count,
			&row.FirstSeen,
			&row.LastSeen,
			&row.TTFTAvgMs,
		); err != nil {
			continue
		}
		if ModelDegradationSuspected(row.SentModel, row.ResponseModel) {
			result = append(result, row)
		}
	}
	return result, rows.Err()
}
