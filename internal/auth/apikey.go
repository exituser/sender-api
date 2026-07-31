package auth

import (
	"crypto/sha256"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func GenerateAPIKey() (raw string, hash string, prefix string, err error) {
	bytes := make([]byte, 32)
	_, err = rand.Read(bytes)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	raw = "re_" + base64.RawURLEncoding.EncodeToString(bytes)

	hash = sha256Hex(raw)

	if len(raw) > 10 {
		prefix = raw[:10] + "..."
	} else {
		prefix = raw
	}

	return raw, hash, prefix, nil
}

func GenerateAPIKeyInternal() (raw string, hash string, prefix string, err error) {
	return GenerateAPIKey()
}

func sha256Hex(data string) string {
	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

func HashAPIKey(key string) string {
	return sha256Hex(key)
}
