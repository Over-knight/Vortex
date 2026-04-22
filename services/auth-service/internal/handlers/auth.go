package handlers

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/Over-knight/vortex/services/auth-service/internal/middleware"
	"github.com/Over-knight/vortex/services/auth-service/internal/models"
)

type AuthHandler struct {
	DB        *pgxpool.Pool
	JWTSecret string
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req models.RegisterRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request body"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to hash password"})
		return
	}

	tx, err := h.DB.Begin(c.Request.Context())
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to start transaction"})
		return
	}
	defer tx.Rollback(c.Request.Context())

	var orgID string
	err = tx.QueryRow(c.Request.Context(), `
		INSERT INTO organizations (name, slug)
		VALUES ($1, $2)
		RETURNING id
	`, req.OrgName, slugify(req.OrgName)).Scan(&orgID)
	if err != nil {
		c.JSON(409, gin.H{"error": "organization name already taken"})
		return
	}

	var user models.User
	err = tx.QueryRow(c.Request.Context(), `
		INSERT INTO users (org_id, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, org_id, email, status, created_at
	`, orgID, req.Email, string(hash)).Scan(
		&user.ID, &user.OrgID, &user.Email, &user.Status, &user.CreatedAt,
	)
	if err != nil {
		c.JSON(409, gin.H{"error": "email already registered"})
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		c.JSON(500, gin.H{"error": "failed to commit"})
		return
	}

	token, expiresIn, err := h.generateJWT(user)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(201, models.AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
		User:        user,
	})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req models.LoginRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request body"})
		return
	}

	var user models.User
	err := h.DB.QueryRow(c.Request.Context(), `
		SELECT id, org_id, email, password_hash, status, created_at
		FROM users WHERE email = $1
	`, req.Email).Scan(
		&user.ID, &user.OrgID, &user.Email, &user.PasswordHash, &user.Status, &user.CreatedAt,
	)
	if err != nil {
		// Intentionally vague — don't reveal whether the email exists.
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(401, gin.H{"error": "invalid credentials"})
		return
	}

	token, expiresIn, err := h.generateJWT(user)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(200, models.AuthResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiresIn,
		User:        user,
	})
}

func (h *AuthHandler) generateJWT(user models.User) (string, int, error) {
	const ttl = 15 * time.Minute
	claims := &middleware.Claims{
		UserID: user.ID,
		OrgID:  user.OrgID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(h.JWTSecret))
	return signed, int(ttl.Seconds()), err
}

func slugify(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
}
