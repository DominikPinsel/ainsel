import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useForm } from 'react-hook-form'
import { ToolEditor } from './ToolEditor'
import type { ImageFormValues } from './ImageDetail.types'

function Wrapper({ toolIndex, tools }: { toolIndex: number | null; tools: ImageFormValues['tools'] }) {
  const { control, register, setValue, watch } = useForm<ImageFormValues>({
    defaultValues: { tools, displayName: '', description: '', imageURL: '', env: [], mcpServers: [] },
  })
  return (
    <ToolEditor
      toolIndex={toolIndex}
      tools={watch('tools')}
      control={control}
      register={register}
      setValue={setValue}
      watch={watch}
      onRemove={() => {}}
    />
  )
}

describe('ToolEditor', () => {
  it('shows full MCP tool name for mcp tools', () => {
    render(
      <Wrapper
        toolIndex={0}
        tools={[{
          name: 'mcp__example-mcp__list',
          kind: 'mcp',
          mcpSource: 'example-mcp',
          enabled: true,
          isNew: false,
          description: 'Lists memories',
          examples: [],
        }]}
      />
    )
    expect(screen.getByText('mcp__example-mcp__list')).toBeInTheDocument()
  })

  it('shows enabled toggle (checkbox) for MCP tools', () => {
    render(
      <Wrapper
        toolIndex={0}
        tools={[{
          name: 'mcp__example-mcp__list',
          kind: 'mcp',
          mcpSource: 'example-mcp',
          enabled: true,
          isNew: false,
          description: '',
          examples: [],
        }]}
      />
    )
    expect(screen.getByRole('checkbox', { name: /enabled/i })).toBeInTheDocument()
  })

  it('toggles enabled state when checkbox is clicked', async () => {
    render(
      <Wrapper
        toolIndex={0}
        tools={[{
          name: 'mcp__example-mcp__list',
          kind: 'mcp',
          mcpSource: 'example-mcp',
          enabled: true,
          isNew: false,
          description: '',
          examples: [],
        }]}
      />
    )
    const checkbox = screen.getByRole('checkbox', { name: /enabled/i })
    expect(checkbox).toBeChecked()
    await userEvent.click(checkbox)
    expect(checkbox).not.toBeChecked()
  })

  it('shows edit form for native tools', () => {
    render(
      <Wrapper
        toolIndex={0}
        tools={[{
          name: 'run_shell',
          kind: 'shell',
          enabled: true,
          isNew: false,
          description: 'Runs a shell command',
          examples: [],
        }]}
      />
    )
    expect(screen.getByLabelText(/name/i)).toBeInTheDocument()
  })
})
