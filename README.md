# Vortex - AWS-Like Cloud Platform on Kubernetes

A cloud infrastructure management platform built on Kubernetes. Provision, manage, and scale cloud resources via REST APIs — inspired by AWS, running on your own infrastructure.

---

## About

**Vortex** abstracts Kubernetes complexity behind a simple REST API. Users describe what they want (a Postgres database, a Redis cache, a compute instance), and Vortex provisions it automatically inside a Kubernetes cluster — no YAML required.

**Core Philosophy:**
- Users see a cloud platform, not Kubernetes
- Infrastructure is provisioned dynamically on demand
- Each project is isolated in its own K8s namespace
- Credentials auto-generated and securely stored per resource

---

## Services

| Service | Port | Purpose |
|---|---|---|
| `auth-service` | 8081 | Auth, org/project management, API keys |
| `infrastructure-api` | 8080 | Resource provisioning (databases, caches, compute) |

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  Client (REST API calls)                                     │
└──────────┬───────────────────────────────────────────────────┘
           │
     ┌─────┴──────┐
     │            │
     ▼            ▼
┌─────────────┐  ┌─────────────────────────┐
│ Auth Service│  │  Infrastructure API     │
│ :8081       │  │  :8080                  │
│             │  │                         │
│ - Register  │  │ - Databases (CRUD+list) │
│ - Login     │  │ - Caches    (CRUD+list) │
│ - API Keys  │  │ - Compute   (CRUD+list) │
│ - Orgs      │  │                         │
│ - Projects  │  │  Validates project_id   │
└──────┬──────┘  └───────────┬─────────────┘
       │                     │
       │ PostgreSQL           │ client-go (K8s API)
       ▼                     ▼
┌─────────────┐  ┌─────────────────────────────────────────────┐
│  Platform   │  │  Kubernetes Cluster (Kind v1.35.0)          │
│  Database   │  │                                             │
│  (users,    │  │  vortex-project-{projectID}/                │
│  orgs,      │  │  ├── db-{id}-secret                         │
│  projects,  │  │  ├── db-{id}  (StatefulSet)                 │
│  api_keys)  │  │  ├── db-{id}  (LoadBalancer Service)        │
└─────────────┘  │  ├── cache-{id} (Deployment)               │
                 │  └── comp-{id}  (Deployment)                │
                 └─────────────────────────────────────────────┘
```

---

## Project Structure

```
Vortex/
├── k8s/
│   ├── api-deployment.yaml        # Infrastructure API + RBAC
│   ├── postgres/                  # Platform DB (auth-service state)
│   ├── redis/                     # Platform cache
│   ├── auth-service/              # Auth service deployment (planned)
│   └── api-gateway/               # API gateway (planned)
│
└── services/
    ├── auth-service/              # Authentication & identity service
    │   ├── main.go
    │   ├── Dockerfile
    │   └── internal/
    │       ├── config/            # Env config
    │       ├── db/                # PostgreSQL connection + migrations
    │       ├── handlers/          # auth.go, projects.go, apikeys.go
    │       ├── middleware/        # JWT + API key validation
    │       └── models/            # User, Org, Project, APIKey types
    │
    └── infrastructure-api/        # Resource provisioning service
        ├── main.go
        ├── Dockerfile
        └── internal/
            ├── vortexkube/        # K8s client
            ├── handlers/          # database.go, cache.go, compute.go
            └── models/            # Request/response types
```

---

## API Reference

### Auth Service (`POST :8081`)

**Public:**
```
POST /v1/auth/register     { email, password, org_name }  → JWT + user
POST /v1/auth/login        { email, password }            → JWT + user
```

**Protected** (`Authorization: Bearer <token>` or `Authorization: ApiKey vrtx_<key>`):
```
POST   /v1/orgs/:org_id/projects              → Create project
GET    /v1/orgs/:org_id/projects              → List projects
GET    /v1/orgs/:org_id/projects/:project_id  → Get project
DELETE /v1/orgs/:org_id/projects/:project_id  → Delete project

POST   /v1/users/api-keys                     → Create API key (returned once)
GET    /v1/users/api-keys                     → List API keys
DELETE /v1/users/api-keys/:key_id             → Revoke API key
```

### Infrastructure API (`POST :8080`)

```
POST   /v1/projects/:project_id/resources/databases             → Provision database
GET    /v1/projects/:project_id/resources/databases             → List databases
GET    /v1/projects/:project_id/resources/databases/:id         → Get status
DELETE /v1/projects/:project_id/resources/databases/:id         → Delete

POST   /v1/projects/:project_id/resources/caches                → Provision cache
GET    /v1/projects/:project_id/resources/caches                → List caches
GET    /v1/projects/:project_id/resources/caches/:id            → Get status
DELETE /v1/projects/:project_id/resources/caches/:id            → Delete

POST   /v1/projects/:project_id/resources/compute               → Provision compute
GET    /v1/projects/:project_id/resources/compute               → List compute
GET    /v1/projects/:project_id/resources/compute/:id           → Get status
DELETE /v1/projects/:project_id/resources/compute/:id           → Delete
```

---

## Usage Example

### 1. Register and get a token
```bash
curl -X POST http://localhost:8081/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"dev@acme.com","password":"secret123","org_name":"Acme Corp"}'

# Response:
# { "access_token": "eyJ...", "expires_in": 900, "user": {...} }
```

### 2. Create a project
```bash
curl -X POST http://localhost:8081/v1/orgs/<org_id>/projects \
  -H "Authorization: Bearer eyJ..." \
  -H "Content-Type: application/json" \
  -d '{"name":"production","region":"us-east-1"}'

# Response: { "id": "<project_id>", "name": "production", ... }
```

### 3. Provision a database (like RDS)
```bash
curl -X POST http://localhost:8080/v1/projects/<project_id>/resources/databases \
  -H "Content-Type: application/json" \
  -d '{
    "name": "prod-db",
    "engine": "postgres",
    "version": "16",
    "size": "db.medium",
    "config": { "storage_gb": 20 }
  }'

# Response:
# {
#   "id": "db-f8a2c5d1",
#   "engine": "postgres",
#   "status": "provisioning",
#   "endpoint": "pending",
#   "username": "vortex",
#   "password": "7f8a9c2d...",
#   "created_at": "2026-04-23T..."
# }
```

### 4. Poll until running
```bash
curl http://localhost:8080/v1/projects/<project_id>/resources/databases/db-f8a2c5d1

# Once the LoadBalancer IP is assigned:
# { "status": "running", "endpoint": "203.0.113.5:5432", ... }
```

---

## Database Instance Sizes

| Size | CPU Request | CPU Limit | Memory Request | Memory Limit |
|---|---|---|---|---|
| `db.small` | 100m | 500m | 256Mi | 512Mi |
| `db.medium` | 250m | 1000m | 512Mi | 1Gi |
| `db.large` | 500m | 2000m | 1Gi | 2Gi |

## Supported Engines

| Engine | Versions | Port |
|---|---|---|
| `postgres` | 14, 15, 16 (default) | 5432 |
| `mysql` | 8.0 (default), 8.4 | 3306 |

---

## Development Status

| Feature | Status |
|---|---|
| Kind cluster | Done |
| Infrastructure API (DB, Cache, Compute) | Done |
| Engine/version/size selection | Done |
| External LoadBalancer endpoints | Done |
| List endpoints for all resource types | Done |
| Auth service (JWT + API keys) | Done |
| Org & project management | Done |
| Platform PostgreSQL schema (auto-migrated) | Done |
| Auth middleware for infrastructure-api | Planned |
| Storage buckets (MinIO/S3) | Planned |
| Networking (VPC/subnet/firewall) | Planned |
| Serverless functions | Planned |
| Metrics & billing | Planned |

---

## Build

```bash
# Auth service
cd services/auth-service
go mod tidy && go build -o auth-service

# Infrastructure API
cd services/infrastructure-api
go mod tidy && go build -o infrastructure-api
```

## Environment Variables

| Variable | Service | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | auth | `postgres://vortex:vortex@localhost:5432/vortex` | Platform DB connection |
| `JWT_SECRET` | auth | `change-me-in-production` | JWT signing secret |
| `PORT` | auth | `8081` | HTTP port |

---

**Built as a cloud platform from scratch on Kubernetes.**
