package service

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/http/httpguts"
)

const (
	PinnedCodexTurnStatesExtraKey         = "pinned_codex_turn_states"
	CodexTurnStateMonitoredModelsExtraKey = "codex_turn_state_monitored_models"

	CodexCookieMaxAgeSeconds     = 240 // __cflb 与 __oailb 参考寿命上限（非阻塞）
	CodexCookieRenewalAge        = 150 // 默认提前触发续期的年龄阈值
	DefaultCodexCookieRenewalAge = 150 // 默认提前触发续期的年龄阈值
)

// PinnedCodexTurnStateEntry 表示固化在账号 Extra 中的一个模型的 turn-state 数据。
type PinnedCodexTurnStateEntry struct {
	State     string     `json:"state"`
	Cflb      string     `json:"cflb,omitempty"`
	Oailb     string     `json:"oailb,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	IssuedAt  *time.Time `json:"issued_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	StateLen  int        `json:"state_len,omitempty"`
}

// CodexMonitoredModelConfig 表示账号配置的主动探针监控模型参数。
type CodexMonitoredModelConfig struct {
	Enabled               bool `json:"enabled"`
	TargetStateLen        int  `json:"target_state_len,omitempty"`
	RefreshAdvanceMinutes int  `json:"refresh_advance_minutes,omitempty"`
	CookieRenewalSeconds  int  `json:"cookie_renewal_seconds,omitempty"`
}

// GetCodexMonitoredModels 返回该账号配置的受监控模型映射表。
func (a *Account) GetCodexMonitoredModels() map[string]CodexMonitoredModelConfig {
	if a == nil || !a.IsOpenAIOAuthLike() || a.Extra == nil {
		return nil
	}
	raw, ok := a.Extra[CodexTurnStateMonitoredModelsExtraKey]
	if !ok || raw == nil {
		return nil
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]CodexMonitoredModelConfig, len(rawMap))
	for k, v := range rawMap {
		modelKey := strings.ToLower(strings.TrimSpace(k))
		if modelKey == "" {
			continue
		}
		cfg := parseCodexMonitoredModelConfig(v)
		if cfg.Enabled {
			result[modelKey] = cfg
		}
	}
	return result
}

func parseCodexMonitoredModelConfig(v any) CodexMonitoredModelConfig {
	cfg := CodexMonitoredModelConfig{
		Enabled:               true,
		TargetStateLen:        292,
		RefreshAdvanceMinutes: 15,
	}
	switch val := v.(type) {
	case bool:
		cfg.Enabled = val
	case map[string]any:
		if e, ok := val["enabled"].(bool); ok {
			cfg.Enabled = e
		}
		if sl, ok := val["target_state_len"].(float64); ok && sl > 0 {
			cfg.TargetStateLen = int(sl)
		} else if sl, ok := val["target_state_len"].(int); ok && sl > 0 {
			cfg.TargetStateLen = sl
		}
		if adv, ok := val["refresh_advance_minutes"].(float64); ok && adv >= 0 {
			cfg.RefreshAdvanceMinutes = int(adv)
		} else if adv, ok := val["refresh_advance_minutes"].(int); ok && adv >= 0 {
			cfg.RefreshAdvanceMinutes = adv
		}
		if crs, ok := val["cookie_renewal_seconds"].(float64); ok && crs > 0 {
			cfg.CookieRenewalSeconds = int(crs)
		} else if crs, ok := val["cookie_renewal_seconds"].(int); ok && crs > 0 {
			cfg.CookieRenewalSeconds = crs
		}
	}
	return cfg
}

// GetPinnedCodexTurnStates 返回该账号配置的所有 pinned turn-state 映射表。
func (a *Account) GetPinnedCodexTurnStates() map[string]PinnedCodexTurnStateEntry {
	if a == nil || !a.IsOpenAIOAuthLike() || a.Extra == nil {
		return nil
	}
	raw, ok := a.Extra[PinnedCodexTurnStatesExtraKey]
	if !ok || raw == nil {
		return nil
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]PinnedCodexTurnStateEntry, len(rawMap))
	for k, v := range rawMap {
		modelKey := strings.ToLower(strings.TrimSpace(k))
		if modelKey == "" {
			continue
		}
		entry := parsePinnedCodexTurnStateEntry(v)
		if entry.State != "" {
			result[modelKey] = entry
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// GetPinnedCodexTurnStateEntry 返回匹配指定 model 的未过期 pinned state 完整记录。
// 若无配置、已过期、非 OpenAI OAuth 账号或不匹配则返回 nil。
func (a *Account) GetPinnedCodexTurnStateEntry(model string) *PinnedCodexTurnStateEntry {
	if a == nil || !a.IsOpenAIOAuthLike() || a.Extra == nil {
		return nil
	}
	normModel := strings.ToLower(strings.TrimSpace(model))
	if normModel == "" {
		return nil
	}
	states := a.GetPinnedCodexTurnStates()
	if len(states) == 0 {
		return nil
	}

	// 1. 精确匹配（最高优先级）
	if entry, ok := states[normModel]; ok {
		if isPinnedStateActive(entry) {
			entryCopy := entry
			return &entryCopy
		}
		return nil
	}

	// 2. 严格的带版本/日期快照别名匹配（按 pattern 长度降序，最长确定性命中）
	// 例如：配置了 gpt-6-astra，请求模型为 gpt-6-astra-20260301（后缀为数字日期）可以安全命中；
	// 但配置 gpt-6 绝不能命中 gpt-6-astra，配置 gpt-5 绝不能命中 gpt-5-codex。
	type candidate struct {
		pattern string
		entry   PinnedCodexTurnStateEntry
	}
	var matches []candidate
	for key, entry := range states {
		if matchesPinnedModelPattern(normModel, key) {
			matches = append(matches, candidate{pattern: key, entry: entry})
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Slice(matches, func(i, j int) bool {
		return len(matches[i].pattern) > len(matches[j].pattern)
	})
	for _, m := range matches {
		if isPinnedStateActive(m.entry) {
			entryCopy := m.entry
			return &entryCopy
		}
	}
	return nil
}

// GetPinnedCodexTurnState 返回匹配指定 model 的未过期 pinned state 字符串。
// 若无配置、已过期、非 OpenAI OAuth 账号或不匹配则返回空字符串。
func (a *Account) GetPinnedCodexTurnState(model string) string {
	entry := a.GetPinnedCodexTurnStateEntry(model)
	if entry != nil {
		return entry.State
	}
	return ""
}

func matchesPinnedModelPattern(model, pattern string) bool {
	if strings.EqualFold(model, pattern) {
		return true
	}
	// 标签匹配：model 为 pattern:tag（例如 gpt-6-astra:latest）
	if strings.HasPrefix(model, pattern+":") {
		return true
	}
	// 日期/版本快照匹配：model 必须以 pattern + "-" 开头，且紧随其后的字符必须是数字（如 -20260301）
	// 严格杜绝 gpt-6 匹配 gpt-6-astra，或 gpt-5 匹配 gpt-5-codex 的跨模型泄漏！
	if strings.HasPrefix(model, pattern+"-") {
		rem := strings.TrimPrefix(model, pattern+"-")
		if rem != "" && rem[0] >= '0' && rem[0] <= '9' {
			return true
		}
	}
	return false
}

func isPinnedStateActive(entry PinnedCodexTurnStateEntry) bool {
	if entry.State == "" {
		return false
	}
	now := time.Now()
	if entry.ExpiresAt != nil && !entry.ExpiresAt.IsZero() {
		if now.After(*entry.ExpiresAt) {
			return false
		}
	}
	// 深度过期校验：若 State 遵循标准 Fernet 格式，则直接校验内部真实时间戳（TTL 3600s）
	// 防止历史残留数据中缺少 expires_at 元数据导致将过期的 State 误判为有效
	if insp, err := InspectCodexTurnState(entry.State); err == nil && insp != nil && insp.Valid {
		if insp.IsExpired {
			return false
		}
	}
	return true
}

func parsePinnedCodexTurnStateEntry(v any) PinnedCodexTurnStateEntry {
	switch val := v.(type) {
	case string:
		state := strings.TrimSpace(val)
		return PinnedCodexTurnStateEntry{
			State:    state,
			StateLen: len(state),
		}
	case map[string]any:
		state, _ := val["state"].(string)
		state = strings.TrimSpace(state)
		cflb, _ := val["cflb"].(string)
		oailb, _ := val["oailb"].(string)
		entry := PinnedCodexTurnStateEntry{
			State:    state,
			Cflb:     strings.TrimSpace(cflb),
			Oailb:    strings.TrimSpace(oailb),
			StateLen: len(state),
		}
		if sl, ok := val["state_len"].(float64); ok && sl > 0 {
			entry.StateLen = int(sl)
		} else if sl, ok := val["state_len"].(int); ok && sl > 0 {
			entry.StateLen = sl
		}
		if exp := parseFlexibleTime(val["expires_at"]); exp != nil {
			entry.ExpiresAt = exp
		}
		if iss := parseFlexibleTime(val["issued_at"]); iss != nil {
			entry.IssuedAt = iss
		}
		if upd := parseFlexibleTime(val["updated_at"]); upd != nil {
			entry.UpdatedAt = upd
		}
		return entry
	default:
		return PinnedCodexTurnStateEntry{}
	}
}

// mergeCookies 将 newCookies 合并写入已有的 Cookie header 字符串中，剔除旧有重名 key 并追加新值。
func mergeCookies(existing string, newCookies map[string]string) string {
	parts := strings.Split(existing, ";")
	var kept []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		eqIdx := strings.IndexByte(trimmed, '=')
		if eqIdx <= 0 {
			kept = append(kept, trimmed)
			continue
		}
		name := strings.TrimSpace(trimmed[:eqIdx])
		if _, ok := newCookies[name]; !ok {
			kept = append(kept, trimmed)
		}
	}
	for name, val := range newCookies {
		if val != "" {
			kept = append(kept, name+"="+val)
		}
	}
	return strings.Join(kept, "; ")
}

// applyCodexLBCookies 将 entry 中的 __cflb 与 __oailb Cookie 安全写入请求头。
func applyCodexLBCookies(headers http.Header, entry *PinnedCodexTurnStateEntry) {
	if headers == nil || entry == nil {
		return
	}
	cookiesToSet := make(map[string]string)
	if entry.Cflb != "" && httpguts.ValidHeaderFieldValue(entry.Cflb) {
		cookiesToSet["__cflb"] = entry.Cflb
	}
	if entry.Oailb != "" && httpguts.ValidHeaderFieldValue(entry.Oailb) {
		cookiesToSet["__oailb"] = entry.Oailb
	}
	if len(cookiesToSet) == 0 {
		return
	}

	existing := headers.Get("Cookie")
	merged := mergeCookies(existing, cookiesToSet)
	if merged != "" && httpguts.ValidHeaderFieldValue(merged) {
		headers.Set("Cookie", merged)
	}
}

// applyPinnedCodexTurnState 若账号在对应 model 上配置并启用了监控，且存在有效未过期的 pinned state，
// 则将其写入请求头 x-codex-turn-state，并注入相应的 __cflb 与 __oailb Cookie。
func applyPinnedCodexTurnState(headers http.Header, account *Account, model string) bool {
	if headers == nil || account == nil || strings.TrimSpace(model) == "" {
		return false
	}

	normModel := strings.ToLower(strings.TrimSpace(model))

	// 守卫：必须在受监控模型列表中明确启用，防止历史未清理残留或未受控模型意外生效
	monitored := account.GetCodexMonitoredModels()
	if len(monitored) == 0 {
		return false
	}
	mcfg, ok := monitored[normModel]
	if !ok || !mcfg.Enabled {
		return false
	}

	entry := account.GetPinnedCodexTurnStateEntry(model)
	if entry == nil || entry.State == "" {
		return false
	}
	// 防御性检查：确保 header 值不含非法字符（如换行符），杜绝任何导致上游请求硬失败的风险
	if !httpguts.ValidHeaderFieldValue(entry.State) {
		return false
	}
	headers.Set(openAICodexTurnStateHeader, entry.State)

	// 注入 __cflb 与 __oailb 负载均衡 Cookie
	applyCodexLBCookies(headers, entry)

	return true
}

func parseFlexibleTime(v any) *time.Time {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case time.Time:
		return &t
	case *time.Time:
		return t
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		if parsed, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return &parsed
		}
		if parsed, err := time.Parse(time.RFC3339, s); err == nil {
			return &parsed
		}
	case float64:
		if t > 0 {
			tm := time.Unix(int64(t), 0).UTC()
			return &tm
		}
	case int64:
		if t > 0 {
			tm := time.Unix(t, 0).UTC()
			return &tm
		}
	}
	return nil
}
