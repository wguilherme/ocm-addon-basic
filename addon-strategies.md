# OCM Addon - Estratégias de Comunicação Spoke → Hub

Este addon implementa as 5 estratégias de comunicação spoke→hub do Open Cluster Management.

## Quick Start

```bash
# Build, load e deploy completo
make kind-load && make addon-deploy
```

---

## Estratégia 1: ConfigMap

**Comando (HUB):**
```bash
make addon-reports
```

**O que faz:** Lista o relatório de pods de todos os spokes.

**Output esperado:**
```
=== Pod Reports from all Spokes ===
--- spoke1-sftm ---
{"cluster":"spoke1-sftm","totalPods":21,"timestamp":"2026-01-19T22:56:33Z"}
```

**Comando detalhado (HUB):**
```bash
kubectl get configmap pod-report -n spoke1-sftm -o jsonpath='{.data.report}' | jq .
```

---

## Estratégia 2: ManagedClusterAddOn Status

**Comando (HUB):**
```bash
kubectl get managedclusteraddon basic-addon -n spoke1-sftm -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="PodCountHealthy")'
```

**O que faz:** Mostra a condition customizada que indica se o cluster tem menos de 100 pods.

**Output esperado (cluster saudável):**
```json
{
  "lastTransitionTime": "2026-01-19T22:56:33Z",
  "message": "Cluster has 21 pods (healthy)",
  "reason": "PodCountWithinLimit",
  "status": "True",
  "type": "PodCountHealthy"
}
```

**Output esperado (cluster com muitos pods):**
```json
{
  "type": "PodCountHealthy",
  "status": "False",
  "reason": "PodCountExceedsLimit",
  "message": "Cluster has 150 pods (exceeds 100 limit)"
}
```

---

## Estratégia 3: AddOnPlacementScore

**Comando (HUB):**
```bash
kubectl get addonplacementscore basic-addon-score -n spoke1-sftm -o yaml
```

**O que faz:** Mostra métricas numéricas que podem ser usadas pelo Placement para ordenação de clusters.

**Output esperado:**
```yaml
apiVersion: cluster.open-cluster-management.io/v1alpha1
kind: AddOnPlacementScore
metadata:
  name: basic-addon-score
  namespace: spoke1-sftm
status:
  validUntil: "2026-01-19T22:58:33Z"
  scores:
    - name: namespaceCount
      value: 60           # Normalized: 100 - (10 * 200 / 50) = 60
    - name: podCount
      value: 79           # Normalized: 100 - (21 * 200 / 200) = 79
```

**Nota:** Os scores são normalizados para o range [-100, 100] conforme exigido pelo OCM.
Quanto menor o número de recursos, maior o score (mais capacidade disponível).

**Uso em Placement (exemplo):**
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
        weight: 1
```

---

## Estratégia 4: ClusterClaim

**Comando (HUB):**
```bash
kubectl get managedcluster spoke1-sftm -o jsonpath='{.status.clusterClaims}' | jq '.[] | select(.name | startswith("basic-addon"))'
```

**O que faz:** Mostra claims customizados do cluster (sincronizados automaticamente do spoke pelo klusterlet).

**Output esperado:**
```json
{
  "name": "basic-addon.k8s-version",
  "value": "v1.34.0"
}
```

**Comando (SPOKE) - ver claim original:**
```bash
kubectl get clusterclaim basic-addon.k8s-version -o yaml
```

---

## Estratégia 5: Work Status Feedback

**Comando (HUB):**
```bash
kubectl get manifestwork -n spoke1-sftm -l open-cluster-management.io/addon-name=basic-addon -o jsonpath='{.items[0].status.resourceStatus.manifests}' | jq '.[] | select(.resourceMeta.name=="basic-addon-agent") | .statusFeedback'
```

**O que faz:** Mostra valores extraídos do deployment do agent via JSONPath (configurado no controller, não no agent).

**Output esperado:**
```json
{
  "values": [
    {
      "fieldValue": {
        "integer": 1,
        "type": "Integer"
      },
      "name": "readyReplicas"
    },
    {
      "fieldValue": {
        "integer": 1,
        "type": "Integer"
      },
      "name": "availableReplicas"
    },
    {
      "fieldValue": {
        "integer": 1,
        "type": "Integer"
      },
      "name": "replicas"
    }
  ]
}
```

---

## Resumo Comparativo

| # | Estratégia | Dado | Score Range | Código Agent | Verificação |
|---|------------|------|-------------|--------------|-------------|
| 1 | ConfigMap | Lista pods | N/A | Sim | `make addon-reports` |
| 2 | AddOn Status | Pod count | N/A | Sim | `kubectl get managedclusteraddon ... -o jsonpath='{.status.conditions}'` |
| 3 | PlacementScore | NS/Pod count | [-100, 100] | Sim | `kubectl get addonplacementscore ...` |
| 4 | ClusterClaim | K8s version | N/A | Sim | `kubectl get managedcluster ... -o jsonpath='{.status.clusterClaims}'` |
| 5 | Work Feedback | Replicas | N/A | Não | `kubectl get manifestwork ... -o jsonpath='{...resourceStatus...}'` |

**Referência:** [OCM Addon Developer Guide](https://open-cluster-management.io/docs/developer-guides/addon/)

---

## Troubleshooting

**Agent não está reportando (SPOKE):**
```bash
kubectl logs -n open-cluster-management-agent-addon deployment/basic-addon-agent --tail=30
```

**Addon não está Available (HUB):**
```bash
kubectl get managedclusteraddon basic-addon -n spoke1-sftm -o yaml
```

**ManifestWork não aplicado (HUB):**
```bash
kubectl get manifestwork -n spoke1-sftm -l open-cluster-management.io/addon-name=basic-addon -o yaml
```

**Controller não está criando Role/RoleBinding (HUB):**
```bash
kubectl logs -n open-cluster-management deployment/basic-addon-controller --tail=50
```

**Permissões RBAC faltando (HUB):**
```bash
kubectl get role -n spoke1-sftm open-cluster-management:basic-addon:agent -o yaml
```
