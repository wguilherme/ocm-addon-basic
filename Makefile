IMAGE ?= basic-addon:latest
HUB_KUBECONFIG ?= ~/.kube/local/platform-operator/config.hub
SPOKE_KUBECONFIG ?= ~/.kube/local/platform-operator/config.spoke1
KIND_HUB ?= hub
KIND_SPOKE ?= spoke1

.PHONY: build run test tidy docker-build docker-push deploy deploy-rbac undeploy enable disable kind-load addon-deploy addon-reports test-addon-operator-kind-prepare

# Build binary locally
build:
	go build -o bin/addon ./cmd/addon

# Run controller locally (like kubebuilder's make run)
run: build deploy-rbac
	./bin/addon controller --kubeconfig=$$(eval echo $(HUB_KUBECONFIG))

# Run tests
test:
	go test ./pkg/... -v -cover

# Download dependencies
tidy:
	go mod tidy

# Build docker image
docker-build:
	docker build -t $(IMAGE) .

# Push docker image
docker-push:
	docker push $(IMAGE)

# Deploy RBAC and addon config (for local development with make run)
deploy-rbac:
	KUBECONFIG=$(HUB_KUBECONFIG) kubectl apply -f deploy/serviceaccount.yaml
	KUBECONFIG=$(HUB_KUBECONFIG) kubectl apply -f deploy/clusterrole.yaml
	KUBECONFIG=$(HUB_KUBECONFIG) kubectl apply -f deploy/clusterrolebinding.yaml
	KUBECONFIG=$(HUB_KUBECONFIG) kubectl apply -f deploy/managedclustersetbinding.yaml
	KUBECONFIG=$(HUB_KUBECONFIG) kubectl apply -f deploy/placement.yaml
	KUBECONFIG=$(HUB_KUBECONFIG) kubectl apply -f deploy/clustermanagementaddon.yaml

# Deploy to hub cluster (full deployment including controller pod)
deploy: deploy-rbac
	kubectl apply -f deploy/deployment.yaml

# Undeploy from hub cluster
undeploy:
	kubectl delete -f deploy/ --ignore-not-found

# Enable addon for a cluster (usage: make enable CLUSTER=cluster1)
enable:
	@if [ -z "$(CLUSTER)" ]; then echo "Usage: make enable CLUSTER=<cluster-name>"; exit 1; fi
	kubectl apply -f - <<EOF
	apiVersion: addon.open-cluster-management.io/v1alpha1
	kind: ManagedClusterAddOn
	metadata:
	  name: basic-addon
	  namespace: $(CLUSTER)
	spec:
	  installNamespace: open-cluster-management-agent-addon
	EOF

# Disable addon for a cluster (usage: make disable CLUSTER=cluster1)
disable:
	@if [ -z "$(CLUSTER)" ]; then echo "Usage: make disable CLUSTER=<cluster-name>"; exit 1; fi
	kubectl delete managedclusteraddon basic-addon -n $(CLUSTER) --ignore-not-found

# Check pod report on hub (usage: make check-report CLUSTER=cluster1)
check-report:
	@if [ -z "$(CLUSTER)" ]; then echo "Usage: make check-report CLUSTER=<cluster-name>"; exit 1; fi
	kubectl get configmap pod-report -n $(CLUSTER) -o jsonpath='{.data.report}' | jq .

# Build docker image and load into Kind clusters (hub + spoke)
kind-load:
	cd .. && docker build -t $(IMAGE) -f addon-framework-basic/Dockerfile .
	kind load docker-image $(IMAGE) --name $(KIND_HUB)
	kind load docker-image $(IMAGE) --name $(KIND_SPOKE)

# Apply all specs to hub cluster and restart agents on spokes
addon-deploy:
	KUBECONFIG=$(HUB_KUBECONFIG) kubectl apply -f deploy/
	KUBECONFIG=$(HUB_KUBECONFIG) kubectl rollout restart deployment/basic-addon-controller -n open-cluster-management
	@echo "Waiting for controller to be ready..."
	@KUBECONFIG=$(HUB_KUBECONFIG) kubectl wait --for=condition=available deployment/basic-addon-controller -n open-cluster-management --timeout=60s
	@echo "Restarting agent on spoke..."
	-KUBECONFIG=$$(eval echo $(SPOKE_KUBECONFIG)) kubectl rollout restart deployment/basic-addon-agent -n open-cluster-management-agent-addon 2>/dev/null || true
	-KUBECONFIG=$$(eval echo $(SPOKE_KUBECONFIG)) kubectl rollout status deployment/basic-addon-agent -n open-cluster-management-agent-addon --timeout=60s 2>/dev/null || true

# List all pod reports from all spokes
addon-reports:
	@echo "=== Pod Reports from all Spokes ==="
	@for ns in $$(KUBECONFIG=$(HUB_KUBECONFIG) kubectl get managedclusteraddon -A -o jsonpath='{.items[*].metadata.namespace}' 2>/dev/null); do \
		echo "--- $$ns ---"; \
		KUBECONFIG=$(HUB_KUBECONFIG) kubectl get configmap pod-report -n $$ns -o jsonpath='{.data.report}' 2>/dev/null | jq -c '{cluster: .clusterName, totalPods: .totalPods, timestamp: .timestamp}' 2>/dev/null || echo "No report"; \
	done

# Show detailed report for a specific cluster (usage: make addon-report CLUSTER=spoke1)
addon-report:
	@if [ -z "$(CLUSTER)" ]; then echo "Usage: make addon-report CLUSTER=<cluster-name>"; exit 1; fi
	@KUBECONFIG=$(HUB_KUBECONFIG) kubectl get configmap pod-report -n $(CLUSTER) -o jsonpath='{.data.report}' | jq .

# Full setup: build image, load into Kind clusters, deploy everything
test-addon-operator-kind-prepare: kind-load addon-deploy
	@echo "=== Addon operator deployed ==="
	@echo "Waiting for controller to be ready..."
	@KUBECONFIG=$(HUB_KUBECONFIG) kubectl wait --for=condition=available deployment/basic-addon-controller -n open-cluster-management --timeout=60s
	@echo "Controller ready. Checking placement..."
	@KUBECONFIG=$(HUB_KUBECONFIG) kubectl get placement -n open-cluster-management
	@echo "Checking managed cluster addons..."
	@KUBECONFIG=$(HUB_KUBECONFIG) kubectl get managedclusteraddon -A
	@echo ""
	@echo "Done! Use 'make addon-reports' to check pod reports from spokes."
