package main

import (
	"fmt"
	"github.com/Over-knight/vortex/services/infrastructure-api/internal/handlers"
	"github.com/Over-knight/vortex/services/infrastructure-api/internal/models"
	"github.com/Over-knight/vortex/services/infrastructure-api/internal/vortexkube"
	"github.com/gin-gonic/gin"
	"log"
)

func main() {
	//initialize kubernetes client
	k8sClient, err := vortexkube.NewK8sClient()
	if err != nil {
		log.Fatalf("Failed to initialize k8s client: %v", err)
	}
	fmt.Println("Connected to kubernetes cluster")

	//initiate gin router
	router := gin.Default()

	//health checkpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	//Database provisioning endpoint
	router.POST("/v1/projects/:project_id/resources/databases", func(c *gin.Context) {
		projectID := c.Param("project_id")

		var req models.DatabaseRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request"})
			return
		}

		//Handler logic goes here
		response, err := handlers.ProvisionDatabase(c.Request.Context(), k8sClient, projectID, req)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, response)
	})

	//Database status endpoint
	router.GET("/v1/projects/:project_id/resources/databases/:resource_id", func(c *gin.Context) {
		projectID := c.Param("project_id")
		resourceID := c.Param("resource_id")

		response, err := handlers.GetDatabaseStatus(c.Request.Context(), k8sClient, projectID, resourceID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, response)
	})

	//Database list endpoint
	router.GET("/v1/projects/:project_id/resources/databases", func(c *gin.Context) {
		projectID := c.Param("project_id")

		response, err := handlers.ListDatabases(c.Request.Context(), k8sClient, projectID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"databases": response})
	})

	//Database deletion endpoint
	router.DELETE("/v1/projects/:project_id/resources/databases/:resource_id", func(c *gin.Context) {
		projectID := c.Param("project_id")
		resourceID := c.Param("resource_id")

		err := handlers.DeleteDatabase(c.Request.Context(), k8sClient, projectID, resourceID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(204, nil)
	})

	//Cache provisioning endpoint
	router.POST("/v1/projects/:project_id/resources/caches", func(c *gin.Context) {
		projectID := c.Param("project_id")

		var req models.CacheRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request"})
			return
		}

		response, err := handlers.ProvisionCache(c.Request.Context(), k8sClient, projectID, req)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, response)
	})

	//Cache status endpoint
	router.GET("/v1/projects/:project_id/resources/caches/:resource_id", func(c *gin.Context) {
		projectID := c.Param("project_id")
		resourceID := c.Param("resource_id")

		response, err := handlers.GetCacheStatus(c.Request.Context(), k8sClient, projectID, resourceID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, response)
	})

	//Cache Total endpoint
	router.GET("/v1/projects/:project_id/resources/caches", func(c *gin.Context) {
		projectID := c.Param("project_id")

		response, err := handlers.ListCaches(c.Request.Context(), k8sClient, projectID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"caches": response})
	})

	//Cache deletion endpoint
	router.DELETE("/v1/projects/:project_id/resources/caches/:resource_id", func(c *gin.Context) {
		projectID := c.Param("project_id")
		resourceID := c.Param("resource_id")

		err := handlers.DeleteCache(c.Request.Context(), k8sClient, projectID, resourceID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(204, nil)
	})

	//Compute provisioning endpoint
	router.POST("/v1/projects/:project_id/resources/compute", func(c *gin.Context) {
		projectID := c.Param("project_id")

		var req models.ComputeRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request"})
			return
		}

		response, err := handlers.ProvisionCompute(c.Request.Context(), k8sClient, projectID, req)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, response)
	})

	//Compute status endpoint
	router.GET("/v1/projects/:project_id/resources/compute/:resource_id", func(c *gin.Context) {
		projectID := c.Param("project_id")
		resourceID := c.Param("resource_id")

		response, err := handlers.GetComputeStatus(c.Request.Context(), k8sClient, projectID, resourceID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, response)
	})

	//Compute list endpoint
	router.GET("/v1/projects/:project_id/resources/compute", func(c *gin.Context) {
		projectID := c.Param("project_id")

		response, err := handlers.ListComputeStatus(c.Request.Context(), k8sClient, projectID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"computes": response})
	})
	//Compute deletion endpoint
	router.DELETE("/v1/projects/:project_id/resources/compute/:resource_id", func(c *gin.Context) {
		projectID := c.Param("project_id")
		resourceID := c.Param("resource_id")

		err := handlers.DeleteCompute(c.Request.Context(), k8sClient, projectID, resourceID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(204, nil)
	})

	log.Println("Starting Infrastructure API on port 8080")
	router.Run(":8080")
}
