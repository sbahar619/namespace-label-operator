# API Reference

## NamespaceLabel Custom Resource

**API Version:** `labels.shahaf.com/v1alpha1`  
**Kind:** `NamespaceLabel`  
**Scope:** Namespaced

### Spec Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `labels` | `map[string]string` | No | `{}` | Labels to apply to the namespace |
| `protectedLabelPatterns` | `[]string` | No | `[]` | Glob patterns for protected labels |
| `protectionMode` | `string` | No | `skip` | Protection behavior: `skip`/`warn`/`fail` |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `applied` | `bool` | Whether labels were successfully applied |
| `protectedLabelsSkipped` | `[]string` | List of protected label keys that were skipped |
| `labelsApplied` | `[]string` | List of label keys that were successfully applied |
| `conditions` | `[]metav1.Condition` | Standard Kubernetes conditions with detailed status messages |

## Examples

### Basic Usage
```yaml
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
```

### With Protection
```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: my-app
spec:
  labels:
    environment: production
    kubernetes.io/managed-by: my-operator  # Will be protected
  protectedLabelPatterns:
    - "kubernetes.io/*"
    - "*.k8s.io/*"
    - "istio.io/*"
  protectionMode: warn
```

### Protection Modes

| Mode | Behavior | Use Case |
|------|----------|----------|
| `skip` | Silently skip protected labels | Default, non-disruptive |
| `warn` | Skip + log warnings | Development, monitoring |
| `fail` | Fail entire reconciliation | Strict environments |

### Common Protection Patterns

| Pattern | Protects | Examples |
|---------|----------|----------|
| `kubernetes.io/*` | Core Kubernetes labels | `kubernetes.io/managed-by` |
| `*.k8s.io/*` | K8s ecosystem labels | `networking.k8s.io/ingress-class` |
| `istio.io/*` | Service mesh labels | `istio.io/rev`, `istio.io/injection` |
| `pod-security.kubernetes.io/*` | Pod security labels | `pod-security.kubernetes.io/enforce` |

## Constraints

- **Name Requirement:** NamespaceLabel CRs must be named `labels` (singleton pattern)
- **Namespace Scope:** CRs only affect their own namespace (security)
- **One Per Namespace:** Only one NamespaceLabel CR allowed per namespace
- **Pattern Matching:** Uses Go's `filepath.Match()` for glob patterns

## Status Example

```yaml
status:
  applied: true
  protectedLabelsSkipped: ["kubernetes.io/managed-by"]
  labelsApplied: ["environment", "team"]
  conditions:
  - type: Ready
    status: "True"
    reason: Synced
    message: "Applied 2 labels, skipped 1 protected label (kubernetes.io/managed-by)"
    lastTransitionTime: "2025-01-01T12:00:00Z"
``` 