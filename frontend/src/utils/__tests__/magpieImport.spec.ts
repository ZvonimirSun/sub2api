import { describe, expect, it } from 'vitest'
import { buildMagpieImportUrl } from '../magpieImport'

const input = {
  baseUrl: 'https://api.example.com/proxy/v1/',
  providerName: 'Test Provider',
  apiKey: 'sk-test&+#secret',
  group: { platform: 'cline' as const }
}

function params(overrides: Partial<Parameters<typeof buildMagpieImportUrl>[0]> = {}) {
  const url = new URL(buildMagpieImportUrl({ ...input, ...overrides }))
  return new URLSearchParams(url.hash.slice(1))
}

describe('buildMagpieImportUrl', () => {
  it('uses the official web bridge with credentials only in the fragment and no model list', () => {
    const url = new URL(buildMagpieImportUrl(input))
    expect(url.origin + url.pathname).toBe('https://usemagpie.ai/import')
    expect(url.search).toBe('')
    const config = params()
    expect(config.get('key')).toBe(input.apiKey)
    expect(config.get('name')).toBe(input.providerName)
    expect(config.has('models')).toBe(false)
    expect(config.has('model')).toBe(false)
    expect(config.get('chat')).toBe('https://api.example.com/proxy/v1')
    expect(config.get('responses')).toBe('https://api.example.com/proxy/v1')
    expect(config.get('anthropic')).toBe('https://api.example.com/proxy')
    expect(config.has('gemini')).toBe(false)
  })

  it('respects the OpenAI Messages gate', () => {
    expect(params({ group: { platform: 'openai' } }).has('anthropic')).toBe(false)
    expect(params({ group: { platform: 'openai', allow_messages_dispatch: true } }).has('anthropic')).toBe(true)
  })

  it('does not advertise non-Claude entry points for Claude-only groups without fallback', () => {
    const config = params({ group: { platform: 'anthropic', claude_code_only: true } })
    expect(config.has('anthropic')).toBe(true)
    expect(config.has('chat')).toBe(false)
    expect(config.has('responses')).toBe(false)
    expect(config.has('gemini')).toBe(false)
    expect(params({ group: { platform: 'anthropic', claude_code_only: true, fallback_group_id: 2 } }).has('chat')).toBe(true)
  })

  it.each(['gemini', 'composite'] as const)('includes Gemini entry for %s', platform => {
    expect(params({ group: { platform } }).get('gemini')).toBe('https://api.example.com/proxy/v1beta')
  })

  it('preserves Antigravity native entry prefixes without changing OpenAI entry points', () => {
    const config = params({ group: { platform: 'antigravity' } })
    expect(config.get('anthropic')).toBe('https://api.example.com/proxy/antigravity')
    expect(config.get('gemini')).toBe('https://api.example.com/proxy/antigravity/v1beta')
    expect(config.get('responses')).toBe('https://api.example.com/proxy/v1')
  })

  it('limits provider names and includes only an HTTPS website', () => {
    expect(params({ providerName: 'x'.repeat(90) }).get('name')).toHaveLength(80)
    expect(params({ website: 'http://panel.example.com' }).has('website')).toBe(false)
    expect(params({ website: 'https://panel.example.com' }).get('website')).toBe('https://panel.example.com')
  })
})
