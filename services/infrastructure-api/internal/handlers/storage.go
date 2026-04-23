package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/minio/minio-go/v7"

	"github.com/Over-knight/vortex/services/infrastructure-api/internal/models"
)

// StorageHandler holds a shared MinIO client used for all bucket operations.
// All projects share the same MinIO instance; isolation is enforced by bucket
// name prefix (vortex-{projectID}-{name}).
type StorageHandler struct {
	Client    *minio.Client
	Endpoint  string
	AccessKey string
	SecretKey string
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9-]+`)

// bucketID builds the MinIO bucket name from a project and user-provided name.
// MinIO requires: lowercase, 3–63 chars, alphanumeric + hyphens only.
func bucketID(projectID, name string) string {
	safe := nonAlnum.ReplaceAllString(strings.ToLower(name), "-")
	id := fmt.Sprintf("vortex-%s-%s", projectID, safe)
	if len(id) > 63 {
		id = id[:63]
	}
	return id
}

// CreateBucket provisions a new S3-compatible bucket for a project.
func (h *StorageHandler) CreateBucket(ctx context.Context, projectID string, req models.StorageBucketRequest) (*models.StorageBucketResponse, error) {
	region := req.Region
	if region == "" {
		region = "us-east-1"
	}

	id := bucketID(projectID, req.Name)

	err := h.Client.MakeBucket(ctx, id, minio.MakeBucketOptions{Region: region})
	if err != nil {
		// Surface a clear error if the bucket already exists.
		if minio.ToErrorResponse(err).Code == "BucketAlreadyOwnedByYou" {
			return nil, fmt.Errorf("bucket %q already exists in this project", req.Name)
		}
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}

	info, err := h.Client.GetBucketLocation(ctx, id)
	if err != nil {
		info = region
	}

	return &models.StorageBucketResponse{
		ID:        id,
		Name:      req.Name,
		Endpoint:  fmt.Sprintf("http://%s", h.Endpoint),
		AccessKey: h.AccessKey,
		SecretKey: h.SecretKey,
		Region:    info,
	}, nil
}

// ListBuckets returns all buckets belonging to a project.
func (h *StorageHandler) ListBuckets(ctx context.Context, projectID string) ([]models.StorageBucketResponse, error) {
	all, err := h.Client.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}

	prefix := fmt.Sprintf("vortex-%s-", projectID)
	var result []models.StorageBucketResponse
	for _, b := range all {
		if !strings.HasPrefix(b.Name, prefix) {
			continue
		}
		// Derive the user-facing name by stripping the prefix.
		name := strings.TrimPrefix(b.Name, prefix)

		size, _ := h.bucketSize(ctx, b.Name)
		result = append(result, models.StorageBucketResponse{
			ID:        b.Name,
			Name:      name,
			Endpoint:  fmt.Sprintf("http://%s", h.Endpoint),
			Region:    "us-east-1",
			SizeBytes: size,
			CreatedAt: b.CreationDate,
		})
	}

	return result, nil
}

// GetBucket returns metadata for a single bucket by its ID.
func (h *StorageHandler) GetBucket(ctx context.Context, projectID, id string) (*models.StorageBucketResponse, error) {
	prefix := fmt.Sprintf("vortex-%s-", projectID)
	if !strings.HasPrefix(id, prefix) {
		return nil, fmt.Errorf("bucket %q not found in this project", id)
	}

	exists, err := h.Client.BucketExists(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("bucket %q not found", id)
	}

	location, _ := h.Client.GetBucketLocation(ctx, id)
	if location == "" {
		location = "us-east-1"
	}

	size, _ := h.bucketSize(ctx, id)
	name := strings.TrimPrefix(id, prefix)

	return &models.StorageBucketResponse{
		ID:        id,
		Name:      name,
		Endpoint:  fmt.Sprintf("http://%s", h.Endpoint),
		Region:    location,
		SizeBytes: size,
	}, nil
}

// DeleteBucket removes a bucket and all objects inside it.
func (h *StorageHandler) DeleteBucket(ctx context.Context, projectID, id string) error {
	prefix := fmt.Sprintf("vortex-%s-", projectID)
	if !strings.HasPrefix(id, prefix) {
		return fmt.Errorf("bucket %q not found in this project", id)
	}

	// Remove all objects first (S3 buckets must be empty before deletion).
	for obj := range h.Client.ListObjects(ctx, id, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return fmt.Errorf("failed to list objects for deletion: %w", obj.Err)
		}
		if err := h.Client.RemoveObject(ctx, id, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			return fmt.Errorf("failed to remove object %s: %w", obj.Key, err)
		}
	}

	if err := h.Client.RemoveBucket(ctx, id); err != nil {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}
	return nil
}

// bucketSize sums the size of all objects in a bucket.
func (h *StorageHandler) bucketSize(ctx context.Context, bucket string) (int64, error) {
	var total int64
	for obj := range h.Client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return total, obj.Err
		}
		total += obj.Size
	}
	return total, nil
}
