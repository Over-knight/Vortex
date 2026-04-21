# Vortex Architecture & Implementation Guide

**Version:** 1.0  
**Last Updated:** April 21, 2026  
**Status:** Production-Ready for Development Deployment

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Architecture Principles](#architecture-principles)
3. [Component Breakdown](#component-breakdown)
4. [Data Models](#data-models)
5. [API Design](#api-design)
6. [Implementation Details](#implementation-details)
7. [Multi-Tenancy Strategy](#multi-tenancy-strategy)
8. [Security Model](#security-model)
9. [Resource Lifecycle](#resource-lifecycle)
10. [Future Extensions](#future-extensions)

---

## System Overview

### What is Vortex?

Vortex is a **cloud-in-a-box platform** that abstracts Kubernetes complexity behind a simple REST API. Instead of users writing YAML manifests, they request resources (databases, caches, compute) through HTTP endpoints, and Vortex provisions them automatically.

**Core Innovation:** Transform Kubernetes from infrastructure tool → cloud platform

### Design Philosophy

- **Separation of Concerns:** Users never interact with Kubernetes directly
- **Dynamic Provisioning:** Resources created on-demand, not pre-provisioned
- **Multi-Tenant Isolation:** Each project operates in its own namespace
- **Secure by Default:** Credentials auto-generated, stored in K8s Secrets
- **Stateless API:** Scalable horizontally; K8s is source of truth for state
- **Observable:** Status endpoints allow monitoring without kubectl

---

## Architecture Principles

### 1. **Control Plane / Data Plane Separation**

```
┌─────────────────────────────────────────────────────────────┐
│ Control Plane (Vortex Infrastructure API)                   │
│ - REST endpoints                                             │
│ - User request handling                                      │
│ - K8s resource generation                                    │
│ - Credential management                                      │
└──────────────────────────┬──────────────────────────────────┘
                           │
                  Kubernetes API (client-go)
                           │
┌──────────────────────────▼──────────────────────────────────┐
│ Data Plane (Kubernetes Cluster)                              │
│ - PostgreSQL (StatefulSet)                                   │
│ - Redis (Deployment)                                         │
│ - User workloads (future)                                    │
│ - Persistent volumes                                         │
└─────────────────────────────────────────────────────────────┘
```

**Rationale:**
- **API as stateless controller:** Can be replicated across multiple instances
- **K8s as state store:** Single source of truth (no separate database needed for state)
- **Decoupling:** Easier to upgrade/scale API independently of workloads

### 2. **Declarative Resource Management**

All resources follow Kubernetes patterns:
- **Declared state:** API creates K8s objects describing desired state
- **Kubernetes enforces:** Scheduling, networking, storage, recovery
- **API doesn't manage:** Scheduling, networking, or lifecycle (K8s handles it)

Example: When user requests a database, we don't start it ourselves—we declare a StatefulSet, and Kubernetes starts it.

### 3. **Namespace-Based Isolation**

Projects are isolated using Kubernetes **namespaces** named `vortex-project-{projectID}`:

```
vortex-project-acme-corp/          ← Namespace (RBAC boundary)
├── db-a1b2c3d4-secret             ← User's database credentials
├── postgres db-a1b2c3d4           ← StatefulSet
├── postgres db-a1b2c3d4           ← Service
├── db-e5f6g7h8-secret             ← Another resource
└── postgres db-e5f6g7h8

vortex-project-startup-xyz/        ← Different namespace
├── db-i9j0k1l2-secret
├── postgres db-i9j0k1l2
└── ...
```

**Benefits:**
- Resource quotas per project (limit CPU, memory, storage)
- Network policies (projects can't see each other's pods)
- RBAC (future: project teams get namespace access)
- Clean deletion (delete namespace = delete entire project)

---

## Component Breakdown

### A. Infrastructure API Service

**Language:** Go 1.23.0  
**Framework:** Gin Web Framework v1.12.0  
**Purpose:** Accept REST requests and provision K8s resources

#### File Structure

```
services/infrastructure-api/
├── main.go                     # Entry point, router definition
├── go.mod                      # Dependencies
├── Dockerfile                  # Container image
│
├── internal/
│   ├── vortexkube/
│   │   └── client.go          # K8s cluster connection
│   │
│   ├── handlers/
│   │   └── database.go        # Business logic for database provisioning
│   │
│   └── models/
│       └── resources.go       # Data transfer objects
│
└── pkg/
    └── utils.go               # Utility functions (UUIDs, passwords)
```

#### Key Design Decisions

**1. Package Organization**
- `internal/` packages: Private, cannot be imported by other services
- `vortexkube/` instead of `kubernetes/`: Avoids naming collision with `k8s.io/client-go/kubernetes`
- `pkg/` would be shared across services (if needed), but utilities are currently inlined

**2. Handler Pattern**
Each resource type (DATABASE, CACHE, COMPUTE) has a handler with:
- `EnsureNamespace()`: Creates project namespace if needed
- `Provision*()`: Creates K8s resources
- `Get*Status()`: Queries current state
- `Delete*()`: Cleans up resources (cascading delete)

**Current Handlers:**
- `database.go`: PostgreSQL provisioning (ProvisionDatabase, GetDatabaseStatus, DeleteDatabase)
- `cache.go`: Redis provisioning (ProvisionCache, GetCacheStatus, DeleteCache)

**3. No Database Backend**
- Kubernetes is the state store
- API is stateless (no persistence layer)
- State queries hit Kubernetes, not a database
- Enables horizontal scaling of API replicas

### D. In-Cluster Deployment Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Kubernetes Cluster (Kind v1.35.0)                           │
│                                                             │
│ ┌───────────────────────────────────────────────────────┐   │
│ │ vortex namespace                                      │   │
│ │                                                       │   │
│ │ ┌─────────────────────────────────────────────────┐   │   │
│ │ │ Infrastructure API Deployment                  │   │   │
│ │ │ - ServiceAccount (pod identity)                │   │   │
│ │ │ - Container: vortex-api:latest                 │   │   │
│ │ │ - Port: 8080                                   │   │   │
│ │ │ - Security: Non-root, read-only FS             │   │   │
│ │ │ - Health probes (liveness/readiness)           │   │   │
│ │ └─────────────────────────────────────────────────┘   │   │
│ │                    ↓                                   │   │
│ │ ┌─────────────────────────────────────────────────┐   │   │
│ │ │ ClusterRole: infrastructure-api                │   │   │
│ │ │ - create/get/list/delete namespaces            │   │   │
│ │ │ - create/get/list/delete secrets               │   │   │
│ │ │ - create/get/list/delete services              │   │   │
│ │ │ - create/get/list/delete statefulsets          │   │   │
│ │ │ - create/get/list/delete deployments           │   │   │
│ │ │ - get/list pods                                │   │   │
│ │ └─────────────────────────────────────────────────┘   │   │
│ └───────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Key Design Decision:** API runs inside cluster
- Pod can use `rest.InClusterConfig()` automatically
- No external kubeconfig needed
- ServiceAccount token injected by Kubernetes
- Scales independently of application layer

### D. In-Cluster Deployment Architecture

```
┌─────────────────────────────────────────────────────────┐
│ Kubernetes Cluster (Kind v1.35.0)                       │
│                                                         │
│ ┌─────────────────────────────────────────────────┐    │
│ │ vortex namespace                                 │    │
│ │                                                  │    │
│ │ ┌──────────────────────────────────────────┐    │    │
│ │ │ Infrastructure API Deployment            │    │    │
│ │ │ - ServiceAccount (pod identity)           │    │    │
│ │ │ - Container: vortex-api:latest            │    │    │
│ │ │ - Port: 8080                              │    │    │
│ │ │ - Security: Non-root, read-only FS        │    │    │
│ │ │ - Health probes (liveness/readiness)      │    │    │
│ │ └──────────────────────────────────────────┘    │    │
│ │                     ↓                             │    │
│ │ ┌──────────────────────────────────────────┐    │    │
│ │ │ ClusterRole: infrastructure-api           │    │    │
│ │ │ - create/get/list/delete namespaces      │    │    │
│ │ │ - create/get/list/delete secrets         │    │    │
│ │ │ - create/get/list/delete services        │    │    │
│ │ │ - create/get/list/delete statefulsets    │    │    │
│ │ │ - create/get/list/delete deployments     │    │    │
│ │ │ - get/list pods                          │    │    │
│ │ └──────────────────────────────────────────┘    │    │
│ └─────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

**Key Design Decision:** API runs inside cluster
- Pod can use `rest.InClusterConfig()` automatically
- No external kubeconfig needed
- ServiceAccount token injected by Kubernetes
- Scales independently of application layer

### B. Kubernetes Cluster (Kind)

**Distribution:** Kind (Kubernetes in Docker)  
**Version:** v1.35.0  
**Nodes:** 1 control-plane  
**Network:** Docker network (localhost:3000 for app services)

#### Static Infrastructure (Currently Running)

```yaml
# PostgreSQL: Stateful database
- StatefulSet: postgres
  Replicas: 1
  Storage: 10Gi PersistentVolume
  Image: postgres:16-alpine
  Service: postgres (ClusterIP, port 5432)

# Redis: Cache layer
- Deployment: redis
  Replicas: 1
  Image: redis:7-alpine
  Service: redis (ClusterIP, port 6379)
```

#### Dynamic Resources (Created by API)

**Databases:** When user requests a database via API:
1. **Secret:** `db-{resourceID}-secret` contains username/password
2. **StatefulSet:** `{resourceID}` runs PostgreSQL
3. **Service:** `{resourceID}` exposes StatefulSet on port 5432
4. Stored in namespace: `vortex-project-{projectID}`

**Caches:** When user requests a Redis cache via API:
1. **Deployment:** `cache-{resourceID}` runs Redis (stateless)
2. **Service:** `cache-{resourceID}` exposes Deployment on port 6379
3. Stored in namespace: `vortex-project-{projectID}`
4. No Secret needed (Redis is stateless, no credentials)

### C. Client-Go Integration

**Library:** k8s.io/client-go v0.35.4  
**Purpose:** Programmatically interact with Kubernetes API

#### Authentication Strategy

```go
// internal/vortexkube/client.go

// Try in-cluster config first (running inside K8s pod)
config, err := rest.InClusterConfig()

// Fallback to kubeconfig (running locally)
if err != nil {
    kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
    config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
}

// Create clientset (API client for all K8s resource types)
clientset, err := kubernetes.NewForConfig(config)
```

**Supported Contexts:**
- Local development: Uses `~/.kube/config`
- In-cluster deployment: Uses service account token

---

## Data Models

### Request Models

**DatabaseRequest**
```go
type DatabaseRequest struct {
    Name    string           `json:"name"`
    Engine  string           `json:"engine"`      // "postgres", "mysql", etc.
    Version string           `json:"version"`     // "16", "15", etc.
    Config  map[string]interface{} `json:"config"` // engine-specific config
}

// Config example:
// {
//   "storage_gb": 10,
//   "replicas": 1,
//   "backup_enabled": true
// }
```

### Response Models

**DatabaseResponse**
```go
type DatabaseResponse struct {
    ID        string    `json:"id"`           // Unique resource ID (UUID)
    Name      string    `json:"name"`
    Status    string    `json:"status"`       // "provisioning" or "running"
    Endpoint  string    `json:"endpoint"`     // "db-{ID}:5432" (internal DNS)
    Username  string    `json:"username"`     // e.g., "vortex"
    Password  string    `json:"password"`     // Secure random 16-char string
    CreatedAt time.Time `json:"created_at"`
}
```

**CacheRequest**
```go
type CacheRequest struct {
    Name   string      `json:"name"`
    Engine string      `json:"engine"`   // "redis" (extensible for memcached, etc.)
    Config CacheConfig `json:"config"`
}

type CacheConfig struct {
    MemoryMB int `json:"memory_mb"`  // Default: 256
    Replicas int `json:"replicas"`   // Default: 1
}
```

**CacheResponse**
```go
type CacheResponse struct {
    ID        string    `json:"id"`           // Unique resource ID (UUID)
    Name      string    `json:"name"`
    Status    string    `json:"status"`       // "provisioning" or "running"
    Endpoint  string    `json:"endpoint"`     // "cache-{ID}:6379" (internal DNS)
    CreatedAt time.Time `json:"created_at"`
    // Note: No password field (Redis stateless, auth handled separately)
}
```

### Status Values

- **`provisioning`**: K8s resource created, pod not ready yet
  - When returned: StatefulSet exists but ReadyReplicas = 0
  - Duration: Usually 10-30 seconds (pulls image, starts container)
  - What user should do: Poll GET endpoint until "running"

- **`running`**: Pod ready and accepting connections
  - When returned: ReadyReplicas > 0 and liveness probe passed
  - Duration: Stable (until user deletes it)
  - What user should do: Open connections, use database

---

## API Design

### Endpoint Structure

**Database Endpoints:**
```
POST   /v1/projects/{project_id}/resources/databases
       → Create database for project
       → Response: DatabaseResponse with status="provisioning"

GET    /v1/projects/{project_id}/resources/databases/{resource_id}
       → Check status of existing resource
       → Response: DatabaseResponse with current status

DELETE /v1/projects/{project_id}/resources/databases/{resource_id}
       → Remove database and all associated K8s objects
       → Response: 200 OK or error
```

**Cache Endpoints:**
```
POST   /v1/projects/{project_id}/resources/caches
       → Create Redis cache for project
       → Response: CacheResponse with status="provisioning"

GET    /v1/projects/{project_id}/resources/caches/{resource_id}
       → Check status of existing cache
       → Response: CacheResponse with current status

DELETE /v1/projects/{project_id}/resources/caches/{resource_id}
       → Remove cache and all associated K8s objects
       → Response: 200 OK or error
```

**Health Endpoint:**
```
GET    /health
       → Returns {"status": "ok"}
       → Used for liveness/readiness probes in Kubernetes
```

### Request-Response Flow

```
Client Request:
  POST /v1/projects/acme/resources/databases
  {
    "name": "prod-db",
    "engine": "postgres",
    "version": "16",
    "config": { "storage_gb": 10 }
  }

Handler (database.go):
  1. Extract projectID ("acme")
  2. EnsureNamespace("acme")  → creates "vortex-project-acme" if needed
  3. GenerateUUID()           → "db-a1b2c3d4"
  4. GeneratePassword()       → "7f8a9c2d5e1b3a4c"
  5. CreateSecret()           → K8s Secret with credentials
  6. CreateStatefulSet()      → PostgreSQL pod declaration
  7. CreateService()          → DNS entry "db-a1b2c3d4:5432"
  8. Return response with status="provisioning"

HTTP Response:
  {
    "id": "db-a1b2c3d4",
    "name": "prod-db",
    "status": "provisioning",
    "endpoint": "db-a1b2c3d4:5432",
    "username": "vortex",
    "password": "7f8a9c2d5e1b3a4c",
    "created_at": "2026-04-21T10:30:00Z"
  }
```

---

## Implementation Details

### 1. Namespace Creation (Idempotent)

**Code Location:** `internal/handlers/database.go` → `EnsureNamespace()`

```go
// Problem: Multiple requests might try to create same namespace
// Solution: Check if exists before creating

namespaceName := fmt.Sprintf("vortex-project-%s", projectID)

// Try to get existing namespace
_, err := k8sClient.Clientset.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
if err == nil {
    // Already exists, return success
    return namespaceName, nil
}

if apierrors.IsNotFound(err) {
    // Doesn't exist, create it
    namespace := &corev1.Namespace{
        ObjectMeta: metav1.ObjectMeta{Name: namespaceName},
    }
    _, err := k8sClient.Clientset.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
    return namespaceName, err
}

// Other error (permission denied, network issue, etc.)
return "", err
```

**Why This Pattern?**
- **Idempotency:** Calling it twice is safe (second call finds existing namespace)
- **Error Distinction:** IsNotFound vs actual failures handled separately
- **Robust:** Network glitches won't cause crashes

### 2. Secret Creation (Per-Instance Credentials)

**Code Location:** `internal/handlers/database.go` → `ProvisionDatabase()` Step 2

```go
secretName := fmt.Sprintf("%s-secret", dbID)
secret := &corev1.Secret{
    ObjectMeta: metav1.ObjectMeta{
        Name:      secretName,
        Namespace: namespace,
    },
    Type: corev1.SecretTypeOpaque,
    StringData: map[string]string{
        "username": "vortex",
        "password": password,  // Cryptographically random
    },
}
_, err := k8sClient.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
```

**Security Benefits:**
- Credentials never logged (in-memory only)
- Credentials stored in K8s etcd (encrypted at rest in production)
- Each database has unique password (breach isolation)
- No credentials in environment variables or code

### 3. StatefulSet Creation (Persistent Workload)

**Code Location:** `internal/handlers/database.go` → `ProvisionDatabase()` Step 3

Why StatefulSet and not Deployment?
- StatefulSet provides **stable hostname** (pod-0, pod-1, pod-2)
- Supports **VolumeClaimTemplate** (persistent storage per replica)
- Maintains order during updates (important for databases)

```go
statefulSet := &appsv1.StatefulSet{
    ObjectMeta: metav1.ObjectMeta{
        Name:      dbID,
        Namespace: namespace,
        Labels: map[string]string{
            "app":     dbID,
            "project": projectID,
            "type":    "database",
        },
    },
    Spec: appsv1.StatefulSetSpec{
        ServiceName: dbID,  // Matches Service name (for DNS)
        Replicas:   int32Ptr(1),
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
                        Name:  "postgres",
                        Image: "postgres:16-alpine",
                        Ports: []corev1.ContainerPort{
                            {ContainerPort: 5432},
                        },
                        Env: []corev1.EnvVar{
                            // Environment variables injected from Secret
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
                                corev1.ResourceCPU:    createQuantity("100m"),
                                corev1.ResourceMemory: createQuantity("256Mi"),
                            },
                            Limits: corev1.ResourceList{
                                corev1.ResourceCPU:    createQuantity("500m"),
                                corev1.ResourceMemory: createQuantity("512Mi"),
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
                    AccessModes: []corev1.PersistentVolumeAccessMode{
                        corev1.ReadWriteOnce,
                    },
                    StorageClassName: stringPtr("standard"),
                    Resources: corev1.VolumeResourceRequirements{
                        Requests: corev1.ResourceList{
                            corev1.ResourceStorage: createQuantity("10Gi"),
                        },
                    },
                },
            },
        },
    },
}
```

**Key Design Points:**
- **VolumeClaimTemplate:** Creates persistent volume automatically
- **Env injection from Secret:** Credentials not hardcoded
- **Resource limits:** Prevents one database from starving others
- **Labels:** Enable filtering, monitoring, RBAC

### 4. Service Creation (DNS Resolution)

**Code Location:** `internal/handlers/database.go` → `ProvisionDatabase()` Step 5

```go
service := &corev1.Service{
    ObjectMeta: metav1.ObjectMeta{
        Name:      dbID,
        Namespace: namespace,
        Labels: map[string]string{
            "app": dbID,
        },
    },
    Spec: corev1.ServiceSpec{
        ClusterIP: "None",  // Headless service (direct pod IP access)
        Selector: map[string]string{
            "app": dbID,
        },
        Ports: []corev1.ServicePort{
            {
                Protocol: corev1.ProtocolTCP,
                Port:     5432,
                TargetPort: intstr.FromInt(5432),
            },
        },
    },
}
```

**Why Headless Service?**
- No load balancing (direct access to pod)
- Stable DNS name: `{dbID}.{namespace}.svc.cluster.local`
- Simplified to: `{dbID}` within same namespace
- Matches StatefulSet naming pattern

### 5. Status Checking

**Code Location:** `internal/handlers/database.go` → `GetDatabaseStatus()`

```go
// Query the StatefulSet to get current status
statefulset, err := k8sClient.Clientset.AppsV1().StatefulSets(namespace).Get(ctx, resourceID, metav1.GetOptions{})
if err != nil {
    if apierrors.IsNotFound(err) {
        return nil, fmt.Errorf("database not found")
    }
    return nil, fmt.Errorf("failed to get database status: %w", err)
}

// Check how many replicas are ready
status := "provisioning"
if statefulset.Status.ReadyReplicas > 0 {
    status = "running"
}

return &models.DatabaseResponse{
    ID:        resourceID,
    Name:      statefulset.Name,
    Status:    status,
    Endpoint:  fmt.Sprintf("%s:5432", resourceID),
    CreatedAt: statefulset.CreationTimestamp.Time,
}, nil
```

### 6. Resource Deletion (Cascading Cleanup)

**Code Location:** `internal/handlers/database.go` → `DeleteDatabase()`

```go
// Delete StatefulSet with foreground propagation
// This waits for all owned resources (pods, pvcs) to be deleted first
deletionPolicy := metav1.DeletePropagationForeground
deletionOptions := metav1.DeleteOptions{
    PropagationPolicy: &deletionPolicy,
}

// Delete StatefulSet (this triggers cascade)
err = k8sClient.Clientset.AppsV1().StatefulSets(namespace).Delete(ctx, resourceID, deletionOptions)

// Delete Service separately (not owned by StatefulSet)
err = k8sClient.Clientset.CoreV1().Services(namespace).Delete(ctx, resourceID, metav1.DeleteOptions{})

// Delete Secret separately (not owned by StatefulSet)
secretName := fmt.Sprintf("%s-secret", resourceID)
err = k8sClient.Clientset.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
```

**Why This Order?**
1. StatefulSet deletion (with foreground) waits for pods → PVC cleanup
2. Service deletion (independent cleanup)
3. Secret deletion (independent cleanup)
4. If StatefulSet deletion fails, database still exists (safe)
5. If Secret deletion fails, credentials still secure

---

## Multi-Tenancy Strategy

### Namespace-Based Isolation

**Pattern:** Each project gets its own Kubernetes namespace

```
├── vortex-project-acme-corp/
│   ├── Secrets (database credentials)
│   ├── StatefulSets (databases)
│   ├── Services (DNS)
│   └── PersistentVolumeClaims (storage)
│
├── vortex-project-startup-xyz/
│   ├── Secrets
│   ├── StatefulSets
│   └── ...
│
└── vortex-project-enterprise-co/
    └── ...
```

### Namespace Naming Convention

```go
namespaceName := fmt.Sprintf("vortex-project-%s", projectID)
```

**Requirements for projectID:**
- Lowercase alphanumeric + hyphens
- Match Kubernetes label value format
- Example: `acme-corp`, `startup-123`, `enterprise-co`

### Resource Quota Per Project (Future)

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: vortex-project-acme-corp
---
apiVersion: v1
kind: ResourceQuota
metadata:
  name: project-quota
  namespace: vortex-project-acme-corp
spec:
  hard:
    requests.cpu: "10"           # Max 10 CPU cores
    requests.memory: "50Gi"      # Max 50GB RAM
    persistentvolumeclaims: "10" # Max 10 volumes
    pods: "100"                  # Max 100 pods
```

### Network Policies (Future)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: isolate-project
  namespace: vortex-project-acme-corp
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector: {}  # Allow from same namespace only
  egress:
  - to:
    - podSelector: {}  # Allow to same namespace only
```

---

## Security Model

### Credential Management

**Generation:**
```go
// Cryptographically secure random password
func GenerateSecurePassword() (string, error) {
    bytes := make([]byte, 16)
    if _, err := rand.Read(bytes); err != nil {
        return "", err
    }
    return hex.EncodeToString(bytes), nil
}
```

**Storage:**
- Kubernetes Secret (encrypted at rest in production)
- Never logged, never in environment variables
- Never in source code or config files

**Retrieval:**
- User gets credentials in response (only time shown)
- Stored by user in their password manager
- API doesn't store them (K8s is source of truth)

### API Security (Future)

```yaml
# RBAC example (future implementation)
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: vortex-project-acme-corp
  name: database-user
rules:
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list"]
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]
```

### Service Account Permissions

When API runs inside K8s:
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: infrastructure-api
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: infrastructure-api
rules:
- apiGroups: [""]
  resources: ["namespaces", "secrets", "services"]
  verbs: ["get", "list", "create", "update", "delete"]
- apiGroups: ["apps"]
  resources: ["statefulsets"]
  verbs: ["get", "list", "create", "update", "delete"]
```

---

## Resource Lifecycle

### Database Lifecycle Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         USER ACTIONS                         │
└──────┬───────────────────────────────────────────────────────┘
       │
       │ POST /v1/projects/acme/resources/databases
       ▼
┌─────────────────────────────────────────────────────────────┐
│ API: Create Resources                                        │
├─────────────────────────────────────────────────────────────┤
│ ✓ EnsureNamespace("acme") → vortex-project-acme             │
│ ✓ Generate ID → db-a1b2c3d4                                 │
│ ✓ Generate password → 7f8a9c2d5e1b3a4c                      │
│ ✓ Create Secret → db-a1b2c3d4-secret                        │
│ ✓ Create StatefulSet → db-a1b2c3d4                          │
│ ✓ Create Service → db-a1b2c3d4                              │
│ → Return 200 OK with status="provisioning"                  │
└──────┬───────────────────────────────────────────────────────┘
       │
       │ Kubernetes scheduling
       ▼
┌─────────────────────────────────────────────────────────────┐
│ K8s: Initialize Pod                                          │
├─────────────────────────────────────────────────────────────┤
│ 1. Scheduler places pod on node                              │
│ 2. Container runtime pulls postgres:16-alpine image          │
│ 3. PostgreSQL process starts                                 │
│ 4. Liveness probe: Is process alive?                         │
│ 5. Readiness probe: Is accepting connections? (No yet)       │
│    → Status: NotReady                                        │
│                                                              │
│ 6. PostgreSQL initialization complete                        │
│ 7. Readiness probe succeeds                                  │
│    → Status: Ready                                           │
│    → ReadyReplicas: 1                                        │
└──────┬───────────────────────────────────────────────────────┘
       │
       │ GET /v1/projects/acme/resources/databases/db-a1b2c3d4
       │ (Poll endpoint until ready)
       ▼
┌─────────────────────────────────────────────────────────────┐
│ API: Check Status                                            │
├─────────────────────────────────────────────────────────────┤
│ Query StatefulSet.Status.ReadyReplicas                       │
│ ReadyReplicas = 1 → status="running"                         │
│ → Return 200 OK with status="running"                        │
└──────┬───────────────────────────────────────────────────────┘
       │
       │ Client connects to database
       │ psql -h db-a1b2c3d4 -U vortex -d vortex_db
       │ (password: 7f8a9c2d5e1b3a4c)
       ▼
┌─────────────────────────────────────────────────────────────┐
│ Database: Operational                                        │
├─────────────────────────────────────────────────────────────┤
│ ✓ Pod running                                                │
│ ✓ Service accepting connections                              │
│ ✓ PersistentVolume mounted                                   │
│ ✓ Data persisted to disk                                     │
│ ✓ Credentials secured in K8s Secret                          │
└──────┬───────────────────────────────────────────────────────┘
       │
       │ [User operates database for days/weeks/months]
       │
       │ DELETE /v1/projects/acme/resources/databases/db-a1b2c3d4
       ▼
┌─────────────────────────────────────────────────────────────┐
│ API: Cleanup Resources (Cascading)                           │
├─────────────────────────────────────────────────────────────┤
│ 1. Delete StatefulSet (with foreground propagation)          │
│    → Kubectl waits for pods to terminate gracefully          │
│ 2. Delete Service                                            │
│ 3. Delete Secret                                             │
│ → Return 200 OK                                              │
│                                                              │
│ K8s Cascading:                                               │
│ - StatefulSet deletion triggers pod termination              │
│ - Pod termination triggers PVC cleanup                       │
│ - PersistentVolume becomes available                         │
│ - All data deleted from disk                                 │
└──────┬───────────────────────────────────────────────────────┘
       │
       ▼
   [Resource fully cleaned up]
```

---

## Future Extensions

### Adding New Resource Types

To add support for new resources (e.g., REDIS, COMPUTE), follow this pattern:

#### 1. Create Handler Package

```go
// internal/handlers/cache.go
package handlers

type CacheRequest struct {
    Name   string
    Engine string  // "redis", "memcached"
}

type CacheResponse struct {
    ID       string
    Status   string
    Endpoint string
    // ... other fields
}

func ProvisionCache(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, req CacheRequest) (*CacheResponse, error) {
    // 1. EnsureNamespace
    // 2. Generate ID
    // 3. Create Secret (if credentials needed)
    // 4. Create Deployment (not StatefulSet - stateless)
    // 5. Create Service
    // 6. Return response
}

func GetCacheStatus(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) (*CacheResponse, error) {
    // Query Deployment.Status.ReadyReplicas
}

func DeleteCache(ctx context.Context, k8sClient *vortexkube.K8sClient, projectID string, resourceID string) error {
    // Delete Deployment, Service
}
```

#### 2. Add Routes in main.go

```go
// POST /v1/projects/:project_id/resources/caches
router.POST("/v1/projects/:project_id/resources/caches", func(c *gin.Context) {
    projectID := c.Param("project_id")
    var req models.CacheRequest
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": err.Error()})
        return
    }
    
    resp, err := handlers.ProvisionCache(context.Background(), k8sClient, projectID, req)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, resp)
})

// Similar for GET and DELETE
```

#### 3. Choose K8s Resource Type

- **Stateful (needs persistent storage):** StatefulSet (like PostgreSQL)
- **Stateless (ephemeral data):** Deployment (like Redis cache)
- **Compute/Batch:** Job or CronJob

### Scaling the API

#### Horizontal Scaling

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: infrastructure-api
spec:
  replicas: 3  # Multiple instances
  selector:
    matchLabels:
      app: infrastructure-api
  template:
    metadata:
      labels:
        app: infrastructure-api
    spec:
      serviceAccountName: infrastructure-api
      containers:
      - name: api
        image: vortex/infrastructure-api:latest
        ports:
        - containerPort: 8080
```

**Why this works:**
- API is stateless (all state in K8s)
- Multiple replicas can handle requests independently
- Client requests can go to any replica
- Each replica connects to same K8s cluster

#### Load Balancing

```yaml
apiVersion: v1
kind: Service
metadata:
  name: infrastructure-api
spec:
  type: LoadBalancer  # Or NodePort for local development
  selector:
    app: infrastructure-api
  ports:
  - port: 80
    targetPort: 8080
```

### High Availability

1. **Replicate API:** Deploy 3+ instances across nodes
2. **Replicate Databases:** StatefulSet replicas (currently 1, can increase)
3. **PersistentVolume Replication:** Use replicated storage backend (Ceph, etc.)
4. **Monitoring:** Prometheus + Grafana for visibility

---

## Testing Strategy

### Unit Tests (Future)

```go
// Test credential generation
func TestGenerateSecurePassword(t *testing.T) {
    pwd1 := GenerateSecurePassword()
    pwd2 := GenerateSecurePassword()
    
    // Should be different
    assert.NotEqual(t, pwd1, pwd2)
    // Should be 32 hex characters (16 bytes)
    assert.Len(t, pwd1, 32)
}
```

### Integration Tests (Future)

```go
// Test full provisioning flow
func TestProvisionDatabase(t *testing.T) {
    // 1. Create test namespace
    // 2. Call ProvisionDatabase
    // 3. Assert Secret created
    // 4. Assert StatefulSet created
    // 5. Assert Service created
    // 6. Poll status until "running"
    // 7. Assert database connectable
    // 8. Cleanup
}
```

### End-to-End Tests (In Progress)

```bash
# 1. Start Kind cluster
kind create cluster --config kind-config.yaml

# 2. Build API
go build -o infrastructure-api

# 3. Deploy to cluster
kubectl apply -f deployment.yaml

# 4. Port-forward to API
kubectl port-forward svc/infrastructure-api 8080:8080 &

# 5. Run tests
curl -X POST http://localhost:8080/v1/projects/test/resources/databases \
     -H "Content-Type: application/json" \
     -d '{"name":"test-db","engine":"postgres","version":"16","config":{"storage_gb":10}}'

# Verify database created
kubectl get statefulsets -n vortex-project-test
```

---

## Glossary

| Term | Definition |
|------|-----------|
| **StatefulSet** | K8s controller for stateful workloads (persistent identity, storage) |
| **Deployment** | K8s controller for stateless workloads (replicas, rolling updates) |
| **Service** | K8s networking abstraction (DNS, load balancing, port exposure) |
| **Namespace** | K8s virtual cluster (isolation, RBAC boundary) |
| **Secret** | K8s object storing sensitive data (passwords, tokens, certs) |
| **PersistentVolumeClaim** | K8s storage request (allocation of PersistentVolume) |
| **Headless Service** | Service without load balancing (direct pod IP access) |
| **client-go** | Official Go client for Kubernetes API |
| **Idempotent** | Operation that produces same result if run multiple times |
| **Cascading Delete** | When deleting parent, children are deleted automatically |

---

## References

- [Kubernetes Stateful Sets Documentation](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/)
- [client-go Library](https://github.com/kubernetes/client-go)
- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
- [Kind Documentation](https://kind.sigs.k8s.io/)
- [Gin Web Framework](https://gin-gonic.com/)

---

**Document Owner:** Vortex Development Team  
**Last Reviewed:** April 21, 2026  
**Status:** Active

