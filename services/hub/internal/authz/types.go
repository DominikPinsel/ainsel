package authz

import "time"

// GroupRole is the permission level a user has within a group.
type GroupRole string

const (
	RoleReader GroupRole = "reader"
	RoleWriter GroupRole = "writer"
	RoleOwner  GroupRole = "owner"
)

// roleCovers returns true if `have` covers `need`.
// owner covers writer covers reader.
func roleCovers(have, need GroupRole) bool {
	rank := map[GroupRole]int{RoleReader: 1, RoleWriter: 2, RoleOwner: 3}
	return rank[have] >= rank[need]
}

// User is a hub-managed user record, synced lazily from Zitadel JWT claims.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Username  string    `json:"username"`
	IsAdmin   bool      `json:"isAdmin"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Group is a named collection of users. Groups are flat — no nesting.
type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// MemberWithUser is a resolved group member for API responses.
type MemberWithUser struct {
	User User      `json:"user"`
	Role GroupRole `json:"role"`
}

// ResourceGroup maps a resource to its group and visibility.
type ResourceGroup struct {
	ResourceType string    `json:"resourceType"`
	ResourceName string    `json:"resourceName"`
	GroupID      string    `json:"groupId"`
	Public       bool      `json:"public"`
	CreatedAt    time.Time `json:"createdAt"`
}
