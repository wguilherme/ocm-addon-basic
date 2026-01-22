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

var scoreGVR = schema.GroupVersionResource{
	Group:    "cluster.open-cluster-management.io",
	Version:  "v1alpha1",
	Resource: "addonplacementscores",
}

const PlacementScoreName = "basic-addon-score"

// normalizeScore converts a count to a score in [-100, 100] range.
// Fewer resources = higher score (more capacity available).
// score = 100 - (count * 200 / maxExpected), clamped to [-100, 100]
func normalizeScore(count, maxExpected int) int64 {
	// Calculate score: 100 when count=0, -100 when count=maxExpected
	score := 100 - (count * 200 / maxExpected)
	// Clamp to valid range
	if score > 100 {
		score = 100
	}
	if score < -100 {
		score = -100
	}
	return int64(score)
}

// syncPlacementScore creates/updates AddOnPlacementScore with namespace count.
// This demonstrates how agents can publish metrics for Placement decisions.
func (o *AgentOptions) syncPlacementScore(ctx context.Context, spokeClient kubernetes.Interface, hubDynamicClient dynamic.Interface) error {
	klog.V(4).Info("Syncing placement score")

	// Count namespaces in spoke
	nsList, err := spokeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}
	namespaceCount := len(nsList.Items)

	// Count pods for another metric
	podList, err := spokeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}
	podCount := len(podList.Items)

	// Build AddOnPlacementScore
	score := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1alpha1",
			"kind":       "AddOnPlacementScore",
			"metadata": map[string]interface{}{
				"name":      PlacementScoreName,
				"namespace": o.SpokeClusterName,
			},
		},
	}

	// Check if exists
	existing, err := hubDynamicClient.Resource(scoreGVR).Namespace(o.SpokeClusterName).Get(ctx, PlacementScoreName, metav1.GetOptions{})
	if err == nil {
		// Update existing - preserve resourceVersion
		score.SetResourceVersion(existing.GetResourceVersion())
	}

	// Set status with scores
	// OCM requires scores in range [-100, 100]
	// We normalize: fewer pods/namespaces = higher score (more capacity)
	// Using inverse: score = 100 - (count * 100 / maxExpected)
	namespaceScore := normalizeScore(namespaceCount, 50)  // max 50 namespaces expected
	podScore := normalizeScore(podCount, 200)              // max 200 pods expected

	now := metav1.Now()
	status := map[string]interface{}{
		"validUntil": now.Add(SyncInterval * 2).Format("2006-01-02T15:04:05Z"),
		"scores": []interface{}{
			map[string]interface{}{
				"name":  "namespaceCount",
				"value": int64(namespaceScore),
			},
			map[string]interface{}{
				"name":  "podCount",
				"value": int64(podScore),
			},
		},
	}
	score.Object["status"] = status

	// Create or update
	if existing == nil {
		_, err = hubDynamicClient.Resource(scoreGVR).Namespace(o.SpokeClusterName).Create(ctx, score, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create placement score: %w", err)
		}
		klog.Infof("Created AddOnPlacementScore: namespaceCount=%d (score=%d), podCount=%d (score=%d)", namespaceCount, namespaceScore, podCount, podScore)
	} else {
		_, err = hubDynamicClient.Resource(scoreGVR).Namespace(o.SpokeClusterName).Update(ctx, score, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update placement score: %w", err)
		}
		klog.Infof("Updated AddOnPlacementScore: namespaceCount=%d (score=%d), podCount=%d (score=%d)", namespaceCount, namespaceScore, podCount, podScore)
	}

	// Update status subresource
	_, err = hubDynamicClient.Resource(scoreGVR).Namespace(o.SpokeClusterName).UpdateStatus(ctx, score, metav1.UpdateOptions{})
	if err != nil {
		klog.V(4).Infof("Failed to update placement score status (may not support status subresource): %v", err)
	}

	return nil
}
