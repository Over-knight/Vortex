# Postman Collection Guide - Vortex Infrastructure API

## Overview

This Postman collection provides a complete testing interface for the Vortex Infrastructure API. It includes:

- **Health checks** to verify API connectivity
- **Database provisioning** endpoints with real-world examples
- **Status polling** workflows with different states
- **Multi-tenancy test scenarios** to validate namespace isolation
- **Pre-configured environment variables** for easy switching between projects

## Import Instructions

### Method 1: Direct Import (Recommended)

1. Open **Postman**
2. Click **Import** (top-left corner)
3. Select **Upload Files**
4. Navigate to and select `POSTMAN_COLLECTION.json`
5. Click **Import**

### Method 2: Raw JSON Import

1. Open **Postman**
2. Click **Import**
3. Select **Paste Raw Text**
4. Copy the entire contents of `POSTMAN_COLLECTION.json`
5. Paste into the text box
6. Click **Import**

## Environment Setup

### Default Variables

The collection comes pre-configured with these variables:

| Variable | Default Value | Purpose |
|----------|---------------|---------|
| `base_url` | `http://localhost:8080` | API endpoint (change for cloud deployment) |
| `project_id` | `acme-corp` | Current project ID (override per request) |
| `database_id` | `db-550e8400-...` | Database ID returned from provisioning |

### Switching Between Environments

To test different scenarios:

1. Click the **Environment** dropdown (top-right)
2. Select or create a new environment
3. Modify variables as needed:
   - `base_url`: For in-cluster access, use `http://infrastructure-api:8080`
   - `project_id`: Change to test multi-tenancy

## Endpoint Reference

### Health & Status

#### `GET /health`
Verifies the API is running and connected to Kubernetes.

**Expected Response (200 OK):**
```json
{
  "status": "ok"
}
```

---

### Database Resources

#### `POST /v1/projects/:project_id/resources/databases`
Provisions a new PostgreSQL database.

**Request Body:**
```json
{
  "name": "production_db",
  "engine": "postgres",
  "version": "16",
  "config": {
    "storage_gb": 10,
    "replicas": 1
  }
}
```

**What Happens:**
1. Creates namespace `vortex-project-{project_id}` if it doesn't exist
2. Generates unique database ID and secure password
3. Creates Kubernetes Secret with credentials
4. Creates StatefulSet for PostgreSQL
5. Creates Service for DNS resolution

**Expected Response (201 Created):**
```json
{
  "id": "db-550e8400-e29b-41d4-a716-446655440000",
  "name": "production_db",
  "status": "provisioning",
  "endpoint": "db-550e8400-e29b-41d4-a716-446655440000:5432",
  "username": "vortex",
  "password": "kL9mP2qXvW8nD4jF6sH1",
  "created_at": "2026-04-21T14:30:00Z"
}
```

**Next Step:** Copy the `id` field and use it for status polling.

---

#### `GET /v1/projects/:project_id/resources/databases/:database_id`
Retrieves the current status of a database.

**Status Values:**
- `provisioning` - Pod is initializing (30-60 seconds typical)
- `running` - Database ready for connections
- `failed` - An error occurred

**Expected Response (200 OK):**
```json
{
  "id": "db-550e8400-e29b-41d4-a716-446655440000",
  "name": "production_db",
  "status": "running",
  "endpoint": "db-550e8400-e29b-41d4-a716-446655440000:5432",
  "username": "vortex",
  "password": "kL9mP2qXvW8nD4jF6sH1",
  "created_at": "2026-04-21T14:30:00Z"
}
```

**Usage Pattern:**
```
Poll every 5-10 seconds until status changes to "running"
```

---

#### `DELETE /v1/projects/:project_id/resources/databases/:database_id`
Deletes a database and all associated resources.

**What Happens:**
1. Deletes StatefulSet with foreground propagation
2. Deletes Service
3. Deletes Secret with credentials

**Note:** The namespace persists to allow future resources.

**Expected Response (204 No Content):**
Empty body, HTTP 204 status only.

---

## Testing Workflows

### Workflow 1: Basic Provisioning (5 minutes)

1. **POST** Health Check
   - Verify API is accessible
   
2. **POST** Provision Database
   - Creates database for `acme-corp`
   - Save the returned `id`
   
3. **GET** Check Status (repeat every 5 seconds)
   - Wait until status is `running`
   - Note the `endpoint` and credentials
   
4. **DELETE** Remove Database
   - Cleans up all resources
   - Verify 204 response

**Expected Time:** 60-90 seconds for PostgreSQL to initialize

---

### Workflow 2: Multi-Tenancy Validation (10 minutes)

This workflow proves that Project A and Project B are completely isolated:

1. **POST** Create Database - Project A
2. **POST** Create Database - Project B
3. **GET** Poll Status - Project A (until running)
4. **GET** Poll Status - Project B (until running)
5. **DELETE** Database - Project A
6. **GET** Verify Project B Still Exists
7. **DELETE** Database - Project B

**Expected Outcome:**
- Project A's namespace: `vortex-project-project-a`
- Project B's namespace: `vortex-project-project-b`
- Deleting Project A's database does NOT affect Project B
- Both databases can run simultaneously without conflict

---

### Workflow 3: Error Handling (5 minutes)

Test error scenarios:

**Invalid Request:**
```
POST /v1/projects/acme-corp/resources/databases
Body: {} (missing required fields)
Expected: 400 Bad Request
```

**Non-existent Resource:**
```
GET /v1/projects/acme-corp/resources/databases/db-invalid-id-12345
Expected: 500 Internal Server Error (resource not found)
```

**Double Delete:**
```
DELETE /v1/projects/acme-corp/resources/databases/db-id (already deleted)
Expected: 500 Internal Server Error
```

---

## Common Issues & Solutions

### Issue: `Connection refused`
**Cause:** API is not running  
**Solution:** Start the API
```bash
cd services/infrastructure-api
go build -o infrastructure-api
./infrastructure-api
```

### Issue: `Kind cluster not accessible`
**Cause:** Kind cluster is down or kubeconfig not set  
**Solution:** Check Kind cluster status
```bash
kind get clusters
docker ps | grep kind
```

### Issue: Database stuck in "provisioning" state
**Cause:** PostgreSQL pod is still starting  
**Solution:** Wait longer (PostgreSQL typically takes 30-60 seconds) or check pod logs
```bash
kubectl logs -n vortex-project-acme-corp db-<id> --tail=20
```

### Issue: 404 Not Found
**Cause:** Incorrect URL or variable substitution not working  
**Solution:** Verify:
1. Variables are set correctly (top-right dropdown)
2. URL path matches exactly
3. `project_id` and `database_id` are actual values, not placeholders

---

## Advanced Testing

### Running via Postman CLI (Newman)

Install Newman:
```bash
npm install -g newman
```

Run the collection:
```bash
newman run POSTMAN_COLLECTION.json --environment your-env.json --delay-request 500
```

This is useful for CI/CD automation.

---

## Next Steps

Once this collection is working:

1. **Deploy to Kind cluster** - Create Kubernetes Deployment manifest
2. **Add Redis caching** - Implement cache provisioning handler
3. **Add observability** - Prometheus metrics and logging
4. **Implement authentication** - JWT/OAuth2 for production

---

## Collection Statistics

- **Endpoints:** 4
- **Sample Requests:** 7
- **Test Workflows:** 3
- **Estimated Test Duration:** 20-30 minutes (all workflows)

---

## Support

For issues or questions:
1. Check the error response body for details
2. Review Kubernetes pod logs: `kubectl logs -n vortex-project-<id> <pod-name>`
3. Verify API startup logs for configuration issues
