// Package addon contém a factory do addon (controller no hub).
package addon

import (
	"embed"
	"fmt"
	"os"

	"k8s.io/client-go/rest"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/addon-framework/pkg/utils"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/totvs/addon-framework-basic/pkg/hub"
)

const (
	AddonName             = "basic-addon"
	DefaultImage          = "basic-addon:latest"
	InstallationNamespace = "open-cluster-management-agent-addon"
)

// FS contém os templates embarcados (manifests/templates).
//
//go:embed manifests
//go:embed manifests/templates
var FS embed.FS

// NewRegistrationOption configura o registro do agent no hub.
// Fluxo: CSR criado → aprovado via CSRApproveCheck → registration-agent gera kubeconfig
func NewRegistrationOption(kubeConfig *rest.Config, addonName, agentName string) *agent.RegistrationOption {
	return &agent.RegistrationOption{
		CSRConfigurations: agent.KubeClientSignerConfigurations(addonName, agentName),
		CSRApproveCheck:   utils.DefaultCSRApprover(agentName), // aprova automaticamente
		PermissionConfig:  hub.AddonRBAC(kubeConfig),           // cria Role/RoleBinding no hub
	}
}

// GetDefaultValues retorna valores para renderizar os templates.
// Campos: {{ .KubeConfigSecret }}, {{ .ClusterName }}, {{ .Image }}, {{ .AddonInstallNamespace }}
func GetDefaultValues(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn) (addonfactory.Values, error) {

	image := os.Getenv("ADDON_IMAGE")
	if image == "" {
		image = DefaultImage
	}

	return addonfactory.StructToValues(struct {
		KubeConfigSecret string
		ClusterName      string
		Image            string
	}{
		KubeConfigSecret: fmt.Sprintf("%s-hub-kubeconfig", addon.Name),
		ClusterName:      cluster.Name,
		Image:            image,
	}), nil
}

// AgentHealthProber retorna o health prober usando Lease.
// O agent atualiza o Lease no spoke, registration-agent observa e reporta status pro hub.
func AgentHealthProber() *agent.HealthProber {
	return &agent.HealthProber{
		Type: agent.HealthProberTypeLease,
	}
}
