package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/Over-knight/vortex/services/infrastructure-api/internal/models"
	"github.com/Over-knight/vortex/services/infrastructure-api/internal/vortexkube"
	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// engineConfig holds resolved values derived from the user's engine/version choice.
type engineConfig struct {
	image   string
	port    int32
	envVars []corev1.EnvVar // engine-specific env vars (credentials added later)
}

// dbSizeSpec maps an AWS-style instance class to concrete resource limits.
type dbSizeSpec struct {
	cpuRequest, cpuLimit       string
	memoryRequest, memoryLimit string
}

var sizeMap = map[string]dbSizeSpec{
	"db.small":  {"100m", "500m", "256Mi", "512Mi"},
	"db.medium": {"250m", "1000m", "512Mi", "1Gi"},
	"db.large":  {"500m", "2000m", "1Gi", "2Gi"},
}

// resolveEngine converts engine+version into a container image, port, and base env vars.
func resolveEngine(engine, version string, secretName string) (engineConfig, error) {
	switch engine {
	case "postgres", "":
		v := version
		if v == "" {
			v = "16"
		}
		return engineConfig{
			image: fmt.Sprintf("postgres:%s-alpine", v),
			port:  5432,
			envVars: []corev1.EnvVar{
				{Name: "POSTGRES_DB", Value: "vortex_db"},
				{
					Name: "POSTGRES_USER",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
							Key:                  "username",
						},
					},
				},
				{
					Name: "POSTGRES_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
							Key:                  "password",
						},
					},
				},
			},
		}, nil

	case "mysql":
		v := version
		if v == "" {
			v = "8.0"
		}
		return engineConfig{
			image: fmt.Sprintf("mysql:%s", v),
			port:  3306,
			envVars: []corev1.EnvVar{
				{Name: "MYSQL_DATABASE", Value: "vortex_db"},
				{
					Name: "MYSQL_USER",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
							Key:                  "username",
						},
					},
				},
				{
					Name: "MYSQL_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
							Key:                  "password",
						},
					},
				},
				// MySQL also requires a root password; reuse the generated password.
				{
					Name: "MYSQL_ROOT_PASSWORD",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
							Key:                  "password",
						},
					},
				},
			},
		}, nil

	default:
		return engineConfig{}, fmt.Errorf("unsupported engine %q: must be \"postgres\" or \"mysql\"", engine)
	}
}

// resolveSizeResources returns resource requests and limits for the given size class.
// Falls back to db.small if the size is unknown or empty.
func resolveSizeResources(size string) dbSizeSpec {
	if spec, ok := sizeMap[size]; ok {
		return spec
	}
	return sizeMap["db.small"]
}

// enginePort returns the default port for an engine by reading the stored label.
func enginePort(engine string) int32 {
	if engine == "mysql" {
		return 3306
	}
	return 5432
}

// EnsureNamespace creates a namespace for the project if it doesn't exist.
func EnsureNamespace(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string) (string, error) {
	namespaceName := fmt.Sprintf("vortex-project-%s", projectID)

	_, err := k8sClient.Clientset.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
	if err == nil {
		return namespaceName, nil
	}

	if apierrors.IsNotFound(err) {
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
				Labels: map[string]string{
					"vortex.io/project": projectID,
					"vortex.io/managed": "true",
				},
			},
		}
		_, createErr := k8sClient.Clientset.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
		if createErr != nil {
			return "", fmt.Errorf("failed to create namespace %s: %w", namespaceName, createErr)
		}
		return namespaceName, nil
	}

	return "", fmt.Errorf("failed to check namespace %s: %w", namespaceName, err)
}

func ProvisionDatabase(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, req models.DatabaseRequest) (*models.DatabaseResponse, error) {
	namespace, err := EnsureNamespace(ctx, k8sClient, projectID)
	if err != nil {
		return nil, err
	}

	dbID := fmt.Sprintf("db-%s", GenerateUUID())
	password := GenerateSecurePassword(16)
	username := "vortex"
	secretName := fmt.Sprintf("%s-secret", dbID)

	// Resolve engine image, port, and env vars from the request.
	eng, err := resolveEngine(req.Engine, req.Version, secretName)
	if err != nil {
		return nil, err
	}

	// Resolve CPU/memory from instance size class.
	sizes := resolveSizeResources(req.Size)

	// Resolve storage — default to 10Gi if not specified.
	storageGB := req.Config.StorageGB
	if storageGB <= 0 {
		storageGB = 10
	}
	storageQuantity := fmt.Sprintf("%dGi", storageGB)

	// Create Secret with credentials.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"username": username,
			"password": password,
		},
	}
	_, err = k8sClient.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	// Create StatefulSet.
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbID,
			Namespace: namespace,
			Labels: map[string]string{
				"app":              dbID,
				"project":          projectID,
				"type":             "database",
				"vortex.io/name":   req.Name,
				"vortex.io/engine": req.Engine,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: dbID,
			Replicas:    int32Ptr(1),
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
							Name:  "db",
							Image: eng.image,
							Ports: []corev1.ContainerPort{
								{ContainerPort: eng.port},
							},
							Env: eng.envVars,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    createQuantity(sizes.cpuRequest),
									corev1.ResourceMemory: createQuantity(sizes.memoryRequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    createQuantity(sizes.cpuLimit),
									corev1.ResourceMemory: createQuantity(sizes.memoryLimit),
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "db-storage",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: stringPtr("standard"),
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: createQuantity(storageQuantity),
							},
						},
					},
				},
			},
		},
	}

	_, err = k8sClient.Clientset.AppsV1().StatefulSets(namespace).Create(ctx, statefulSet, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create statefulset: %w", err)
	}

	// LoadBalancer service — external IP is assigned asynchronously by the cloud provider.
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbID,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"app": dbID},
			Ports: []corev1.ServicePort{
				{Port: eng.port, TargetPort: intstr.FromInt32(eng.port)},
			},
		},
	}
	_, err = k8sClient.Clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	return &models.DatabaseResponse{
		ID:        dbID,
		Name:      req.Name,
		Engine:    req.Engine,
		Status:    "provisioning",
		Endpoint:  "pending", // LB external IP not yet assigned
		Username:  username,
		Password:  password,
		CreatedAt: metav1.Now().Time,
	}, nil
}

// GetDatabaseStatus retrieves the current status of a provisioned database.
func GetDatabaseStatus(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) (*models.DatabaseResponse, error) {
	namespace := fmt.Sprintf("vortex-project-%s", projectID)

	statefulset, err := k8sClient.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, resourceID, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get statefulset %s in namespace %s: %w", resourceID, namespace, err)
	}

	secretName := fmt.Sprintf("%s-secret", resourceID)
	secret, err := k8sClient.Clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", secretName, err)
	}

	// Resolve the external endpoint from the LoadBalancer service.
	svc, err := k8sClient.Clientset.CoreV1().Services(namespace).Get(ctx, resourceID, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get service %s: %w", resourceID, err)
	}

	engine := statefulset.Labels["vortex.io/engine"]
	port := enginePort(engine)
	endpoint := resolveEndpoint(svc, port)

	status := "provisioning"
	if statefulset.Status.ReadyReplicas > 0 {
		status = "running"
	}

	return &models.DatabaseResponse{
		ID:        resourceID,
		Name:      statefulset.Labels["vortex.io/name"],
		Engine:    engine,
		Status:    status,
		Endpoint:  endpoint,
		Username:  string(secret.Data["username"]),
		Password:  string(secret.Data["password"]),
		CreatedAt: statefulset.CreationTimestamp.Time,
	}, nil
}

// resolveEndpoint returns the external host:port once the LoadBalancer has been assigned,
// or "pending" while the cloud provider is still provisioning it.
func resolveEndpoint(svc *corev1.Service, port int32) string {
	ingress := svc.Status.LoadBalancer.Ingress
	if len(ingress) == 0 {
		return "pending"
	}
	host := ingress[0].Hostname
	if host == "" {
		host = ingress[0].IP
	}
	if host == "" {
		return "pending"
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// ListDatabases returns all databases provisioned for a project.
func ListDatabases(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string) ([]models.DatabaseResponse, error) {
	namespace := fmt.Sprintf("vortex-project-%s", projectID)

	list, err := k8sClient.Clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "type=database",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list databases in namespace %s: %w", namespace, err)
	}

	databases := make([]models.DatabaseResponse, 0, len(list.Items))
	for _, ss := range list.Items {
		status := "provisioning"
		if ss.Status.ReadyReplicas > 0 {
			status = "running"
		}

		engine := ss.Labels["vortex.io/engine"]
		port := enginePort(engine)

		svc, err := k8sClient.Clientset.CoreV1().Services(namespace).Get(ctx, ss.Name, metav1.GetOptions{})
		endpoint := "pending"
		if err == nil {
			endpoint = resolveEndpoint(svc, port)
		}

		databases = append(databases, models.DatabaseResponse{
			ID:        ss.Name,
			Name:      ss.Labels["vortex.io/name"],
			Engine:    engine,
			Status:    status,
			Endpoint:  endpoint,
			CreatedAt: ss.CreationTimestamp.Time,
			// Credentials omitted from list responses
		})
	}

	return databases, nil
}

// DeleteDatabase removes all resources associated with a provisioned database.
func DeleteDatabase(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) error {
	namespace := fmt.Sprintf("vortex-project-%s", projectID)
	deletionPolicy := metav1.DeletePropagationForeground
	deletionOptions := metav1.DeleteOptions{
		PropagationPolicy: &deletionPolicy,
	}

	err := k8sClient.Clientset.AppsV1().StatefulSets(namespace).Delete(ctx, resourceID, deletionOptions)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete statefulset %s: %w", resourceID, err)
	}

	err = k8sClient.Clientset.CoreV1().Services(namespace).Delete(ctx, resourceID, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete service %s: %w", resourceID, err)
	}

	secretName := fmt.Sprintf("%s-secret", resourceID)
	err = k8sClient.Clientset.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete secret %s: %w", secretName, err)
	}

	return nil
}

func int32Ptr(i int32) *int32    { return &i }
func stringPtr(s string) *string { return &s }

func createQuantity(s string) resource.Quantity {
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return resource.Quantity{}
	}
	return q
}

func GenerateUUID() string {
	return uuid.New().String()[:8]
}

func GenerateSecurePassword(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)[:length]
}
