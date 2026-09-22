import apiClient from '../client'

export interface StateInspection {
  valid: boolean
  length: number
  raw_state?: string
  issued_at: string
  expires_at: string
  remaining_seconds: number
  age_seconds: number
  is_expired: boolean
  error?: string
}

export interface ProbeHistoryItem {
  at: string
  account_id: number
  model: string
  status_code: number
  latency_ms: number
  success: boolean
  state_len: number
  target_state_len: number
  proxy_used?: string
  error_message?: string
  retry_after_seconds?: number
}

export interface ModelSlotStatus {
  model: string
  enabled: boolean
  target_state_len: number
  refresh_advance_minutes: number
  inspection?: StateInspection
  degradation_status: 'no_record' | 'handled' | 'degraded' | 'unknown'
  recent_degradation?: {
    account_id: number
    account_name: string
    requested_model: string
    sent_model: string
    response_model: string
    count: number
    first_seen: string
    last_seen: string
    ttft_avg_ms?: number
  }
  in_cooldown: boolean
  cooldown_until?: string
  last_probe?: ProbeHistoryItem
  cflb?: string
  oailb?: string
  cookie_age_seconds?: number
  cookie_remaining_seconds?: number
  cookie_renewal_seconds?: number
}

import type { AccountPlatform, AccountType } from '@/types'

export interface CodexAccountState {
  account_id: number
  account_name: string
  platform: AccountPlatform
  type: AccountType
  status: string
  models: ModelSlotStatus[]
}

export interface CodexTurnStateOverview {
  total_accounts: number
  total_models: number
  healthy_models: number
  expiring_models: number
  degraded_models: number
  static_proxy_count?: number
  dynamic_proxy_count?: number
  accounts: CodexAccountState[]
  updated_at: string
}

export interface SlotStats {
  total_probes: number
  successes: number
  failures: number
  last_latency_ms: number
  avg_latency_ms: number
}

export interface CodexTurnStateProxyConfig {
  static_proxies: string[]
  dynamic_proxies: string[]
  proxies?: string[]
  cookie_renewal_seconds?: number
}

export const codexTurnStateApi = {
  getState() {
    return apiClient.get<CodexTurnStateOverview>('/admin/codex-turn-state/state')
  },
  addMonitoredModel(data: {
    account_id: number
    model: string
    target_state_len?: number
    refresh_advance_minutes?: number
    cookie_renewal_seconds?: number
  }) {
    return apiClient.post('/admin/codex-turn-state/accounts', data)
  },
  removeMonitoredModel(accountId: number, model: string) {
    return apiClient.delete(`/admin/codex-turn-state/accounts/${accountId}/${encodeURIComponent(model)}`)
  },
  probeModel(data: { account_id: number; model: string; force?: boolean }) {
    return apiClient.post<ProbeHistoryItem>('/admin/codex-turn-state/probe', data)
  },
  getStats() {
    return apiClient.get<Record<string, SlotStats>>('/admin/codex-turn-state/stats')
  },
  getHistory() {
    return apiClient.get<Record<string, ProbeHistoryItem[]>>('/admin/codex-turn-state/history')
  },
  clearHistory(accountId?: number, model?: string) {
    return apiClient.delete('/admin/codex-turn-state/history', {
      params: { account_id: accountId, model },
    })
  },
  toggleMonitoredModel(accountId: number, model: string, enabled: boolean) {
    return apiClient.put(`/admin/codex-turn-state/accounts/${accountId}/${encodeURIComponent(model)}/toggle`, {
      enabled,
    })
  },
  getProxies() {
    return apiClient.get<CodexTurnStateProxyConfig>('/admin/codex-turn-state/proxies')
  },
  saveProxies(data: { static_proxies: string[]; dynamic_proxies: string[]; cookie_renewal_seconds?: number }) {
    return apiClient.post('/admin/codex-turn-state/proxies', data)
  },
  getDegraded(window?: string) {
    return apiClient.get('/admin/codex-turn-state/degraded', { params: { window } })
  },
}
