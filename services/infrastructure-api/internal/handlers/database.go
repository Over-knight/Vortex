package handlers

import (
    "context"
    "fmt"
    
    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/util/intstr"
    "k8s.io/apimachinery/pkg/api/resource"
    "github.com/Over-knight/vortex/services/infrastructure-api/internal/kubernetes"
    "github.com/Over-knight/vortex/services/infrastructure-api/internal/models"
    "github.com/Over-knight/vortex/services/infrastructure-api/pkg/utils"
)

func ProvisionDatabase(ctx context.Context, k8sClient *kubernetes.K8sClient, projectID string, req models.DatabaseRequest) (*models.DatabaseResponse, error) {
	//1. Generate a unique ID and password
	dbID := fmt.Sprintf("db-%s", utils.GenerateUUID())
	password := utils.GenerateSecurePassword(16)
	username := "vortex"

	//2. Create a secret with credentials
	secretName := fmt.Sprintf("%s-secret", dbID)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
			Namespace: "vortex",
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"username": username,
			"password": password,
		},
	}
	_, err := k8sClient.Clientset.CoreV1().Secrets("vortex").Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	//3. Create a statefulSet 
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: dbID,
			Namespace: "vortex",
			Labels: map[string]string{
				"app": dbID,
				"project": projectID,
				"type": "database",
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: dbID,
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": dbID},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": dbID},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
					{
						Name: "postgres",
						Image: "postgres:16-alpine",
						Ports: []corev1.ContainerPort{
							{ContainerPort: 5432},
						},
						Env: []corev1.EnvVar{
							{Name: "POSTGRES_DB", Value: "vortex_db"},
							{
								Name: "POSTGRES_USER",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
										Key: "username",
									},
								},
							},
							{
								Name: "POSTGRES_PASSWORD",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
                                        Key: "password",
									},
								},
							},
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: mustParseQuantity("100m"),
								corev1.ResourceMemory: mustParseQuantity("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU: mustParseQuantity("500m"),
								corev1.ResourceMemory: mustParseQuantity("512Mi"),
							},
						},
					},
				},
			},
		},
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "postgres-storage",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					StorageClassName: stringPtr("standard"),
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: mustParseQuantity("10Gi"),
						},
					},
				},
			},
		},
	}

	//4. Create the statefulset in k8s
	_, err = k8sClient.Clientset.AppsV1().StatefulSets("vortex").Create(ctx, statefulSet, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create statefulset: %w", err)
	}	

	//5. Create a service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: dbID,
			Namespace: "vortex",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None", //Headless service
			Selector: map[string]string{"app": dbID},
			Ports: []corev1.ServicePort{
			{Port: 5432, TargetPort: intstr.FromInt(5432)},
			},
		},
	}
	_, err = k8sClient.Clientset.CoreV1().Services("vortex").Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}	

	//step 6: Return response (status is "provisioning" until pod is ready)
	return &models.DatabaseResponse{
		ID:        dbID,
		Name:      req.Name,
		Status:    "provisioning",
		Endpoint:  fmt.Sprintf("%s:5432", dbID),
		Username:  username,
		Password:  password,
		CreatedAt: metav1.Now().Time,
	}, nil
}

//Helper functions
func int32Ptr(i int32) *int32 { return &i }
func stringPtr(s string) *string { return &s }
func mustParseQuantity(s string) resource.Quantity {
	q, _ := resource.ParseQuantity(s)
	return q
}

