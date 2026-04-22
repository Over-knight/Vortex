package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Claims struct {
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// Auth validates either a Bearer JWT or an ApiKey header.
func Auth(jwtSecret string, db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "authorization header required"})
			return
		}

		switch {
		case strings.HasPrefix(header, "Bearer "):
			handleJWT(c, jwtSecret, strings.TrimPrefix(header, "Bearer "))
		case strings.HasPrefix(header, "ApiKey "):
			handleAPIKey(c, db, strings.TrimPrefix(header, "ApiKey "))
		default:
			c.AbortWithStatusJSON(401, gin.H{"error": "use 'Bearer <token>' or 'ApiKey <key>'"})
		}
	}
}

func handleJWT(c *gin.Context, secret, tokenStr string) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		c.AbortWithStatusJSON(401, gin.H{"error": "invalid or expired token"})
		return
	}
	c.Set("user_id", claims.UserID)
	c.Set("org_id", claims.OrgID)
	c.Set("email", claims.Email)
	c.Next()
}

func handleAPIKey(c *gin.Context, db *pgxpool.Pool, key string) {
	hash := sha256hex(key)
	var userID, orgID, email string
	err := db.QueryRow(c.Request.Context(), `
		SELECT u.id, u.org_id, u.email
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		WHERE ak.key_hash = $1
		  AND (ak.expires_at IS NULL OR ak.expires_at > NOW())
	`, hash).Scan(&userID, &orgID, &email)
	if err != nil {
		c.AbortWithStatusJSON(401, gin.H{"error": "invalid API key"})
		return
	}
	c.Set("user_id", userID)
	c.Set("org_id", orgID)
	c.Set("email", email)
	c.Next()
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
