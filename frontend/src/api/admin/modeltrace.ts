import { apiClient } from '../client'
import type { AccountPlatform, AccountType } from '@/types'

export interface ModelTraceRecord {
  detected_model: string
  probability: number
  latency_ms: number
  tested_at: string
  error?: string
}

export interface ModelTraceModelConfig {
  latest?: ModelTraceRecord
  history?: ModelTraceRecord[]
}

export interface ModelTraceMonitorItem {
  account_id: number
  account_name: string
  platform: AccountPlatform
  type: AccountType
  enabled: boolean
  cron: string
  last_run_at?: string
  models: Record<string, ModelTraceModelConfig>
}

export interface UpdateModelTraceConfigRequest {
  enabled: boolean
  cron_expression: string
  models: string[]
}

export interface ModelTraceGlobalConfig {
  enabled: boolean
  probe_count: number
  interval_seconds: number
  reasoning_effort: string
  cron_expression: string
}

export const modelTraceAPI = {
  async listMonitors(): Promise<ModelTraceMonitorItem[]> {
    const { data } = await apiClient.get<ModelTraceMonitorItem[]>('/admin/modeltrace/monitors')
    return data
  },
  async getGlobalConfig(): Promise<ModelTraceGlobalConfig> {
    const { data } = await apiClient.get<ModelTraceGlobalConfig>('/admin/modeltrace/global-config')
    return data
  },
  async updateGlobalConfig(cfg: ModelTraceGlobalConfig): Promise<{ success: boolean; config: ModelTraceGlobalConfig }> {
    const { data } = await apiClient.put<{ success: boolean; config: ModelTraceGlobalConfig }>('/admin/modeltrace/global-config', cfg)
    return data
  },
  async updateAccountConfig(accountId: number, reqData: UpdateModelTraceConfigRequest): Promise<void> {
    await apiClient.put(`/admin/modeltrace/accounts/${accountId}`, reqData)
  },
  async toggleAccountEnabled(accountId: number, enabled: boolean): Promise<{ success: boolean; enabled: boolean }> {
    const { data } = await apiClient.put<{ success: boolean; enabled: boolean }>(`/admin/modeltrace/accounts/${accountId}/toggle`, { enabled })
    return data
  },
  async probeModel(accountId: number, modelId: string): Promise<ModelTraceRecord> {
    const { data } = await apiClient.post<ModelTraceRecord>(`/admin/modeltrace/accounts/${accountId}/models/${encodeURIComponent(modelId)}/run`, undefined, { timeout: 660000 })
    return data
  },
}

export default modelTraceAPI
