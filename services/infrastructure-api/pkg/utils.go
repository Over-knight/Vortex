package utils

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/google/uuid"
)

func GenerateUUID() string {
	return uuid.New().String()[:8]
}

func GenerateSecurePassword(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)[:length]
}