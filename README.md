# 🌀 Vortex - AWS-Like Cloud Platform on Kubernetes

A cloud infrastructure management platform built on Kubernetes. Provision, manage, and scale cloud resources via REST APIs instead of manual configuration. Inspired by AWS architecture—think EC2, RDS, ElastiCache, but running on your infrastructure.

---

## 📖 About This Project

**Vortex** is a cloud-in-a-box platform that lets users request infrastructure (databases, caches, compute instances) through an API. Instead of manually writing Kubernetes YAML files, users describe what they want, and Vortex provisions it automatically using Kubernetes under the hood.

**Core Philosophy:**
- Users don't see Kubernetes complexity—they see a cloud platform
- Infrastructure is provisioned dynamically, not statically
- Multi-tenant support (each project gets isolated resources)
- Secure credential management per resource instance

---

## 🏗️ Architecture

### High-Level Flow

```
                    ┌─────────────────────┐
                    │   User/Client       │
                    │   (REST API calls)  │
                    └──────────┬──────────┘
                               │
                               │ HTTP Request
                    POST /v1/projects/{id}/resources/databases
                               │
                               ▼
                    ┌──────────────────────────────────────────┐
                    │  Infrastructure API Service (Go)         │
                    │                                          │
                    │  ├─ Endpoint: /health                   │
                    │  ├─ POST: /resources/databases          │
                    │  ├─ GET: /resources/{id}                │
                    │  └─ DELETE: /resources/{id}             │
                    │                                          │
                    │  Internal Modules:                       │
                    │  ├─ kubernetes/client.go (K8s API conn) │
                    │  ├─ handlers/database.go (provisioning) │
                    │  ├─ models/resources.go (data models)   │
                    │  └─ pkg/utils.go (crypto, UUIDs)        │
                    └──────────────┬───────────────────────────┘
                                   │
                      client-go library (K8s API calls)
                                   │
                                   ▼
                    ┌──────────────────────────────┐
                    │ Kubernetes Control Plane     │
                    │ (Kind Cluster v1.35.0)       │
                    └──────────────┬───────────────┘
                                   │
                ┌──────────────────┼──────────────────┐
                │                  │                  │
                ▼                  ▼                  ▼
        ┌─────────────────┐ ┌──────────────┐ ┌──────────────┐
        │ PostgreSQL      │ │   Redis      │ │   Services   │
        │ StatefulSet     │ │ Deployment   │ │ (Future)     │
        │ + PersistentVC  │ │ (stateless)  │ │              │
        │ (10GB storage)  │ │              │ │              │
        └─────────────────┘ └──────────────┘ └──────────────┘
                │
        ┌───────▼────────────────┐
        │ Persistent Data        │
        │ (survives pod restarts)│
        └────────────────────────┘
```

### Resource Provisioning Flow

When a user requests a database:

```
1. User sends:  POST /v1/projects/acme/resources/databases
                {
                  "name": "prod-db",
                  "engine": "postgres",
                  "config": { "storage_gb": 10 }
                }

2. API Service:
   - Generates unique ID (db-a1b2c3d4)
   - Generates secure password
   - Creates K8s Secret (db-a1b2c3d4-secret)
   - Creates K8s StatefulSet (PostgreSQL)
   - Creates K8s Service (DNS entry: db-a1b2c3d4:5432)

3. User receives:
   {
     "id": "db-a1b2c3d4",
     "status": "provisioning",
     "endpoint": "db-a1b2c3d4:5432",
     "username": "vortex",
     "password": "SECURE_RANDOM_PASS"
   }

4. Kubernetes:
   - Pod starts initializing
   - Liveness/Readiness probes monitor health
   - Status becomes "running" when ready

5. User connects to database at:
   Host: db-a1b2c3d4
   Port: 5432
   User: vortex
   Pass: (from step 3)
```

---

## 📂 Project Structure

```
Vortex/
├── README.md                        # This file
├── kind-config.yaml                 # Kubernetes cluster configuration
│
├── k8s/                             # Infrastructure deployment manifests
│   ├── secrets.yaml                 # Base credentials (PostgreSQL user/pass)
│   │
│   ├── postgres/
│   │   ├── statefulset.yaml         # PostgreSQL database server
│   │   ├── service.yaml             # Exposes postgres at postgres:5432
│   │   └── pvc.yaml                 # Persistent storage (10GB)
│   │
│   └── redis/
│       ├── deployment.yaml          # Redis cache server
│       └── service.yaml             # Exposes redis at redis:6379
│
└── services/
    └── infrastructure-api/          # Main provisioning service
        ├── main.go                  # Entry point, Gin router setup
        ├── go.mod                   # Go module definition
        │
        ├── internal/
        │   ├── kubernetes/
        │   │   └── client.go        # K8s API client (connects to cluster)
        │   │
        │   ├── handlers/
        │   │   └── database.go      # Request handler (provisions resources)
        │   │
        │   └── models/
        │       └── resources.go     # Data structures (requests/responses)
        │
        ├── pkg/
        │   └── utils.go             # Helper functions (UUID, passwords)
        │
        └── Dockerfile               # Container image definition
```

---

## ✅ What Has Been Accomplished

### 1. **Development Environment Setup**
- ✅ Kind Kubernetes cluster running locally (v1.35.0)
- ✅ Single control-plane node with port mappings (3000 for apps)
- ✅ Persistent volume provisioning enabled

### 2. **Infrastructure Layer**
- ✅ PostgreSQL 16 deployed as StatefulSet
  - 10GB persistent storage
  - Auto-recovery with liveness/readiness probes
  - Secured with K8s Secrets
  - Accessible at `postgres:5432` inside cluster

- ✅ Redis 7 deployed as Deployment
  - Stateless in-memory cache
  - Health checks configured
  - Accessible at `redis:6379` inside cluster

### 3. **Infrastructure API Service (Go)**
- ✅ Go project structure with proper package organization
- ✅ Gin REST framework integration
- ✅ `/health` endpoint (for monitoring)
- ✅ K8s client connection (`client-go` library)

### 4. **Core Provisioning Logic**
- ✅ Request/Response models defined
- ✅ Password generation (cryptographically secure)
- ✅ UUID generation for unique resource IDs
- ✅ Database provisioning handler structure
  - Creates K8s Secrets per instance
  - Generates StatefulSet with PostgreSQL
  - Creates Service for DNS access
  - Returns connection details to user

### 5. **Code Quality**
- ✅ Proper error handling patterns
- ✅ K8s object building with type safety
- ✅ Resource isolation per project
- ✅ Secure credential handling (not hardcoded)

---

## 🚀 How It Works

### Example: Provision a Database

**Step 1: User Makes Request**
```bash
curl -X POST http://localhost:8080/v1/projects/acme-corp/resources/databases \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-db",
    "engine": "postgres",
    "version": "16",
    "config": { "storage_gb": 20 }
  }'
```

**Step 2: API Response**
```json
{
  "id": "db-f8a2c5d1",
  "name": "production-db",
  "status": "provisioning",
  "endpoint": "db-f8a2c5d1:5432",
  "username": "vortex",
  "password": "7f8a9c2d5e1b3a4c",
  "created_at": "2026-04-18T14:30:00Z"
}
```

**Step 3: Behind the Scenes**
1. Infrastructure API receives request
2. Generates ID: `db-f8a2c5d1`
3. Generates password: random 16-char string
4. Creates K8s Secret `db-f8a2c5d1-secret` with credentials
5. Creates StatefulSet for PostgreSQL
6. Creates Service for DNS resolution
7. Kubernetes scheduler places pod on node
8. PostgreSQL starts initializing
9. Readiness probe checks if accepting connections
10. Status changes from "provisioning" → "running"

**Step 4: User Connects**
```bash
psql -h db-f8a2c5d1 -U vortex -d vortex_db
# Password: 7f8a9c2d5e1b3a4c
```

---

## 🔄 Current Development Status

| Feature | Status | Notes |
|---------|--------|-------|
| Kind cluster | ✅ Done | v1.35.0 running on Windows via Docker |
| PostgreSQL | ✅ Done | Stateful, persistent storage (10GB) |
| Redis | ✅ Done | Stateless, in-memory cache |
| API framework | ✅ Done | Gin + Go with proper package structure |
| K8s client | ✅ Done | client-go v0.35.4 integration |
| Database provisioning | ✅ Done | Full endpoint (POST/GET/DELETE) |
| Resource isolation | ✅ Done | Project-based namespaces |
| Security | ✅ Done | Per-instance K8s Secrets |
| Error handling | ✅ Done | Proper Go patterns with type safety |
| Multi-tenancy | ✅ Done | vortex-project-{projectID} isolation |
| Status tracking | ✅ Done | StatefulSet readiness monitoring |
| Resource cleanup | ✅ Done | Cascading delete with foreground propagation |
| Docker image | ✅ Done | Multi-stage build, non-root user |
| **K8s Deployment** | 🚧 Next | Deploy API service to cluster |
| **Integration tests** | 🚧 Next | End-to-end workflow testing |
| **Redis provisioning** | 📋 Planned | CACHE resource handler |
| **Compute provisioning** | 📋 Planned | COMPUTE resource handler |

---

## 🎯 Next Steps

1. **Deploy Infrastructure API to Cluster**
   - Build Docker image
   - Deploy as K8s Deployment
   - Test via cluster DNS

2. **Complete CACHE_INSTANCE Provisioning**
   - Similar handler pattern as database
   - Redis Deployment creation
   - Return connection string

3. **Add Monitoring**
   - Health check endpoints
   - Status tracking
   - Event logging

4. **Build CLI**
   - `vortex create database --name prod-db`
   - `vortex list resources`
   - `vortex delete resource --id db-xxx`

---

## 📚 Key Technologies

| Layer | Technology | Why |
|-------|-----------|-----|
| Orchestration | Kubernetes (Kind) | Industry standard, scalable |
| API Language | Go | Fast, concurrent, minimal overhead |
| API Framework | Gin | Lightweight, high performance |
| Database | PostgreSQL | Production-grade, ACID-compliant |
| Cache | Redis | Fast in-memory operations |
| K8s Interaction | client-go | Official Kubernetes Go client |

---

## 📝 How to Use This Codebase

### Build the Infrastructure API
```bash
cd services/infrastructure-api
go mod tidy
go build -o infrastructure-api
```

### Check K8s Cluster Status
```bash
kubectl get nodes
kubectl get pods -n vortex
kubectl get svc -n vortex
```

### View PostgreSQL Logs
```bash
kubectl logs -f statefulset/postgres -n vortex
```

### Connect to PostgreSQL
```bash
kubectl port-forward -n vortex svc/postgres 5432:5432
psql -h localhost -U vortex -d vortex_db
```

---

**Built with ❤️ as a cloud platform from scratch.**