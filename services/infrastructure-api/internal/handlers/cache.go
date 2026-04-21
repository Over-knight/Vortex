package handlers

import (
	"context"
	"fmt"

	"github.com/Over-knight/vortex/services/infrastructure-api/internal/models"
	"github.com/Over-knight/vortex/services/infrastructure-api/internal/vortexkube"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ProvisionCache creates a Redis cache instance for a project
func ProvisionCache(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, req models.CacheRequest) (*models.CacheResponse, error) {
	// Step 0: Ensure project namespace exists
	namespace, err := EnsureNamespace(ctx, k8sClient, projectID)
	if err != nil {
		return nil, err
	}

	// Step 1: Generate a unique ID
	cacheID := fmt.Sprintf("cache-%s", GenerateUUID())

	// Step 2: Create Deployment for Redis
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cacheID,
			Namespace: namespace,
			Labels: map[string]string{
				"app":     cacheID,
				"project": projectID,
				"type":    "cache",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": cacheID},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": cacheID},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "redis:7-alpine",
							Ports: []corev1.ContainerPort{
								{ContainerPort: 6379},
							},
							Command: []string{"redis-server"},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: createQuantity("128Mi"),
									corev1.ResourceCPU:    createQuantity("50m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: createQuantity("256Mi"),
									corev1.ResourceCPU:    createQuantity("200m"),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "ping"},
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "ping"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
							},
						},
					},
				},
			},
		},
	}

	_, err = k8sClient.Clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	// Step 3: Create Service for DNS
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cacheID,
			Namespace: namespace,
			Labels: map[string]string{
				"app": cacheID,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": cacheID},
			Ports: []corev1.ServicePort{
				{
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	_, err = k8sClient.Clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	// Step 4: Return response with status "provisioning"
	return &models.CacheResponse{
		ID:        cacheID,
		Name:      req.Name,
		Status:    "provisioning",
		Endpoint:  fmt.Sprintf("%s:6379", cacheID),
		CreatedAt: metav1.Now().Time,
	}, nil
}

// GetCacheStatus retrieves the current status of a provisioned cache
func GetCacheStatus(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) (*models.CacheResponse, error) {
	// Construct the namespace name
	namespace := fmt.Sprintf("vortex-project-%s", projectID)

	// Query the Deployment to check its status
	deployment, err := k8sClient.Clientset.AppsV1().Deployments(namespace).Get(ctx, resourceID, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	// Determine status based on replicas
	status := "provisioning"
	if deployment.Status.ReadyReplicas > 0 {
		status = "running"
	}

	return &models.CacheResponse{
		ID:        resourceID,
		Name:      resourceID, // Would need to store name separately in production
		Status:    status,
		Endpoint:  fmt.Sprintf("%s:6379", resourceID),
		CreatedAt: deployment.CreationTimestamp.Time,
	}, nil
}

// DeleteCache removes all resources associated with a provisioned cache
func DeleteCache(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) error {
	// Construct the namespace name
	namespace := fmt.Sprintf("vortex-project-%s", projectID)
	deletionPolicy := metav1.DeletePropagationForeground
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletionPolicy,
	}

	// Delete Deployment
	err := k8sClient.Clientset.AppsV1().Deployments(namespace).Delete(ctx, resourceID, deleteOptions)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	// Delete Service
	err = k8sClient.Clientset.CoreV1().Services(namespace).Delete(ctx, resourceID, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	return nil
}

// ListCacheStatus retrieves the current status of a all cache
func ListCaches(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string) ([]*models.CacheResponse, error) {
	//construct the namespace name
	namespace := fmt.Sprintf("vortex-project-%s", projectID)

	//List all deployments with  type=cache label
	deployments, err := k8sClient.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "type=cache",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list caches in namespace %s: %w", namespace, err)
	}
	//Convert deployments to cache responses
	caches := []*models.CacheResponse{}
	for _, deployment := range deployments.Items {
		status := "provisioning"
		if deployment.Status.ReadyReplicas > 0 {
			status = "running"
		}

		cache := &models.CacheResponse{
			ID:        deployment.Name,
			Name:      deployment.Labels["app"],
			Status:    status,
			Endpoint:  fmt.Sprintf("%s:6379", deployment.Name),
			CreatedAt: deployment.CreationTimestamp.Time,
		}
		caches = append(caches, cache)
	}
	return caches, nil
}
