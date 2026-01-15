IMAGE ?= basic-addon:latest

.PHONY: build run docker-build docker-push deploy deploy-rbac undeploy tidy enable disable

# Build binary locally
build:
	go build -o bin/addon ./cmd/addon

# Run locally (like kubebuilder's make run)
# Prerequisites: kubectl context pointing to hub cluster, RBAC applied
run: build
	./bin/addon

# Download dependencies
tidy:
	go mod tidy

# Build docker image
docker-build:
	docker build -t $(IMAGE) .

# Push docker image
docker-push:
	docker push $(IMAGE)

# Deploy RBAC only (for local development with make run)
deploy-rbac:
	kubectl apply -f deploy/serviceaccount.yaml
	kubectl apply -f deploy/clusterrole.yaml
	kubectl apply -f deploy/clusterrolebinding.yaml
	kubectl apply -f deploy/clustermanagementaddon.yaml

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
