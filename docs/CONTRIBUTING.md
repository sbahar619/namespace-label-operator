# Contributing to NamespaceLabel Operator

Thank you for your interest in contributing to the NamespaceLabel Operator! This document provides guidelines and information for contributors.

## Code of Conduct

This project adheres to the Kubernetes Community Code of Conduct. By participating, you are expected to uphold this code. Please report unacceptable behavior to the project maintainers.

## Ways to Contribute

### 🐛 Bug Reports

- **Search existing issues** before creating a new one
- **Use the bug report template** when available
- **Provide detailed information**:
  - Kubernetes version
  - Operator version
  - Steps to reproduce
  - Expected vs actual behavior
  - Relevant logs and manifests

### 💡 Feature Requests

- **Check existing feature requests** first
- **Describe the use case** clearly
- **Explain why this would be valuable** to other users
- **Consider implementation complexity** and backwards compatibility

### 📖 Documentation

- **Fix typos and improve clarity**
- **Add examples and tutorials**
- **Update API documentation**
- **Translate documentation** (when internationalization is supported)

### 🔧 Code Contributions

- **Start with good first issues** if you're new to the project
- **Discuss large changes** in an issue before implementing
- **Follow coding standards** and existing patterns
- **Include tests** for new functionality
- **Update documentation** as needed

## Development Setup

### Prerequisites

- **Go 1.21+**
- **Docker 17.03+**
- **kubectl v1.11.3+**
- **Access to a Kubernetes cluster** (Kind, minikube, or cloud cluster)
- **make** (GNU Make)

### Local Development

1. **Fork and clone the repository:**
   ```bash
   git clone https://github.com/YOUR_USERNAME/namespace-label-operator
   cd namespace-label-operator
   ```

2. **Install dependencies:**
   ```bash
   go mod download
   ```

3. **Run tests:**
   ```bash
   make test
   ```

4. **Run locally against a cluster:**
   ```bash
   # Install CRDs
   make install
   
   # Run controller locally
   make run
   ```

5. **Build and test container image:**
   ```bash
   # Build image
   make docker-build IMG=namespacelabel:dev
   
   # Deploy to test cluster
   make deploy IMG=namespacelabel:dev
   ```

### Development Workflow

1. **Create a feature branch:**
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes:**
   - Write code following the style guide
   - Add or update tests
   - Update documentation
   - Run tests locally

3. **Commit your changes:**
   ```bash
   git add .
   git commit -m "feat: add new protection mode"
   ```

4. **Push and create PR:**
   ```bash
   git push origin feature/your-feature-name
   # Create pull request on GitHub
   ```

## Coding Standards

### Go Code Style

- **Follow standard Go conventions**: `gofmt`, `golint`, `go vet`
- **Use meaningful variable names**: `namespaceName` not `n`
- **Add comments for exported functions**: Document purpose and parameters
- **Handle errors appropriately**: Don't ignore errors, log them or return them
- **Use structured logging**: `log.Info("message", "key", value)`

### Example Good Code

```go
// UpdateNamespaceLabels applies the specified labels to a namespace with protection
func (r *NamespaceLabelReconciler) updateNamespaceLabels(
    ctx context.Context, 
    namespace *corev1.Namespace, 
    desiredLabels map[string]string,
    protectionConfig ProtectionConfig) error {
    
    l := log.FromContext(ctx)
    
    if namespace.Labels == nil {
        namespace.Labels = make(map[string]string)
    }
    
    changed := false
    for key, value := range desiredLabels {
        if current, exists := namespace.Labels[key]; !exists || current != value {
            namespace.Labels[key] = value
            changed = true
            l.Info("Applied label", "namespace", namespace.Name, "key", key, "value", value)
        }
    }
    
    if changed {
        return r.Update(ctx, namespace)
    }
    
    return nil
}
```

### Controller Patterns

- **Use controller-runtime patterns**: Follow existing reconciliation logic
- **Handle finalizers properly**: Always clean up resources
- **Update status consistently**: Provide clear error messages
- **Use appropriate requeueing**: Don't spam the API server

### Testing Standards

- **Write unit tests for business logic**
- **Add integration tests for controller behavior**
- **Include e2e tests for complex scenarios**
- **Test error conditions and edge cases**
- **Maintain test coverage above 80%**

## Testing Guidelines

### Unit Tests

```go
func TestProtectionLogic(t *testing.T) {
    tests := []struct {
        name           string
        desiredLabels  map[string]string
        existingLabels map[string]string
        patterns       []string
        mode          ProtectionMode
        expected       ProtectionResult
    }{
        {
            name: "skip protected label",
            desiredLabels: map[string]string{
                "kubernetes.io/managed-by": "operator",
                "app": "myapp",
            },
            existingLabels: map[string]string{
                "kubernetes.io/managed-by": "system",
            },
            patterns: []string{"kubernetes.io/*"},
            mode:     ProtectionModeSkip,
            expected: ProtectionResult{
                AllowedLabels: map[string]string{"app": "myapp"},
                ProtectedSkipped: []string{"kubernetes.io/managed-by"},
            },
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result := applyProtectionLogic(
                tt.desiredLabels,
                tt.existingLabels,
                map[string]string{}, // prevApplied
                tt.patterns,
                tt.mode,
                false, // ignoreExisting
            )
            
            assert.Equal(t, tt.expected.AllowedLabels, result.AllowedLabels)
            assert.Equal(t, tt.expected.ProtectedSkipped, result.ProtectedSkipped)
        })
    }
}
```

### E2E Tests

```go
var _ = Describe("Protection Features", func() {
    It("should respect protection patterns", func() {
        By("Creating namespace with existing protected label")
        ns := &corev1.Namespace{
            ObjectMeta: metav1.ObjectMeta{
                Name: testNS,
                Labels: map[string]string{
                    "kubernetes.io/managed-by": "system",
                },
            },
        }
        Expect(k8sClient.Create(ctx, ns)).To(Succeed())
        
        By("Creating NamespaceLabel with protection")
        cr := &labelsv1alpha1.NamespaceLabel{
            ObjectMeta: metav1.ObjectMeta{
                Name:      "labels",
                Namespace: testNS,
            },
            Spec: labelsv1alpha1.NamespaceLabelSpec{
                Labels: map[string]string{
                    "kubernetes.io/managed-by": "operator", // Should be skipped
                    "app": "myapp", // Should be applied
                },
                ProtectedLabelPatterns: []string{"kubernetes.io/*"},
                ProtectionMode: "skip",
            },
        }
        Expect(k8sClient.Create(ctx, cr)).To(Succeed())
        
        By("Verifying protection behavior")
        Eventually(func() bool {
            updatedNS := &corev1.Namespace{}
            err := k8sClient.Get(ctx, types.NamespacedName{Name: testNS}, updatedNS)
            if err != nil {
                return false
            }
            
            return updatedNS.Labels["kubernetes.io/managed-by"] == "system" && // Protected
                   updatedNS.Labels["app"] == "myapp" // Applied
        }, time.Minute, time.Second).Should(BeTrue())
    })
})
```

### Running Tests

```bash
# Unit tests
make test

# E2E tests (requires cluster)
make test-e2e

# Test with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Linting
make lint
```

## API Changes

### Backwards Compatibility

- **Don't break existing APIs** without a deprecation period
- **Add new fields as optional** with sensible defaults
- **Use API versioning** for breaking changes
- **Provide migration guides** for major updates

### CRD Updates

1. **Update the Go structs** in `api/v1alpha1/`
2. **Add validation tags** using kubebuilder markers
3. **Update documentation** in comments
4. **Regenerate CRDs**: `make manifests`
5. **Test backwards compatibility**

### Example API Addition

```go
type NamespaceLabelSpec struct {
    // ... existing fields ...
    
    // NewField provides additional configuration
    // +optional
    // +kubebuilder:default="default-value"
    NewField string `json:"newField,omitempty"`
}
```

## Documentation

### API Documentation

- **Document all public APIs** with clear examples
- **Use godoc conventions** for Go code
- **Include field descriptions** in struct tags
- **Provide usage examples** for complex features

### User Documentation

- **Write from the user's perspective**
- **Include practical examples**
- **Cover common use cases**
- **Explain troubleshooting steps**

### Documentation Structure

```
docs/
├── API.md              # Complete API reference
├── ARCHITECTURE.md     # Design and implementation
├── CONTRIBUTING.md     # This file
├── TROUBLESHOOTING.md  # Common issues and solutions
└── tutorials/          # Step-by-step guides
    ├── getting-started.md
    ├── advanced-protection.md
    └── gitops-integration.md
```

## Pull Request Process

### Before Submitting

- [ ] **Run all tests** and ensure they pass
- [ ] **Run linting** and fix any issues
- [ ] **Update documentation** if needed
- [ ] **Test manually** in a real cluster
- [ ] **Check backwards compatibility**

### PR Description Template

```markdown
## Description
Brief description of what this PR does.

## Related Issue
Fixes #123

## Type of Change
- [ ] Bug fix (non-breaking change)
- [ ] New feature (non-breaking change)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] E2E tests pass
- [ ] Manual testing completed

## Screenshots/Logs
(If applicable)

## Checklist
- [ ] Code follows the style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] Tests added/updated
```

### Review Process

1. **Automated checks** must pass (CI/CD)
2. **At least one maintainer** must approve
3. **All conversations resolved**
4. **No merge conflicts**

### After Merge

- **Delete feature branch**
- **Update local main branch**
- **Close related issues**

## Release Process

### Version Scheme

We follow [Semantic Versioning](https://semver.org/):

- **MAJOR**: Breaking changes
- **MINOR**: New features (backwards compatible)
- **PATCH**: Bug fixes (backwards compatible)

### Release Checklist

1. **Update version** in relevant files
2. **Update CHANGELOG.md**
3. **Create release PR**
4. **Tag release** after merge
5. **Build and push images**
6. **Update documentation**

## Community

### Communication Channels

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: Questions and general discussion
- **Pull Requests**: Code review and collaboration

### Maintainer Responsibilities

- **Review PRs** in a timely manner
- **Provide constructive feedback**
- **Help onboard new contributors**
- **Maintain project direction and quality**

### Recognition

Contributors will be recognized in:
- **CONTRIBUTORS.md** file
- **Release notes**
- **GitHub contributors page**

## Getting Help

### For Contributors

- **Read existing code** to understand patterns
- **Ask questions** in issues or discussions
- **Start with small changes** to learn the codebase
- **Join community meetings** (if/when established)

### For Maintainers

- **Be welcoming** to new contributors
- **Provide clear feedback**
- **Document decisions** and rationale
- **Help with onboarding**

## License

By contributing to this project, you agree that your contributions will be licensed under the Apache License 2.0.

Thank you for contributing to the NamespaceLabel Operator! 🎉 