package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Over-knight/vortex/services/auth-service/internal/models"
)

type APIKeyHandler struct {
	DB *pgxpool.Pool
}

func (h *APIKeyHandler) Create(c *gin.Context) {
	userID := c.GetString("user_id")

	var req models.CreateAPIKeyRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request body"})
		return
	}

	scope := req.Scope
	if scope == "" {
		scope = "full"
	}

	// Format: vrtx_<32 random hex chars> — similar to AWS access key style.
	raw := make([]byte, 16)
	rand.Read(raw)
	key := "vrtx_" + hex.EncodeToString(raw)
	prefix := key[:12] // shown in list responses so users can identify keys

	sum := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(sum[:])

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			c.JSON(400, gin.H{"error": "expires_at must be RFC3339 format"})
			return
		}
		expiresAt = &t
	}

	var k models.APIKey
	err := h.DB.QueryRow(c.Request.Context(), `
		INSERT INTO api_keys (user_id, name, key_hash, key_prefix, scope, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, name, key_prefix, scope, created_at, expires_at
	`, userID, req.Name, keyHash, prefix, scope, expiresAt).Scan(
		&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.Scope, &k.CreatedAt, &k.ExpiresAt,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to create API key"})
		return
	}

	c.JSON(201, models.APIKeyCreateResponse{
		APIKey: k,
		Key:    key, // Only returned once — store it now.
	})
}

func (h *APIKeyHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")

	rows, err := h.DB.Query(c.Request.Context(), `
		SELECT id, user_id, name, key_prefix, scope, created_at, expires_at
		FROM api_keys WHERE user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to list API keys"})
		return
	}
	defer rows.Close()

	keys := []models.APIKey{}
	for rows.Next() {
		var k models.APIKey
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyPrefix, &k.Scope, &k.CreatedAt, &k.ExpiresAt); err != nil {
			continue
		}
		keys = append(keys, k)
	}

	c.JSON(200, gin.H{"api_keys": keys})
}

func (h *APIKeyHandler) Delete(c *gin.Context) {
	userID := c.GetString("user_id")

	result, err := h.DB.Exec(c.Request.Context(), `
		DELETE FROM api_keys WHERE id = $1 AND user_id = $2
	`, c.Param("key_id"), userID)
	if err != nil || result.RowsAffected() == 0 {
		c.JSON(404, gin.H{"error": "API key not found"})
		return
	}

	c.JSON(204, nil)
}
