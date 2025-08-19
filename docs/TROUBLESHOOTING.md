# NamespaceLabel Operator Troubleshooting Guide

This guide helps you diagnose and resolve common issues with the NamespaceLabel operator.

## Quick Diagnostic Commands

### Check Operator Status

```bash
# Verify operator is running
kubectl get pods -n namespacelabel-system

# Check operator logs
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager

# Verify CRD is installed
kubectl get crd namespacelabels.labels.shahaf.com

# List all NamespaceLabel resources
kubectl get namespacelabel -A
```

### Check NamespaceLabel Status

```bash
# Detailed status of a specific NamespaceLabel
kubectl describe namespacelabel labels -n <namespace>

# Check namespace labels
kubectl get namespace <namespace> --show-labels

# Check namespace annotations
kubectl get namespace <namespace> -o yaml | grep -A 10 annotations
```

## Common Issues and Solutions

### 1. NamespaceLabel CR Not Found

#### Symptoms
```bash
$ kubectl get namespacelabel labels -n my-namespace
Error from server (NotFound): namespacelabels.labels.shahaf.com "labels" not found
```

#### Diagnosis
```bash
# Check if CRD exists
kubectl get crd namespacelabels.labels.shahaf.com

# Check if namespace exists
kubectl get namespace my-namespace

# Check RBAC permissions
kubectl auth can-i get namespacelabel -n my-namespace
```

#### Solutions

**CRD Not Installed:**
```bash
# Install the operator
kubectl apply -f https://raw.githubusercontent.com/sbahar619/namespace-label-operator/main/dist/install.yaml
```

**Missing RBAC:**
```bash
# Grant access to your user/group
kubectl create rolebinding namespacelabel-access \
  --clusterrole=namespacelabel-editor-role \
  --user=$(kubectl config current-context) \
  --namespace=my-namespace
```

### 2. Labels Not Applied to Namespace

#### Symptoms
```bash
$ kubectl get namespace my-app --show-labels
NAME     STATUS   AGE   LABELS
my-app   Active   5m    <missing expected labels>
```

#### Diagnosis
```bash
# Check NamespaceLabel status
kubectl get namespacelabel labels -n my-app -o yaml

# Check operator logs
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager --tail=50

# Check if namespace exists
kubectl get namespace my-app
```

#### Solutions

**Namespace Not Found:**
```bash
# Create the namespace first
kubectl create namespace my-app

# Then apply NamespaceLabel
kubectl apply -f your-namespacelabel.yaml
```

**Controller Not Running:**
```bash
# Check operator pod status
kubectl get pods -n namespacelabel-system

# Restart if needed
kubectl rollout restart deployment namespacelabel-controller-manager -n namespacelabel-system
```

**Protection Rules Blocking:**
```yaml
# Check if labels are being protected
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: my-app
spec:
  labels:
    kubernetes.io/managed-by: "my-operator"  # This might be protected
  protectedLabelPatterns:
    - "kubernetes.io/*"  # This would block the above label
  protectionMode: skip
```

### 3. Invalid NamespaceLabel Name

#### Symptoms
```yaml
status:
  applied: false
  message: "NamespaceLabel CR must be named 'labels'"
```

#### Diagnosis
```bash
# Check NamespaceLabel name
kubectl get namespacelabel -n my-namespace -o wide
```

#### Solution
```bash
# Delete incorrectly named CR
kubectl delete namespacelabel wrong-name -n my-namespace

# Create correctly named CR
kubectl apply -f - <<EOF
apiVersion: labels.shahaf.com/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels  # Must be "labels"
  namespace: my-namespace
spec:
  labels:
    environment: "production"
EOF
```

### 4. Protection Mode Failures

#### Symptoms
```yaml
status:
  applied: false
  message: "Reconciliation failed due to protected label conflicts"
```

#### Diagnosis
```bash
# Check protection configuration
kubectl get namespacelabel labels -n my-namespace -o jsonpath='{.spec.protectedLabelPatterns}'

# Check existing namespace labels
kubectl get namespace my-namespace -o jsonpath='{.metadata.labels}'

# Check protection mode
kubectl get namespacelabel labels -n my-namespace -o jsonpath='{.spec.protectionMode}'
```

#### Solutions

**Change Protection Mode:**
```yaml
spec:
  protectionMode: warn  # Change from "fail" to "warn" or "skip"
```

**Update Protection Patterns:**
```yaml
spec:
  protectedLabelPatterns:
    - "kubernetes.io/*"
    # Remove overly broad patterns or add exceptions
```

### 5. RBAC Permission Denied

#### Symptoms
```bash
$ kubectl apply -f namespacelabel.yaml
Error from server (Forbidden): namespacelabels.labels.shahaf.com "labels" is forbidden
```

#### Diagnosis
```bash
# Check your permissions
kubectl auth can-i create namespacelabel -n my-namespace
kubectl auth can-i get namespacelabel -n my-namespace

# Check existing RoleBindings
kubectl get rolebinding -n my-namespace | grep namespacelabel
```

#### Solutions

**Grant Editor Access:**
```bash
kubectl create rolebinding namespacelabel-access \
  --clusterrole=namespacelabel-editor-role \
  --user=$(kubectl config view --minify -o jsonpath='{.contexts[0].context.user}') \
  --namespace=my-namespace
```

**Grant to Group:**
```bash
kubectl create rolebinding namespacelabel-team-access \
  --clusterrole=namespacelabel-editor-role \
  --group=my-team \
  --namespace=my-namespace
```

### 6. Operator Pod Crashes

#### Symptoms
```bash
$ kubectl get pods -n namespacelabel-system
NAME                                    READY   STATUS             RESTARTS   AGE
namespacelabel-controller-manager-xxx   0/1     CrashLoopBackOff   5          10m
```

#### Diagnosis
```bash
# Check pod logs
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager

# Check events
kubectl get events -n namespacelabel-system --sort-by='.lastTimestamp'

# Check resource limits
kubectl describe pod -n namespacelabel-system -l control-plane=controller-manager
```

#### Solutions

**Resource Limits:**
```yaml
# Update deployment resource limits
spec:
  template:
    spec:
      containers:
      - name: manager
        resources:
          limits:
            memory: "512Mi"
            cpu: "500m"
          requests:
            memory: "256Mi"
            cpu: "100m"
```

**RBAC Issues:**
```bash
# Reinstall operator with correct RBAC
kubectl delete -f install.yaml
kubectl apply -f install.yaml
```

### 7. Finalizer Blocking Deletion

#### Symptoms
```bash
$ kubectl delete namespacelabel labels -n my-namespace
# Hangs indefinitely

$ kubectl get namespacelabel labels -n my-namespace -o yaml
metadata:
  finalizers:
  - labels.shahaf.com/finalizer
  deletionTimestamp: "2025-01-18T10:30:00Z"
```

#### Diagnosis
```bash
# Check if operator is running
kubectl get pods -n namespacelabel-system

# Check operator logs for errors
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager | grep finalizer
```

#### Solutions

**Operator Running but Stuck:**
```bash
# Force restart operator
kubectl rollout restart deployment namespacelabel-controller-manager -n namespacelabel-system
```

**Operator Not Running (Emergency):**
```bash
# DANGEROUS: Remove finalizer manually (only if operator is completely gone)
kubectl patch namespacelabel labels -n my-namespace -p '{"metadata":{"finalizers":[]}}' --type=merge
```

### 8. Status Not Updating

#### Symptoms
```yaml
# Status shows old information or empty conditions
status:
  applied: false
  message: ""
  conditions: []
```

#### Diagnosis
```bash
# Check operator logs
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager | grep "status"

# Check if operator has permission to update status
kubectl auth can-i update namespacelabel/status -n my-namespace --as=system:serviceaccount:namespacelabel-system:namespacelabel-controller-manager
```

#### Solutions

**RBAC Missing:**
```yaml
# Ensure operator has status permissions (should be automatic)
rules:
- apiGroups: ["labels.shahaf.com"]
  resources: ["namespacelabels/status"]
  verbs: ["get", "update", "patch"]
```

**Force Reconciliation:**
```bash
# Add annotation to trigger reconciliation
kubectl annotate namespacelabel labels -n my-namespace reconcile.timestamp="$(date)"
```

## Debugging Workflows

### 1. Complete Diagnostic Workflow

```bash
#!/bin/bash
# Complete diagnostic script

NAMESPACE=${1:-default}

echo "=== NamespaceLabel Operator Diagnostics ==="
echo "Namespace: $NAMESPACE"
echo

echo "1. Checking operator status..."
kubectl get pods -n namespacelabel-system
echo

echo "2. Checking CRD..."
kubectl get crd namespacelabels.labels.shahaf.com
echo

echo "3. Checking NamespaceLabel resource..."
kubectl get namespacelabel labels -n $NAMESPACE -o yaml
echo

echo "4. Checking namespace labels..."
kubectl get namespace $NAMESPACE --show-labels
echo

echo "5. Checking recent operator logs..."
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager --tail=20
echo

echo "6. Checking RBAC permissions..."
kubectl auth can-i get namespacelabel -n $NAMESPACE
kubectl auth can-i create namespacelabel -n $NAMESPACE
```

### 2. Log Analysis

**Common Log Patterns:**

```bash
# Successful reconciliation
grep "NamespaceLabel successfully processed" logs.txt

# Protection events
grep "Label protection warning" logs.txt

# Errors
grep "ERROR" logs.txt

# Finalizer operations
grep "finalizer" logs.txt
```

### 3. Resource Validation

```bash
# Validate NamespaceLabel syntax
kubectl apply --dry-run=client -f namespacelabel.yaml

# Validate against cluster
kubectl apply --dry-run=server -f namespacelabel.yaml
```

## Performance Issues

### 1. Slow Reconciliation

#### Symptoms
- Labels take long time to appear
- High reconciliation frequency

#### Solutions
```bash
# Check resource usage
kubectl top pods -n namespacelabel-system

# Increase controller resources
kubectl patch deployment namespacelabel-controller-manager -n namespacelabel-system -p '{"spec":{"template":{"spec":{"containers":[{"name":"manager","resources":{"requests":{"memory":"512Mi","cpu":"200m"}}}]}}}}'

# Check for excessive requeuing
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager | grep "Requeue"
```

### 2. Memory Issues

```bash
# Monitor memory usage
kubectl top pods -n namespacelabel-system

# Check for memory leaks
kubectl describe pod -n namespacelabel-system -l control-plane=controller-manager
```

## Prevention Best Practices

### 1. Monitoring Setup

```yaml
# ServiceMonitor for Prometheus
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: namespacelabel-controller-metrics
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  endpoints:
  - port: https
```

### 2. Alerting Rules

```yaml
# PrometheusRule for alerts
groups:
- name: namespacelabel-operator
  rules:
  - alert: NamespaceLabelOperatorDown
    expr: up{job="namespacelabel-controller-metrics"} == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "NamespaceLabel operator is down"
```

### 3. Regular Health Checks

```bash
# Add to monitoring scripts
kubectl get pods -n namespacelabel-system
kubectl get namespacelabel -A --no-headers | wc -l
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager --tail=1 | grep ERROR && echo "ERRORS FOUND" || echo "OK"
```

## Getting Help

### 1. Gathering Support Information

```bash
# Create support bundle
kubectl cluster-info dump > cluster-info.yaml
kubectl get namespacelabel -A -o yaml > namespacelabels.yaml
kubectl logs -n namespacelabel-system deployment/namespacelabel-controller-manager > operator-logs.txt
kubectl get events -A --sort-by='.lastTimestamp' > events.yaml
```

### 2. Community Resources

- **GitHub Issues**: [Report bugs and request features](https://github.com/sbahar619/namespace-label-operator/issues)
- **Discussions**: [Ask questions and share experiences](https://github.com/sbahar619/namespace-label-operator/discussions)
- **Documentation**: [Check latest docs](https://github.com/sbahar619/namespace-label-operator/tree/main/docs)

### 3. Before Reporting Issues

1. **Search existing issues** for similar problems
2. **Provide complete information**:
   - Kubernetes version
   - Operator version
   - NamespaceLabel YAML
   - Error messages and logs
   - Steps to reproduce
3. **Include diagnostic output** from the commands above

This troubleshooting guide should help you resolve most common issues with the NamespaceLabel operator. If you encounter issues not covered here, please contribute by opening an issue or submitting a documentation update. 