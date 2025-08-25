# Image URL to use for building/pushing image targets
CONTROLLER_IMG ?= controller:latest
WEBHOOK_IMG ?= webhook:latest

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

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Code Generation

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

##@ Code Quality

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter.
	$(GOLANGCI_LINT) run

##@ Testing

.PHONY: test
test: manifests generate fmt vet envtest ginkgo ## Run unit tests. Use GINKGO_FOCUS=label to run specific tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) -v --procs=16 --compilers=16 --show-node-events --coverprofile cover.out $(if $(GINKGO_FOCUS),--label-filter="$(GINKGO_FOCUS)") ./internal/... ./api/...

# GINKGO_FOCUS can be set to run specific tests by label
# Examples:
#   make test GINKGO_FOCUS=controller     # Run only controller unit tests
#   make test GINKGO_FOCUS=webhook        # Run only webhook unit tests
#   make test-e2e GINKGO_FOCUS=controller # Run only controller e2e tests  
#   make test-e2e GINKGO_FOCUS=webhook    # Run only webhook e2e tests
GINKGO_FOCUS ?=

.PHONY: test-e2e
test-e2e: ginkgo ## Run E2E tests in parallel. Use GINKGO_FOCUS=label to run specific tests.
	$(GINKGO) -v --procs=16 --compilers=16 --fail-on-pending --show-node-events --timeout=15m $(if $(GINKGO_FOCUS),--label-filter="$(GINKGO_FOCUS)") ./test/e2e/

.PHONY: test-e2e-debug
test-e2e-debug: ## Run E2E tests sequentially for debugging. Use GINKGO_FOCUS=label to run specific tests.
	go test ./test/e2e/ -v -timeout 15m --ginkgo.v --ginkgo.fail-on-pending $(if $(GINKGO_FOCUS),--ginkgo.label-filter="$(GINKGO_FOCUS)")

##@ Development

.PHONY: build
build: generate ## Build manager binary.
	go build -o bin/manager cmd/controller/main.go

.PHONY: run
run: generate ## Run a controller from your host.
	go run ./cmd/controller/main.go

##@ Build

.PHONY: controller-docker-build
controller-docker-build: ## Build docker image with the controller.
	$(CONTAINER_TOOL) build -t ${CONTROLLER_IMG} -f cmd/controller/Dockerfile .

.PHONY: controller-docker-push
controller-docker-push: ## Push docker image with the controller.
	$(CONTAINER_TOOL) push ${CONTROLLER_IMG}

.PHONY: webhook-docker-build
webhook-docker-build: ## Build docker image with the webhook.
	$(CONTAINER_TOOL) build -t ${WEBHOOK_IMG} -f cmd/webhook/Dockerfile .

.PHONY: webhook-docker-push
webhook-docker-push: ## Push docker image with the webhook.
	$(CONTAINER_TOOL) push ${WEBHOOK_IMG}

.PHONY: generate-installer
generate-installer: manifests kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	@echo "🏗️ Setting image references for installer..."
	@cd config/manager && $(KUSTOMIZE) edit set image controller=${CONTROLLER_IMG}
	@cd config/webhook && $(KUSTOMIZE) edit set image webhook=${WEBHOOK_IMG}
	@echo "📦 Building complete installer..."
	$(KUSTOMIZE) build config/default > dist/install.yaml
	@echo "✅ Complete installer generated at dist/install.yaml"

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = true
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy the complete operator (controller and webhook) to the K8s cluster.
	@echo "🏗️ Setting image references..."
	@cd config/manager && $(KUSTOMIZE) edit set image controller=${CONTROLLER_IMG}
	@cd config/webhook && $(KUSTOMIZE) edit set image webhook=${WEBHOOK_IMG}
	@echo "🚀 Deploying to cluster..."
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -
	@echo "🔐 Generating webhook certificates..."
	@./hack/generate-webhook-certs.sh

.PHONY: undeploy
undeploy: kustomize ## Undeploy the complete operator from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -
	@echo "🔄 Resetting image references to defaults..."
	@cd config/manager && $(KUSTOMIZE) edit set image controller=controller:main
	@cd config/webhook && $(KUSTOMIZE) edit set image webhook=webhook:main
	@echo "✅ Image references reset successfully"

##@ Monitoring

.PHONY: deploy-status
deploy-status: ## Show detailed deployment status.
	@echo "📊 Deployment Status for $(CONTROLLER_IMG) and $(WEBHOOK_IMG):"
	@echo ""
	@echo "🏗️  Controller Deployment:"
	@$(KUBECTL) get deployment $(CONTROLLER_DEPLOYMENT) -n $(CONTROLLER_NAMESPACE) -o wide 2>/dev/null || echo "❌ Controller not deployed"
	@echo ""
	@echo "🔗 Webhook Deployment:"
	@$(KUBECTL) get deployment namespacelabel-webhook-server -n $(CONTROLLER_NAMESPACE) -o wide 2>/dev/null || echo "❌ Webhook not deployed"
	@echo ""
	@echo "🚀 All Pods:"
	@$(KUBECTL) get pods -n $(CONTROLLER_NAMESPACE) -o wide 2>/dev/null || echo "❌ No pods found"
	@echo ""
	@echo "📋 Recent Events:"
	@$(KUBECTL) get events -n $(CONTROLLER_NAMESPACE) --sort-by='.lastTimestamp' | tail -10 2>/dev/null || echo "❌ No events found"

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
GINKGO ?= $(LOCALBIN)/ginkgo-$(GINKGO_VERSION)

# Tool Versions
KUSTOMIZE_VERSION ?= v5.3.0
CONTROLLER_TOOLS_VERSION ?= v0.14.0
ENVTEST_VERSION ?= release-0.17
GOLANGCI_LINT_VERSION ?= v1.57.2
GINKGO_VERSION ?= v2.15.0

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

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo locally if necessary.
$(GINKGO): $(LOCALBIN)
	$(call go-install-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo,$(GINKGO_VERSION))

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
