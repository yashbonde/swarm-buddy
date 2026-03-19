package toroid

import (
	"crypto/rand"
	"fmt"
	"time"
)

// NewSessionID generates a monotonic, human-readable session ID.
// It uses the current Unix timestamp as a base to ensure monotonicity,
// followed by a short random alphanumeric string for uniqueness and readability.
func NewSessionID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const randLen = 4
	
	// Use Unix seconds for coarse monotonicity
	now := time.Now().Unix()
	
	// Generate random suffix
	b := make([]byte, randLen)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp only if crypto/rand fails
		return fmt.Sprintf("%d", now)
	}
	
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	
	return fmt.Sprintf("%d-%s", now, string(b))
}
