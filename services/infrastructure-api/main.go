package main

import (
	"fmt"
	"github.com/Over-knight/vortex/internal/kubernetes"
	"github.com/gin-gonic/gin"
	"github.com/Over-knight/vortex/internal/models"
	"log"
	"github.com/Over-knight/vortex/internal/handlers"
)

func main() {
	//initialize kubernetes client
	k8sClient, err := kubernetes.NewK8sClient()
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
	log.Println("Starting Infrastructure API on port 8080")
	router.Run(":8080")
}
