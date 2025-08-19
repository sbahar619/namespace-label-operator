# Examples

## Basic Usage

### Simple Labels
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

### With Protection
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
    kubernetes.io/managed-by: my-operator
  protectedLabelPatterns:
    - "kubernetes.io/*"
    - "istio.io/*"
  protectionMode: warn
EOF
```

## Files

- `basic-example.yaml` - Simple label application
- `protection-patterns.yaml` - Protection examples with different modes

## Usage

```bash
# Apply examples
kubectl apply -f examples/basic-example.yaml
kubectl apply -f examples/protection-patterns.yaml

# Check results
kubectl get namespacelabel -A
kubectl get namespace --show-labels
``` 