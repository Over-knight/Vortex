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

// ProvisionCompute creates a Kubernetes Deployment for user application code
func ProvisionCompute(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, req models.ComputeRequest) (*models.ComputeResponse, error) {
	// Step 0: Ensure project namespace exists
	namespace, err := EnsureNamespace(ctx, k8sClient, projectID)
	if err != nil {
		return nil, err
	}

	// Step 1: Generate a unique ID
	computeID := fmt.Sprintf("comp-%s", GenerateUUID())

	// Step 2: Build container ports from request
	containerPorts := []corev1.ContainerPort{}
	for _, p := range req.Ports {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			ContainerPort: p.Port,
			Protocol:      corev1.Protocol(p.Protocol),
		})
	}

	// Step 3: Create Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      computeID,
			Namespace: namespace,
			Labels: map[string]string{
				"app":     computeID,
				"project": projectID,
				"type":    "compute",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": computeID},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": computeID},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: req.Image,
							Ports: containerPorts,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    createQuantity(defaultIfEmpty(req.CPU, "250m")),
									corev1.ResourceMemory: createQuantity(defaultIfEmpty(req.Memory, "256Mi")),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    createQuantity(defaultIfEmpty(req.CPU, "500m")),
									corev1.ResourceMemory: createQuantity(defaultIfEmpty(req.Memory, "512Mi")),
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromInt(80),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromInt(80),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								FailureThreshold:    2,
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

	// Step 4: Create Service
	servicePorts := []corev1.ServicePort{}
	for _, p := range req.Ports {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Port:       p.Port,
			TargetPort: intstr.FromInt(int(p.Port)),
			Protocol:   corev1.Protocol(p.Protocol),
		})
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      computeID,
			Namespace: namespace,
			Labels: map[string]string{
				"app":     computeID,
				"project": projectID,
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": computeID},
			Ports:    servicePorts,
		},
	}

	_, err = k8sClient.Clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	// Step 5: Build endpoints list
	endpoints := []string{}
	for _, p := range req.Ports {
		endpoints = append(endpoints, fmt.Sprintf("%s:%d", computeID, p.Port))
	}

	// Step 6: Return response
	return &models.ComputeResponse{
		ID:        computeID,
		Name:      req.Name,
		Status:    "provisioning",
		Endpoints: endpoints,
		CreatedAt: metav1.Now().Time,
	}, nil
}

// GetComputeStatus retrieves the current status of a provisioned compute instance
func GetComputeStatus(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) (*models.ComputeResponse, error) {
	namespace := fmt.Sprintf("vortex-project-%s", projectID)

	// Get the Deployment
	deployment, err := k8sClient.Clientset.AppsV1().Deployments(namespace).Get(ctx, resourceID, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s in namespace %s: %w", resourceID, namespace, err)
	}

	// Get the Service to extract endpoints
	service, err := k8sClient.Clientset.CoreV1().Services(namespace).Get(ctx, resourceID, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get service %s in namespace %s: %w", resourceID, namespace, err)
	}

	// Build endpoints from service ports
	endpoints := []string{}
	if service != nil {
		for _, port := range service.Spec.Ports {
			endpoints = append(endpoints, fmt.Sprintf("%s:%d", resourceID, port.Port))
		}
	}

	// Determine status based on replica readiness
	status := "provisioning"
	if deployment.Status.ReadyReplicas > 0 {
		status = "running"
	}

	return &models.ComputeResponse{
		ID:        resourceID,
		Name:      deployment.Labels["app"],
		Status:    status,
		Endpoints: endpoints,
		CreatedAt: deployment.CreationTimestamp.Time,
	}, nil
}

// DeleteCompute removes all resources associated with a provisioned compute instance
func DeleteCompute(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) error {
	namespace := fmt.Sprintf("vortex-project-%s", projectID)
	deletionPolicy := metav1.DeletePropagationForeground
	deletionOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletionPolicy,
	}

	// Step 1: Delete Deployment (cascades to pods)
	err := k8sClient.Clientset.AppsV1().Deployments(namespace).Delete(ctx, resourceID, deletionOptions)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment %s in namespace %s: %w", resourceID, namespace, err)
	}

	// Step 2: Delete Service
	err = k8sClient.Clientset.CoreV1().Services(namespace).Delete(ctx, resourceID, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete service %s in namespace %s: %w", resourceID, namespace, err)
	}

	return nil
}

// Helper function to provide default value if string is empty
func defaultIfEmpty(s string, defaultValue string) string {
	if s == "" {
		return defaultValue
	}
	return s
}
