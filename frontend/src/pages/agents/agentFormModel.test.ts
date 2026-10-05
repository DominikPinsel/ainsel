import { describe, expect, it } from 'vitest'
import { agentSchema, LLM_PROVIDERS, llmProviderLabel } from './agentFormModel'

describe('llmProviderLabel', () => {
  it('maps every form option id to its label', () => {
    for (const { value, label } of LLM_PROVIDERS) {
      expect(llmProviderLabel(value)).toBe(label)
    }
  })

  it('treats an absent provider as the forms do — None', () => {
    expect(llmProviderLabel(undefined)).toBe('None')
    expect(llmProviderLabel('')).toBe('None')
  })

  it('falls through unknown provider ids unchanged', () => {
    expect(llmProviderLabel('future-provider')).toBe('future-provider')
  })
})

describe('agentSchema', () => {
  it('requires a custom provider URL when provider is custom', () => {
    const result = agentSchema.safeParse({
      name: 'a',
      imageRef: { name: 'img' },
      llm: { model: 'm', provider: 'custom' },
      persona: { id: 'p' },
      replicas: 1,
    })
    expect(result.success).toBe(false)
    expect(JSON.stringify(result.error?.issues)).toContain('customProviderUrl')
  })
})
