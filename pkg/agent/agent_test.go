package agent

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildPodReport(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		pods        []corev1.Pod
		wantPods    int
	}{
		{
			name:        "empty pod list",
			clusterName: "cluster1",
			pods:        []corev1.Pod{},
			wantPods:    0,
		},
		{
			name:        "single pod",
			clusterName: "cluster1",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
					},
					Spec: corev1.PodSpec{
						NodeName: "node1",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			wantPods: 1,
		},
		{
			name:        "multiple pods",
			clusterName: "cluster2",
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "kube-system",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodPending,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod3",
						Namespace: "production",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodFailed,
					},
				},
			},
			wantPods: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := buildPodReport(tt.clusterName, tt.pods)

			if report.ClusterName != tt.clusterName {
				t.Errorf("ClusterName = %s, want %s", report.ClusterName, tt.clusterName)
			}

			if report.TotalPods != tt.wantPods {
				t.Errorf("TotalPods = %d, want %d", report.TotalPods, tt.wantPods)
			}

			if len(report.Pods) != tt.wantPods {
				t.Errorf("len(Pods) = %d, want %d", len(report.Pods), tt.wantPods)
			}

			if report.Timestamp.IsZero() {
				t.Error("Timestamp should not be zero")
			}

			if report.Timestamp.After(time.Now()) {
				t.Error("Timestamp should not be in the future")
			}
		})
	}
}

func TestBuildPodReportPodInfo(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-pod",
				Namespace: "my-namespace",
			},
			Spec: corev1.PodSpec{
				NodeName: "my-node",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
			},
		},
	}

	report := buildPodReport("test-cluster", pods)

	if len(report.Pods) != 1 {
		t.Fatalf("expected 1 pod info, got %d", len(report.Pods))
	}

	podInfo := report.Pods[0]

	if podInfo.Name != "my-pod" {
		t.Errorf("PodInfo.Name = %s, want my-pod", podInfo.Name)
	}

	if podInfo.Namespace != "my-namespace" {
		t.Errorf("PodInfo.Namespace = %s, want my-namespace", podInfo.Namespace)
	}

	if podInfo.Status != "Running" {
		t.Errorf("PodInfo.Status = %s, want Running", podInfo.Status)
	}

	if podInfo.NodeName != "my-node" {
		t.Errorf("PodInfo.NodeName = %s, want my-node", podInfo.NodeName)
	}
}

func TestNewAgentOptions(t *testing.T) {
	opts := NewAgentOptions("test-addon")

	if opts.AddonName != "test-addon" {
		t.Errorf("AddonName = %s, want test-addon", opts.AddonName)
	}

	if opts.HubKubeconfigFile != "" {
		t.Errorf("HubKubeconfigFile should be empty by default")
	}

	if opts.SpokeClusterName != "" {
		t.Errorf("SpokeClusterName should be empty by default")
	}

	if opts.AddonNamespace != "" {
		t.Errorf("AddonNamespace should be empty by default")
	}
}
