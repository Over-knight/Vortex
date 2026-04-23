package models

import "time"

//DatabaseRequest is what the client sends
type DatabaseRequest struct {
	Name    string   `json:"name" binding:"required"`
	Engine  string   `json:"engine" binding:"required"` // "postgres"
	Version string   `json:"version"`                   // "16"
	Size    string   `json:"size"`                      // "db.small"
	Config  DBConfig `json:"config"`
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
	Engine    string    `json:"engine"`
	Status    string    `json:"status"`   // "provisioning", "running", "failed"
	Endpoint  string    `json:"endpoint"` // external host:port, "pending" until LB is assigned
	Username  string    `json:"username"`
	Password  string    `json:"password"` // Return only once!
	CreatedAt time.Time `json:"created_at"`
}

// CacheRequest is what the client sends for cache provisioning
type CacheRequest struct {
	Name   string      `json:"name" binding:"required"`
	Engine string      `json:"engine"` // "redis" (for future compatibility)
	Config CacheConfig `json:"config"`
}

// CacheConfig holds cache-specific settings
type CacheConfig struct {
	MemoryMB int `json:"memory_mb"` // Memory limit for Redis
	Replicas int `json:"replicas"`  // Usually 1 for stateless
}

// CacheResponse is what the API returns for cache operations
type CacheResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`   // "provisioning", "running", "failed"
	Endpoint  string    `json:"endpoint"` // "cache-123:6379"
	CreatedAt time.Time `json:"created_at"`
}

// ComputeRequest is what the client sends for application deployment
type ComputeRequest struct {
	Name    string          `json:"name" binding:"required"`
	Image   string          `json:"image" binding:"required"` // e.g., "nginx:latest"
	CPU     string          `json:"cpu"`                      // e.g., "500m"
	Memory  string          `json:"memory"`                   // e.g., "512Mi"
	Ports   []ComputePort   `json:"ports"`
	Volumes []VolumeRequest `json:"volumes"` // persistent disks to attach
}

// ComputePort defines a network port for compute instances
type ComputePort struct {
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"` // "TCP" or "UDP"
}

// VolumeRequest describes a persistent disk to attach to a compute instance
type VolumeRequest struct {
	Name      string `json:"name" binding:"required"` // identifier, e.g. "data"
	SizeGB    int    `json:"size_gb"`                 // default: 10
	MountPath string `json:"mount_path" binding:"required"` // e.g. "/data"
}

// VolumeInfo is returned in ComputeResponse to describe attached volumes
type VolumeInfo struct {
	Name      string `json:"name"`
	SizeGB    int    `json:"size_gb"`
	MountPath string `json:"mount_path"`
}

// ComputeResponse is what the API returns for compute operations
type ComputeResponse struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Status    string       `json:"status"`           // "provisioning", "running", "failed"
	Endpoints []string     `json:"endpoints"`        // e.g., ["comp-123:8080"]
	Volumes   []VolumeInfo `json:"volumes,omitempty"` // attached persistent disks
	CreatedAt time.Time    `json:"created_at"`
}

// StorageBucketRequest is what the client sends to create a bucket
type StorageBucketRequest struct {
	Name   string `json:"name" binding:"required"`
	Region string `json:"region"` // default: "us-east-1"
}

// StorageBucketResponse is what the API returns for storage operations
type StorageBucketResponse struct {
	ID        string    `json:"id"`                    // bucket name (globally unique within MinIO)
	Name      string    `json:"name"`                  // user-provided name
	Endpoint  string    `json:"endpoint"`              // S3-compatible API endpoint
	AccessKey string    `json:"access_key,omitempty"`  // only returned on creation
	SecretKey string    `json:"secret_key,omitempty"`  // only returned on creation
	Region    string    `json:"region"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}
