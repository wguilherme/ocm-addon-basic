# Deploy do Basic Addon

## Pré-requisitos

- Kind clusters: `hub` e `spoke1`
- OCM instalado e spoke registrado no hub

## 1. Build da imagem

```bash
cd /Volumes/SSD/Projects/totvs/TOTVSApps-Management
docker build -t basic-addon:latest -f addon-framework-basic/Dockerfile .
```

## 2. Carregar imagem nos clusters Kind

```bash
kind load docker-image basic-addon:latest --name hub
kind load docker-image basic-addon:latest --name spoke1
```

## 3. Implantar controller no hub

```bash
export KUBECONFIG=$(kind get kubeconfig --name hub)
# ou
kind get kubeconfig --name hub > /tmp/hub-kubeconfig.yaml
export KUBECONFIG=/tmp/hub-kubeconfig.yaml

kubectl apply -f addon-framework-basic/deploy/
```

Verificar:
```bash
kubectl get pods -n open-cluster-management -l app=basic-addon-controller
kubectl logs -n open-cluster-management deployment/basic-addon-controller
```

## 4. Habilitar addon para um spoke

```bash
# Verificar nome do managed cluster
kubectl get managedclusters

# Criar ManagedClusterAddOn (substituir CLUSTER_NAME)
kubectl apply -f - <<EOF
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: basic-addon
  namespace: <CLUSTER_NAME>
spec:
  installNamespace: open-cluster-management-agent-addon
EOF
```

## 5. Verificar agent no spoke

```bash
kind get kubeconfig --name spoke1 > /tmp/spoke1-kubeconfig.yaml
export KUBECONFIG=/tmp/spoke1-kubeconfig.yaml

kubectl get pods -n open-cluster-management-agent-addon
kubectl logs -n open-cluster-management-agent-addon deployment/basic-addon-agent
```

## 6. Verificar pod-report no hub

```bash
export KUBECONFIG=/tmp/hub-kubeconfig.yaml

# Verificar ConfigMap
kubectl get configmap pod-report -n <CLUSTER_NAME>

# Ver conteúdo formatado
kubectl get configmap pod-report -n <CLUSTER_NAME> -o jsonpath='{.data.report}' | jq .
```

## Comandos úteis

```bash
# Status do addon
kubectl get managedclusteraddon -n <CLUSTER_NAME>

# ManifestWork gerado
kubectl get manifestwork -n <CLUSTER_NAME>

# Desabilitar addon
kubectl delete managedclusteraddon basic-addon -n <CLUSTER_NAME>

# Rebuild e redeploy
docker build -t basic-addon:latest -f addon-framework-basic/Dockerfile .
kind load docker-image basic-addon:latest --name hub
kind load docker-image basic-addon:latest --name spoke1
kubectl rollout restart deployment/basic-addon-controller -n open-cluster-management
```
