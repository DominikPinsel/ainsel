// Package localauth implements AInsel's local username/password
// authentication: argon2id password hashing, HS256 session JWTs issued by
// the hub itself, and an HTTP middleware that validates those JWTs. It is
// the "auth.mode=local" alternative to external OIDC (see issue #108).
package localauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters per the OWASP Password Storage Cheat Sheet
// (argon2id, 19 MiB memory, 2 iterations, parallelism 1).
const (
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

var (
	// ErrInvalidHash is returned when a stored hash cannot be parsed.
	ErrInvalidHash = errors.New("localauth: malformed password hash")
	// ErrNoCredentials is returned when a user has no local password set.
	ErrNoCredentials = errors.New("localauth: user has no local password")
)

// HashPassword hashes a plaintext password with argon2id in self-describing
// PHC format: $argon2id$v=19$m=<mem>,t=<time>,p=<threads>$<salt>$<hash>.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("localauth: generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads, b64(salt), b64(key)), nil
}

// VerifyPassword checks a plaintext password against a stored PHC-format
// hash. The comparison is constant-time. A malformed hash or an empty stored
// hash (user has no local credentials) returns an error; callers should
// treat all failures as "invalid credentials" without distinguishing why.
func VerifyPassword(password, encoded string) error {
	if encoded == "" {
		return ErrNoCredentials
	}
	parts := strings.Split(encoded, "$")
	// Leading '$' makes parts[0] empty: ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return ErrInvalidHash
	}
	var mem, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &time, &threads); err != nil {
		return ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return ErrInvalidHash
	}
	if len(want) == 0 || threads == 0 {
		return ErrInvalidHash
	}

	got := argon2.IDKey([]byte(password), salt, time, mem, threads, uint32(len(want)))
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return errors.New("localauth: password mismatch")
	}
	return nil
}
