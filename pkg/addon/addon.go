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
	workapiv1 "open-cluster-management.io/api/work/v1"

	"github.com/totvs/addon-framework-basic/pkg/hub"
)

const (
	AddonName                    = "basic-addon"
	DefaultBasicAddonImage       = "basic-addon:latest"
	InstallationNamespace        = "open-cluster-management-agent-addon"
)

//go:embed manifests
//go:embed manifests/templates
var FS embed.FS

// NewRegistrationOption returns the registration option for the addon agent.
// This enables the agent to get a kubeconfig to communicate with the hub.
func NewRegistrationOption(kubeConfig *rest.Config, addonName, agentName string) *agent.RegistrationOption {
	return &agent.RegistrationOption{
		CSRConfigurations: agent.KubeClientSignerConfigurations(addonName, agentName),
		CSRApproveCheck:   utils.DefaultCSRApprover(agentName),
		PermissionConfig:  hub.AddonRBAC(kubeConfig),
	}
}

// GetDefaultValues returns the default values for the addon manifests.
// These values are injected into the Go templates.
func GetDefaultValues(cluster *clusterv1.ManagedCluster,
	addon *addonapiv1alpha1.ManagedClusterAddOn) (addonfactory.Values, error) {

	image := os.Getenv("ADDON_IMAGE")
	if len(image) == 0 {
		image = DefaultBasicAddonImage
	}

	manifestConfig := struct {
		KubeConfigSecret string
		ClusterName      string
		Image            string
	}{
		KubeConfigSecret: fmt.Sprintf("%s-hub-kubeconfig", addon.Name),
		ClusterName:      cluster.Name,
		Image:            image,
	}

	return addonfactory.StructToValues(manifestConfig), nil
}

// AgentHealthProber returns the health prober configuration for the addon.
// Uses WorkProber with FeedbackRules to demonstrate Strategy 5: Work Status Feedback.
// This extracts readyReplicas and availableReplicas from the agent deployment.
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
								{Name: "replicas", Path: ".status.replicas"},
							},
						},
					},
				},
			},
			HealthChecker: func(fields []agent.FieldResult, cluster *clusterv1.ManagedCluster,
				addon *addonapiv1alpha1.ManagedClusterAddOn) error {
				for _, field := range fields {
					if field.ResourceIdentifier.Name != "basic-addon-agent" {
						continue
					}
					for _, value := range field.FeedbackResult.Values {
						if value.Name == "readyReplicas" && value.Value.Integer != nil && *value.Value.Integer >= 1 {
							return nil
						}
					}
				}
				return fmt.Errorf("basic-addon agent is not ready")
			},
		},
	}
}
