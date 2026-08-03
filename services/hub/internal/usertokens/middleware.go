package usertokens

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// UsernameResolver resolves a user ID to a display username.
type UsernameResolver func(ctx context.Context, userID string) (string, error)

// NewMiddleware returns an HTTP middleware that accepts Bearer tokens prefixed
// with "ainsel_". Such tokens are validated against the store; on success the
// resolved user is placed in the request context and the request is forwarded.
// All other tokens fall through to next unchanged, allowing OIDC middleware
// to handle them in the normal chain.
//
// If the token is invalid or expired, NewMiddleware responds 401 directly.
func NewMiddleware(store *Store, resolveUsername UsernameResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := extractBearer(r.Header.Get("Authorization"))
			if tok == "" || !strings.HasPrefix(tok, "ainsel_") {
				next.ServeHTTP(w, r)
				return
			}

			t, err := store.Validate(r.Context(), tok)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					http.Error(w, "invalid token", http.StatusUnauthorized)
					return
				}
				http.Error(w, "token validation error", http.StatusInternalServerError)
				return
			}

			username, err := resolveUsername(r.Context(), t.UserID)
			if err != nil {
				http.Error(w, "user lookup failed", http.StatusInternalServerError)
				return
			}

			ctx := oidc.ContextWithUser(r.Context(), &oidc.User{Sub: t.UserID, Username: username})
			ctx = oidc.ContextWithToken(ctx, tok)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearer(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}
