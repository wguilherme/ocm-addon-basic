// Package main é o entry point do addon.
// Contém dois subcomandos: "controller" (roda no hub) e "agent" (roda nos spokes).
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

const (
	// CommandController é o nome do subcomando do controller.
	// Convenção do addon-framework: "controller" para hub, "agent" para spoke.
	CommandController = "controller"

	// ControllerName é o nome do controller usado em logs.
	ControllerName = "basic-addon-controller"
)

// main inicializa o CLI do addon.
//
// Uso:
//
//	addon controller  # Inicia o controller no hub
//	addon agent       # Inicia o agent no spoke
func main() {
	rand.Seed(time.Now().UTC().UnixNano())

	// Normaliza flags para usar hífen como separador (ex: --hub-kubeconfig)
	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(goflag.CommandLine)

	command := newCommand()
	if err := command.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

// newCommand cria o comando raiz com os subcomandos controller e agent.
func newCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "addon",
		Short: "basic-addon - OCM addon para coleta de relatórios de pods",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			os.Exit(1)
		},
	}

	// Exibe versão do addon
	if v := version.Get().String(); len(v) == 0 {
		cmd.Version = "<unknown>"
	} else {
		cmd.Version = v
	}

	// Adiciona subcomandos
	cmd.AddCommand(newControllerCommand())
	cmd.AddCommand(agent.NewAgentCommand(addon.AddonName))

	return cmd
}

// newControllerCommand cria o subcomando "controller".
// O controller roda no hub e observa ManagedClusterAddOn.
func newControllerCommand() *cobra.Command {
	cmd := cmdfactory.
		NewControllerCommandConfig(ControllerName, version.Get(), runController).
		NewCommand()
	cmd.Use = CommandController
	cmd.Short = "Inicia o controller do addon"

	return cmd
}

// runController é a função principal do controller.
//
// Fluxo:
// 1. Cria o AddonManager (gerencia o ciclo de vida dos addons)
// 2. Configura o RegistrationOption (como o agent se registra no hub)
// 3. Cria o AgentAddon usando factory (define manifests, values, health probe)
// 4. Adiciona o AgentAddon ao manager
// 5. Inicia o manager (começa a observar ManagedClusterAddOn)
//
// Quando um ManagedClusterAddOn é criado:
// 1. Controller observa o evento
// 2. Renderiza os templates com GetDefaultValues
// 3. Cria ManifestWork no namespace do spoke
// 4. work-agent no spoke aplica os manifests
// 5. Agent começa a rodar e enviar relatórios
func runController(ctx context.Context, kubeConfig *rest.Config) error {
	klog.Info("Iniciando controller do basic-addon")

	// AddonManager é o componente central do addon-framework.
	// Gerencia o ciclo de vida dos addons e observa ManagedClusterAddOn.
	mgr, err := addonmanager.New(kubeConfig)
	if err != nil {
		return err
	}

	// RegistrationOption configura como o agent se registra.
	// utilrand.String(5) gera um nome único para o agent (usado no CSR).
	registrationOption := addon.NewRegistrationOption(
		kubeConfig,
		addon.AddonName,
		utilrand.String(5),
	)

	// NewAgentAddonFactory cria o addon usando padrão factory.
	// - FS: Sistema de arquivos embarcado com templates
	// - "manifests/templates": Caminho dos templates
	// - WithGetValuesFuncs: Função que retorna valores para os templates
	// - WithAgentRegistrationOption: Configuração de registro
	// - WithAgentHealthProber: Configuração de health check
	agentAddon, err := addonfactory.NewAgentAddonFactory(addon.AddonName, addon.FS, "manifests/templates").
		WithGetValuesFuncs(addon.GetDefaultValues).
		WithAgentRegistrationOption(registrationOption).
		WithAgentHealthProber(addon.AgentHealthProber()).
		BuildTemplateAgentAddon()
	if err != nil {
		klog.Errorf("Falha ao criar agent addon: %v", err)
		return err
	}

	// Adiciona o addon ao manager
	err = mgr.AddAgent(agentAddon)
	if err != nil {
		return err
	}

	// Inicia o manager (bloqueia até ctx.Done)
	err = mgr.Start(ctx)
	if err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}
