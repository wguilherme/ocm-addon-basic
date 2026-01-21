package main

import (
	"context"
	goflag "flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/rest"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"

	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	cmdfactory "open-cluster-management.io/addon-framework/pkg/cmd/factory"
	"open-cluster-management.io/addon-framework/pkg/version"

	"github.com/totvs/addon-framework-basic/pkg/addon"
	"github.com/totvs/addon-framework-basic/pkg/agent"
)

// esta função serve para 
func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	command := newCommand()
	// teste 
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)		
	}
}

func newCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "basic-addon - OCM addon for collecting pod reports",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			os.Exit(1)
		},
	}

	if v := version.Get().String(); len(v) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v
	}

	cmd.AddCommand(newControllerCommand())
	cmd.AddCommand(agent.NewAgentCommand(addon.AddonName))

	return cmd
}

func newControllerCommand() *cobra.Command {
	cmd := cmdfactory.
		NewControllerCommandConfig("basic-addon-controller", version.Get(), runController).
		NewCommand()
	cmd.Use = "controller"
	cmd.Short = "Start the addon controller"

	return cmd
}

func runController(ctx context.Context, kubeConfig *rest.Config) error {
	klog.Info("Starting basic-addon controller")

	mgr, err := addonmanager.New(kubeConfig)
	if err != nil {
		return err
	}

	registrationOption := addon.NewRegistrationOption(
		kubeConfig,
		addon.AddonName,
		utilrand.String(5),
	)

	agentAddon, err := addonfactory.NewAgentAddonFactory(addon.AddonName, addon.FS, "manifests/templates").
		WithGetValuesFuncs(addon.GetDefaultValues).
		WithAgentRegistrationOption(registrationOption).
		WithAgentHealthProber(addon.AgentHealthProber()).
		BuildTemplateAgentAddon()
	if err != nil {
		klog.Errorf("Failed to build agent addon: %v", err)
		return err
	}

	err = mgr.AddAgent(agentAddon)
	if err != nil {
		return err
	}

	err = mgr.Start(ctx)
	if err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}
