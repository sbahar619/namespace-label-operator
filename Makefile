# Image URL to use for building/pushing image targets
IMG ?= controller:latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary
ENVTEST_K8S_VERSION = 1.29.0

# Controller deployment configuration
CONTROLLER_NAMESPACE ?= namespacelabel-system
CONTROLLER_DEPLOYMENT ?= namespacelabel-controller-manager
DEPLOYMENT_TIMEOUT ?= 300s

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-e2e
test-e2e: ## Run the e2e tests against the current cluster.
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: test-e2e-full
test-e2e-full: check-img deploy-controller wait-ready ## Run full e2e test suite with deployment.
	@echo "🧪 Running comprehensive e2e tests..."
	go test ./test/e2e/ -v -ginkgo.v -ginkgo.focus="NamespaceLabel E2E Tests" -timeout 15m

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes.
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Enhanced Deployment

.PHONY: full-deploy
full-deploy: check-img build-and-push deploy-controller wait-ready ## Complete deployment workflow: build, push, deploy, and wait for readiness.
	@echo "🎉 Full deployment completed successfully!"
	@echo "📋 Controller Status:"
	@$(KUBECTL) get deployment $(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE)
	@echo "📊 Pod Status:"
	@$(KUBECTL) get pods -n $(CONTROLLER_NAMESPACE) -l control-plane=controller-manager

.PHONY: check-img
check-img: ## Validate that IMG environment variable is set.
	@if [ -z "$(IMG)" ] || [ "$(IMG)" = "controller:latest" ]; then \
		echo "❌ Error: Please set IMG environment variable to your image repository"; \
		echo "💡 Example: export IMG=quay.io/username/namespacelabel:v1.0.0"; \
		echo "💡 Or run: make full-deploy IMG=your-registry/namespacelabel:tag"; \
		exit 1; \
	fi
	@echo "✅ Using image: $(IMG)"

.PHONY: build-and-push
build-and-push: check-img docker-build docker-push ## Build and push container image.
	@echo "✅ Image $(IMG) built and pushed successfully"

.PHONY: deploy-controller
deploy-controller: install deploy ## Install CRDs and deploy controller.
	@echo "✅ Controller deployed successfully"

.PHONY: wait-ready
wait-ready: ## Wait for controller deployment to be ready.
	@echo "⏳ Waiting for controller to be ready (timeout: $(DEPLOYMENT_TIMEOUT))..."
	@$(KUBECTL) wait --for=condition=available deployment/$(CONTROLLER_DEPLOYMENT) \
		-n $(CONTROLLER_NAMESPACE) --timeout=$(DEPLOYMENT_TIMEOUT)
	@echo "✅ Controller is ready!"

.PHONY: deploy-status
deploy-status: ## Show detailed deployment status.
	@echo "📊 Deployment Status for $(IMG):"
	@echo ""
	@echo "🏗️  Controller Deployment:"
	@$(KUBECTL) get deployment $(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE) -o wide 2>/dev/null || echo "❌ Controller not deployed"
	@echo ""
	@echo "🚀 Controller Pods:"
	@$(KUBECTL) get pods -n $(CONTROLLER_NAMESPACE) -l control-plane=controller-manager -o wide 2>/dev/null || echo "❌ No controller pods found"
	@echo ""
	@echo "📋 Recent Events:"
	@$(KUBECTL) get events -n $(CONTROLLER_NAMESPACE) --sort-by='.lastTimestamp' | tail -10 2>/dev/null || echo "❌ No events found"

.PHONY: deploy-logs
deploy-logs: ## Show controller logs (last 50 lines).
	@echo "📋 Controller Logs (last 50 lines):"
	@$(KUBECTL) logs deployment/$(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE) --tail=50 --timestamps 2>/dev/null || echo "❌ Controller not found"

.PHONY: deploy-logs-follow
deploy-logs-follow: ## Follow controller logs in real-time.
	@echo "📋 Following controller logs (Ctrl+C to stop):"
	@$(KUBECTL) logs deployment/$(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE) -f --timestamps

.PHONY: cleanup
cleanup: undeploy uninstall-safe ## Complete cleanup: undeploy controller and remove CRDs.
	@echo "🧹 Cleanup completed"
	@echo "💡 To redeploy, run: make full-deploy IMG=your-image:tag"

.PHONY: uninstall-safe
uninstall-safe: ## Safely uninstall CRDs (ignores not-found errors).
	$(MAKE) uninstall ignore-not-found=true

##@ Dependencies

# Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

# Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)

# Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.14.0
ENVTEST_VERSION ?= release-0.17
GOLANGCI_LINT_VERSION ?= v1.57.2

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,${GOLANGCI_LINT_VERSION})

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary (ideally with version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv "$$(echo "$(1)" | sed "s/-$(3)$$//")" $(1) ;\
}
endef
