package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
)

func GenerateAccessToken(keys *KeyPair, userID, email, role, username string) (string, error) {
	claims := jwt.MapClaims{
		"sub":      userID,
		"email":    email,
		"role":     role,
		"username": username,
		"exp":      time.Now().Add(AccessTokenTTL).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keys.Kid
	return token.SignedString(keys.Private)
}

// Opaque refresh token: random bytes, only the SHA-256 hash is stored in DB.
func GenerateRefreshToken() (plain string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	hash = hashToken(plain)
	return plain, hash, nil
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func HashRefreshToken(plain string) string {
	return hashToken(plain)
}
