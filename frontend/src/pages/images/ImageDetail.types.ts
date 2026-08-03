import type { ToolKind } from '../../api/agentImages'

export type ExampleFormValue = {
  title: string
  snippet: string
}

export type ToolFormValue = {
  name: string
  kind: ToolKind
  description: string
  mcpSource?: string
  enabled: boolean
  isNew: boolean
  examples: ExampleFormValue[]
}

export type EnvVarFormValue = {
  name: string
  value: string
  secret: boolean
}

export type ImageFormValues = {
  displayName: string
  description: string
  imageURL: string
  groupId?: string
  tools: ToolFormValue[]
  env: EnvVarFormValue[]
  mcpServers: string[]
  enabledSkills: string[]
}
