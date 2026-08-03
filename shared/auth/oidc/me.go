package oidc

import (
	"encoding/json"
	"net/http"
)

// meResponse is the JSON shape returned by MeHandler. It mirrors User but uses
// lowercase field names so consumers (the frontend, integration tests) get a
// stable wire contract independent of the Go struct's exported field names.
type meResponse struct {
	Sub      string   `json:"sub"`
	Email    string   `json:"email"`
	Username string   `json:"username"`
	Roles    []string `json:"roles"`
}

// MeHandler returns an http.Handler that serves GET /api/v1/auth/me by
// echoing the *User attached to the request context (typically by
// NewMiddleware) as JSON.
//
// If no *User is in the context — i.e. the request bypassed the auth
// middleware — the handler responds with 401 Unauthorized. This is defensive:
// the router should only mount MeHandler behind the middleware, but a missing
// context user should never leak a 200 with an empty body.
func MeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := FromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Normalize a nil Roles slice to [] in JSON. Otherwise encoding/json
		// emits "null", which would force every client to handle two shapes.
		roles := u.Roles
		if roles == nil {
			roles = []string{}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(meResponse{
			Sub:      u.Sub,
			Email:    u.Email,
			Username: u.Username,
			Roles:    roles,
		})
	})
}
