package addon

import (
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager/addontesting"
)

func TestGetDefaultValues(t *testing.T) {
	tests := []struct {
		name           string
		cluster        *clusterv1.ManagedCluster
		addon          *addonapiv1alpha1.ManagedClusterAddOn
		envImage       string
		expectedValues map[string]interface{}
	}{
		{
			name:    "default values",
			cluster: newManagedCluster("cluster1"),
			addon:   newManagedClusterAddOn("basic-addon", "cluster1"),
			expectedValues: map[string]interface{}{
				"KubeConfigSecret": "basic-addon-hub-kubeconfig",
				"ClusterName":      "cluster1",
				"Image":            DefaultBasicAddonImage,
			},
		},
		{
			name:     "custom image from env",
			cluster:  newManagedCluster("cluster2"),
			addon:    newManagedClusterAddOn("basic-addon", "cluster2"),
			envImage: "myregistry/basic-addon:v1.0.0",
			expectedValues: map[string]interface{}{
				"KubeConfigSecret": "basic-addon-hub-kubeconfig",
				"ClusterName":      "cluster2",
				"Image":            "myregistry/basic-addon:v1.0.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envImage != "" {
				os.Setenv("ADDON_IMAGE", tt.envImage)
				defer os.Unsetenv("ADDON_IMAGE")
			}

			values, err := GetDefaultValues(tt.cluster, tt.addon)
			if err != nil {
				t.Fatalf("GetDefaultValues() error = %v", err)
			}

			for key, expected := range tt.expectedValues {
				if values[key] != expected {
					t.Errorf("GetDefaultValues()[%s] = %v, want %v", key, values[key], expected)
				}
			}
		})
	}
}

func TestAgentHealthProber(t *testing.T) {
	prober := AgentHealthProber()

	if prober == nil {
		t.Fatal("AgentHealthProber() returned nil")
	}

	// Changed from DeploymentAvailability to Work to support Strategy 5: Work Status Feedback
	if prober.Type != "Work" {
		t.Errorf("AgentHealthProber().Type = %v, want Work", prober.Type)
	}

	if prober.WorkProber == nil {
		t.Fatal("AgentHealthProber().WorkProber is nil, expected WorkHealthProber")
	}

	if len(prober.WorkProber.ProbeFields) == 0 {
		t.Error("AgentHealthProber().WorkProber.ProbeFields is empty")
	}

	// Verify the deployment probe is configured
	found := false
	for _, field := range prober.WorkProber.ProbeFields {
		if field.ResourceIdentifier.Name == "basic-addon-agent" {
			found = true
			if len(field.ProbeRules) == 0 {
				t.Error("ProbeRules for basic-addon-agent is empty")
			}
		}
	}
	if !found {
		t.Error("ProbeField for basic-addon-agent not found")
	}
}

func TestManifestAddonAgent(t *testing.T) {
	tests := []struct {
		name             string
		cluster          *clusterv1.ManagedCluster
		addon            *addonapiv1alpha1.ManagedClusterAddOn
		verifyDeployment func(t *testing.T, objs []runtime.Object)
	}{
		{
			name:    "generates correct manifests",
			cluster: addontesting.NewManagedCluster("cluster1"),
			addon:   addontesting.NewAddon("basic-addon", "cluster1"),
			verifyDeployment: func(t *testing.T, objs []runtime.Object) {
				if len(objs) != 3 {
					t.Fatalf("expected 3 manifests (deployment, sa, clusterrolebinding), got %d", len(objs))
				}

				deployment := findDeployment(objs)
				if deployment == nil {
					t.Fatal("expected deployment in manifests")
				}

				if deployment.Name != "basic-addon-agent" {
					t.Errorf("deployment name = %s, want basic-addon-agent", deployment.Name)
				}

				if deployment.Namespace != addonfactory.AddonDefaultInstallNamespace {
					t.Errorf("deployment namespace = %s, want %s", deployment.Namespace, addonfactory.AddonDefaultInstallNamespace)
				}

				sa := findServiceAccount(objs)
				if sa == nil {
					t.Fatal("expected serviceaccount in manifests")
				}

				if sa.Name != "basic-addon-agent-sa" {
					t.Errorf("serviceaccount name = %s, want basic-addon-agent-sa", sa.Name)
				}

				crb := findClusterRoleBinding(objs)
				if crb == nil {
					t.Fatal("expected clusterrolebinding in manifests")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agentAddon, err := addonfactory.NewAgentAddonFactory(AddonName, FS, "manifests/templates").
				WithGetValuesFuncs(GetDefaultValues).
				WithAgentRegistrationOption(NewRegistrationOption(nil, AddonName, "test-agent")).
				WithAgentHealthProber(AgentHealthProber()).
				BuildTemplateAgentAddon()
			if err != nil {
				t.Fatalf("failed to build agent addon: %v", err)
			}

			objects, err := agentAddon.Manifests(tt.cluster, tt.addon)
			if err != nil {
				t.Fatalf("failed to get manifests: %v", err)
			}

			tt.verifyDeployment(t, objects)
		})
	}
}

func newManagedCluster(name string) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func newManagedClusterAddOn(name, namespace string) *addonapiv1alpha1.ManagedClusterAddOn {
	return &addonapiv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func findDeployment(objs []runtime.Object) *appsv1.Deployment {
	for _, obj := range objs {
		if deployment, ok := obj.(*appsv1.Deployment); ok {
			return deployment
		}
	}
	return nil
}

func findServiceAccount(objs []runtime.Object) *corev1.ServiceAccount {
	for _, obj := range objs {
		if sa, ok := obj.(*corev1.ServiceAccount); ok {
			return sa
		}
	}
	return nil
}

func findClusterRoleBinding(objs []runtime.Object) *rbacv1.ClusterRoleBinding {
	for _, obj := range objs {
		if crb, ok := obj.(*rbacv1.ClusterRoleBinding); ok {
			return crb
		}
	}
	return nil
}
