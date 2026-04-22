package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"github.com/Over-knight/vortex/services/auth-service/internal/config"
	"github.com/Over-knight/vortex/services/auth-service/internal/db"
	"github.com/Over-knight/vortex/services/auth-service/internal/handlers"
	"github.com/Over-knight/vortex/services/auth-service/internal/middleware"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database schema ready")

	router := gin.Default()

	auth := &handlers.AuthHandler{DB: pool, JWTSecret: cfg.JWTSecret}
	projects := &handlers.ProjectHandler{DB: pool}
	apikeys := &handlers.APIKeyHandler{DB: pool}

	// Public
	router.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	router.POST("/v1/auth/register", auth.Register)
	router.POST("/v1/auth/login", auth.Login)

	// Protected — requires Bearer JWT or ApiKey header
	protected := router.Group("/")
	protected.Use(middleware.Auth(cfg.JWTSecret, pool))
	{
		// Projects
		protected.POST("/v1/orgs/:org_id/projects", projects.Create)
		protected.GET("/v1/orgs/:org_id/projects", projects.List)
		protected.GET("/v1/orgs/:org_id/projects/:project_id", projects.Get)
		protected.DELETE("/v1/orgs/:org_id/projects/:project_id", projects.Delete)

		// API keys
		protected.POST("/v1/users/api-keys", apikeys.Create)
		protected.GET("/v1/users/api-keys", apikeys.List)
		protected.DELETE("/v1/users/api-keys/:key_id", apikeys.Delete)
	}

	log.Printf("Starting Auth Service on :%s", cfg.Port)
	router.Run(":" + cfg.Port)
}
