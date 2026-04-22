# Vortex Architecture & Implementation Guide

**Version:** 2.0
**Last Updated:** April 23, 2026
**Status:** Active Development

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Services](#services)
3. [Data Models](#data-models)
4. [API Design](#api-design)
5. [Auth & Identity](#auth--identity)
6. [Resource Provisioning](#resource-provisioning)
7. [Multi-Tenancy](#multi-tenancy)
8. [Security Model](#security-model)
9. [Resource Lifecycle](#resource-lifecycle)
10. [Remaining Work](#remaining-work)

---

## System Overview

Vortex is a **cloud-in-a-box platform** that abstracts Kubernetes complexity behind a simple REST API. Users request resources (databases, caches, compute) through HTTP endpoints, and Vortex provisions them as Kubernetes workloads — no YAML required.

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│  Clients (CLI, SDK, web UI)                                     │
└──────────┬──────────────────────────────┬────────────────────────┘
           │                              │
           ▼                              ▼
┌─────────────────────┐      ┌────────────────────────────────────┐
│  Auth Service       │      │  Infrastructure API                │
│  Go + Gin  :8081   │      │  Go + Gin  :8080                   │
│                     │      │                                    │
│  /v1/auth/*         │      │  /v1/projects/:id/resources/       │
│  /v1/orgs/*         │      │    databases  (CRUD + list)        │
│  /v1/users/*        │      │    caches     (CRUD + list)        │
│                     │      │    compute    (CRUD + list)        │
│  JWT issuance       │      │                                    │
│  API key mgmt       │      │  K8s provisioning via client-go    │
└──────────┬──────────┘      └──────────────────┬─────────────────┘
           │                                    │
           ▼                                    ▼
┌─────────────────────┐      ┌────────────────────────────────────┐
│  Platform PostgreSQL│      │  Kubernetes Cluster (Kind v1.35)   │
│                     │      │                                    │
│  organizations      │      │  vortex-project-{projectID}/       │
│  users              │      │  ├── db-{id}-secret                │
│  projects           │      │  ├── db-{id}  StatefulSet          │
│  api_keys           │      │  ├── db-{id}  LoadBalancer Svc     │
│                     │      │  ├── cache-{id} Deployment         │
└─────────────────────┘      │  └── comp-{id}  Deployment         │
                             └────────────────────────────────────┘
```

---

## Services

### Auth Service (`services/auth-service/`)

**Language:** Go 1.23 / Gin / pgx v5
**Port:** 8081
**Database:** PostgreSQL (platform state)

Handles all identity concerns: user registration, JWT issuance, API key lifecycle, and org/project management. Runs independently of the infrastructure API so auth can be scaled or swapped without touching provisioning logic.

```
internal/
├── config/config.go        — env var loading
├── db/postgres.go          — connection + idempotent schema migration
├── middleware/auth.go      — validates Bearer JWT or ApiKey header
├── handlers/
│   ├── auth.go             — Register, Login
│   ├── projects.go         — Project CRUD
│   └── apikeys.go          — API key Create/List/Delete
└── models/models.go        — all request/response/DB types
```

**Schema (auto-migrated on startup):**
```sql
organizations  (id, name, slug, plan, created_at)
users          (id, org_id, email, password_hash, status, created_at)
projects       (id, org_id, name, region, status, created_at)
api_keys       (id, user_id, name, key_hash, key_prefix, scope, created_at, expires_at)
```

### Infrastructure API (`services/infrastructure-api/`)

**Language:** Go 1.23 / Gin / client-go v0.35.4
**Port:** 8080
**State store:** Kubernetes (no separate DB — K8s objects are source of truth)

Provisions and manages cloud resources as Kubernetes workloads. Stateless — can be scaled horizontally since all state lives in K8s.

```
internal/
├── vortexkube/client.go    — K8s client (in-cluster + kubeconfig fallback)
├── handlers/
│   ├── database.go         — PostgreSQL/MySQL provisioning
│   ├── cache.go            — Redis provisioning
│   └── compute.go          — Generic workload provisioning
└── models/resources.go     — request/response types
```

---

## Data Models

### Auth Service Models

```go
type Organization struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Slug      string    `json:"slug"`
    Plan      string    `json:"plan"`
    CreatedAt time.Time `json:"created_at"`
}

type User struct {
    ID        string    `json:"id"`
    OrgID     string    `json:"org_id"`
    Email     string    `json:"email"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
}

type Project struct {
    ID        string    `json:"id"`
    OrgID     string    `json:"org_id"`
    Name      string    `json:"name"`
    Region    string    `json:"region"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
}

type APIKey struct {
    ID        string     `json:"id"`
    UserID    string     `json:"user_id"`
    Name      string     `json:"name"`
    KeyPrefix string     `json:"key_prefix"`  // first 12 chars, shown in list
    Scope     string     `json:"scope"`
    CreatedAt time.Time  `json:"created_at"`
    ExpiresAt *time.Time `json:"expires_at,omitempty"`
}
```

### Infrastructure API Models

```go
type DatabaseRequest struct {
    Name    string   `json:"name"`
    Engine  string   `json:"engine"`  // "postgres" | "mysql"
    Version string   `json:"version"` // "16", "15", "8.0"
    Size    string   `json:"size"`    // "db.small" | "db.medium" | "db.large"
    Config  DBConfig `json:"config"`
}

type DBConfig struct {
    StorageGB int `json:"storage_gb"` // default: 10
}

type DatabaseResponse struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Engine    string    `json:"engine"`
    Status    string    `json:"status"`   // "provisioning" | "running"
    Endpoint  string    `json:"endpoint"` // "host:port" or "pending"
    Username  string    `json:"username"`
    Password  string    `json:"password"`
    CreatedAt time.Time `json:"created_at"`
}
```

### Instance Size Classes

| Size | CPU Request | CPU Limit | Memory Request | Memory Limit |
|---|---|---|---|---|
| `db.small` | 100m | 500m | 256Mi | 512Mi |
| `db.medium` | 250m | 1000m | 512Mi | 1Gi |
| `db.large` | 500m | 2000m | 1Gi | 2Gi |

### Supported Engines

| Engine | Default Version | Port | Container Image |
|---|---|---|---|
| `postgres` | 16 | 5432 | `postgres:{version}-alpine` |
| `mysql` | 8.0 | 3306 | `mysql:{version}` |

---

## API Design

### Auth Service Endpoints

```
# Public
POST /v1/auth/register   { email, password, org_name }
POST /v1/auth/login      { email, password }

# Protected (Bearer JWT or ApiKey header)
POST   /v1/orgs/:org_id/projects
GET    /v1/orgs/:org_id/projects
GET    /v1/orgs/:org_id/projects/:project_id
DELETE /v1/orgs/:org_id/projects/:project_id

POST   /v1/users/api-keys
GET    /v1/users/api-keys
DELETE /v1/users/api-keys/:key_id
```

### Infrastructure API Endpoints

```
GET    /health

POST   /v1/projects/:project_id/resources/databases
GET    /v1/projects/:project_id/resources/databases
GET    /v1/projects/:project_id/resources/databases/:resource_id
DELETE /v1/projects/:project_id/resources/databases/:resource_id

POST   /v1/projects/:project_id/resources/caches
GET    /v1/projects/:project_id/resources/caches
GET    /v1/projects/:project_id/resources/caches/:resource_id
DELETE /v1/projects/:project_id/resources/caches/:resource_id

POST   /v1/projects/:project_id/resources/compute
GET    /v1/projects/:project_id/resources/compute
GET    /v1/projects/:project_id/resources/compute/:resource_id
DELETE /v1/projects/:project_id/resources/compute/:resource_id
```

---

## Auth & Identity

### JWT Flow

```
Register/Login → bcrypt password check → HS256 JWT (15 min TTL)
                                       → { access_token, expires_in, user }
```

JWT claims:
```go
type Claims struct {
    UserID string
    OrgID  string
    Email  string
    jwt.RegisteredClaims  // ExpiresAt, IssuedAt
}
```

### API Key Flow

```
POST /v1/users/api-keys
  → generate: vrtx_<32 hex chars>
  → store:    SHA-256(key) in api_keys.key_hash
  → return:   { key: "vrtx_...", key_prefix: "vrtx_abc1234" }
              (raw key shown exactly once — same pattern as AWS)

Subsequent requests: Authorization: ApiKey vrtx_...
  → middleware SHA-256s the key, queries DB, resolves user+org
```

### Authorization on Protected Routes

Every protected route validates the caller's `org_id` (from the token) against the URL's `:org_id`. Users can only access their own org's resources.

```go
if orgID != c.GetString("org_id") {
    c.JSON(403, gin.H{"error": "forbidden"})
    return
}
```

---

## Resource Provisioning

### Database Provisioning Flow

```
POST /v1/projects/{project_id}/resources/databases
  {engine, version, size, config.storage_gb}

1. EnsureNamespace("vortex-project-{projectID}")
2. resolveEngine(engine, version) → image, port, env vars
3. resolveSizeResources(size)    → CPU/memory requests+limits
4. storageGB = config.storage_gb || 10
5. Create K8s Secret  (db-{id}-secret)
6. Create StatefulSet (engine container, PVC)
7. Create LoadBalancer Service
8. Return { status: "provisioning", endpoint: "pending" }

GET /v1/projects/{project_id}/resources/databases/{id}
  → Check StatefulSet.Status.ReadyReplicas
  → Check Service.Status.LoadBalancer.Ingress for external IP
  → Return { status: "running", endpoint: "203.0.113.5:5432" }
```

### Kubernetes Objects Created Per Database

| Object | Name | Purpose |
|---|---|---|
| Secret | `db-{id}-secret` | Stores username + password |
| StatefulSet | `db-{id}` | Runs the database container |
| PersistentVolumeClaim | `db-storage-db-{id}-0` | Disk storage |
| Service (LoadBalancer) | `db-{id}` | External IP + port |

### Labels Stored on StatefulSet

```
app:              db-{id}
project:          {projectID}
type:             database
vortex.io/name:   {user-provided name}
vortex.io/engine: postgres | mysql
```

These labels are the only persistent metadata — `GetDatabaseStatus` and `ListDatabases` read from them since K8s is the sole state store.

### Service Type: LoadBalancer

Databases use `LoadBalancer` services (not headless) so they get an external IP from the cloud provider. The endpoint is `"pending"` until the cloud provider assigns one, then resolves to `host:port`. This mirrors RDS endpoint behavior.

---

## Multi-Tenancy

Each project gets its own Kubernetes namespace: `vortex-project-{projectID}`

```
vortex-project-acme-prod/
├── db-a1b2c3d4-secret
├── db-a1b2c3d4  (StatefulSet + PVC + Service)
└── cache-e5f6g7h8  (Deployment + Service)

vortex-project-startup-xyz/
├── db-i9j0k1l2-secret
└── db-i9j0k1l2
```

**Benefits:**
- RBAC boundary — can grant teams namespace-scoped kubectl access
- Resource quotas per project (future)
- Network policies for cross-tenant isolation (future)
- Clean teardown: deleting a namespace removes all project resources

---

## Security Model

### Credential Management

- Passwords generated with `crypto/rand` (16 bytes → 32 hex chars)
- Stored in K8s Secrets (encrypted at rest in production etcd)
- Injected into containers via `secretKeyRef` env vars — never in image or logs
- Returned to user once on creation; never retrievable again via list endpoints

### API Key Security

- Raw key (`vrtx_<32 hex>`) never stored — only `SHA-256(key)` in DB
- `key_prefix` (first 12 chars) stored for display — lets users identify keys without exposing the secret
- Optional `expires_at` for time-bounded keys
- Constant-time comparison via SHA-256 lookup (no timing oracle)

### Service Account Permissions (Infrastructure API)

```yaml
rules:
- apiGroups: [""]
  resources: [namespaces, secrets, services, pods]
  verbs: [get, list, create, delete]
- apiGroups: [apps]
  resources: [statefulsets, deployments]
  verbs: [get, list, create, delete]
```

---

## Resource Lifecycle

```
POST /databases
  └── K8s: Secret + StatefulSet + Service created
  └── Status: "provisioning"

[K8s schedules pod, pulls image, initializes database]

GET /databases/{id}
  └── ReadyReplicas > 0 → Status: "running"
  └── LB ingress assigned → Endpoint: "203.0.113.5:5432"

[User connects, operates database]

DELETE /databases/{id}
  └── StatefulSet deleted (foreground propagation — waits for pods)
  └── Service deleted
  └── Secret deleted
  └── PVC deleted by K8s cascade
```

---

## Remaining Work

### Tier 1 — Foundation (in progress)
- [x] Auth service (JWT, API keys, org/project management)
- [ ] Infrastructure API validates `project_id` against auth service DB
- [ ] Auth middleware on infrastructure API endpoints

### Tier 2 — AWS Parity
- [ ] Storage buckets (MinIO — S3-compatible)
- [ ] Networking (VPC/subnet/firewall rule management)
- [ ] Persistent volume management for compute
- [ ] Database backups / snapshots

### Tier 3 — Differentiation
- [ ] Serverless functions (OpenFaaS/Knative on K8s)
- [ ] Metrics collection (Prometheus scraping per-resource)
- [ ] API Gateway service (routing, rate limiting)

### Tier 4 — Monetization & Compliance
- [ ] Billing (usage records, invoices)
- [ ] Role-based access control (org-level RBAC)
- [ ] Audit log

---

## Glossary

| Term | Definition |
|---|---|
| StatefulSet | K8s controller for stateful workloads (stable identity, persistent storage) |
| Deployment | K8s controller for stateless workloads |
| LoadBalancer Service | K8s service that provisions an external IP from the cloud provider |
| Headless Service | Service without load balancing (direct pod DNS) |
| Namespace | K8s virtual cluster — isolation and RBAC boundary |
| Secret | K8s object for sensitive data (encrypted at rest) |
| PersistentVolumeClaim | K8s storage request |
| client-go | Official Go client for the Kubernetes API |

---

**Last Reviewed:** April 23, 2026
