# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0

# Controller deployment namespace and configuration
CONTROLLER_NAMESPACE ?= namespacelabel-system
CONTROLLER_DEPLOYMENT ?= namespacelabel-controller-manager
DEPLOYMENT_TIMEOUT ?= 300s

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

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

# Utilize Kind or modify the e2e tests to load the image locally, enabling compatibility with other vendors.
.PHONY: test-e2e  # Run the e2e tests against a Kind k8s instance that is spun up.
test-e2e:
	go test ./test/e2e/ -v -ginkgo.v

.PHONY: test-e2e-namespacelabel  # Run comprehensive NamespaceLabel e2e tests
test-e2e-namespacelabel:
	go test ./test/e2e/ -v -ginkgo.v -ginkgo.focus="NamespaceLabel E2E Tests" -timeout 15m

.PHONY: test-e2e-full  # Run full e2e test suite with deployment
test-e2e-full: check-img deploy-controller wait-ready
	@echo "🧪 Running comprehensive e2e tests..."
	go test ./test/e2e/ -v -ginkgo.v -ginkgo.focus="NamespaceLabel E2E Tests" -timeout 15m

.PHONY: test-e2e-kind  # Run e2e tests against Kind cluster with image loading
test-e2e-kind: manifests generate
	@echo "Building image for Kind..."
	$(MAKE) docker-build IMG=namespacelabel:e2e-test
	@echo "Detecting Kind cluster..."
	@KIND_CLUSTER_NAME=$$(kind get clusters | head -1); \
	if [ -z "$$KIND_CLUSTER_NAME" ]; then \
		echo "❌ No Kind clusters found. Please create one with: kind create cluster"; \
		exit 1; \
	fi; \
	echo "📦 Loading image to Kind cluster: $$KIND_CLUSTER_NAME"; \
	kind load docker-image namespacelabel:e2e-test --name $$KIND_CLUSTER_NAME
	@echo "Installing CRDs and deploying controller..."
	$(MAKE) install
	$(MAKE) deploy IMG=namespacelabel:e2e-test
	@echo "Waiting for controller to be ready..."
	$(KUBECTL) wait --for=condition=available deployment/$(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE) --timeout=$(DEPLOYMENT_TIMEOUT)
	@echo "Running e2e tests..."
	go test ./test/e2e/ -v -ginkgo.v -ginkgo.focus="NamespaceLabel E2E Tests" -timeout 15m

.PHONY: test-e2e-current  # Run e2e tests against current kubectl context (any cluster)
test-e2e-current: manifests generate
	@echo "Current kubectl context: $$(kubectl config current-context)"
	@echo "Building and deploying to current cluster..."
	$(MAKE) docker-build IMG=namespacelabel:e2e-test
	@echo "Installing CRDs and deploying controller..."
	$(MAKE) install
	$(MAKE) deploy IMG=namespacelabel:e2e-test
	@echo "Waiting for controller to be ready..."
	$(KUBECTL) wait --for=condition=available deployment/$(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE) --timeout=$(DEPLOYMENT_TIMEOUT)
	@echo "Running e2e tests..."
	go test ./test/e2e/ -v -ginkgo.v -ginkgo.focus="NamespaceLabel E2E Tests" -timeout 15m

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

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
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Enhanced Deployment Workflow

.PHONY: full-deploy
full-deploy: check-img build-and-push deploy-controller wait-ready ## Complete deployment workflow: build, push, deploy, and wait for readiness
	@echo "🎉 Full deployment completed successfully!"
	@echo "📋 Controller Status:"
	@$(KUBECTL) get deployment $(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE)
	@echo "📊 Pod Status:"
	@$(KUBECTL) get pods -n $(CONTROLLER_NAMESPACE) -l control-plane=controller-manager

.PHONY: check-img
check-img: ## Validate that IMG environment variable is set
	@if [ -z "$(IMG)" ] || [ "$(IMG)" = "controller:latest" ]; then \
		echo "❌ Error: Please set IMG environment variable to your image repository"; \
		echo "💡 Example: export IMG=quay.io/username/namespacelabel:v1.0.0"; \
		echo "💡 Or run: make full-deploy IMG=your-registry/namespacelabel:tag"; \
		exit 1; \
	fi
	@echo "✅ Using image: $(IMG)"

.PHONY: build-and-push
build-and-push: check-img docker-build docker-push ## Build and push container image
	@echo "✅ Image $(IMG) built and pushed successfully"

.PHONY: deploy-controller
deploy-controller: install deploy ## Install CRDs and deploy controller
	@echo "✅ Controller deployed successfully"

.PHONY: wait-ready
wait-ready: ## Wait for controller deployment to be ready
	@echo "⏳ Waiting for controller to be ready (timeout: $(DEPLOYMENT_TIMEOUT))..."
	@$(KUBECTL) wait --for=condition=available deployment/$(CONTROLLER_DEPLOYMENT) \
		-n $(CONTROLLER_NAMESPACE) --timeout=$(DEPLOYMENT_TIMEOUT)
	@echo "✅ Controller is ready!"

.PHONY: quick-deploy
quick-deploy: check-img build-and-push deploy-controller ## Quick deployment without waiting (for faster iteration)
	@echo "🚀 Quick deployment completed! Controller is starting up..."

.PHONY: deploy-status
deploy-status: ## Show detailed deployment status
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
deploy-logs: ## Show controller logs (last 50 lines)
	@echo "📋 Controller Logs (last 50 lines):"
	@$(KUBECTL) logs deployment/$(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE) --tail=50 --timestamps 2>/dev/null || echo "❌ Controller not found"

.PHONY: deploy-logs-follow
deploy-logs-follow: ## Follow controller logs in real-time
	@echo "📋 Following controller logs (Ctrl+C to stop):"
	@$(KUBECTL) logs deployment/$(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE) -f --timestamps

.PHONY: full-cleanup
full-cleanup: undeploy uninstall-safe ## Complete cleanup: undeploy controller and remove CRDs
	@echo "🧹 Full cleanup completed"
	@echo "💡 To redeploy, run: make full-deploy IMG=your-image:tag"

.PHONY: uninstall-safe
uninstall-safe: ## Safely uninstall CRDs (ignores not-found errors)
	$(MAKE) uninstall ignore-not-found=true

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize-$(KUSTOMIZE_VERSION)
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)

## Tool Versions
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
