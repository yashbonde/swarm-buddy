package toroid

import (
	"crypto/rand"
	"fmt"
	"time"
)

// NewSessionID generates a monotonic, human-readable session ID.
// Format: <unix_seconds>-<4char_random>
func NewSessionID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const randLen = 4

	now := time.Now().Unix()

	b := make([]byte, randLen)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", now)
	}

	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}

	return fmt.Sprintf("%d-%s", now, string(b))
}
