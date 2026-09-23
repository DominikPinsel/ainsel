// Package eventqueue implements a PostgreSQL-backed event queue that replaces
// the former NATS JetStream. Events are inserted by connectors (via the hub ingest API),
// routed by the hub router into agent_tasks, and claimed by agent runtimes via
// HTTP long-poll with LISTEN/NOTIFY wakeup.
package eventqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Event is an incoming connector event stored in the events table.
type Event struct {
	ID string `json:"id"`
	// Connector is the source label: the WebhookConnector CR name for
	// webhook events, or one of the hub-published pseudo-sources ("cron",
	// "chat") for events the hub emits itself.
	Connector string `json:"connector"`
	// ChannelID is the id of the channel this event was born in. Empty for
	// events stored before channels existed and for labels with no provisioned
	// channel.
	ChannelID  string          `json:"channelId,omitempty"`
	Headers    json.RawMessage `json:"headers"`
	Data       json.RawMessage `json:"data"`
	Raw        string          `json:"raw"`
	ReceivedAt time.Time       `json:"received_at"`
}

// Task is a unit of work dispatched to an agent, stored in agent_tasks.
type Task struct {
	ID           int64           `json:"id"`
	EventID      string          `json:"event_id"`
	AgentName    string          `json:"agent_name"`
	TriggerName  string          `json:"trigger_name"`
	InvocationID string          `json:"invocation_id"`
	Headers      json.RawMessage `json:"headers"`
	Payload      json.RawMessage `json:"payload"`
	Attempts     int             `json:"attempts"`
	Status       string          `json:"status"`

	// Error is the last failure message reported for this task ("" when none).
	Error string `json:"error,omitempty"`
	// CreatedAt is when the task was enqueued.
	CreatedAt time.Time `json:"createdAt,omitempty"`
	// CompletedAt is when the task reached a terminal status, if it did.
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// Store provides event queue operations backed by PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store using the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Pool returns the underlying connection pool. Exposed for tests that need to
// manipulate queue state directly.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// InsertEvent stores a new event. Duplicate IDs are silently ignored. The
// birth channel is stamped by the caller — the ingest endpoint resolves the
// connector label to its channel, and the cron and chat publishers name the
// agent inbox the event is born in.
func (s *Store) InsertEvent(ctx context.Context, evt Event) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO events (id, connector, channel_id, headers, data, raw)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (id) DO NOTHING`,
		evt.ID, evt.Connector, nilIfEmpty(evt.ChannelID), evt.Headers, evt.Data, evt.Raw,
	)
	if err != nil {
		return fmt.Errorf("eventqueue: insert event %q: %w", evt.ID, err)
	}
	return nil
}

// nilIfEmpty maps an optional text FK to NULL so referential integrity is not
// violated by an empty string.
func nilIfEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// FetchUnrouted returns up to limit events that have not yet been routed,
// ordered by receive time.
func (s *Store) FetchUnrouted(ctx context.Context, limit int) ([]Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, connector, channel_id, headers, data, raw
		 FROM events
		 WHERE routed_at IS NULL
		 ORDER BY received_at ASC
		 LIMIT $1`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: fetch unrouted: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var channelID *string
		if err := rows.Scan(&e.ID, &e.Connector, &channelID, &e.Headers, &e.Data, &e.Raw); err != nil {
			return nil, fmt.Errorf("eventqueue: scan event: %w", err)
		}
		if channelID != nil {
			e.ChannelID = *channelID
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// MarkRouted stamps an event as routed so it is not picked up again.
func (s *Store) MarkRouted(ctx context.Context, eventID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE events SET routed_at = now() WHERE id = $1`, eventID,
	)
	if err != nil {
		return fmt.Errorf("eventqueue: mark routed %q: %w", eventID, err)
	}
	return nil
}

// EnqueueTask inserts a task for an agent and notifies any long-poll waiters.
// Duplicate (event_id, agent_name) pairs are silently ignored.
func (s *Store) EnqueueTask(ctx context.Context, task Task) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO agent_tasks (event_id, agent_name, trigger_name, invocation_id, headers, payload)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (event_id, agent_name) DO NOTHING`,
		task.EventID, task.AgentName, task.TriggerName, task.InvocationID, task.Headers, task.Payload,
	)
	if err != nil {
		return fmt.Errorf("eventqueue: enqueue task for %q: %w", task.AgentName, err)
	}
	return s.NotifyAgent(ctx, task.AgentName)
}

// ClaimTask atomically claims the next pending task for an agent using
// SELECT FOR UPDATE SKIP LOCKED. Returns nil, nil if no task is available.
func (s *Store) ClaimTask(ctx context.Context, agentName string) (*Task, error) {
	var t Task
	err := s.pool.QueryRow(ctx,
		`UPDATE agent_tasks
		 SET status = 'claimed', claimed_at = now(), attempts = attempts + 1
		 WHERE id = (
		     SELECT id FROM agent_tasks
		     WHERE agent_name = $1
		       AND status = 'pending'
		       AND (retry_after IS NULL OR retry_after <= now())
		     ORDER BY created_at ASC
		     FOR UPDATE SKIP LOCKED
		     LIMIT 1
		 )
		 RETURNING id, event_id, agent_name, trigger_name, invocation_id, headers, payload, attempts`,
		agentName,
	).Scan(&t.ID, &t.EventID, &t.AgentName, &t.TriggerName, &t.InvocationID, &t.Headers, &t.Payload, &t.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("eventqueue: claim task for %q: %w", agentName, err)
	}
	return &t, nil
}

// AckTask marks a task as completed.
func (s *Store) AckTask(ctx context.Context, taskID int64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agent_tasks SET status = 'completed', completed_at = now() WHERE id = $1`,
		taskID,
	)
	if err != nil {
		return fmt.Errorf("eventqueue: ack task %d: %w", taskID, err)
	}
	return nil
}

// NakTask returns a task to pending with a retry delay, or marks it failed
// if max_attempts is reached.
func (s *Store) NakTask(ctx context.Context, taskID int64, delay time.Duration, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agent_tasks
		 SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
		     retry_after = now() + make_interval(secs => $2),
		     error = $3
		 WHERE id = $1`,
		taskID, delay.Seconds(), errMsg,
	)
	if err != nil {
		return fmt.Errorf("eventqueue: nak task %d: %w", taskID, err)
	}
	return nil
}

// GetTask returns a task by ID, verifying it belongs to the given agent.
func (s *Store) GetTask(ctx context.Context, taskID int64, agentName string) (*Task, error) {
	var t Task
	err := s.pool.QueryRow(ctx,
		`SELECT id, event_id, agent_name, trigger_name, invocation_id, headers, payload, attempts
		 FROM agent_tasks WHERE id = $1 AND agent_name = $2`,
		taskID, agentName,
	).Scan(&t.ID, &t.EventID, &t.AgentName, &t.TriggerName, &t.InvocationID, &t.Headers, &t.Payload, &t.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("eventqueue: get task %d: %w", taskID, err)
	}
	return &t, nil
}

// NotifyAgent sends a pg_notify on the "agent_tasks" channel with the agent
// name as payload, waking any long-poll waiters for that agent.
func (s *Store) NotifyAgent(ctx context.Context, agentName string) error {
	_, err := s.pool.Exec(ctx, `SELECT pg_notify('agent_tasks', $1)`, agentName)
	if err != nil {
		return fmt.Errorf("eventqueue: notify agent %q: %w", agentName, err)
	}
	return nil
}

// WaitForTask blocks until a task is available for agentName or timeout
// expires. Uses pg LISTEN/NOTIFY for efficient wakeup instead of busy-polling.
// Returns nil, nil on timeout with no task.
func (s *Store) WaitForTask(ctx context.Context, agentName string, timeout time.Duration) (*Task, error) {
	// Try immediate claim first.
	task, err := s.ClaimTask(ctx, agentName)
	if err != nil || task != nil {
		return task, err
	}

	// Open a dedicated connection for LISTEN.
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: acquire conn for listen: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN agent_tasks"); err != nil {
		return nil, fmt.Errorf("eventqueue: listen: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), "UNLISTEN agent_tasks") }()

	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, nil
		}

		// Wait for notification with a context deadline.
		waitCtx, cancel := context.WithTimeout(ctx, remaining)
		notification, err := conn.Conn().WaitForNotification(waitCtx)
		cancel()

		if err != nil {
			// Context cancelled (shutdown) or timeout — try one last claim.
			task, claimErr := s.ClaimTask(ctx, agentName)
			if claimErr != nil {
				return nil, claimErr
			}
			return task, nil
		}

		// Only wake if the notification is for our agent.
		if notification.Payload == agentName {
			task, err := s.ClaimTask(ctx, agentName)
			if err != nil {
				return nil, err
			}
			if task != nil {
				return task, nil
			}
			// Spurious notification (task already claimed by another replica); loop.
		}
	}
}

// ReapedTask describes a task whose claim was reset by ReapStaleClaims.
type ReapedTask struct {
	ID        int64
	AgentName string
}

// ReapStaleClaims resets tasks that have been in 'claimed' status longer than
// timeout without being acked or naked. Tasks whose attempts have reached
// max_attempts are marked 'failed'; all others are returned to 'pending' with
// a 30-second retry delay. Returns the list of reaped tasks for logging.
func (s *Store) ReapStaleClaims(ctx context.Context, timeout time.Duration) ([]ReapedTask, error) {
	rows, err := s.pool.Query(ctx,
		`UPDATE agent_tasks
		 SET status = CASE WHEN attempts >= max_attempts THEN 'failed' ELSE 'pending' END,
		     retry_after = now() + interval '30 seconds',
		     error = 'claim timeout: agent did not ack/nak within deadline'
		 WHERE status = 'claimed'
		   AND claimed_at < now() - make_interval(secs => $1)
		 RETURNING id, agent_name`,
		timeout.Seconds(),
	)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: reap stale claims: %w", err)
	}
	defer rows.Close()

	var reaped []ReapedTask
	for rows.Next() {
		var rt ReapedTask
		if err := rows.Scan(&rt.ID, &rt.AgentName); err != nil {
			return nil, fmt.Errorf("eventqueue: scan reaped task: %w", err)
		}
		reaped = append(reaped, rt)
	}
	return reaped, rows.Err()
}

// StreamInfo returns summary statistics for the event queue, used by
// observability endpoints.
type StreamInfo struct {
	EventsTotal    int64 `json:"events_total"`
	EventsUnrouted int64 `json:"events_unrouted"`
	TasksPending   int64 `json:"tasks_pending"`
	TasksClaimed   int64 `json:"tasks_claimed"`
	TasksCompleted int64 `json:"tasks_completed"`
	TasksFailed    int64 `json:"tasks_failed"`
}

// GetStreamInfo returns queue statistics.
func (s *Store) GetStreamInfo(ctx context.Context) (*StreamInfo, error) {
	var info StreamInfo
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM events) AS events_total,
			(SELECT count(*) FROM events WHERE routed_at IS NULL) AS events_unrouted,
			(SELECT count(*) FROM agent_tasks WHERE status = 'pending') AS tasks_pending,
			(SELECT count(*) FROM agent_tasks WHERE status = 'claimed') AS tasks_claimed,
			(SELECT count(*) FROM agent_tasks WHERE status = 'completed') AS tasks_completed,
			(SELECT count(*) FROM agent_tasks WHERE status = 'failed') AS tasks_failed
	`).Scan(&info.EventsTotal, &info.EventsUnrouted, &info.TasksPending, &info.TasksClaimed, &info.TasksCompleted, &info.TasksFailed)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: stream info: %w", err)
	}
	return &info, nil
}

// SubjectFilter constrains queries to events whose derived subject —
// "<connector>.<eventType>", where the event type comes from the webhook
// event-type header (any header ending in "-Event", falling back to a
// generic "type" header) — matches a pattern. The zero value matches all
// events.
type SubjectFilter struct {
	// Connector, if non-empty, restricts matches to this connector (case-insensitive).
	Connector string
	// EventType, if non-empty, restricts matches to this event type (case-insensitive).
	EventType string
	// None marks a pattern that can never match; queries can short-circuit.
	None bool
}

// ParseSubjectFilter parses a subject pattern such as "forgejo.push",
// "forgejo.*", "*.push" or "forgejo.>". Tokens are split on "."; "*"
// matches any single token and a trailing ">" matches any remainder.
// Subjects have exactly two levels (connector and event type), so patterns
// with extra non-wildcard tokens never match. An empty pattern matches
// everything.
func ParseSubjectFilter(pattern string) SubjectFilter {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" || pattern == ">" || pattern == "*" {
		return SubjectFilter{}
	}
	tokens := strings.Split(pattern, ".")
	var sf SubjectFilter
	for _, tok := range tokens {
		if tok == "" {
			return SubjectFilter{None: true}
		}
	}
	switch tokens[0] {
	case ">", "*":
	default:
		sf.Connector = tokens[0]
	}
	if len(tokens) == 1 {
		return sf
	}
	switch tokens[1] {
	case ">", "*":
	default:
		sf.EventType = tokens[1]
	}
	// Deeper subjects do not exist; extra tokens only match as wildcards.
	for _, tok := range tokens[2:] {
		if tok != ">" && tok != "*" {
			return SubjectFilter{None: true}
		}
	}
	return sf
}

// Matches reports whether an event with the given connector and derived
// event type satisfies the filter.
func (sf SubjectFilter) Matches(connector, eventType string) bool {
	if sf.None {
		return false
	}
	if sf.Connector != "" && !strings.EqualFold(sf.Connector, connector) {
		return false
	}
	if sf.EventType != "" && !strings.EqualFold(sf.EventType, eventType) {
		return false
	}
	return true
}

// EventFilter constrains QueryEvents/CountEvents queries. Zero values mean
// "no filter".
type EventFilter struct {
	// Connector limits results to events from this connector.
	Connector string
	// Channel limits results to events born in this channel (events.channel_id).
	Channel string
	// Channels limits results to events born in any of these channels — the
	// set a grouping channel reaches, so one query answers "what flowed through
	// this channel".
	Channels []string
	// Since limits results to events received at or after this time.
	Since time.Time
	// Status filters by derived activity status: "matched", "unmatched" or
	// "error". Any other value applies no filter; callers are expected to
	// validate.
	Status string
	// Agent limits results to events routed to this agent.
	Agent string
	// Subject is a subject pattern (see ParseSubjectFilter) of the form
	// "<connector>.<eventType>" with "*"/">" wildcards that filters events
	// by their derived event type from the webhook event-type header.
	Subject string
}

// conditions renders the filter as SQL conditions with positional parameters.
// The events table is aliased as e.
func (f EventFilter) conditions() (conds []string, args []any) {
	if f.Connector != "" {
		args = append(args, f.Connector)
		conds = append(conds, fmt.Sprintf("e.connector = $%d", len(args)))
	}
	if f.Channel != "" {
		args = append(args, f.Channel)
		conds = append(conds, fmt.Sprintf("e.channel_id = $%d", len(args)))
	}
	if len(f.Channels) > 0 {
		args = append(args, f.Channels)
		conds = append(conds, fmt.Sprintf("e.channel_id = ANY($%d)", len(args)))
	}
	if !f.Since.IsZero() {
		args = append(args, f.Since)
		conds = append(conds, fmt.Sprintf("e.received_at >= $%d", len(args)))
	}
	if f.Agent != "" {
		args = append(args, f.Agent)
		conds = append(conds, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM agent_tasks t WHERE t.event_id = e.id AND t.agent_name = $%d)", len(args)))
	}
	switch f.Status {
	case "unmatched":
		conds = append(conds,
			"NOT EXISTS (SELECT 1 FROM agent_tasks t WHERE t.event_id = e.id)")
	case "matched":
		conds = append(conds,
			"EXISTS (SELECT 1 FROM agent_tasks t WHERE t.event_id = e.id)",
			"NOT EXISTS (SELECT 1 FROM agent_tasks t WHERE t.event_id = e.id AND t.status = 'failed')")
	case "error":
		conds = append(conds,
			"EXISTS (SELECT 1 FROM agent_tasks t WHERE t.event_id = e.id AND t.status = 'failed')")
	}
	// Subject filter: parse a subject pattern of the form "<connector>.<eventType>"
	// with "*"/">" wildcards and add SQL conditions. A pattern that can never
	// match (e.g. three non-wildcard tokens) adds a literal false condition.
	if f.Subject != "" {
		sf := ParseSubjectFilter(f.Subject)
		if sf.None {
			conds = append(conds, "false")
		} else {
			if sf.Connector != "" {
				args = append(args, sf.Connector)
				conds = append(conds, fmt.Sprintf("lower(e.connector) = $%d", len(args)))
			}
			if sf.EventType != "" {
				// Mirrors trigger.CanonicalEventType: any header ending in "-Event",
				// falling back to a generic "type" entry.
				args = append(args, sf.EventType)
				conds = append(conds, fmt.Sprintf(
					`(EXISTS (SELECT 1 FROM jsonb_each_text(e.headers) kv WHERE kv.key ILIKE '%%-event' AND lower(kv.value) = $%d) OR lower(coalesce(e.headers->>'type', '')) = $%d)`,
					len(args), len(args)))
			}
		}
	}
	return conds, args
}

// QueryEvents returns a page of events matching the filter, newest first.
// Limit/offset pagination uses (received_at DESC, id DESC) ordering so pages
// are stable even for events sharing a timestamp.
func (s *Store) QueryEvents(ctx context.Context, f EventFilter, limit, offset int) ([]Event, error) {
	conds, args := f.conditions()
	query := `SELECT id, connector, channel_id, headers, data, raw, received_at FROM events e`
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, " AND ")
	}
	args = append(args, limit, offset)
	query += fmt.Sprintf(` ORDER BY e.received_at DESC, e.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: query events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		var channelID *string
		if err := rows.Scan(&e.ID, &e.Connector, &channelID, &e.Headers, &e.Data, &e.Raw, &e.ReceivedAt); err != nil {
			return nil, fmt.Errorf("eventqueue: scan event: %w", err)
		}
		if channelID != nil {
			e.ChannelID = *channelID
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// CountEvents returns the number of events matching the filter.
func (s *Store) CountEvents(ctx context.Context, f EventFilter) (int, error) {
	conds, args := f.conditions()
	query := `SELECT count(*) FROM events e`
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, " AND ")
	}
	var total int
	if err := s.pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("eventqueue: count events: %w", err)
	}
	return total, nil
}

// RecentEvents returns the most recent events, optionally filtered by connector
// and/or a subject pattern (see ParseSubjectFilter), and/or restricted to events
// received at or after since (a zero since means no lower bound).
func (s *Store) RecentEvents(ctx context.Context, count int, connector, subjectFilter string, since time.Time) ([]Event, error) {
	return s.QueryEvents(ctx, EventFilter{Connector: connector, Since: since, Subject: subjectFilter}, count, 0)
}

// GetEvent returns a single event by ID, or nil if it does not exist.
func (s *Store) GetEvent(ctx context.Context, id string) (*Event, error) {
	var e Event
	var channelID *string
	err := s.pool.QueryRow(ctx,
		`SELECT id, connector, channel_id, headers, data, raw, received_at FROM events WHERE id = $1`, id,
	).Scan(&e.ID, &e.Connector, &channelID, &e.Headers, &e.Data, &e.Raw, &e.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("eventqueue: get event %q: %w", id, err)
	}
	if channelID != nil {
		e.ChannelID = *channelID
	}
	return &e, nil
}

// TasksForEvents returns all agent tasks for the given event IDs in a single
// query. The returned tasks carry their Status and InvocationID so callers
// can derive an activity status per event and look up the corresponding
// invocation for run-state information.
func (s *Store) TasksForEvents(ctx context.Context, eventIDs []string) ([]Task, error) {
	if len(eventIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, event_id, agent_name, trigger_name, invocation_id, status,
		        attempts, error, created_at, completed_at
		 FROM agent_tasks
		 WHERE event_id = ANY($1)
		 ORDER BY id ASC`, eventIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: tasks for events: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.EventID, &t.AgentName, &t.TriggerName, &t.InvocationID, &t.Status,
			&t.Attempts, &t.Error, &t.CreatedAt, &t.CompletedAt); err != nil {
			return nil, fmt.Errorf("eventqueue: scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// ---------------------------------------------------------------------------
// Channel views
//
// The `channels` table itself belongs to internal/channels; the queries below
// answer "what happened to the events and tasks this package owns, grouped by
// the channel they relate to", which is why they live here.
// ---------------------------------------------------------------------------

// BirthStat counts the events a single channel received by birth — events
// that were born in it — within a time window.
type BirthStat struct {
	ChannelID string `json:"channelId"`
	Events    int    `json:"events"`
	// Unmatched are births no subscription transferred onward.
	Unmatched int `json:"unmatched"`
	// Failed are births whose delivery ended in a failed task.
	Failed int `json:"failed"`
}

// BirthStats counts events by birth channel from since onwards. Channels with
// no events in the window are absent from the result.
func (s *Store) BirthStats(ctx context.Context, since time.Time) ([]BirthStat, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT e.channel_id,
		        count(*) AS events,
		        count(*) FILTER (WHERE NOT EXISTS (
		            SELECT 1 FROM agent_tasks t WHERE t.event_id = e.id)) AS unmatched,
		        count(*) FILTER (WHERE EXISTS (
		            SELECT 1 FROM agent_tasks t WHERE t.event_id = e.id AND t.status = 'failed')) AS failed
		   FROM events e
		  WHERE e.channel_id IS NOT NULL AND e.received_at >= $1
		  GROUP BY e.channel_id`, since)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: birth stats: %w", err)
	}
	defer rows.Close()

	var out []BirthStat
	for rows.Next() {
		var st BirthStat
		if err := rows.Scan(&st.ChannelID, &st.Events, &st.Unmatched, &st.Failed); err != nil {
			return nil, fmt.Errorf("eventqueue: birth stats: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// DeliveryStat counts the events delivered into one agent's inbox within a
// time window. Keyed by agent name because deliveries are recorded as tasks;
// the caller maps names back to channels.
type DeliveryStat struct {
	AgentName string `json:"agentName"`
	Events    int    `json:"events"`
	Failed    int    `json:"failed"`
}

// DeliveryStats counts tasks by agent from since onwards.
func (s *Store) DeliveryStats(ctx context.Context, since time.Time) ([]DeliveryStat, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.agent_name,
		        count(DISTINCT t.event_id) AS events,
		        count(DISTINCT t.event_id) FILTER (WHERE t.status = 'failed') AS failed
		   FROM agent_tasks t
		  WHERE t.created_at >= $1
		  GROUP BY t.agent_name`, since)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: delivery stats: %w", err)
	}
	defer rows.Close()

	var out []DeliveryStat
	for rows.Next() {
		var st DeliveryStat
		if err := rows.Scan(&st.AgentName, &st.Events, &st.Failed); err != nil {
			return nil, fmt.Errorf("eventqueue: delivery stats: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ChannelStamp pairs a label (a connector label, or an agent name for
// directly-published events) with the id of the channel that carries it.
type ChannelStamp struct {
	Label     string
	ChannelID string
}

// StampBirthChannels assigns a birth channel to events that have none yet,
// matched on their connector label. It runs once per reconcile pass so history
// written before channels existed becomes readable through them. Returns the
// number of events stamped.
func (s *Store) StampBirthChannels(ctx context.Context, stamps []ChannelStamp) (int64, error) {
	if len(stamps) == 0 {
		return 0, nil
	}
	labels := make([]string, len(stamps))
	ids := make([]string, len(stamps))
	for i, st := range stamps {
		labels[i], ids[i] = st.Label, st.ChannelID
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE events e
		    SET channel_id = m.channel_id
		   FROM (SELECT * FROM unnest($1::text[], $2::text[]) AS m(label, channel_id)) m
		  WHERE e.channel_id IS NULL AND e.connector = m.label`,
		labels, ids)
	if err != nil {
		return 0, fmt.Errorf("eventqueue: stamp birth channels: %w", err)
	}
	return tag.RowsAffected(), nil
}

// StampInboxBirthChannels assigns a birth channel to events the hub published
// directly into an agent's inbox — the labels in `directLabels`, which are
// cron ticks and chat messages. Their delivery record says which agent received
// them, which is the channel they were born in. Returns the number stamped.
func (s *Store) StampInboxBirthChannels(ctx context.Context, directLabels []string, stamps []ChannelStamp) (int64, error) {
	if len(stamps) == 0 || len(directLabels) == 0 {
		return 0, nil
	}
	names := make([]string, len(stamps))
	ids := make([]string, len(stamps))
	for i, st := range stamps {
		names[i], ids[i] = st.Label, st.ChannelID
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE events e
		    SET channel_id = m.channel_id
		   FROM (SELECT * FROM unnest($1::text[], $2::text[]) AS m(agent_name, channel_id)) m
		   JOIN agent_tasks t ON t.agent_name = m.agent_name
		  WHERE e.channel_id IS NULL AND t.event_id = e.id AND e.connector = ANY($3)`,
		names, ids, directLabels)
	if err != nil {
		return 0, fmt.Errorf("eventqueue: stamp inbox birth channels: %w", err)
	}
	return tag.RowsAffected(), nil
}

// UnstampedSourceLabels returns the distinct connector labels of events with no
// birth channel. The channel reconciler uses it to provision streams that
// history refers to but the registry no longer describes — a connector that was
// deleted before channels existed still has events that deserve a home.
func (s *Store) UnstampedSourceLabels(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT connector FROM events WHERE channel_id IS NULL ORDER BY connector`)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: unstamped source labels: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, fmt.Errorf("eventqueue: unstamped source labels: %w", err)
		}
		out = append(out, label)
	}
	return out, rows.Err()
}

// UnstampedInboxAgents returns the distinct agent names that received events
// published directly into an inbox (the given source labels) where those events
// still have no birth channel. Agents deleted before channels existed appear
// here, so their history keeps a stream instead of pointing at nothing.
func (s *Store) UnstampedInboxAgents(ctx context.Context, directLabels []string) ([]string, error) {
	if len(directLabels) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT t.agent_name
		   FROM agent_tasks t JOIN events e ON e.id = t.event_id
		  WHERE e.channel_id IS NULL AND e.connector = ANY($1)
		  ORDER BY t.agent_name`, directLabels)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: unstamped inbox agents: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("eventqueue: unstamped inbox agents: %w", err)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
