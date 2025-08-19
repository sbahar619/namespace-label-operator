# NamespaceLabel Operator

Kubernetes operator for managing namespace labels with protection patterns.

## 🚀 Quick Start

### Install & Deploy

**Option 1: One-click install (recommended for end users)**
```bash
# Install from GitHub releases
kubectl apply -f https://github.com/dana-team/namespacelabel/releases/latest/download/install.yaml

# Or install from local build
kubectl apply -f dist/install.yaml
```

**Option 2: Development deployment**
```bash
# Deploy the operator with custom image
make full-deploy IMG=my-registry/namespacelabel:latest

# Or install CRDs only
make install
```

### Create a NamespaceLabel
```bash
kubectl apply -f - <<EOF
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: my-app
spec:
  labels:
    environment: production
    team: backend
    tier: critical
EOF
```

## 🛡️ Label Protection

Protect important labels from being overwritten:

```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: my-app
spec:
  labels:
    environment: production
    kubernetes.io/managed-by: my-operator  # Will be blocked
  
  # Protection patterns (glob patterns)
  protectedLabelPatterns:
    - "kubernetes.io/*"
    - "*.k8s.io/*"
    - "istio.io/*"
  
  # Protection behavior: skip (silent) | warn (log) | fail (error)
  protectionMode: warn
```

**Protection Modes:**
- `skip` - Silently skip protected labels ✅ (default)
- `warn` - Skip protected labels + log warnings ⚠️
- `fail` - Fail entire reconciliation ❌

## 🔧 Development

### Build & Test
```bash
# Build locally
make build

# Run unit tests
make test

# Run E2E tests (requires cluster)
make test-e2e

# Run tests sequentially (for debugging)
make test-e2e-debug

# Lint code
make lint
```

### Local Development
```bash
# Generate manifests after code changes
make manifests

# Run controller locally (requires cluster access)
make run

# Format and vet code
make fmt vet
```

### Container Images
```bash
# Build container image
make docker-build IMG=my-registry/namespacelabel:v1.0.0

# Push to registry
make docker-push IMG=my-registry/namespacelabel:v1.0.0

# Build installer manifest
make build-installer
```

## 🚢 Deployment

### Complete Deployment
```bash
# Full deployment workflow (build + push + deploy + wait)
make full-deploy IMG=my-registry/namespacelabel:v1.0.0
```

### Step-by-Step Deployment
```bash
# 1. Install CRDs
make install

# 2. Deploy controller
make deploy IMG=my-registry/namespacelabel:v1.0.0

# 3. Check status
make deploy-status

# 4. View logs
make deploy-logs
```

### Monitoring
```bash
# Follow logs in real-time
make deploy-logs-follow

# Check deployment status
make deploy-status

# Wait for controller to be ready
make wait-ready
```

### Cleanup
```bash
# Remove everything
make cleanup

# Or step by step
make undeploy    # Remove controller
make uninstall   # Remove CRDs
```

## 📋 API Reference

### NamespaceLabel Spec

| Field | Type | Description |
|-------|------|-------------|
| `labels` | `map[string]string` | Labels to apply to namespace |
| `protectedLabelPatterns` | `[]string` | Glob patterns for protected labels |
| `protectionMode` | `string` | Protection behavior: `skip`/`warn`/`fail` |

### Examples

**Basic Usage:**
```yaml
spec:
  labels:
    app: web-app
    environment: production
```

**With Protection:**
```yaml
spec:
  labels:
    app: web-app
    environment: production
  protectedLabelPatterns:
    - "kubernetes.io/*"
    - "istio.io/*"
  protectionMode: warn
```

## 🔐 RBAC

The operator creates these ClusterRoles:

- `namespacelabel-editor-role` - For users to manage NamespaceLabel CRs
- `namespacelabel-viewer-role` - Read-only access to NamespaceLabel CRs

**Grant access to users:**
```bash
kubectl create clusterrolebinding alice-namespacelabel-editor \
  --clusterrole=namespacelabel-editor-role \
  --user=alice@company.com
```

## 🆘 Troubleshooting

**Common Issues:**

1. **Labels not applied** - Check controller logs: `make deploy-logs`
2. **Protection conflicts** - Review `protectedLabelPatterns` and `protectionMode`
3. **Permission denied** - Ensure user has `namespacelabel-editor-role`
4. **Controller not ready** - Check deployment: `make deploy-status`

**Debug Commands:**
```bash
# Check controller status
kubectl get deployment -n namespacelabel-system

# View NamespaceLabel status  
kubectl get namespacelabel labels -n my-app -o yaml

# Check namespace labels
kubectl get namespace my-app --show-labels
```

## 📄 License

Apache License 2.0

