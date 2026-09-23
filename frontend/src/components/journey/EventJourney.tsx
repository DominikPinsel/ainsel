import { Link } from 'react-router-dom'
import {
  channelPath,
  channelIdForProducer,
  isDirectSource,
} from '../../api/channels'
import { Tag } from '../../primitives/Tag'
import { formatISO } from '../../utils/time'
import './EventJourney.css'

/**
 * Visual trace of one event through the channel system: born in a channel
 * (or born directly in an agent's inbox, for schedules and chat), then
 * transferred into further channels by their subscriptions. Channels are
 * addressed by id — connector `forgejo` and agent `forgejo` are two
 * channels; the transfer steps land in the agent's inbox channel.
 */

type JourneyStep = {
  key: string
  channel: string
  /** Where the event came from, e.g. 'produced here', 'via subscription X'. */
  origin: string
  at?: string
  durationMs?: number
  error?: string
  dotTone: Tone
  pulse?: boolean
}

type Tone = 'ok' | 'err' | 'warn' | 'stale' | 'default'

type TagVariant = 'default' | 'ok' | 'warn' | 'err' | 'stale'

const TONE_VARIANT: Record<Tone, TagVariant> = {
  ok: 'ok',
  err: 'err',
  warn: 'warn',
  stale: 'stale',
  default: 'default',
}

function runTone(status?: string): Tone {
  if (status === 'success') return 'ok'
  if (status === 'failure' || status === 'timeout') return 'err'
  if (status === 'running') return 'warn'
  return 'default'
}

function ChannelChip({
  name,
  mark,
  to,
}: {
  name: string
  mark: 'produced' | 'fanout' | 'direct' | 'none'
  to?: string
}) {
  const cls = `ch-mark ${mark === 'produced' ? 'produced' : mark === 'direct' ? 'direct' : ''}`
  const inner = (
    <>
      <span className={cls} aria-hidden="true" />
      {name}
    </>
  )
  return to ? (
    <Link to={to} className="journey-channel">
      {inner}
    </Link>
  ) : (
    <span className="journey-channel">{inner}</span>
  )
}

export function EventJourney({
  connector,
  timestamp,
  matches,
}: {
  connector?: string
  timestamp: string
  matches: { trigger: string; agent: string; runStatus?: string; durationMs?: number; error?: string }[]
}) {
  const steps: JourneyStep[] = []

  if (connector) {
    steps.push({
      key: 'home',
      channel: connector,
      origin: isDirectSource(connector)
        ? connector === 'chat'
          ? 'born directly in the agent inboxes — chat message'
          : 'born directly in the agent inboxes — scheduled tick'
        : 'born here',
      at: timestamp,
      dotTone: 'default',
    })
  }

  for (const m of matches) {
    steps.push({
      key: `fanout:${m.agent}`,
      channel: m.agent,
      origin: m.trigger ? `via subscription ${m.trigger}` : 'via transfer',
      durationMs: m.durationMs,
      error: m.error,
      dotTone: runTone(m.runStatus),
      pulse: m.runStatus === 'running',
    })
  }

  const unmatched = steps.length === 1 && connector !== undefined

  return (
    <ol className="journey" data-testid="event-journey">
      {steps.map((s, i) => (
        <li key={s.key} className="journey-step">
          <div className="journey-rail">
            <span className={`journey-dot ${s.dotTone}${s.pulse ? ' pulse' : ''}`} aria-hidden="true" />
          </div>
          <div className="journey-body">
            <div className="journey-headline">
              <ChannelChip
                name={s.channel}
                mark={i === 0 ? (isDirectSource(s.channel) ? 'direct' : 'produced') : 'fanout'}
                to={
                  i === 0
                    ? isDirectSource(s.channel)
                      ? undefined
                      : channelPath(channelIdForProducer(s.channel))
                    : channelPath(`agent:${s.channel}`)
                }
              />
              {i > 0 ? <Tag variant={TONE_VARIANT[s.dotTone]}>{s.dotTone === 'ok' ? 'delivered' : s.dotTone === 'err' ? 'failed' : s.dotTone === 'warn' ? 'running' : 'queued'}</Tag> : <Tag>home</Tag>}
            </div>
            <div className="journey-meta">
              <span>{s.origin}</span>
              {s.at ? <span>· {formatISO(s.at)}</span> : null}
              {s.durationMs !== undefined ? <span>· run {(s.durationMs / 1000).toFixed(1)}s</span> : null}
            </div>
            {s.error ? <div className="journey-error">{s.error}</div> : null}
          </div>
        </li>
      ))}
      {unmatched ? (
        <li className="journey-step">
          <div className="journey-rail">
            <span className="journey-dot stale" aria-hidden="true" />
          </div>
          <div className="journey-body">
            <div className="journey-terminal">
              Unmatched — no subscription picked this event up. It stays in the <b>{connector}</b> channel
              and will not be delivered to any agent.
            </div>
          </div>
        </li>
      ) : null}
    </ol>
  )
}