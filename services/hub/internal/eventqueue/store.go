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
	ID         string          `json:"id"`
	Connector  string          `json:"connector"`
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

// InsertEvent stores a new connector event. Duplicate IDs are silently ignored.
func (s *Store) InsertEvent(ctx context.Context, evt Event) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO events (id, connector, headers, data, raw)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO NOTHING`,
		evt.ID, evt.Connector, evt.Headers, evt.Data, evt.Raw,
	)
	if err != nil {
		return fmt.Errorf("eventqueue: insert event %q: %w", evt.ID, err)
	}
	return nil
}

// FetchUnrouted returns up to limit events that have not yet been routed,
// ordered by receive time.
func (s *Store) FetchUnrouted(ctx context.Context, limit int) ([]Event, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, connector, headers, data, raw
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
		if err := rows.Scan(&e.ID, &e.Connector, &e.Headers, &e.Data, &e.Raw); err != nil {
			return nil, fmt.Errorf("eventqueue: scan event: %w", err)
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

// RecentEvents returns the most recent events, optionally filtered by connector
// and/or restricted to events received at or after since (a zero since means
// no lower bound).
func (s *Store) RecentEvents(ctx context.Context, count int, connector string, since time.Time) ([]Event, error) {
	query := `SELECT id, connector, headers, data, raw, received_at FROM events`
	args := []any{}
	conds := []string{}
	if connector != "" {
		args = append(args, connector)
		conds = append(conds, fmt.Sprintf("connector = $%d", len(args)))
	}
	if !since.IsZero() {
		args = append(args, since)
		conds = append(conds, fmt.Sprintf("received_at >= $%d", len(args)))
	}
	if len(conds) > 0 {
		query += ` WHERE ` + strings.Join(conds, " AND ")
	}
	args = append(args, count)
	query += fmt.Sprintf(` ORDER BY received_at DESC LIMIT $%d`, len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("eventqueue: recent events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Connector, &e.Headers, &e.Data, &e.Raw, &e.ReceivedAt); err != nil {
			return nil, fmt.Errorf("eventqueue: scan recent event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// GetEvent returns a single event by ID, or nil if it does not exist.
func (s *Store) GetEvent(ctx context.Context, id string) (*Event, error) {
	var e Event
	err := s.pool.QueryRow(ctx,
		`SELECT id, connector, headers, data, raw, received_at FROM events WHERE id = $1`, id,
	).Scan(&e.ID, &e.Connector, &e.Headers, &e.Data, &e.Raw, &e.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("eventqueue: get event %q: %w", id, err)
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
		`SELECT id, event_id, agent_name, trigger_name, invocation_id, status
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
		if err := rows.Scan(&t.ID, &t.EventID, &t.AgentName, &t.TriggerName, &t.InvocationID, &t.Status); err != nil {
			return nil, fmt.Errorf("eventqueue: scan task: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
