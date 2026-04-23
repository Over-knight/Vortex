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

func ProvisionCompute(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, req models.ComputeRequest) (*models.ComputeResponse, error) {
	namespace, err := EnsureNamespace(ctx, k8sClient, projectID)
	if err != nil {
		return nil, err
	}

	computeID := fmt.Sprintf("comp-%s", GenerateUUID())

	// Build container ports.
	containerPorts := make([]corev1.ContainerPort, 0, len(req.Ports))
	for _, p := range req.Ports {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			ContainerPort: p.Port,
			Protocol:      corev1.Protocol(p.Protocol),
		})
	}

	// Build PVCs, pod Volumes, and VolumeMounts from the volume requests.
	var (
		volumeMounts []corev1.VolumeMount
		podVolumes   []corev1.Volume
		volumeInfos  []models.VolumeInfo
	)
	for _, v := range req.Volumes {
		sizeGB := v.SizeGB
		if sizeGB <= 0 {
			sizeGB = 10
		}
		pvcName := fmt.Sprintf("%s-%s", computeID, v.Name)

		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: namespace,
				Labels: map[string]string{
					"app":  computeID,
					"type": "compute-volume",
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				StorageClassName: stringPtr("standard"),
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: createQuantity(fmt.Sprintf("%dGi", sizeGB)),
					},
				},
			},
		}
		_, err := k8sClient.Clientset.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create PVC %s: %w", pvcName, err)
		}

		podVolumes = append(podVolumes, corev1.Volume{
			Name: v.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      v.Name,
			MountPath: v.MountPath,
		})
		volumeInfos = append(volumeInfos, models.VolumeInfo{
			Name:      v.Name,
			SizeGB:    sizeGB,
			MountPath: v.MountPath,
		})
	}

	// Build the Deployment.
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      computeID,
			Namespace: namespace,
			Labels: map[string]string{
				"app":            computeID,
				"project":        projectID,
				"type":           "compute",
				"vortex.io/name": req.Name,
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
					Volumes: podVolumes,
					Containers: []corev1.Container{
						{
							Name:         "app",
							Image:        req.Image,
							Ports:        containerPorts,
							VolumeMounts: volumeMounts,
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

	// Build Service.
	servicePorts := make([]corev1.ServicePort, 0, len(req.Ports))
	for _, p := range req.Ports {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Port:       p.Port,
			TargetPort: intstr.FromInt32(p.Port),
			Protocol:   corev1.Protocol(p.Protocol),
		})
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      computeID,
			Namespace: namespace,
			Labels:    map[string]string{"app": computeID, "project": projectID},
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

	endpoints := make([]string, 0, len(req.Ports))
	for _, p := range req.Ports {
		endpoints = append(endpoints, fmt.Sprintf("%s:%d", computeID, p.Port))
	}

	return &models.ComputeResponse{
		ID:        computeID,
		Name:      req.Name,
		Status:    "provisioning",
		Endpoints: endpoints,
		Volumes:   volumeInfos,
		CreatedAt: metav1.Now().Time,
	}, nil
}

func GetComputeStatus(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) (*models.ComputeResponse, error) {
	namespace := fmt.Sprintf("vortex-project-%s", projectID)

	deployment, err := k8sClient.Clientset.AppsV1().Deployments(namespace).Get(ctx, resourceID, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment %s: %w", resourceID, err)
	}

	service, err := k8sClient.Clientset.CoreV1().Services(namespace).Get(ctx, resourceID, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get service %s: %w", resourceID, err)
	}

	endpoints := []string{}
	if service != nil {
		for _, port := range service.Spec.Ports {
			endpoints = append(endpoints, fmt.Sprintf("%s:%d", resourceID, port.Port))
		}
	}

	// Reconstruct attached volumes from PVCs labelled for this compute instance.
	pvcs, _ := k8sClient.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s,type=compute-volume", resourceID),
	})
	var volumes []models.VolumeInfo
	if pvcs != nil {
		for _, p := range pvcs.Items {
			storage := p.Spec.Resources.Requests[corev1.ResourceStorage]
			gb, _ := storage.AsInt64()
			volumes = append(volumes, models.VolumeInfo{
				Name:   p.Name,
				SizeGB: int(gb / (1024 * 1024 * 1024)),
			})
		}
	}

	status := "provisioning"
	if deployment.Status.ReadyReplicas > 0 {
		status = "running"
	}

	return &models.ComputeResponse{
		ID:        resourceID,
		Name:      deployment.Labels["vortex.io/name"],
		Status:    status,
		Endpoints: endpoints,
		Volumes:   volumes,
		CreatedAt: deployment.CreationTimestamp.Time,
	}, nil
}

func ListComputeStatus(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string) ([]*models.ComputeResponse, error) {
	namespace := fmt.Sprintf("vortex-project-%s", projectID)

	deployments, err := k8sClient.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "type=compute",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list compute in namespace %s: %w", namespace, err)
	}

	computes := make([]*models.ComputeResponse, 0, len(deployments.Items))
	for _, d := range deployments.Items {
		service, err := k8sClient.Clientset.CoreV1().Services(namespace).Get(ctx, d.Name, metav1.GetOptions{})
		endpoints := []string{}
		if err == nil {
			for _, port := range service.Spec.Ports {
				endpoints = append(endpoints, fmt.Sprintf("%s:%d", d.Name, port.Port))
			}
		}

		status := "provisioning"
		if d.Status.ReadyReplicas > 0 {
			status = "running"
		}

		computes = append(computes, &models.ComputeResponse{
			ID:        d.Name,
			Name:      d.Labels["vortex.io/name"],
			Status:    status,
			Endpoints: endpoints,
			CreatedAt: d.CreationTimestamp.Time,
		})
	}

	return computes, nil
}

func DeleteCompute(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) error {
	namespace := fmt.Sprintf("vortex-project-%s", projectID)
	foreground := metav1.DeletePropagationForeground
	opts := metav1.DeleteOptions{PropagationPolicy: &foreground}

	err := k8sClient.Clientset.AppsV1().Deployments(namespace).Delete(ctx, resourceID, opts)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete deployment %s: %w", resourceID, err)
	}

	err = k8sClient.Clientset.CoreV1().Services(namespace).Delete(ctx, resourceID, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete service %s: %w", resourceID, err)
	}

	// Delete any attached PVCs.
	pvcs, err := k8sClient.Clientset.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s,type=compute-volume", resourceID),
	})
	if err == nil {
		for _, pvc := range pvcs.Items {
			k8sClient.Clientset.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{})
		}
	}

	return nil
}

func defaultIfEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
