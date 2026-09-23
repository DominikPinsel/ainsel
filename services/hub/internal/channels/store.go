package channels

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors surfaced by the store and mapped to HTTP statuses by the API.
var (
	// ErrNotFound is returned when a channel or bridge does not exist.
	ErrNotFound = errors.New("channel not found")
	// ErrInUse is returned when a custom channel still has bridges attached;
	// deleting it would silently drop the subscriptions its users configured.
	ErrInUse = errors.New("channel has bridges attached")
	// ErrNotCustom is returned when a mutation is applied to a
	// provisioned channel — its name and description belong to its registry
	// entity, not to whoever is looking at it.
	ErrNotCustom = errors.New("channel is provisioned by its registry entity and cannot be edited here")
	// ErrBridgeExists is returned when the same channel→channel edge is added
	// twice.
	ErrBridgeExists = errors.New("bridge between these channels already exists")
	// ErrCycle is returned when a bridge would close a loop in the channel
	// graph. Transfers are guarded at runtime too, but a cyclic graph means an
	// event can never finish moving, so the edge is refused at write time.
	ErrCycle = errors.New("bridge would create a cycle in the channel graph")
	// ErrSelfEdge is returned when a channel is bridged to itself.
	ErrSelfEdge = errors.New("a channel cannot be bridged to itself")
	// ErrNoCustomEndpoint is returned when neither end of a bridge is a custom
	// channel. Connector→agent edges are triggers; a second record for the same
	// pair would put event routing in two places.
	ErrNoCustomEndpoint = errors.New("a bridge must attach at least one custom channel")
)

// maxWalkDepth bounds how far a single traversal follows bridges. Grouping
// channels are shallow by design; the bound keeps a misconfigured graph from
// turning one event into a table scan.
const maxWalkDepth = 8

// Store is the database layer for channels and bridges.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires a Store against the given pgxpool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// newID returns a channel/bridge id. Format: "{prefix}-{8 hex}", matching the
// hub's other generated resources.
func newID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}

const channelColumns = `id, kind, name, description, COALESCE(entity_ref, ''), orphaned, created_at, updated_at`

func scanChannel(row pgx.Row) (Channel, error) {
	var c Channel
	var kind string
	err := row.Scan(&c.ID, &kind, &c.Name, &c.Description, &c.EntityRef, &c.Orphaned, &c.CreatedAt, &c.UpdatedAt)
	c.Kind = Kind(kind)
	return c, err
}

// Ensure provisions the channel for a registry entity, or refreshes the one
// that already exists. It is the single provisioning primitive: the reconcile
// loop calls it for every Agent and WebhookConnector CR, and the event ingest
// path calls it for a connector label it has not seen yet, so a channel always
// exists before an event born in it needs an id.
//
// Empty name/description leave the stored values alone — a caller that only
// knows the entity ref must not wipe a nicer display label.
func (s *Store) Ensure(ctx context.Context, kind Kind, entityRef, name, description string) (*Channel, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("channels.Ensure: unknown kind %q", kind)
	}
	if entityRef == "" {
		return nil, errors.New("channels.Ensure: entity_ref is required")
	}
	if kind == KindCustom {
		return nil, errors.New("channels.Ensure: custom channels are created with Create, not Ensure")
	}
	c, err := scanChannel(s.pool.QueryRow(ctx,
		`INSERT INTO channels (id, kind, name, description, entity_ref, orphaned, created_at, updated_at)
		 VALUES ($1, $2, COALESCE(NULLIF($3, ''), $4), COALESCE(NULLIF($5, ''), $6), $7, false, now(), now())
		 ON CONFLICT (kind, entity_ref) WHERE entity_ref IS NOT NULL DO UPDATE SET
		     name        = COALESCE(NULLIF(EXCLUDED.name, ''), channels.name),
		     description = COALESCE(NULLIF(EXCLUDED.description, ''), channels.description),
		     orphaned    = false,
		     updated_at  = now()
		 RETURNING `+channelColumns,
		newID("ch"), string(kind), name, entityRef, description, DefaultDescription(kind, entityRef), entityRef,
	))
	if err != nil {
		return nil, fmt.Errorf("channels.Ensure %s/%s: %w", kind, entityRef, err)
	}
	return &c, nil
}

// Create inserts a user-authored custom channel.
func (s *Store) Create(ctx context.Context, name, description string) (*Channel, error) {
	if description == "" {
		description = DefaultDescription(KindCustom, name)
	}
	c, err := scanChannel(s.pool.QueryRow(ctx,
		`INSERT INTO channels (id, kind, name, description)
		 VALUES ($1, 'custom', $2, $3)
		 RETURNING `+channelColumns,
		newID("ch"), name, description,
	))
	if err != nil {
		return nil, fmt.Errorf("channels.Create: %w", err)
	}
	return &c, nil
}

// Get returns a channel by id, or ErrNotFound.
func (s *Store) Get(ctx context.Context, id string) (*Channel, error) {
	c, err := scanChannel(s.pool.QueryRow(ctx,
		`SELECT `+channelColumns+` FROM channels WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("channels.Get: %w", err)
	}
	return &c, nil
}

// GetByEntity returns the provisioned channel for a registry entity.
func (s *Store) GetByEntity(ctx context.Context, kind Kind, entityRef string) (*Channel, error) {
	c, err := scanChannel(s.pool.QueryRow(ctx,
		`SELECT `+channelColumns+` FROM channels WHERE kind = $1 AND entity_ref = $2`, string(kind), entityRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("channels.GetByEntity: %w", err)
	}
	return &c, nil
}

// List returns channels ordered for display: kind, then label, then id so that
// two channels sharing a label keep a stable order. When kind is empty all
// kinds are returned.
func (s *Store) List(ctx context.Context, kind Kind) ([]Channel, error) {
	query := `SELECT ` + channelColumns + ` FROM channels`
	args := []any{}
	if kind != "" {
		query += ` WHERE kind = $1`
		args = append(args, string(kind))
	}
	query += ` ORDER BY kind, name, id`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("channels.List: %w", err)
	}
	defer rows.Close()

	var out []Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, fmt.Errorf("channels.List: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Update renames a channel or changes its description. Only custom channels
// carry authored labels; for provisioned channels the registry entity owns
// them, so the call is refused with ErrNotCustom.
func (s *Store) Update(ctx context.Context, id string, name, description *string) (*Channel, error) {
	// The kind guard is part of the statement, so a channel that turns out to
	// be provisioned between the read and the write cannot be renamed anyway.
	updated, err := s.pool.Exec(ctx,
		`UPDATE channels
		 SET name = COALESCE($2, name),
		     description = COALESCE($3, description),
		     updated_at = now()
		 WHERE id = $1 AND kind = 'custom'`,
		id, name, description)
	if err != nil {
		return nil, fmt.Errorf("channels.Update: %w", err)
	}
	if updated.RowsAffected() == 0 {
		if _, getErr := s.Get(ctx, id); errors.Is(getErr, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, ErrNotCustom
	}
	return s.Get(ctx, id)
}

// Delete removes a custom channel. Provisioned channels cannot be deleted here
// — their lifecycle is their entity's — and a channel with bridges attached is
// refused so nobody loses a subscription by accident. Bridges are removed with
// the channel by ON DELETE CASCADE only after the explicit check below.
func (s *Store) Delete(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("channels.Delete: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var kind string
	err = tx.QueryRow(ctx, `SELECT kind FROM channels WHERE id = $1 FOR UPDATE`, id).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("channels.Delete: %w", err)
	}
	if Kind(kind) != KindCustom {
		return ErrNotCustom
	}

	var attached int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM channel_bridges WHERE from_channel = $1 OR to_channel = $1`, id,
	).Scan(&attached); err != nil {
		return fmt.Errorf("channels.Delete: count bridges: %w", err)
	}
	if attached > 0 {
		return ErrInUse
	}

	if _, err := tx.Exec(ctx, `DELETE FROM channels WHERE id = $1`, id); err != nil {
		return fmt.Errorf("channels.Delete: %w", err)
	}
	return tx.Commit(ctx)
}

// MarkOrphans flags channels of one kind whose registry entity is gone and
// returns how many rows it changed. The live refs are the entity_refs still
// present in the registry; passing an empty slice orphans every channel of that
// kind, so callers must only call this after a successful registry listing.
func (s *Store) MarkOrphans(ctx context.Context, kind Kind, liveRefs []string) (int64, error) {
	if !kind.Valid() || kind == KindCustom {
		return 0, fmt.Errorf("channels.MarkOrphans: kind %q is not provisioned from a registry", kind)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE channels SET orphaned = true, updated_at = now()
		 WHERE kind = $1 AND entity_ref IS NOT NULL AND orphaned = false
		   AND NOT (entity_ref = ANY($2))`,
		string(kind), liveRefs)
	if err != nil {
		return 0, fmt.Errorf("channels.MarkOrphans: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ---------------------------------------------------------------------------
// Bridges
// ---------------------------------------------------------------------------

const bridgeColumns = `id, name, from_channel, to_channel, created_at, updated_at`

func scanBridge(row pgx.Row) (Bridge, error) {
	var b Bridge
	err := row.Scan(&b.ID, &b.Name, &b.FromChannel, &b.ToChannel, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}

// CreateBridge attaches one channel's stream to another. Both endpoints must
// exist, at least one must be a custom channel, and the edge must not close a
// loop. The returned error is one of the sentinels above.
func (s *Store) CreateBridge(ctx context.Context, fromID, toID, name string) (*Bridge, error) {
	if fromID == toID {
		return nil, ErrSelfEdge
	}
	from, to, err := s.pair(ctx, fromID, toID)
	if err != nil {
		return nil, err
	}
	if from.Kind != KindCustom && to.Kind != KindCustom {
		return nil, ErrNoCustomEndpoint
	}
	// Adding from→to closes a loop when to already reaches from.
	reaches, err := s.Reaches(ctx, toID, fromID)
	if err != nil {
		return nil, err
	}
	if reaches {
		return nil, ErrCycle
	}
	if name == "" {
		name = from.Name + " → " + to.Name
	}

	id := newID("br")
	tag, err := s.pool.Exec(ctx,
		`INSERT INTO channel_bridges (id, name, from_channel, to_channel)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (from_channel, to_channel) DO NOTHING`,
		id, name, fromID, toID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrBridgeExists
		}
		return nil, fmt.Errorf("channels.CreateBridge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrBridgeExists
	}
	b, err := s.GetBridge(ctx, id)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Store) pair(ctx context.Context, fromID, toID string) (*Channel, *Channel, error) {
	from, err := s.Get(ctx, fromID)
	if err != nil {
		return nil, nil, err
	}
	to, err := s.Get(ctx, toID)
	if err != nil {
		return nil, nil, err
	}
	return from, to, nil
}

// GetBridge returns one bridge by id.
func (s *Store) GetBridge(ctx context.Context, id string) (*Bridge, error) {
	b, err := scanBridge(s.pool.QueryRow(ctx,
		`SELECT `+bridgeColumns+` FROM channel_bridges WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("channels.GetBridge: %w", err)
	}
	return &b, nil
}

// ListBridges returns bridges, optionally limited to one channel (either end).
func (s *Store) ListBridges(ctx context.Context, channelID string) ([]Bridge, error) {
	query := `SELECT ` + bridgeColumns + ` FROM channel_bridges`
	args := []any{}
	if channelID != "" {
		query += ` WHERE from_channel = $1 OR to_channel = $1`
		args = append(args, channelID)
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("channels.ListBridges: %w", err)
	}
	defer rows.Close()

	var out []Bridge
	for rows.Next() {
		b, err := scanBridge(rows)
		if err != nil {
			return nil, fmt.Errorf("channels.ListBridges: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// DeleteBridge removes one bridge.
func (s *Store) DeleteBridge(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM channel_bridges WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("channels.DeleteBridge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Traversal
// ---------------------------------------------------------------------------

// walkRow is one node reached by following bridges.
type walkRow struct {
	channelID  string
	kind       Kind
	entityRef  string
	depth      int
	bridgeID   string
	bridgeName string
}

// Walk follows bridges forward from a channel and returns every channel it
// reaches, shallowest first. The start channel itself is not returned. Paths
// are cycle-guarded by construction (a channel already on the path is not
// entered again) and bounded by maxWalkDepth.
func (s *Store) Walk(ctx context.Context, fromChannelID string) ([]walkRow, error) {
	rows, err := s.pool.Query(ctx,
		`WITH RECURSIVE walk AS (
		     SELECT c.id AS id, c.kind::text AS kind, COALESCE(c.entity_ref, '') AS entity_ref,
		            0 AS depth, ARRAY[c.id]::text[] AS path,
		            ''::text AS bridge_id, ''::text AS bridge_name
		       FROM channels c WHERE c.id = $1
		     UNION ALL
		     SELECT b.to_channel, c2.kind::text, COALESCE(c2.entity_ref, ''), w.depth + 1,
		            w.path || b.to_channel, b.id, b.name
		       FROM walk w
		       JOIN channel_bridges b ON b.from_channel = w.id
		       JOIN channels c2 ON c2.id = b.to_channel
		      WHERE NOT b.to_channel = ANY(w.path) AND w.depth < $2
		 )
		 SELECT id, kind, entity_ref, depth, bridge_id, bridge_name
		   FROM walk WHERE depth > 0 ORDER BY depth, id`,
		fromChannelID, maxWalkDepth)
	if err != nil {
		return nil, fmt.Errorf("channels.Walk: %w", err)
	}
	defer rows.Close()

	var out []walkRow
	for rows.Next() {
		var w walkRow
		if err := rows.Scan(&w.channelID, &w.kind, &w.entityRef, &w.depth, &w.bridgeID, &w.bridgeName); err != nil {
			return nil, fmt.Errorf("channels.Walk: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Deliveries returns the agent inboxes an event born in fromChannelID is
// transferred into by bridges, one per destination inbox, attributed to the
// bridge that ends the shortest path to it.
func (s *Store) Deliveries(ctx context.Context, fromChannelID string) ([]Delivery, error) {
	walked, err := s.Walk(ctx, fromChannelID)
	if err != nil {
		return nil, err
	}
	// Walk is ordered shallowest-first, so the first hit per inbox wins and
	// an event is delivered to a given agent at most once.
	seen := make(map[string]bool, len(walked))
	var out []Delivery
	for _, w := range walked {
		if w.kind != KindAgent || w.entityRef == "" || seen[w.channelID] {
			continue
		}
		seen[w.channelID] = true
		out = append(out, Delivery{
			AgentChannel: w.channelID,
			AgentName:    w.entityRef,
			BridgeID:     w.bridgeID,
			BridgeName:   w.bridgeName,
		})
	}
	return out, nil
}

// Reaches reports whether a path of one or more bridges leads from one channel
// to another. A channel never reaches itself: only real paths count, which is
// what makes this usable as a cycle test before adding an edge.
func (s *Store) Reaches(ctx context.Context, fromChannelID, toChannelID string) (bool, error) {
	var found bool
	err := s.pool.QueryRow(ctx,
		`WITH RECURSIVE walk AS (
		     SELECT $1::text AS id, ARRAY[$1]::text[] AS path
		     UNION ALL
		     SELECT b.to_channel, w.path || b.to_channel
		       FROM walk w JOIN channel_bridges b ON b.from_channel = w.id
		      WHERE NOT b.to_channel = ANY(w.path)
		 )
		 SELECT EXISTS (SELECT 1 FROM walk WHERE id = $2 AND NOT ($2 = $1))`,
		fromChannelID, toChannelID,
	).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("channels.Reaches: %w", err)
	}
	return found, nil
}

// ReachingInto returns the channel ids whose events arrive in the given
// channel — that is the channel itself plus everything that reaches it through
// bridges. Used to count and list what flows through a grouping channel.
func (s *Store) ReachingInto(ctx context.Context, channelID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`WITH RECURSIVE walk AS (
		     SELECT c.id, ARRAY[c.id]::text[] AS path
		       FROM channels c WHERE c.id = $1
		     UNION ALL
		     SELECT b.from_channel, w.path || b.from_channel
		       FROM walk w JOIN channel_bridges b ON b.to_channel = w.id
		      WHERE NOT b.from_channel = ANY(w.path)
		 )
		 SELECT DISTINCT id FROM walk ORDER BY id`, channelID)
	if err != nil {
		return nil, fmt.Errorf("channels.ReachingInto: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("channels.ReachingInto: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
