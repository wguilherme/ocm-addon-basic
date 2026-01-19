package agent

import (
	"context"
	"encoding/json"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	cmdfactory "open-cluster-management.io/addon-framework/pkg/cmd/factory"
	"open-cluster-management.io/addon-framework/pkg/version"
)

const (
	PodReportConfigMapName = "pod-report"
	SyncInterval           = 60 * time.Second
)

// PodReport is the structure sent to the hub with pod information.
// This structure is extensible - add more fields as needed.
type PodReport struct {
	ClusterName string    `json:"clusterName"`
	Timestamp   time.Time `json:"timestamp"`
	TotalPods   int       `json:"totalPods"`
	Pods        []PodInfo `json:"pods"`
}

// PodInfo contains information about a single pod.
// Add more fields as needed for your use case.
type PodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	NodeName  string `json:"nodeName,omitempty"`
}

// NewAgentCommand creates the agent subcommand.
func NewAgentCommand(addonName string) *cobra.Command {
	o := NewAgentOptions(addonName)
	cmd := cmdfactory.
		NewControllerCommandConfig("basic-addon-agent", version.Get(), o.RunAgent).
		NewCommand()
	cmd.Use = "agent"
	cmd.Short = "Start the addon agent"

	o.AddFlags(cmd)
	return cmd
}

// AgentOptions defines the flags for the agent.
type AgentOptions struct {
	HubKubeconfigFile string
	SpokeClusterName  string
	AddonName         string
	AddonNamespace    string
}

// NewAgentOptions returns the flags with default values.
func NewAgentOptions(addonName string) *AgentOptions {
	return &AgentOptions{AddonName: addonName}
}

// AddFlags registers the agent flags.
func (o *AgentOptions) AddFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&o.HubKubeconfigFile, "hub-kubeconfig", o.HubKubeconfigFile,
		"Location of kubeconfig file to connect to hub cluster.")
	flags.StringVar(&o.SpokeClusterName, "cluster-name", o.SpokeClusterName,
		"Name of the spoke cluster.")
	flags.StringVar(&o.AddonNamespace, "addon-namespace", o.AddonNamespace,
		"Installation namespace of addon.")
	flags.StringVar(&o.AddonName, "addon-name", o.AddonName,
		"Name of the addon.")
}

// RunAgent starts the agent that collects pod info and sends to hub.
func (o *AgentOptions) RunAgent(ctx context.Context, kubeconfig *rest.Config) error {
	klog.Info("Starting basic-addon agent")

	// Build spoke client (local cluster)
	spokeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return err
	}
	klog.Info("Connected to spoke cluster")

	// Build hub client
	hubRestConfig, err := clientcmd.BuildConfigFromFlags("", o.HubKubeconfigFile)
	if err != nil {
		return err
	}
	hubClient, err := kubernetes.NewForConfig(hubRestConfig)
	if err != nil {
		return err
	}
	klog.Infof("Connected to hub cluster, will report to namespace: %s", o.SpokeClusterName)

	// Start sync loop
	ticker := time.NewTicker(SyncInterval)
	defer ticker.Stop()

	// Run immediately once, then on ticker
	if err := o.syncPodReport(ctx, spokeClient, hubClient); err != nil {
		klog.Errorf("Failed to sync pod report: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			klog.Info("Agent shutting down")
			return nil
		case <-ticker.C:
			if err := o.syncPodReport(ctx, spokeClient, hubClient); err != nil {
				klog.Errorf("Failed to sync pod report: %v", err)
			}
		}
	}
}

// syncPodReport collects pods from spoke and sends report to hub.
func (o *AgentOptions) syncPodReport(ctx context.Context, spokeClient, hubClient kubernetes.Interface) error {
	klog.V(4).Info("Syncing pod report")

	// List all pods in the spoke cluster
	podList, err := spokeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	// Build pod report
	report := buildPodReport(o.SpokeClusterName, podList.Items)

	// Serialize to JSON
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return err
	}

	// Create or update ConfigMap in hub
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PodReportConfigMapName,
			Namespace: o.SpokeClusterName,
			Labels: map[string]string{
				"app":                          "basic-addon",
				"addon.open-cluster-management.io/hosted-manifest-location": "none",
			},
		},
		Data: map[string]string{
			"report": string(reportJSON),
		},
	}

	existing, err := hubClient.CoreV1().ConfigMaps(o.SpokeClusterName).Get(ctx, PodReportConfigMapName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = hubClient.CoreV1().ConfigMaps(o.SpokeClusterName).Create(ctx, configMap, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		klog.Infof("Created pod report ConfigMap with %d pods", len(report.Pods))
		return nil
	}
	if err != nil {
		return err
	}

	// Update existing
	configMap.ResourceVersion = existing.ResourceVersion
	_, err = hubClient.CoreV1().ConfigMaps(o.SpokeClusterName).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	klog.Infof("Updated pod report ConfigMap with %d pods", len(report.Pods))
	return nil
}

// buildPodReport creates a PodReport from a list of pods.
func buildPodReport(clusterName string, pods []corev1.Pod) PodReport {
	podInfos := make([]PodInfo, 0, len(pods))
	for _, pod := range pods {
		podInfos = append(podInfos, PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
			NodeName:  pod.Spec.NodeName,
		})
	}

	return PodReport{
		ClusterName: clusterName,
		Timestamp:   time.Now().UTC(),
		TotalPods:   len(pods),
		Pods:        podInfos,
	}
}
