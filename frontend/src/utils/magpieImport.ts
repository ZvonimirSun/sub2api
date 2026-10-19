import { getCcSwitchImportTargets } from './ccswitchImport'

type ImportGroup = Parameters<typeof getCcSwitchImportTargets>[0]

function withoutTrailingSlashes(url: string): string {
  return url.replace(/\/+$/, '')
}

function versioned(url: string, version: 'v1' | 'v1beta'): string {
  const base = withoutTrailingSlashes(url).replace(/\/(v1|v1beta)$/, '')
  return `${base}/${version}`
}

export function buildMagpieImportUrl(input: {
  group?: ImportGroup
  baseUrl: string
  providerName: string
  apiKey: string
  website?: string
}): string {
  const targets = getCcSwitchImportTargets(input.group)
  const root = withoutTrailingSlashes(input.baseUrl.trim()).replace(/\/(v1|v1beta)$/, '')
  const nativeRoot = input.group?.platform === 'antigravity' ? `${root}/antigravity` : root
  const params = new URLSearchParams()
  params.set('name', input.providerName.slice(0, 80))
  if (targets.includes('codex')) {
    params.set('chat', versioned(root, 'v1'))
    params.set('responses', versioned(root, 'v1'))
  }
  if (targets.includes('claude')) {
    params.set('anthropic', nativeRoot)
  }
  if (targets.includes('gemini')) {
    params.set('gemini', versioned(nativeRoot, 'v1beta'))
  }
  params.set('key', input.apiKey)
  if (input.website?.startsWith('https://')) params.set('website', input.website)
  return `https://usemagpie.ai/import#${params.toString()}`
}
