package localauth

import (
	"net/http"
	"strings"

	"github.com/DominikPinsel/ainsel/shared/auth/oidc"
)

// NewMiddleware returns an HTTP middleware that accepts Bearer tokens which
// claim to be local session JWTs (issuer "ainsel-hub"). Such tokens are
// verified against the shared signing secret; on success the user is placed
// in the request context via the same oidc.ContextWithUser mechanism the
// OIDC and user-token middlewares use, so all existing handlers and authz
// checks work unchanged.
//
// Tokens that are not local JWTs fall through to next unchanged, allowing
// the next middleware in the chain (typically OIDC) to handle them. Tokens
// that ARE local but fail verification get a 401 directly — they must not
// fall through, or an expired/tampered local token would surface as the
// confusing "invalid OIDC token" error.
//
// Note: the admin flag in the token is for display only (e.g. /auth/me
// roles). All authorization decisions re-check the database via the authz
// Checker, so revoking a role takes effect immediately despite the
// stateless token.
func NewMiddleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := extractBearer(r.Header.Get("Authorization"))
			if tok == "" || !LooksLocal(tok) {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := VerifyToken(secret, tok)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			var roles []string
			if claims.Admin {
				roles = []string{"admin"}
			}
			ctx := oidc.ContextWithUser(r.Context(), &oidc.User{
				Sub:      claims.Sub,
				Username: claims.Username,
				Roles:    roles,
			})
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

// RequireAuth rejects requests that carry no authenticated user in the
// context. The token middlewares (user tokens, local JWTs) deliberately fall
// through on anonymous requests; in local mode there is no OIDC middleware
// that would reject a missing bearer token, so this closer is what keeps the
// API from being open. It must sit inside the token middlewares (closer to
// the handler) so every authentication attempt has run first.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := oidc.FromContext(r.Context()); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
