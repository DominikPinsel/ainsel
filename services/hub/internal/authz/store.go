package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

// Store is the authz Postgres layer.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new authz store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func newID() string { return ulid.Make().String() }

// --- Users ---

// UpsertUser creates or updates a user from OIDC claims. Returns the user.
// Empty email/username values are treated as "not provided" and will not
// overwrite existing non-empty values in the database.
func (s *Store) UpsertUser(ctx context.Context, sub, email, username string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (id, email, username, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())
		ON CONFLICT (id) DO UPDATE SET
			email    = CASE WHEN $2 = '' THEN users.email    ELSE $2 END,
			username = CASE WHEN $3 = '' THEN users.username  ELSE $3 END,
			updated_at = now()
		RETURNING id, email, username, is_admin, created_at, updated_at
	`, sub, email, username).Scan(&u.ID, &u.Email, &u.Username, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}
	return &u, nil
}

// GetUser returns a user by ID.
func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, username, is_admin, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Username, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("authz.GetUser: %w", err)
	}
	return &u, nil
}

// ListUsers returns all users ordered by username.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, email, username, is_admin, created_at, updated_at
		FROM users ORDER BY username
	`)
	if err != nil {
		return nil, fmt.Errorf("authz.ListUsers: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("authz.ListUsers: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SetAdmin sets or clears the is_admin flag.
func (s *Store) SetAdmin(ctx context.Context, userID string, isAdmin bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET is_admin = $1, updated_at = now() WHERE id = $2`, isAdmin, userID)
	if err != nil {
		return fmt.Errorf("authz.SetAdmin: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearUsername resets a user's stored username to '', triggering a fresh
// sync from the OIDC userinfo endpoint on the user's next authenticated request.
func (s *Store) ClearUsername(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE users SET username = '', updated_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("authz.ClearUsername: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Groups ---

// CreateGroup creates a new group.
func (s *Store) CreateGroup(ctx context.Context, name, description string) (*Group, error) {
	g := Group{ID: newID(), Name: name, Description: description}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO groups (id, name, description, created_at, updated_at)
		VALUES ($1, $2, $3, now(), now())
		RETURNING created_at, updated_at
	`, g.ID, g.Name, g.Description).Scan(&g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	return &g, nil
}

// GetGroup returns a group by ID.
func (s *Store) GetGroup(ctx context.Context, id string) (*Group, error) {
	var g Group
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, description, created_at, updated_at
		FROM groups WHERE id = $1
	`, id).Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("authz.GetGroup: %w", err)
	}
	return &g, nil
}

// ListGroups returns all groups ordered by name.
func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, created_at, updated_at
		FROM groups ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("authz.ListGroups: %w", err)
	}
	defer rows.Close()
	var out []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, fmt.Errorf("authz.ListGroups: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// UpdateGroup updates a group's name and description.
func (s *Store) UpdateGroup(ctx context.Context, id, name, description string) (*Group, error) {
	var g Group
	err := s.pool.QueryRow(ctx, `
		UPDATE groups SET name = $2, description = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, name, description, created_at, updated_at
	`, id, name, description).Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("authz.UpdateGroup: %w", err)
	}
	return &g, nil
}

// DeleteGroup removes a group. Cascades to members and resource_groups.
func (s *Store) DeleteGroup(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("authz.DeleteGroup: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AddGroupMember adds a user to a group with the given role.
func (s *Store) AddGroupMember(ctx context.Context, groupID, userID string, role GroupRole) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO group_members (group_id, user_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (group_id, user_id) DO UPDATE SET role = $3
	`, groupID, userID, string(role))
	if err != nil {
		return fmt.Errorf("authz.AddGroupMember: %w", err)
	}
	return nil
}

// RemoveGroupMember removes a user from a group.
func (s *Store) RemoveGroupMember(ctx context.Context, groupID, userID string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	if err != nil {
		return fmt.Errorf("authz.RemoveGroupMember: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListGroupMembers returns all users in a group with their roles.
func (s *Store) ListGroupMembers(ctx context.Context, groupID string) ([]MemberWithUser, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, u.email, u.username, u.is_admin, u.created_at, u.updated_at, gm.role
		FROM users u JOIN group_members gm ON gm.user_id = u.id
		WHERE gm.group_id = $1 ORDER BY u.username
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("authz.ListGroupMembers: %w", err)
	}
	defer rows.Close()
	var out []MemberWithUser
	for rows.Next() {
		var m MemberWithUser
		var role string
		if err := rows.Scan(&m.User.ID, &m.User.Email, &m.User.Username, &m.User.IsAdmin, &m.User.CreatedAt, &m.User.UpdatedAt, &role); err != nil {
			return nil, fmt.Errorf("authz.ListGroupMembers: %w", err)
		}
		m.Role = GroupRole(role)
		out = append(out, m)
	}
	return out, rows.Err()
}

// --- Group Resolution ---

// UserGroupIDs returns all group IDs a user belongs to (flat, no recursion).
func (s *Store) UserGroupIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT group_id FROM group_members WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("authz.UserGroupIDs: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("authz.UserGroupIDs: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UserGroupRoles returns the user's role in each group they belong to.
func (s *Store) UserGroupRoles(ctx context.Context, userID string) (map[string]GroupRole, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT group_id, role FROM group_members WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("authz.UserGroupRoles: %w", err)
	}
	defer rows.Close()
	out := make(map[string]GroupRole)
	for rows.Next() {
		var gid, role string
		if err := rows.Scan(&gid, &role); err != nil {
			return nil, fmt.Errorf("authz.UserGroupRoles: %w", err)
		}
		out[gid] = GroupRole(role)
	}
	return out, rows.Err()
}

// --- Resource Groups ---

// SetResourceGroup assigns a resource to a group.
func (s *Store) SetResourceGroup(ctx context.Context, resourceType, resourceName, groupID string, public bool) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO resource_groups (resource_type, resource_name, group_id, public, created_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (resource_type, resource_name) DO UPDATE SET group_id = $3, public = $4
	`, resourceType, resourceName, groupID, public)
	if err != nil {
		return fmt.Errorf("authz.SetResourceGroup: %w", err)
	}
	return nil
}

// GetResourceGroup returns the group mapping for a resource.
func (s *Store) GetResourceGroup(ctx context.Context, resourceType, resourceName string) (*ResourceGroup, error) {
	var rg ResourceGroup
	err := s.pool.QueryRow(ctx, `
		SELECT resource_type, resource_name, group_id, public, created_at
		FROM resource_groups WHERE resource_type = $1 AND resource_name = $2
	`, resourceType, resourceName).Scan(&rg.ResourceType, &rg.ResourceName, &rg.GroupID, &rg.Public, &rg.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("authz.GetResourceGroup: %w", err)
	}
	return &rg, nil
}

// DeleteResourceGroup removes the group mapping for a resource.
func (s *Store) DeleteResourceGroup(ctx context.Context, resourceType, resourceName string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM resource_groups WHERE resource_type = $1 AND resource_name = $2
	`, resourceType, resourceName)
	if err != nil {
		return fmt.Errorf("authz.DeleteResourceGroup: %w", err)
	}
	return nil
}

// SetResourcePublic toggles the public flag on a resource.
func (s *Store) SetResourcePublic(ctx context.Context, resourceType, resourceName string, public bool) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE resource_groups SET public = $3 WHERE resource_type = $1 AND resource_name = $2
	`, resourceType, resourceName, public)
	if err != nil {
		return fmt.Errorf("authz.SetResourcePublic: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListResourcesByGroups returns resource names of a type that belong to
// any of the given groups. If includePublic is true, also returns public
// resources from other groups.
func (s *Store) ListResourcesByGroups(ctx context.Context, resourceType string, groupIDs []string, includePublic bool) ([]string, error) {
	var rows pgx.Rows
	var err error
	if includePublic {
		rows, err = s.pool.Query(ctx, `
			SELECT resource_name FROM resource_groups
			WHERE resource_type = $1 AND (group_id = ANY($2) OR public = true)
			ORDER BY resource_name
		`, resourceType, groupIDs)
	} else {
		rows, err = s.pool.Query(ctx, `
			SELECT resource_name FROM resource_groups
			WHERE resource_type = $1 AND group_id = ANY($2)
			ORDER BY resource_name
		`, resourceType, groupIDs)
	}
	if err != nil {
		return nil, fmt.Errorf("authz.ListResourcesByGroups: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("authz.ListResourcesByGroups: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
