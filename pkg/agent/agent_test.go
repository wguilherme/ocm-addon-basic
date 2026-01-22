package agent

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildReport(t *testing.T) {
	// Arrange
	o := &AgentOptions{SpokeClusterName: "cluster1"}
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "kube-system"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
	}

	// Act
	report := o.buildReport(pods)

	// Assert
	if report.ClusterName != "cluster1" {
		t.Errorf("ClusterName = %s, want cluster1", report.ClusterName)
	}
	if report.TotalPods != 2 {
		t.Errorf("TotalPods = %d, want 2", report.TotalPods)
	}
	if len(report.Pods) != 2 {
		t.Errorf("len(Pods) = %d, want 2", len(report.Pods))
	}
	if report.Pods[0].Name != "pod1" {
		t.Errorf("Pods[0].Name = %s, want pod1", report.Pods[0].Name)
	}
}
