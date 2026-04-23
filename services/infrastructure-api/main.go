package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/Over-knight/vortex/services/infrastructure-api/internal/handlers"
	"github.com/Over-knight/vortex/services/infrastructure-api/internal/models"
	"github.com/Over-knight/vortex/services/infrastructure-api/internal/vortexkube"
)

func main() {
	// Kubernetes client
	k8sClient, err := vortexkube.NewK8sClient()
	if err != nil {
		log.Fatalf("Failed to initialize k8s client: %v", err)
	}
	fmt.Println("Connected to kubernetes cluster")

	// MinIO client — optional; storage endpoints return 503 when not configured.
	var storageHandler *handlers.StorageHandler
	minioEndpoint := os.Getenv("MINIO_ENDPOINT")   // e.g. "minio.vortex.svc.cluster.local:9000"
	minioAccessKey := os.Getenv("MINIO_ACCESS_KEY") // root user
	minioSecretKey := os.Getenv("MINIO_SECRET_KEY") // root password

	if minioEndpoint != "" && minioAccessKey != "" && minioSecretKey != "" {
		mc, err := minio.New(minioEndpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
			Secure: false,
		})
		if err != nil {
			log.Printf("Warning: failed to initialise MinIO client: %v — storage endpoints disabled", err)
		} else {
			storageHandler = &handlers.StorageHandler{
				Client:    mc,
				Endpoint:  minioEndpoint,
				AccessKey: minioAccessKey,
				SecretKey: minioSecretKey,
			}
			log.Printf("Connected to MinIO at %s", minioEndpoint)
		}
	} else {
		log.Println("Warning: MINIO_ENDPOINT/ACCESS_KEY/SECRET_KEY not set — storage endpoints disabled")
	}

	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// ── Databases ──────────────────────────────────────────────────────────────
	router.POST("/v1/projects/:project_id/resources/databases", func(c *gin.Context) {
		projectID := c.Param("project_id")
		var req models.DatabaseRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}
		response, err := handlers.ProvisionDatabase(c.Request.Context(), k8sClient, projectID, req)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, response)
	})

	router.GET("/v1/projects/:project_id/resources/databases", func(c *gin.Context) {
		projectID := c.Param("project_id")
		response, err := handlers.ListDatabases(c.Request.Context(), k8sClient, projectID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"databases": response})
	})

	router.GET("/v1/projects/:project_id/resources/databases/:resource_id", func(c *gin.Context) {
		response, err := handlers.GetDatabaseStatus(c.Request.Context(), k8sClient, c.Param("project_id"), c.Param("resource_id"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, response)
	})

	router.DELETE("/v1/projects/:project_id/resources/databases/:resource_id", func(c *gin.Context) {
		if err := handlers.DeleteDatabase(c.Request.Context(), k8sClient, c.Param("project_id"), c.Param("resource_id")); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(204, nil)
	})

	// ── Caches ─────────────────────────────────────────────────────────────────
	router.POST("/v1/projects/:project_id/resources/caches", func(c *gin.Context) {
		projectID := c.Param("project_id")
		var req models.CacheRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}
		response, err := handlers.ProvisionCache(c.Request.Context(), k8sClient, projectID, req)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, response)
	})

	router.GET("/v1/projects/:project_id/resources/caches", func(c *gin.Context) {
		response, err := handlers.ListCaches(c.Request.Context(), k8sClient, c.Param("project_id"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"caches": response})
	})

	router.GET("/v1/projects/:project_id/resources/caches/:resource_id", func(c *gin.Context) {
		response, err := handlers.GetCacheStatus(c.Request.Context(), k8sClient, c.Param("project_id"), c.Param("resource_id"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, response)
	})

	router.DELETE("/v1/projects/:project_id/resources/caches/:resource_id", func(c *gin.Context) {
		if err := handlers.DeleteCache(c.Request.Context(), k8sClient, c.Param("project_id"), c.Param("resource_id")); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(204, nil)
	})

	// ── Compute ────────────────────────────────────────────────────────────────
	router.POST("/v1/projects/:project_id/resources/compute", func(c *gin.Context) {
		projectID := c.Param("project_id")
		var req models.ComputeRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}
		response, err := handlers.ProvisionCompute(c.Request.Context(), k8sClient, projectID, req)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, response)
	})

	router.GET("/v1/projects/:project_id/resources/compute", func(c *gin.Context) {
		response, err := handlers.ListComputeStatus(c.Request.Context(), k8sClient, c.Param("project_id"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"computes": response})
	})

	router.GET("/v1/projects/:project_id/resources/compute/:resource_id", func(c *gin.Context) {
		response, err := handlers.GetComputeStatus(c.Request.Context(), k8sClient, c.Param("project_id"), c.Param("resource_id"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, response)
	})

	router.DELETE("/v1/projects/:project_id/resources/compute/:resource_id", func(c *gin.Context) {
		if err := handlers.DeleteCompute(c.Request.Context(), k8sClient, c.Param("project_id"), c.Param("resource_id")); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(204, nil)
	})

	// ── Storage buckets ────────────────────────────────────────────────────────
	storageUnavailable := func(c *gin.Context) {
		c.JSON(503, gin.H{"error": "storage service not configured — set MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY"})
	}

	router.POST("/v1/projects/:project_id/resources/storage", func(c *gin.Context) {
		if storageHandler == nil {
			storageUnavailable(c)
			return
		}
		var req models.StorageBucketRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}
		response, err := storageHandler.CreateBucket(c.Request.Context(), c.Param("project_id"), req)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, response)
	})

	router.GET("/v1/projects/:project_id/resources/storage", func(c *gin.Context) {
		if storageHandler == nil {
			storageUnavailable(c)
			return
		}
		response, err := storageHandler.ListBuckets(c.Request.Context(), c.Param("project_id"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"buckets": response})
	})

	router.GET("/v1/projects/:project_id/resources/storage/:bucket_id", func(c *gin.Context) {
		if storageHandler == nil {
			storageUnavailable(c)
			return
		}
		response, err := storageHandler.GetBucket(c.Request.Context(), c.Param("project_id"), c.Param("bucket_id"))
		if err != nil {
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, response)
	})

	router.DELETE("/v1/projects/:project_id/resources/storage/:bucket_id", func(c *gin.Context) {
		if storageHandler == nil {
			storageUnavailable(c)
			return
		}
		if err := storageHandler.DeleteBucket(c.Request.Context(), c.Param("project_id"), c.Param("bucket_id")); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(204, nil)
	})

	log.Println("Starting Infrastructure API on port 8080")
	router.Run(":8080")
}
