package addon

import (
	"os"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestGetDefaultValues(t *testing.T) {
	// Arrange
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
	}
	addon := &addonapiv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{Name: "basic-addon", Namespace: "cluster1"},
	}

	// Act
	values, err := GetDefaultValues(cluster, addon)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if values["ClusterName"] != "cluster1" {
		t.Errorf("ClusterName = %v, want cluster1", values["ClusterName"])
	}
	if values["Image"] != DefaultImage {
		t.Errorf("Image = %v, want %v", values["Image"], DefaultImage)
	}
}

func TestGetDefaultValuesCustomImage(t *testing.T) {
	// Arrange
	os.Setenv("ADDON_IMAGE", "custom:v1")
	defer os.Unsetenv("ADDON_IMAGE")

	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
	}
	addon := &addonapiv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{Name: "basic-addon", Namespace: "cluster1"},
	}

	// Act
	values, err := GetDefaultValues(cluster, addon)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if values["Image"] != "custom:v1" {
		t.Errorf("Image = %v, want custom:v1", values["Image"])
	}
}

func TestAgentHealthProber(t *testing.T) {
	// Act
	prober := AgentHealthProber()

	// Assert
	if prober.Type != agent.HealthProberTypeLease {
		t.Errorf("Type = %v, want Lease", prober.Type)
	}
}
