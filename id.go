package gig

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const defaultHashLength = 4

// GenerateID produces a short, prefix-based ID like "gig-a3f8".
// The hash is derived from a UUID + current timestamp to ensure uniqueness.
func GenerateID(prefix string, hashLen int) string {
	if prefix == "" {
		prefix = "gig"
	}
	if hashLen < 3 || hashLen > 8 {
		hashLen = defaultHashLength
	}

	raw := fmt.Sprintf("%s-%d", uuid.New().String(), time.Now().UnixNano())
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])[:hashLen]

	return fmt.Sprintf("%s-%s", prefix, hash)
}
