import type { CSSProperties, ReactNode } from 'react'
import type { ConversationMessage, ConversationRole } from '../../../api/conversations'
import { Tag } from '../../../primitives/Tag'
import { formatISO } from '../../../utils/time'

type Props = { messages: ConversationMessage[]; total?: number }

/**
 * Renders the agent conversation captured for an invocation, in message
 * order. `content` is a JSON string produced by the runtime serializer:
 * user/assistant messages hold an array of blocks (text / thinking /
 * toolCall), toolResult messages hold `{toolCallId, isError, content}`.
 * Parsing is defensive — anything unexpected falls back to raw text.
 */
export function ConversationTranscript({ messages, total }: Props) {
  if (messages.length === 0) {
    return (
      <div className="label" style={{ padding: '4px 0' }}>
        No conversation recorded.
      </div>
    )
  }
  const truncated = total !== undefined && messages.length < total
  return (
    <div>
      {messages.map((m) => (
        <TranscriptMessage key={m.id} message={m} />
      ))}
      {truncated ? (
        <div className="label" style={{ padding: '6px 2px 2px', color: 'var(--ink-3)' }}>
          Showing {messages.length} of {total} messages.
        </div>
      ) : null}
    </div>
  )
}

const ROLE_LABELS: Record<ConversationRole, string> = {
  user: 'User',
  assistant: 'Assistant',
  toolResult: 'Tool result',
}

function roleLabel(role: string): string {
  return ROLE_LABELS[role as ConversationRole] ?? role
}

function parseContent(raw: string): unknown {
  try {
    return JSON.parse(raw)
  } catch {
    return raw
  }
}

function TranscriptMessage({ message }: { message: ConversationMessage }) {
  const parsed = parseContent(message.content)
  const isError =
    message.role === 'toolResult' &&
    typeof parsed === 'object' &&
    parsed !== null &&
    (parsed as { isError?: unknown }).isError === true

  return (
    <article
      aria-label={roleLabel(message.role)}
      style={{
        borderTop: 'var(--hair) solid var(--ink)',
        padding: '12px 2px 4px',
        display: 'grid',
        gap: 8,
      }}
    >
      <header
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'baseline',
          gap: 12,
        }}
      >
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <span className="label" style={{ fontWeight: 700, color: 'var(--ink)' }}>
            {roleLabel(message.role)}
          </span>
          {isError ? <Tag variant="err">ERROR</Tag> : null}
        </span>
        {message.role === 'assistant' ? (
          <AssistantMeta message={message} />
        ) : (
          <span className="num" style={{ fontSize: 10, color: 'var(--ink-3)' }}>
            {formatISO(message.createdAt)}
          </span>
        )}
      </header>
      {message.role === 'toolResult' ? (
        <ToolResultBody value={parsed} isError={isError} />
      ) : (
        <Blocks value={parsed} />
      )}
    </article>
  )
}

function AssistantMeta({ message }: { message: ConversationMessage }) {
  const bits: string[] = []
  if (message.model) bits.push(message.model)
  if (message.inputTokens != null || message.outputTokens != null) {
    bits.push(`${message.inputTokens ?? 0}→${message.outputTokens ?? 0} tok`)
  }
  if (message.stopReason) bits.push(message.stopReason)
  if (bits.length === 0) return null
  return (
    <span
      className="num"
      style={{ fontSize: 10, color: 'var(--ink-3)', letterSpacing: '0.06em' }}
    >
      {bits.join(' · ')}
    </span>
  )
}

const TEXT_STYLE: CSSProperties = {
  margin: 0,
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-word',
  lineHeight: 1.55,
}

function Blocks({ value }: { value: unknown }) {
  if (typeof value === 'string') return <p style={TEXT_STYLE}>{value}</p>
  if (Array.isArray(value)) {
    return (
      <>
        {value.map((block, i) => (
          <Block key={i} block={block} />
        ))}
      </>
    )
  }
  return <p style={TEXT_STYLE}>{JSON.stringify(value)}</p>
}

function Block({ block }: { block: unknown }) {
  if (typeof block === 'string') return <p style={TEXT_STYLE}>{block}</p>
  if (typeof block !== 'object' || block === null) {
    return <p style={TEXT_STYLE}>{String(block)}</p>
  }
  const b = block as Record<string, unknown>
  switch (b.type) {
    case 'text':
      return <p style={TEXT_STYLE}>{String(b.text ?? '')}</p>
    case 'thinking':
      return (
        <div
          style={{
            borderLeft: '2px solid var(--ink-3)',
            paddingLeft: 10,
            margin: '2px 0',
          }}
        >
          <span className="label">Thinking</span>
          <p style={{ ...TEXT_STYLE, fontStyle: 'italic', color: 'var(--ink-3)', marginTop: 4 }}>
            {String(b.text ?? '')}
          </p>
        </div>
      )
    case 'toolCall':
      return (
        <CodeBlock
          code={`${String(b.name ?? 'tool')}(${JSON.stringify(b.arguments ?? {})})`}
        />
      )
    default:
      return <CodeBlock code={`${String(b.type ?? 'unknown')}: ${JSON.stringify(b)}`} />
  }
}

/** toolResult content: `{toolCallId, isError, content: Block[]}` → code block.
 *  `isError` is computed once in TranscriptMessage and passed down. */
function ToolResultBody({ value, isError }: { value: unknown; isError: boolean }) {
  let inner: unknown = value
  if (typeof value === 'object' && value !== null && !Array.isArray(value)) {
    inner = (value as { content?: unknown }).content ?? value
  }
  return <CodeBlock code={blocksToText(inner)} error={isError} />
}

function blocksToText(inner: unknown): string {
  if (typeof inner === 'string') return inner
  if (Array.isArray(inner)) {
    return inner
      .map((b) => {
        if (typeof b === 'string') return b
        if (typeof b === 'object' && b !== null && 'text' in b) {
          return String((b as { text: unknown }).text ?? '')
        }
        return JSON.stringify(b)
      })
      .join('\n')
  }
  return JSON.stringify(inner, null, 2)
}

function CodeBlock({ code, error }: { code: string; error?: boolean }): ReactNode {
  return (
    <pre
      style={{
        margin: 0,
        padding: '10px 12px',
        fontFamily: 'var(--mono)',
        fontSize: 12,
        lineHeight: 1.55,
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
        overflowX: 'auto',
        background: error ? 'var(--signal-haze)' : 'var(--paper-2)',
        border: `var(--hair) solid ${error ? 'var(--signal)' : 'var(--ink)'}`,
        borderLeft: `3px solid ${error ? 'var(--signal)' : 'var(--ink-3)'}`,
      }}
    >
      <code>{code}</code>
    </pre>
  )
}
