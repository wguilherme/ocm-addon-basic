package hub

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestAddonRBACWithNilConfig(t *testing.T) {
	// When kubeConfig is nil, it should return nil without error
	permissionFunc := AddonRBAC(nil)

	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-cluster",
		},
	}

	addon := &addonapiv1alpha1.ManagedClusterAddOn{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "basic-addon",
			Namespace: "test-cluster",
		},
	}

	err := permissionFunc(cluster, addon)
	if err != nil {
		t.Errorf("AddonRBAC with nil config should return nil, got: %v", err)
	}
}
