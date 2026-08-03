import { z } from 'zod'

export const exampleSchema = z.object({
  title: z.string(),
  snippet: z.string(),
})

export const toolSchema = z.object({
  name: z.string().min(1, 'Tool name is required'),
  kind: z.enum(['container', 'shell', 'mcp']),
  description: z.string(),
  mcpSource: z.string().optional(),
  enabled: z.boolean(),
  isNew: z.boolean(),
  examples: z.array(exampleSchema),
})

export const envVarSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  value: z.string(),
  secret: z.boolean(),
})

export const imageSchema = z.object({
  displayName: z.string(),
  description: z.string(),
  imageURL: z.string().min(1, 'Image URL is required'),
  groupId: z.string().optional(),
  tools: z.array(toolSchema),
  env: z.array(envVarSchema),
  mcpServers: z.array(z.string()),
  enabledSkills: z.array(z.string()),
})

export type ExampleSchemaValues = z.infer<typeof exampleSchema>
export type ToolSchemaValues = z.infer<typeof toolSchema>
export type EnvVarSchemaValues = z.infer<typeof envVarSchema>
export type ImageSchemaValues = z.infer<typeof imageSchema>
