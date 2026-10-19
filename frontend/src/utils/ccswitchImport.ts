import type { Group, GroupPlatform } from '@/types'
import { buildCodexApiKeyAuthConfig } from '@/utils/codexAuthConfig'
import { getPlatformSpec } from '@/constants/platformCatalog'

export const OPENAI_CC_SWITCH_CODEX_MODEL = 'gpt-6.1-sol'
export const GROK_CC_SWITCH_MODEL = 'grok-4.5'

export type CcSwitchClientType = 'claude' | 'codex' | 'gemini' | 'grok'

type CcSwitchImportGroup = Pick<Group, 'platform'> & Partial<Pick<Group,
  'claude_code_only' | 'fallback_group_id' | 'allow_messages_dispatch'>>

export function getCcSwitchImportTargets(group?: CcSwitchImportGroup | null): CcSwitchClientType[] {
  if (!group) return []
  const platform = group.platform
  const spec = getPlatformSpec(platform)
  if ((!spec && platform !== 'composite') || platform === 'typesafe') return []

  // Mirror gateway entry-point guards, not the provider's native protocols:
  // Cline and other multi-protocol providers bridge Messages and Responses.
  const targets: CcSwitchClientType[] = []
  if (platform !== 'openai' || group.allow_messages_dispatch) targets.push('claude')
  if (group.claude_code_only && !group.fallback_group_id) return targets
  targets.push('codex')
  if (spec?.gateway === 'gemini' || spec?.gateway === 'antigravity' || platform === 'composite') {
    targets.push('gemini')
  }
  if (platform === 'grok') targets.push('grok')
  return targets
}

export interface CcSwitchImportConfig {
  app: string
  endpoint: string
  model?: string
}

export interface CcSwitchImportDeeplinkInput {
  baseUrl: string
  additionalEndpoints?: string[]
  homepage?: string
  platform?: GroupPlatform | null
  clientType: CcSwitchClientType
  providerName: string
  apiKey: string
  usageScript: string
}

/**
 * Balance query CC Switch runs against the imported provider. CC Switch fills
 * `{{baseUrl}}` with the provider's base URL as stored — Grok imports carry a
 * trailing `/v1` (see `withV1Endpoint`), other imports may not, and users
 * may edit it either way afterwards — then evaluates the script, so the URL
 * strips an existing `/v1` instead of blindly appending one (`/v1/v1/usage`
 * is a 404 and CC Switch shows "query failed").
 */
export const CC_SWITCH_USAGE_SCRIPT = `({
    request: {
      url: "{{baseUrl}}".replace(/\\/+$/, "").replace(/\\/v1$/, "") + "/v1/usage",
      method: "GET",
      headers: { "Authorization": "Bearer {{apiKey}}" }
    },
    extractor: function(response) {
      const remaining = response?.remaining ?? response?.quota?.remaining ?? response?.balance;
      const unit = response?.unit ?? response?.quota?.unit ?? "USD";
      return {
        isValid: response?.is_active ?? response?.isValid ?? true,
        remaining,
        unit
      };
    }
  })`

function withV1Endpoint(baseUrl: string): string {
  const normalizedBaseUrl = baseUrl.replace(/\/+$/, '')
  return normalizedBaseUrl.endsWith('/v1') ? normalizedBaseUrl : `${normalizedBaseUrl}/v1`
}

function withoutTrailingSlashes(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, '')
}

export function resolveCcSwitchImportConfig(
  platform: GroupPlatform | undefined | null,
  clientType: CcSwitchClientType,
  baseUrl: string
): CcSwitchImportConfig {
  switch (clientType) {
    case 'claude':
      return {
        app: 'claude',
        endpoint: platform === 'antigravity' ? `${withoutTrailingSlashes(baseUrl)}/antigravity` : baseUrl
      }
    case 'codex':
      return {
        app: 'codex',
        // CC Switch's Codex provider appends the API path itself.
        endpoint: withoutTrailingSlashes(baseUrl),
        model: OPENAI_CC_SWITCH_CODEX_MODEL
      }
    case 'gemini':
      return {
        app: 'gemini',
        endpoint: platform === 'antigravity' ? `${withoutTrailingSlashes(baseUrl)}/antigravity` : baseUrl
      }
    case 'grok':
      return {
        app: 'grokbuild',
        endpoint: withV1Endpoint(baseUrl),
        model: GROK_CC_SWITCH_MODEL
      }
  }
}

export function buildCcSwitchImportDeeplink(input: CcSwitchImportDeeplinkInput): string {
  const config = resolveCcSwitchImportConfig(input.platform, input.clientType, input.baseUrl)
  const endpoints = [input.baseUrl, ...(input.additionalEndpoints || [])]
    .map(endpoint => endpoint.trim())
    .filter((endpoint, index, all) => endpoint && all.indexOf(endpoint) === index)
    .map(endpoint => resolveCcSwitchImportConfig(input.platform, input.clientType, endpoint).endpoint)
    .filter((endpoint, index, all) => all.indexOf(endpoint) === index)
  const entries: [string, string][] = [
    ['resource', 'provider'],
    ['app', config.app],
    ['name', input.providerName],
    ['homepage', input.homepage || input.baseUrl],
    ['endpoint', endpoints.join(',')],
    ['apiKey', input.apiKey],
    ['configFormat', 'json'],
    ['usageEnabled', 'true'],
    ['usageScript', btoa(input.usageScript)],
    ['usageAutoInterval', '30']
  ]

  if (config.model) {
    entries.splice(2, 0, ['model', config.model])
  }

  if (config.app === 'codex') {
    // CC Switch's embedded-config import preserves provider auth and headers.
    // It deliberately leaves global [features] settings to the user.
    const codexConfig = `model_provider = "sub2api"
model = ${JSON.stringify(config.model)}
web_search = "live"

[model_providers.sub2api]
name = "Sub2API"
base_url = ${JSON.stringify(config.endpoint)}
wire_api = "responses"
${buildCodexApiKeyAuthConfig(input.apiKey)}
`
    const payload = JSON.stringify({ config: codexConfig })
    const bytes = new TextEncoder().encode(payload)
    entries.push(['config', btoa(Array.from(bytes, (byte) => String.fromCharCode(byte)).join(''))])
  }

  return `ccswitch://v1/import?${new URLSearchParams(entries).toString()}`
}
