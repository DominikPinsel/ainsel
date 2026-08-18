package localauth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer identifies JWTs minted by the hub's local auth. The middleware uses
// it to decide whether a bearer token is "ours" (strict verification) or
// belongs to the next middleware in the chain (fall-through to OIDC).
const Issuer = "ainsel-hub"

// TokenTTL is the lifetime of a local session JWT. Stateless by design:
// there is no server-side session and no refresh flow — users re-login when
// the token expires. Revocation is deferred (roadmap issue #108, Phase 3).
const TokenTTL = 12 * time.Hour

// Claims is the validated claim set of a local session token.
type Claims struct {
	Sub      string // user ID, always prefixed "local:"
	Username string // login name without the prefix
	Admin    bool
	ExpiresAt time.Time
}

// IssueToken signs a local session JWT (HS256) for the given user. It
// returns the signed token and its expiry time.
func IssueToken(secret []byte, userID, username string, isAdmin bool) (string, time.Time, error) {
	if len(secret) == 0 {
		return "", time.Time{}, errors.New("localauth: empty signing secret")
	}
	now := time.Now()
	exp := now.Add(TokenTTL)
	claims := jwt.MapClaims{
		"iss":      Issuer,
		"sub":      userID,
		"username": username,
		"admin":    isAdmin,
		"iat":      now.Unix(),
		"exp":      exp.Unix(),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("localauth: sign token: %w", err)
	}
	return tok, exp, nil
}

// VerifyToken parses and validates a local session token: HS256 only,
// matching issuer, expiry required. It does NOT check whether the token is
// local vs foreign — callers use LooksLocal for that routing decision.
func VerifyToken(secret []byte, raw string) (*Claims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
	)
	parsed, err := parser.Parse(raw, func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("localauth: %w", err)
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("localauth: unexpected claims type")
	}

	c := &Claims{}
	if v, ok := mc["sub"].(string); ok {
		c.Sub = v
	}
	if v, ok := mc["username"].(string); ok {
		c.Username = v
	}
	if v, ok := mc["admin"].(bool); ok {
		c.Admin = v
	}
	if v, err := mc.GetExpirationTime(); err == nil && v != nil {
		c.ExpiresAt = v.Time
	}
	if c.Sub == "" {
		return nil, errors.New("localauth: token missing sub")
	}
	return c, nil
}

// LooksLocal reports whether a raw bearer token claims to be a local session
// JWT (unverified peek at the iss claim). Anything else falls through to the
// next middleware in the chain.
func LooksLocal(raw string) bool {
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	claims := jwt.MapClaims{}
	// Ignore the error: an unparsable token is simply "not local" and the
	// next middleware (OIDC) will reject it with its own error.
	_, _, _ = parser.ParseUnverified(raw, claims)
	iss, _ := claims["iss"].(string)
	return iss == Issuer
}
