package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Over-knight/vortex/services/auth-service/internal/models"
)

type ProjectHandler struct {
	DB *pgxpool.Pool
}

func (h *ProjectHandler) Create(c *gin.Context) {
	orgID := c.Param("org_id")
	if orgID != c.GetString("org_id") {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}

	var req models.CreateProjectRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request body"})
		return
	}

	region := req.Region
	if region == "" {
		region = "us-east-1"
	}

	var p models.Project
	err := h.DB.QueryRow(c.Request.Context(), `
		INSERT INTO projects (org_id, name, region)
		VALUES ($1, $2, $3)
		RETURNING id, org_id, name, region, status, created_at
	`, orgID, req.Name, region).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Region, &p.Status, &p.CreatedAt,
	)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to create project"})
		return
	}

	c.JSON(201, p)
}

func (h *ProjectHandler) List(c *gin.Context) {
	orgID := c.Param("org_id")
	if orgID != c.GetString("org_id") {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}

	rows, err := h.DB.Query(c.Request.Context(), `
		SELECT id, org_id, name, region, status, created_at
		FROM projects WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to list projects"})
		return
	}
	defer rows.Close()

	projects := []models.Project{}
	for rows.Next() {
		var p models.Project
		if err := rows.Scan(&p.ID, &p.OrgID, &p.Name, &p.Region, &p.Status, &p.CreatedAt); err != nil {
			continue
		}
		projects = append(projects, p)
	}

	c.JSON(200, gin.H{"projects": projects})
}

func (h *ProjectHandler) Get(c *gin.Context) {
	orgID := c.Param("org_id")
	if orgID != c.GetString("org_id") {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}

	var p models.Project
	err := h.DB.QueryRow(c.Request.Context(), `
		SELECT id, org_id, name, region, status, created_at
		FROM projects WHERE id = $1 AND org_id = $2
	`, c.Param("project_id"), orgID).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Region, &p.Status, &p.CreatedAt,
	)
	if err != nil {
		c.JSON(404, gin.H{"error": "project not found"})
		return
	}

	c.JSON(200, p)
}

func (h *ProjectHandler) Delete(c *gin.Context) {
	orgID := c.Param("org_id")
	if orgID != c.GetString("org_id") {
		c.JSON(403, gin.H{"error": "forbidden"})
		return
	}

	result, err := h.DB.Exec(c.Request.Context(), `
		DELETE FROM projects WHERE id = $1 AND org_id = $2
	`, c.Param("project_id"), orgID)
	if err != nil || result.RowsAffected() == 0 {
		c.JSON(404, gin.H{"error": "project not found"})
		return
	}

	c.JSON(204, nil)
}
