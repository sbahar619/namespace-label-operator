# NamespaceLabel API Reference

This document provides detailed information about the NamespaceLabel Custom Resource Definition (CRD) API.

## API Version

- **Group**: `labels.shahaf.com`
- **Version**: `v1alpha1`
- **Kind**: `NamespaceLabel`

## Resource Structure

```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels                    # Must be "labels" (singleton pattern)
  namespace: target-namespace     # Namespace to apply labels to
spec:
  # Specification fields
status:
  # Status fields (read-only)
```

## Specification (spec)

### `labels` (map[string]string)

**Required**: No  
**Default**: `{}`

Map of key-value pairs to apply to the namespace.

```yaml
spec:
  labels:
    environment: "production"
    team: "backend"
    cost-center: "engineering"
    monitoring: "enabled"
```

### `protectedLabelPatterns` ([]string)

**Required**: No  
**Default**: `[]`

List of glob patterns for label keys that should be protected from modification. If a label matches any pattern and already exists with a different value, the behavior is controlled by `protectionMode`.

```yaml
spec:
  protectedLabelPatterns:
    - "kubernetes.io/*"              # Protect all kubernetes.io labels
    - "*.k8s.io/*"                  # Protect all k8s.io domain labels
    - "pod-security.kubernetes.io/*" # Protect pod security labels
    - "istio.io/*"                  # Protect service mesh labels
    - "compliance"                  # Protect specific label
```

**Pattern Syntax**: Uses Go's `filepath.Match` function
- `*` matches any sequence of characters
- `?` matches any single character
- `[class]` matches any character in class
- `\` escapes special characters

### `protectionMode` (string)

**Required**: No  
**Default**: `"skip"`  
**Valid values**: `"skip"`, `"warn"`, `"fail"`

Controls behavior when attempting to modify protected labels:

| Mode | Behavior |
|------|----------|
| `skip` | Silently skip protected labels without error |
| `warn` | Skip protected labels but log warnings and update status |
| `fail` | Fail the entire reconciliation if any protected labels are attempted |

```yaml
spec:
  protectionMode: warn
```

### `ignoreExistingProtectedLabels` (bool)

**Required**: No  
**Default**: `false`

When `true`, allows the operator to manage labels that match protected patterns if they were previously applied by this operator (tracked in namespace annotation).

```yaml
spec:
  ignoreExistingProtectedLabels: true
```

## Status (status)

The status section is read-only and managed by the operator.

### `applied` (bool)

Indicates whether the labels were successfully applied to the namespace.

```yaml
status:
  applied: true
```

### `message` (string)

Human-readable message providing details about the last reconciliation attempt.

```yaml
status:
  message: "Applied 3 labels to namespace 'production', skipped 1 protected label"
```

### `protectedLabelsSkipped` ([]string)

List of label keys that were skipped due to protection rules.

```yaml
status:
  protectedLabelsSkipped:
    - "kubernetes.io/managed-by"
    - "pod-security.kubernetes.io/enforce"
```

### `labelsApplied` ([]string)

List of label keys that were successfully applied to the namespace.

```yaml
status:
  labelsApplied:
    - "environment"
    - "team"
    - "cost-center"
```

### `conditions` ([]metav1.Condition)

Standard Kubernetes conditions providing detailed status information.

```yaml
status:
  conditions:
  - type: Ready
    status: "True"
    reason: Synced
    message: "Successfully applied labels to namespace"
    lastTransitionTime: "2025-01-18T10:30:00Z"
    observedGeneration: 1
```

#### Condition Types

| Type | Description |
|------|-------------|
| `Ready` | Indicates whether the NamespaceLabel is successfully reconciled |

#### Condition Reasons

| Reason | Description |
|--------|-------------|
| `Synced` | Labels successfully applied |
| `InvalidName` | NamespaceLabel name is not "labels" |
| `NamespaceNotFound` | Target namespace does not exist |
| `ProtectedLabelConflict` | Protected label conflict in fail mode |
| `AnnotationError` | Failed to update tracking annotation |

## Annotations

The operator uses annotations on the namespace to track state:

### `labels.shahaf.com/applied`

**Managed by**: Operator  
**Purpose**: JSON representation of labels applied by the operator

This annotation tracks which labels were applied by the operator to enable safe cleanup when labels are removed from the spec.

```yaml
metadata:
  annotations:
    labels.shahaf.com/applied: '{"environment":"production","team":"backend"}'
```

## Finalizers

### `labels.shahaf.com/finalizer`

**Managed by**: Operator  
**Purpose**: Ensures cleanup of operator-managed labels before CR deletion

The operator adds this finalizer to ensure that when a NamespaceLabel CR is deleted, any labels it applied to the namespace are safely removed (unless they're still desired by other CRs in the same namespace).

## Validation Rules

### Name Validation

- **Rule**: NamespaceLabel resources must be named `"labels"`
- **Reason**: Enforces singleton pattern (one NamespaceLabel per namespace)
- **Behavior**: CRs with other names will have status updated with error message

### Namespace Scope

- **Rule**: NamespaceLabel CRs can only affect their own namespace
- **Reason**: Multi-tenant security
- **Behavior**: The operator ignores the target namespace and always uses the CR's namespace

## Examples

### Complete Example

```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: production-app
  finalizers:
    - labels.shahaf.com/finalizer
spec:
  labels:
    environment: "production"
    team: "backend"
    cost-center: "engineering"
    compliance: "sox"
    monitoring: "enabled"
  protectedLabelPatterns:
    - "kubernetes.io/*"
    - "*.k8s.io/*"
    - "pod-security.kubernetes.io/*"
    - "istio.io/*"
    - "compliance"
  protectionMode: warn
  ignoreExistingProtectedLabels: false
status:
  applied: true
  message: "Applied 4 labels to namespace 'production-app', skipped 1 protected label"
  protectedLabelsSkipped:
    - "compliance"
  labelsApplied:
    - "environment"
    - "team"
    - "cost-center"
    - "monitoring"
  conditions:
  - type: Ready
    status: "True"
    reason: Synced
    message: "Successfully applied labels to namespace"
    lastTransitionTime: "2025-01-18T10:30:00Z"
    observedGeneration: 1
```

### Minimal Example

```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: my-app
spec:
  labels:
    environment: "development"
```

### Protection-only Example

```yaml
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: secure-app
spec:
  labels: {}  # No labels to apply
  protectedLabelPatterns:
    - "kubernetes.io/*"
    - "security.*"
  protectionMode: fail  # Strict protection
```

## Migration and Compatibility

### From Manual Label Management

When migrating from manual namespace label management:

1. Create NamespaceLabel CR with current labels
2. Operator will detect existing labels and leave them unchanged
3. Future changes go through the CR

### Updating Protection Patterns

Protection patterns can be updated without affecting existing labels:

1. Update `protectedLabelPatterns` in the CR
2. Operator applies new protection rules on next reconciliation
3. Already-applied labels remain unchanged unless they conflict with new rules

### API Evolution

This is version `v1alpha1`. Future API versions will:
- Maintain backward compatibility where possible
- Provide migration guides for breaking changes
- Use standard Kubernetes deprecation policies 