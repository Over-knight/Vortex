package models

import "time"

//DatabaseRequest is what the client sends 
type DatabaseRequest struct {
    Name   string `json:"name" binding:"required"`
    Engine string `json:"engine" binding:"required"` // "postgres"
    Version string `json:"version"`                   // "16"
    Size   string `json:"size"`                       // "db.small"
    Config DBConfig `json:"config"`
}

// DBConfig holds database-specific settings
type DBConfig struct {
    StorageGB int `json:"storage_gb"`
    Replicas  int `json:"replicas"`
}

// DatabaseResponse is what your API returns
type DatabaseResponse struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Status    string    `json:"status"` // "provisioning", "running", "failed"
    Endpoint  string    `json:"endpoint"` // "postgres-db-123:5432" (empty until ready)
    Username  string    `json:"username"`
    Password  string    `json:"password"` // Return only once!
    CreatedAt time.Time `json:"created_at"`
}