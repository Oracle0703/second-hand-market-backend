package common

import (
	"crypto/sha256"
	"encoding/hex"
)

func SHA256(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
