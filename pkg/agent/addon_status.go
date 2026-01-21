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

var addonGVR = schema.GroupVersionResource{
	Group:    "addon.open-cluster-management.io",
	Version:  "v1alpha1",
	Resource: "managedclusteraddons",
}

// syncAddonStatus updates the ManagedClusterAddOn status with pod count condition.
// This demonstrates how agents can report health/status back to hub via addon conditions.
func (o *AgentOptions) syncAddonStatus(ctx context.Context, spokeClient kubernetes.Interface, hubDynamicClient dynamic.Interface) error {
	klog.V(4).Info("Syncing addon status")

	// Count pods in spoke
	podList, err := spokeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}
	podCount := len(podList.Items)

	// Get current ManagedClusterAddOn
	addon, err := hubDynamicClient.Resource(addonGVR).Namespace(o.SpokeClusterName).Get(ctx, o.AddonName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get addon: %w", err)
	}

	// Determine condition based on pod count
	conditionStatus := metav1.ConditionTrue
	reason := "PodCountWithinLimit"
	message := fmt.Sprintf("Cluster has %d pods (healthy)", podCount)
	if podCount > 100 {
		conditionStatus = metav1.ConditionFalse
		reason = "PodCountExceedsLimit"
		message = fmt.Sprintf("Cluster has %d pods (exceeds 100 limit)", podCount)
	}

	// Build condition
	now := metav1.Now()
	newCondition := map[string]interface{}{
		"type":               "PodCountHealthy",
		"status":             string(conditionStatus),
		"reason":             reason,
		"message":            message,
		"lastTransitionTime": now.Format("2006-01-02T15:04:05Z"),
	}

	// Get or create status.conditions
	status, found, _ := unstructured.NestedMap(addon.Object, "status")
	if !found {
		status = make(map[string]interface{})
	}

	conditions, found, _ := unstructured.NestedSlice(status, "conditions")
	if !found {
		conditions = []interface{}{}
	}

	// Update or append our condition
	updated := false
	for i, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "PodCountHealthy" {
			conditions[i] = newCondition
			updated = true
			break
		}
	}
	if !updated {
		conditions = append(conditions, newCondition)
	}

	// Set conditions back
	status["conditions"] = conditions
	addon.Object["status"] = status

	// Update status subresource
	_, err = hubDynamicClient.Resource(addonGVR).Namespace(o.SpokeClusterName).UpdateStatus(ctx, addon, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update addon status: %w", err)
	}

	klog.Infof("Updated addon status with PodCountHealthy condition: %s (%d pods)", reason, podCount)
	return nil
}
