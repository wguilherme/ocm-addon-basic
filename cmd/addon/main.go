package main

import (
	"context"
	"embed"
	"os"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/addon-framework/pkg/agent"
)

//go:embed manifests
var FS embed.FS

const addonName = "basic-addon"

func main() {
	kubeConfig, err := getKubeConfig()
	if err != nil {
		klog.Errorf("failed to get kubeconfig: %v", err)
		os.Exit(1)
	}

	addonMgr, err := addonmanager.New(kubeConfig)
	if err != nil {
		klog.Errorf("unable to setup addon manager: %v", err)
		os.Exit(1)
	}

	agentAddon, err := addonfactory.NewAgentAddonFactory(addonName, FS, "manifests").
		WithAgentHealthProber(&agent.HealthProber{
			Type: agent.HealthProberTypeDeploymentAvailability,
		}).
		BuildTemplateAgentAddon()
	if err != nil {
		klog.Errorf("failed to build agent addon: %v", err)
		os.Exit(1)
	}

	err = addonMgr.AddAgent(agentAddon)
	if err != nil {
		klog.Errorf("failed to add addon agent: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()
	go func() {
		if err := addonMgr.Start(ctx); err != nil {
			klog.Fatal(err)
		}
	}()

	<-ctx.Done()
}

// getKubeConfig returns kubeconfig for the addon manager.
// It tries in-cluster config first, then falls back to KUBECONFIG env or ~/.kube/config.
func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first (when running inside a pod)
	cfg, err := rest.InClusterConfig()
	if err == nil {
		klog.Info("Using in-cluster kubeconfig")
		return cfg, nil
	}

	// Fall back to kubeconfig file (for local development)
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = clientcmd.RecommendedHomeFile // ~/.kube/config
	}

	klog.Infof("Using kubeconfig from: %s", kubeconfig)
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
