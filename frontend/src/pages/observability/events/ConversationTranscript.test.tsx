import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ConversationTranscript } from './ConversationTranscript'
import type { ConversationMessage } from '../../../api/conversations'

function msg(partial: Partial<ConversationMessage> & Pick<ConversationMessage, 'id' | 'role' | 'content'>): ConversationMessage {
  return {
    agentName: 'review-bot',
    createdAt: '2026-06-10T08:31:26Z',
    ...partial,
  }
}

describe('ConversationTranscript', () => {
  it('renders the empty state when there are no messages', () => {
    render(<ConversationTranscript messages={[]} />)
    expect(screen.getByText('No conversation recorded.')).toBeInTheDocument()
  })

  it('renders a user text message', () => {
    render(
      <ConversationTranscript
        messages={[
          msg({
            id: 1,
            role: 'user',
            content: '[{"type":"text","text":"Handle the event"}]',
          }),
        ]}
      />,
    )
    expect(screen.getByText('User')).toBeInTheDocument()
    expect(screen.getByText('Handle the event')).toBeInTheDocument()
  })

  it('renders assistant thinking, text and toolCall blocks with usage metadata', () => {
    render(
      <ConversationTranscript
        messages={[
          msg({
            id: 2,
            role: 'assistant',
            content: JSON.stringify([
              { type: 'thinking', text: 'Let me inspect the diff' },
              { type: 'text', text: 'Done reviewing' },
              { type: 'toolCall', id: 'call-1', name: 'read_file', arguments: { path: 'a.go' } },
            ]),
            model: 'claude-x',
            inputTokens: 100,
            outputTokens: 50,
            stopReason: 'endTurn',
          }),
        ]}
      />,
    )
    expect(screen.getByText('Assistant')).toBeInTheDocument()
    expect(screen.getByText('Thinking')).toBeInTheDocument()
    expect(screen.getByText('Let me inspect the diff')).toBeInTheDocument()
    expect(screen.getByText('Done reviewing')).toBeInTheDocument()
    expect(screen.getByText(/read_file\(/)).toBeInTheDocument()
    expect(screen.getByText(/"path":"a\.go"/)).toBeInTheDocument()
    // usage metadata in the assistant header
    expect(screen.getByText(/claude-x/)).toBeInTheDocument()
    expect(screen.getByText(/100→50 tok/)).toBeInTheDocument()
    expect(screen.getByText(/endTurn/)).toBeInTheDocument()
  })

  it('renders a tool result as a code block', () => {
    render(
      <ConversationTranscript
        messages={[
          msg({
            id: 3,
            role: 'toolResult',
            content: JSON.stringify({
              toolCallId: 'call-1',
              isError: false,
              content: [{ type: 'text', text: 'package main' }],
            }),
          }),
        ]}
      />,
    )
    expect(screen.getByText('Tool result')).toBeInTheDocument()
    expect(screen.getByText('package main')).toBeInTheDocument()
  })

  it('marks errored tool results', () => {
    render(
      <ConversationTranscript
        messages={[
          msg({
            id: 4,
            role: 'toolResult',
            content: JSON.stringify({
              toolCallId: 'call-2',
              isError: true,
              content: [{ type: 'text', text: 'permission denied' }],
            }),
          }),
        ]}
      />,
    )
    expect(screen.getByText('permission denied')).toBeInTheDocument()
    expect(screen.getByText('ERROR')).toBeInTheDocument()
  })

  it('labels unknown block types and shows their JSON', () => {
    render(
      <ConversationTranscript
        messages={[
          msg({
            id: 5,
            role: 'assistant',
            content: '[{"type":"image","url":"https://x/y.png"}]',
          }),
        ]}
      />,
    )
    expect(screen.getByText(/image/)).toBeInTheDocument()
    expect(screen.getByText(/"url":"https:\/\/x\/y\.png"/)).toBeInTheDocument()
  })

  it('falls back to the raw string when content is not valid JSON', () => {
    render(
      <ConversationTranscript
        messages={[msg({ id: 6, role: 'user', content: 'plain unserialized prompt' })]}
      />,
    )
    expect(screen.getByText('plain unserialized prompt')).toBeInTheDocument()
  })
})
