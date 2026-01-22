IMAGE ?= basic-addon:latest

.PHONY: build run test tidy docker-build deploy undeploy enable disable check-report

build:
	go build -o bin/addon ./cmd/addon

run: build
	./bin/addon controller

test:
	go test ./pkg/... -v -cover

tidy:
	go mod tidy

docker-build:
	docker build -t $(IMAGE) .

deploy:
	kubectl apply -f deploy/

undeploy:
	kubectl delete -f deploy/ --ignore-not-found

enable:
	@if [ -z "$(CLUSTER)" ]; then echo "Usage: make enable CLUSTER=<cluster-name>"; exit 1; fi
	@kubectl apply -f - <<EOF
	apiVersion: addon.open-cluster-management.io/v1alpha1
	kind: ManagedClusterAddOn
	metadata:
	  name: basic-addon
	  namespace: $(CLUSTER)
	spec:
	  installNamespace: open-cluster-management-agent-addon
	EOF

disable:
	@if [ -z "$(CLUSTER)" ]; then echo "Usage: make disable CLUSTER=<cluster-name>"; exit 1; fi
	kubectl delete managedclusteraddon basic-addon -n $(CLUSTER) --ignore-not-found

check-report:
	@if [ -z "$(CLUSTER)" ]; then echo "Usage: make check-report CLUSTER=<cluster-name>"; exit 1; fi
	kubectl get configmap pod-report -n $(CLUSTER) -o jsonpath='{.data.report}' | jq .
