package application

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func randomID(prefix string) string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return prefix + "_" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func operationKey(operation, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", ErrIdempotencyKey
	}
	if len(key) > 160 {
		return "", fmt.Errorf("幂等键长度不能超过 160")
	}
	return operation + ":" + key, nil
}
