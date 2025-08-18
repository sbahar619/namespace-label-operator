# NamespaceLabel Operator Examples

This directory contains practical examples and tutorials for using the NamespaceLabel operator in various scenarios.

## Quick Start Examples

### [basic-usage.yaml](basic-usage.yaml)
Simple example showing how to apply labels to a namespace.

### [protection-patterns.yaml](protection-patterns.yaml)
Examples of different protection patterns and modes.

### [gitops-integration/](gitops-integration/)
Examples for integrating with GitOps workflows (ArgoCD, Flux).

## Environment-Specific Examples

### [development/](development/)
Development environment configurations with relaxed protection.

### [production/](production/)
Production environment with strict protection and compliance labels.

### [multi-tenant/](multi-tenant/)
Multi-tenant cluster examples with RBAC and isolation.

## Advanced Use Cases

### [policy-integration/](policy-integration/)
Integration with policy engines (OPA Gatekeeper, Kyverno).

### [monitoring/](monitoring/)
Monitoring and observability setup.

### [backup-restore/](backup-restore/)
Backup and disaster recovery scenarios.

## Platform Integration

### [terraform/](terraform/)
Terraform modules for automated deployment.

### [helm/](helm/)
Helm charts and values examples.

### [kustomize/](kustomize/)
Kustomize overlays for different environments.

## Testing Examples

### [test-scenarios/](test-scenarios/)
Test cases and validation scenarios.

## Getting Started

1. **Choose an example** that matches your use case
2. **Review the documentation** in each example directory
3. **Modify the manifests** for your environment
4. **Apply the examples** to your cluster

## Example Structure

Each example typically includes:
- `README.md` - Explanation and usage instructions
- `*.yaml` - Kubernetes manifests
- `kustomization.yaml` - Kustomize configuration (when applicable)
- `values.yaml` - Helm values (when applicable)

## Contributing Examples

We welcome contributions of new examples! Please:
1. Create a new directory for your example
2. Include clear documentation
3. Test the example in a real cluster
4. Submit a pull request

## Support

If you have questions about these examples:
- Check the main [documentation](../docs/)
- Open an [issue](https://github.com/sbahar619/namespace-label-operator/issues)
- Start a [discussion](https://github.com/sbahar619/namespace-label-operator/discussions) 