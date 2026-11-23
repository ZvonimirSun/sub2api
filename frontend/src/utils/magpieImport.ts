import {
  getProviderImportTargets,
  stripTrailingSlashes,
  type ProviderImportGroup,
} from './providerImportTargets'

function versioned(url: string, version: 'v1' | 'v1beta'): string {
  const base = stripTrailingSlashes(url).replace(/\/(v1|v1beta)$/, '')
  return `${base}/${version}`
}

export function buildMagpieImportUrl(input: {
  group?: ProviderImportGroup | null
  baseUrl: string
  providerName: string
  apiKey: string
  website?: string
}): string {
  const targets = getProviderImportTargets(input.group)
  const root = stripTrailingSlashes(input.baseUrl.trim()).replace(/\/(v1|v1beta)$/, '')
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
