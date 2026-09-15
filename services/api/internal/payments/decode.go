package payments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func decodeJSON(payload []byte, into any) error {
	if len(payload) == 0 || payload[0] != '{' {
		return errNotJSON
	}
	return json.Unmarshal(payload, into)
}

// hashOf identifies a callback whose own id could not be read, so that a
// replay of the same body still deduplicates rather than being stored twice.
func hashOf(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:16])
}
