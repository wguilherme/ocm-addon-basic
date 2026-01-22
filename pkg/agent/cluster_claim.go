package agent

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

var claimGVR = schema.GroupVersionResource{
	Group:    "cluster.open-cluster-management.io",
	Version:  "v1alpha1",
	Resource: "clusterclaims",
}

const ClusterClaimName = "basic-addon.k8s-version"

// syncClusterClaim creates ManagedClusterClaim in the SPOKE cluster.
// The klusterlet automatically syncs claims to the hub's ManagedCluster status.
// This demonstrates how agents can expose cluster properties.
func (o *AgentOptions) syncClusterClaim(ctx context.Context, spokeClient kubernetes.Interface, spokeDynamicClient dynamic.Interface) error {
	klog.V(4).Info("Syncing cluster claim")

	// Get Kubernetes version from spoke
	serverVersion, err := spokeClient.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("failed to get server version: %w", err)
	}

	// Build ClusterClaim (applied to SPOKE, not hub!)
	claim := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1alpha1",
			"kind":       "ClusterClaim",
			"metadata": map[string]interface{}{
				"name": ClusterClaimName,
				"labels": map[string]interface{}{
					"app": "basic-addon",
				},
			},
			"spec": map[string]interface{}{
				"value": serverVersion.GitVersion,
			},
		},
	}

	// Check if exists
	existing, err := spokeDynamicClient.Resource(claimGVR).Get(ctx, ClusterClaimName, metav1.GetOptions{})
	if err == nil {
		// Update existing
		claim.SetResourceVersion(existing.GetResourceVersion())
		_, err = spokeDynamicClient.Resource(claimGVR).Update(ctx, claim, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update cluster claim: %w", err)
		}
		klog.Infof("Updated ClusterClaim %s: %s", ClusterClaimName, serverVersion.GitVersion)
	} else {
		// Create new
		_, err = spokeDynamicClient.Resource(claimGVR).Create(ctx, claim, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create cluster claim: %w", err)
		}
		klog.Infof("Created ClusterClaim %s: %s", ClusterClaimName, serverVersion.GitVersion)
	}

	return nil
}
