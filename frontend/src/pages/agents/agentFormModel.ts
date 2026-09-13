import { z } from 'zod'
import type {
  AgentAlibabaCloud,
  AgentCustomProvider,
  AgentOllamaCloud,
  AgentOpenCode,
  AgentRequest,
} from '../../api/agents'

export const LLM_PROVIDERS = [
  { value: '', label: 'None' },
  { value: 'ollama-cloud', label: 'Ollama Cloud' },
  { value: 'opencode', label: 'OpenCode' },
  { value: 'alibaba-cloud', label: 'Alibaba Token Plan' },
  { value: 'custom', label: 'Custom' },
] as const

export const API_KEY_LABELS: Record<string, string> = {
  'ollama-cloud': 'Ollama Cloud API Key',
  opencode: 'OpenCode API Key',
  'alibaba-cloud': 'Alibaba Token Plan API Key',
  custom: 'API Key',
}

/**
 * Validation for both agent write forms: the creation wizard and the edit
 * form. Kept in one module so the two cannot drift.
 */
export const agentSchema = z
  .object({
    name: z.string().min(1, 'Name is required'),
    description: z.string().optional(),
    imageRef: z.object({ name: z.string().min(1, 'Image is required') }),
    llm: z.object({
      model: z.string().min(1, 'Model is required'),
      provider: z.string().optional(),
      maxTurns: z.coerce.number().int().min(1).optional(),
      temperature: z.coerce.number().min(0).max(2).optional(),
    }),
    providerApiKey: z.string().optional(),
    customProviderUrl: z.string().optional(),
    persona: z.object({ id: z.string().min(1, 'Persona is required') }),
    replicas: z.coerce.number().int().min(0).optional(),
    groupId: z.string().optional(),
  })
  .superRefine((data, ctx) => {
    if (data.llm.provider === 'custom' && !data.customProviderUrl?.trim()) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['customProviderUrl'],
        message: 'URL is required for custom providers',
      })
    }
  })

// zod 4 tracks input/output types separately (coercions & defaults make
// inputs wider); useForm needs both so the resolver types line up.
export type AgentFormInput = z.input<typeof agentSchema>
export type AgentFormValues = z.infer<typeof agentSchema>

export const agentDefaults: AgentFormInput = {
  name: '',
  description: '',
  imageRef: { name: '' },
  llm: { model: '', provider: 'ollama-cloud', maxTurns: 100, temperature: 1 },
  customProviderUrl: '',
  providerApiKey: '',
  persona: { id: '' },
  replicas: 1,
  groupId: '',
}

/**
 * Builds the write-side agent body from validated form values.
 *
 * A provider credential block is attached only when a key was entered, so an
 * edit that leaves the key blank keeps the stored one. The group is only sent
 * on create: an agent's group is fixed for its lifetime.
 */
export function buildAgentRequest(
  values: AgentFormValues,
  opts: { isEdit: boolean },
): AgentRequest {
  const provider = values.llm.provider ?? ''
  const apiKey = values.providerApiKey ?? ''
  const key = apiKey ? { apiKey } : undefined

  const ollamaCloud: AgentOllamaCloud | undefined =
    provider === 'ollama-cloud' ? key : undefined
  const openCode: AgentOpenCode | undefined = provider === 'opencode' ? key : undefined
  const alibabaCloud: AgentAlibabaCloud | undefined =
    provider === 'alibaba-cloud' ? key : undefined
  const customProvider: AgentCustomProvider | undefined =
    provider === 'custom'
      ? { url: values.customProviderUrl ?? '', ...(key ?? {}) }
      : undefined

  return {
    name: values.name,
    description: values.description || undefined,
    imageRef: values.imageRef,
    llm: values.llm,
    ollamaCloud,
    openCode,
    alibabaCloud,
    customProvider,
    persona: { id: values.persona.id },
    replicas: values.replicas,
    groupId: opts.isEdit ? undefined : values.groupId,
  }
}
