# OCM Spoke → Hub Communication Strategies

Este documento descreve as 5 estratégias implementadas neste addon para comunicação spoke→hub no Open Cluster Management (OCM).

## Visão Geral

| # | Estratégia | Dado Exemplo | Código no Agent | Onde Aplica |
|---|------------|--------------|-----------------|-------------|
| 1 | ConfigMap | Lista de pods | Sim | Hub |
| 2 | ManagedClusterAddOn Status | Pod count condition | Sim | Hub |
| 3 | AddOnPlacementScore | Namespace/pod count | Sim | Hub |
| 4 | ManagedClusterClaim | K8s version | Sim | Spoke |
| 5 | Work Status Feedback | Replicas ready | Não (spec) | ManifestWork |

---

## Estratégia 1: ConfigMap

**Arquivo**: `pkg/agent/agent.go` (função `syncPodReport`)

**O que é**: Cria/atualiza um ConfigMap no namespace do cluster no hub com dados arbitrários.

**Quando usar**: Para dados estruturados maiores que não cabem em conditions ou scores.

**Exemplo de dado**: Lista completa de pods do cluster.

```go
configMap := &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "pod-report",
        Namespace: o.SpokeClusterName, // namespace do cluster no hub
    },
    Data: map[string]string{
        "report": string(reportJSON),
    },
}
hubClient.CoreV1().ConfigMaps(o.SpokeClusterName).Create(ctx, configMap, ...)
```

**Verificação**:
```bash
kubectl get configmap pod-report -n <cluster-name> -o jsonpath='{.data.report}' | jq .
# ou
make addon-reports
```

---

## Estratégia 2: ManagedClusterAddOn Status

**Arquivo**: `pkg/agent/addon_status.go`

**O que é**: Atualiza conditions no status do ManagedClusterAddOn para indicar estado/saúde customizada.

**Quando usar**: Para indicadores de saúde/estado que o hub deve monitorar.

**Exemplo de dado**: Condition indicando se o cluster tem menos de 100 pods.

```go
newCondition := map[string]interface{}{
    "type":    "PodCountHealthy",
    "status":  "True", // ou "False" se > 100 pods
    "reason":  "PodCountWithinLimit",
    "message": fmt.Sprintf("Cluster has %d pods", podCount),
}
// Adiciona à lista de conditions e atualiza o status
hubDynamicClient.Resource(addonGVR).Namespace(o.SpokeClusterName).UpdateStatus(ctx, addon, ...)
```

**Verificação**:
```bash
kubectl get managedclusteraddon basic-addon -n <cluster-name> -o jsonpath='{.status.conditions}' | jq .
```

---

## Estratégia 3: AddOnPlacementScore

**Arquivo**: `pkg/agent/placement_score.go`

**O que é**: Publica métricas numéricas que o Placement pode usar para ordenação/seleção de clusters.

**Quando usar**: Para influenciar decisões de scheduling baseadas em métricas do cluster.

**Exemplo de dado**: Contagem de namespaces e pods.

```go
score := &unstructured.Unstructured{
    Object: map[string]interface{}{
        "apiVersion": "cluster.open-cluster-management.io/v1alpha1",
        "kind":       "AddOnPlacementScore",
        "metadata": map[string]interface{}{
            "name":      "basic-addon-score",
            "namespace": o.SpokeClusterName,
        },
        "status": map[string]interface{}{
            "scores": []interface{}{
                map[string]interface{}{"name": "namespaceCount", "value": namespaceCount},
                map[string]interface{}{"name": "podCount", "value": podCount},
            },
        },
    },
}
hubDynamicClient.Resource(scoreGVR).Namespace(o.SpokeClusterName).Create(ctx, score, ...)
```

**Verificação**:
```bash
kubectl get addonplacementscore -n <cluster-name> -o yaml
```

**Uso em Placement**:
```yaml
apiVersion: cluster.open-cluster-management.io/v1beta1
kind: Placement
spec:
  prioritizerPolicy:
    configurations:
      - scoreCoordinate:
          type: AddOn
          addOn:
            resourceName: basic-addon-score
            scoreName: podCount
```

---

## Estratégia 4: ManagedClusterClaim (ClusterClaim)

**Arquivo**: `pkg/agent/cluster_claim.go`

**O que é**: Cria claims no SPOKE que são automaticamente sincronizados para o ManagedCluster no hub pelo klusterlet.

**Quando usar**: Para expor propriedades estáticas ou semi-estáticas do cluster.

**Exemplo de dado**: Versão do Kubernetes.

```go
claim := &unstructured.Unstructured{
    Object: map[string]interface{}{
        "apiVersion": "cluster.open-cluster-management.io/v1alpha1",
        "kind":       "ClusterClaim",
        "metadata": map[string]interface{}{
            "name": "basic-addon.k8s-version",
        },
        "spec": map[string]interface{}{
            "value": serverVersion.GitVersion,
        },
    },
}
// Aplica no SPOKE, não no hub!
spokeDynamicClient.Resource(claimGVR).Create(ctx, claim, ...)
```

**Verificação**:
```bash
# No spoke
kubectl get clusterclaim

# No hub (aparece automaticamente no ManagedCluster)
kubectl get managedcluster <cluster-name> -o jsonpath='{.status.clusterClaims}' | jq .
```

---

## Estratégia 5: Work Status Feedback

**Arquivo**: `pkg/addon/addon.go` (função `AgentHealthProber`)

**O que é**: Extrai valores específicos de recursos aplicados via ManifestWork usando JSONPath.

**Diferencial**: NÃO requer código no agent! É configuração no controller.

**Exemplo de dado**: readyReplicas e availableReplicas do deployment do agent.

```go
func AgentHealthProber() *agent.HealthProber {
    return &agent.HealthProber{
        Type: agent.HealthProberTypeWork,
        WorkProber: &agent.WorkHealthProber{
            ProbeFields: []agent.ProbeField{
                {
                    ResourceIdentifier: workapiv1.ResourceIdentifier{
                        Group:     "apps",
                        Resource:  "deployments",
                        Name:      "basic-addon-agent",
                        Namespace: InstallationNamespace,
                    },
                    ProbeRules: []workapiv1.FeedbackRule{
                        {
                            Type: workapiv1.JSONPathsType,
                            JsonPaths: []workapiv1.JsonPath{
                                {Name: "readyReplicas", Path: ".status.readyReplicas"},
                                {Name: "availableReplicas", Path: ".status.availableReplicas"},
                            },
                        },
                    },
                },
            },
            HealthChecker: func(fields []agent.FieldResult, ...) error {
                // Verifica se readyReplicas >= 1
            },
        },
    }
}
```

**Verificação**:
```bash
kubectl get manifestwork -n <cluster-name> -o yaml | grep -A30 resourceStatus
```

---

## Comparativo

| Estratégia | Volume de Dados | Frequência | Complexidade | Uso Principal |
|------------|-----------------|------------|--------------|---------------|
| ConfigMap | Alto | Baixa-Média | Baixa | Relatórios detalhados |
| AddOn Status | Baixo | Média | Baixa | Health conditions |
| PlacementScore | Baixo | Média | Média | Scheduling decisions |
| ClusterClaim | Baixo | Baixa | Baixa | Propriedades estáticas |
| Work Feedback | Baixo | Alta | Nenhuma* | Status de recursos |

\* Work Feedback não requer código no agent.

---

## Testando Todas as Estratégias

```bash
# 1. Build e deploy
make kind-load && make addon-deploy

# 2. Verificar ConfigMap (Estratégia 1)
make addon-reports

# 3. Verificar AddOn Status (Estratégia 2)
kubectl get managedclusteraddon basic-addon -n spoke1-sftm -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="PodCountHealthy")'

# 4. Verificar PlacementScore (Estratégia 3)
kubectl get addonplacementscore -n spoke1-sftm -o yaml

# 5. Verificar ClusterClaim (Estratégia 4)
kubectl get managedcluster spoke1-sftm -o jsonpath='{.status.clusterClaims}' | jq '.[] | select(.name | startswith("basic-addon"))'

# 6. Verificar Work Status Feedback (Estratégia 5)
kubectl get manifestwork -n spoke1-sftm -l open-cluster-management.io/addon-name=basic-addon -o jsonpath='{.items[0].status.resourceStatus}' | jq .
```
