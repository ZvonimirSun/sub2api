import type { Group } from '@/types'
import { getPlatformSpec } from '@/constants/platformCatalog'

export type ProviderImportTarget = 'claude' | 'codex' | 'gemini' | 'grok'

export type ProviderImportGroup = Pick<Group, 'platform'> &
  Partial<Pick<Group, 'claude_code_only' | 'fallback_group_id' | 'allow_messages_dispatch'>>

export function stripTrailingSlashes(url: string): string {
  return url.replace(/\/+$/, '')
}

export function getProviderImportTargets(group?: ProviderImportGroup | null): ProviderImportTarget[] {
  if (!group) return []
  const platform = group.platform
  const spec = getPlatformSpec(platform)
  if ((!spec && platform !== 'composite') || platform === 'typesafe') return []

  // Mirror gateway entry-point guards, not the provider's native protocols:
  // Cline and other multi-protocol providers bridge Messages and Responses.
  const targets: ProviderImportTarget[] = []
  if (platform !== 'openai' || group.allow_messages_dispatch) targets.push('claude')
  if (group.claude_code_only && !group.fallback_group_id) return targets
  targets.push('codex')
  if (spec?.gateway === 'gemini' || spec?.gateway === 'antigravity' || platform === 'composite') {
    targets.push('gemini')
  }
  if (platform === 'grok') targets.push('grok')
  return targets
}
