# NamespaceLabel Operator

A Kubernetes operator that manages namespace labels through custom resources, providing a safe, auditable, and scalable way to apply labels to namespaces with built-in protection for critical management labels.

[![Go Report Card](https://goreportcard.com/badge/github.com/sbahar619/namespace-label-operator)](https://goreportcard.com/report/github.com/sbahar619/namespace-label-operator)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## 🎯 Overview

The NamespaceLabel operator allows you to manage namespace labels declaratively using Kubernetes custom resources. Instead of manually editing namespace manifests or using imperative kubectl commands, you can define desired labels in a `NamespaceLabel` custom resource, and the operator will ensure the namespace stays in sync.

### Key Features

- 🛡️ **Label Protection** - Protect critical management labels from accidental overwrites
- 🔒 **Multi-tenant Security** - NamespaceLabel CRs can only affect their own namespace
- 📋 **Singleton Pattern** - One `labels` CR per namespace for consistency
- 🔄 **Declarative Management** - GitOps-friendly label management
- 📊 **Status Reporting** - Clear status on applied vs skipped labels
- 🧹 **Automatic Cleanup** - Safe removal of operator-managed labels
- 🏷️ **Flexible Protection** - Configurable protection patterns and modes

## 🚀 Quick Start

### Prerequisites

- Kubernetes 1.21+
- kubectl configured to access your cluster
- Cluster admin permissions for installation

### Installation

1. **Install the operator:**
   ```bash
   kubectl apply -f https://raw.githubusercontent.com/sbahar619/namespace-label-operator/main/dist/install.yaml
   ```

2. **Verify installation:**
   ```bash
   kubectl get pods -n namespacelabel-system
   kubectl get crd namespacelabels.labels.shahaf.com
   ```

3. **Create your first NamespaceLabel:**
   ```bash
   kubectl apply -f - <<EOF
   apiVersion: labels.shahaf.com/v1alpha1
   kind: NamespaceLabel
   metadata:
     name: labels
     namespace: default
   spec:
     labels:
       environment: "development"
       team: "platform"
       managed-by: "namespacelabel-operator"
   EOF
   ```

4. **Check the results:**
   ```bash
   kubectl get namespace default --show-labels
   kubectl get namespacelabel labels -n default -o yaml
   ```

## 📖 Usage Examples

### Basic Label Management

```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels                    # Must be named "labels"
  namespace: my-application       # Affects this namespace only
spec:
  labels:
    environment: "production"
    team: "backend"
    cost-center: "engineering"
    monitoring: "enabled"
```

### With Label Protection

```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: production-app
spec:
  labels:
    environment: "production"
    tier: "critical"
    backup-policy: "daily"
  
  # Protect important management labels
  protectedLabelPatterns:
    - "kubernetes.io/*"           # Protect all kubernetes.io labels
    - "*.k8s.io/*"               # Protect k8s ecosystem labels
    - "istio.io/*"               # Protect service mesh labels
    - "pod-security.kubernetes.io/*"  # Protect pod security labels
  
  protectionMode: warn            # skip, warn, or fail
```

### GitOps Integration

```yaml
# In your GitOps repository
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: team-backend
spec:
  labels:
    environment: "${ENVIRONMENT}"      # Substituted by ArgoCD/Flux
    team: "backend"
    git-commit: "${COMMIT_SHA}"
  protectedLabelPatterns:
    - "kubernetes.io/*"
  protectionMode: skip
```

## 🏗️ Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  NamespaceLabel │    │   Controller     │    │   Namespace     │
│       CR        │───▶│   Reconciler     │───▶│    Labels       │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                │
                                ▼
                       ┌──────────────────┐
                       │   Protection     │
                       │     Logic        │
                       └──────────────────┘
```

The operator watches for `NamespaceLabel` custom resources and applies the specified labels to the corresponding namespace. It includes sophisticated protection logic to prevent overwriting critical management labels.

## 🛡️ Label Protection

### Protection Patterns

Use glob patterns to protect important labels:

| Pattern | Protects | Example Labels |
|---------|----------|----------------|
| `kubernetes.io/*` | Core Kubernetes labels | `kubernetes.io/managed-by` |
| `*.k8s.io/*` | K8s ecosystem | `networking.k8s.io/ingress-class` |
| `istio.io/*` | Service mesh | `istio.io/rev` |
| `pod-security.kubernetes.io/*` | Pod security | `pod-security.kubernetes.io/enforce` |

### Protection Modes

| Mode | Behavior | Use Case |
|------|----------|----------|
| `skip` | Silently skip protected labels | Default, non-disruptive |
| `warn` | Skip with warnings in logs/status | Compliance monitoring |
| `fail` | Fail reconciliation completely | Strict enforcement |

### Example Protection Configuration

```yaml
spec:
  protectedLabelPatterns:
    - "kubernetes.io/*"
    - "*.k8s.io/*"
    - "compliance.*"
  protectionMode: warn
```

## 🔐 Security & RBAC

### Tenant Access

The operator provides ClusterRoles for tenant access:

- **`namespacelabel-editor-role`** - Full CRUD access to NamespaceLabel CRDs
- **`namespacelabel-viewer-role`** - Read-only access

#### Grant Access to Tenants

```bash
# Grant editor access to a team
kubectl create rolebinding namespacelabel-access \
  --clusterrole=namespacelabel-editor-role \
  --group=backend-team \
  --namespace=team-backend

# Grant read-only access
kubectl create rolebinding namespacelabel-viewer \
  --clusterrole=namespacelabel-viewer-role \
  --group=monitoring-team \
  --namespace=any-namespace
```

### Security Features

- **Namespace Isolation**: CRs can only affect their own namespace
- **Singleton Pattern**: Only one `labels` CR allowed per namespace
- **Protection Logic**: Prevents overwriting critical labels
- **Audit Trail**: All changes logged and tracked

## 📊 Monitoring & Observability

### Status Fields

```yaml
status:
  applied: true
  message: "Applied 3 labels to namespace 'production', skipped 1 protected label"
  protectedLabelsSkipped: ["kubernetes.io/managed-by"]
  labelsApplied: ["environment", "team", "tier"]
  conditions:
  - type: Ready
    status: "True"
    reason: Synced
    message: "Successfully applied labels"
```

### Metrics & Logging

```bash
# View controller logs
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager

# Check operator status
kubectl get pods -n namespacelabel-system
kubectl get namespacelabel -A
```

## 🛠️ Development

### Building from Source

```bash
# Clone repository
git clone https://github.com/sbahar619/namespace-label-operator
cd namespace-label-operator

# Run tests
make test

# Build and run locally
make run

# Build container image
make docker-build IMG=my-registry/namespacelabel:latest
make docker-push IMG=my-registry/namespacelabel:latest
```

### Running Tests

```bash
# Unit tests
make test

# E2E tests (requires Kind cluster)
kind create cluster --name namespacelabel-test
make test-e2e
```

## 📚 Documentation

- [API Reference](docs/API.md) - Complete API documentation
- [Architecture Guide](docs/ARCHITECTURE.md) - Detailed architecture overview
- [Troubleshooting](docs/TROUBLESHOOTING.md) - Common issues and solutions
- [Contributing](docs/CONTRIBUTING.md) - Development guidelines
- [Examples](examples/) - Usage examples and tutorials

## 🤝 Contributing

Contributions are welcome! Please see our [Contributing Guide](docs/CONTRIBUTING.md) for details.

### Development Setup

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## 📄 License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## 🆘 Support

- **Issues**: [GitHub Issues](https://github.com/sbahar619/namespace-label-operator/issues)
- **Discussions**: [GitHub Discussions](https://github.com/sbahar619/namespace-label-operator/discussions)
- **Documentation**: [docs/](docs/)

## 🏷️ Versioning

We use [Semantic Versioning](https://semver.org/). For available versions, see the [tags on this repository](https://github.com/sbahar619/namespace-label-operator/tags).

## ⭐ Star History

If you find this project useful, please consider giving it a star! ⭐

