package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// generateID creates a unique resource ID with the given prefix.
// Format: {prefix}-{8 hex chars}, e.g. "c-1a2b3c4d".
func generateID(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}
