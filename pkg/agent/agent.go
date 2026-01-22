// Package agent contém o código do agent que roda nos spokes.
// O agent é deployado pelo work-agent (OCM) através do ManifestWork gerado pelo controller.
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
	"open-cluster-management.io/addon-framework/pkg/lease"
	"open-cluster-management.io/addon-framework/pkg/version"
)

const (
	// AgentName é o nome do agent.
	AgentName = "basic-addon-agent"

	// ConfigMapName é o nome do ConfigMap criado no hub (reports).
	ConfigMapName = "pod-report"

	// SyncInterval define o intervalo entre sincronizações do relatório.
	SyncInterval = 60 * time.Second

	// CommandAgent é o nome do subcomando.
	// Convenção do addon-framework: "controller" para hub, "agent" para spoke.
	CommandAgent = "agent"

	// Nomes das flags - seguem convenção dos exemplos do addon-framework.
	// Estas flags são passadas via args do Deployment (ver manifests/templates/deployment.yaml).
	FlagHubKubeconfig  = "hub-kubeconfig"  // Caminho do kubeconfig para conectar ao hub
	FlagClusterName    = "cluster-name"    // Nome do spoke cluster
	FlagAddonNamespace = "addon-namespace" // Namespace onde o addon está instalado
	FlagAddonName      = "addon-name"      // Nome do addon
)

// PodReport é o dado enviado para o hub.
// Contém informações sobre os pods do spoke.
type PodReport struct {
	ClusterName string    `json:"clusterName"`
	Timestamp   time.Time `json:"timestamp"`
	TotalPods   int       `json:"totalPods"`
	Pods        []PodInfo `json:"pods"`
}

// PodInfo contém informações básicas de um pod.
type PodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
}

// AgentOptions define a configuração do agent.
// Estes campos são preenchidos pelas flags do comando.
// Não é uma interface do OCM, mas segue a convenção dos exemplos do addon-framework.
type AgentOptions struct {
	HubKubeconfigFile string // Kubeconfig para conectar ao hub (criado pelo registration-agent)
	SpokeClusterName  string // Nome do cluster spoke (usado como namespace no hub)
	AddonName         string // Nome do addon
	AddonNamespace    string // Namespace onde o addon está instalado no spoke
}

// NewAgentCommand cria o subcomando "agent".
//
// Fluxo:
// 1. cmdfactory.NewControllerCommandConfig registra RunAgent como callback
// 2. O framework adiciona automaticamente a flag --kubeconfig
// 3. Nossas flags customizadas (--hub-kubeconfig, etc) são adicionadas aqui
// 4. Quando o comando executa, o framework lê --kubeconfig e passa para RunAgent
func NewAgentCommand(addonName string) *cobra.Command {
	o := &AgentOptions{AddonName: addonName}
	cmd := cmdfactory.
		NewControllerCommandConfig(AgentName, version.Get(), o.RunAgent).
		NewCommand()
	cmd.Use = CommandAgent
	cmd.Short = "Inicia o agent do addon"

	flags := cmd.Flags()
	flags.StringVar(&o.HubKubeconfigFile, FlagHubKubeconfig, "", "Kubeconfig para conectar ao hub")
	flags.StringVar(&o.SpokeClusterName, FlagClusterName, "", "Nome do spoke cluster")
	flags.StringVar(&o.AddonNamespace, FlagAddonNamespace, "", "Namespace onde o addon está instalado")
	flags.StringVar(&o.AddonName, FlagAddonName, addonName, "Nome do addon")

	return cmd
}

// RunAgent inicia o loop principal do agent.
//
// Parâmetros:
//   - ctx: Contexto para cancelamento (SIGTERM/SIGINT tratados pelo framework)
//   - kubeconfig: Configuração para conectar ao spoke (cluster local).
//     Este kubeconfig é passado automaticamente pelo addon-framework através do cmdfactory.
//     O framework lê a flag --kubeconfig (ou usa in-cluster config se vazio) e passa aqui.
//
// Fluxo:
// 1. Cria cliente para o spoke (cluster local onde o agent roda)
// 2. Cria cliente para o hub (usando --hub-kubeconfig, criado pelo registration-agent)
// 3. Inicia o LeaseUpdater (health check - o hub verifica se o lease está sendo atualizado)
// 4. Inicia loop de sync: coleta pods e envia relatório para o hub
// O próprio OCM injeta automaticamente o kubeconfig, por se tratar de um addon.
// cada addon tem seu próprio kubeconfig
// o registration vê que o addon precisa de credenciais e : cria csr no hub
// controller do hub aprova via CSRApproveCheck (aprova automaticamente)
// o certificado é gerado e cria secret com kubeconfig do hub
// secret é ontado no pod do agent
func (o *AgentOptions) RunAgent(ctx context.Context, kubeconfig *rest.Config) error {
	klog.Info("Iniciando agent")

	// Cliente do spoke (cluster local onde o agent está rodando)
	spokeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return err
	}

	// Cliente do hub (que que vai criar o configmap de report)
	// O kubeconfig do hub
	hubConfig, err := clientcmd.BuildConfigFromFlags("", o.HubKubeconfigFile)
	if err != nil {
		return err
	}
	hubClient, err := kubernetes.NewForConfig(hubConfig)
	if err != nil {
		return err
	}
	klog.Infof("Conectado ao hub, enviando para namespace: %s", o.SpokeClusterName)

	// LeaseUpdater mantém o Lease atualizado no spoke.
	// O registration-agent no spoke verifica se o Lease está sendo atualizado.
	// Se parar de atualizar, o addon é marcado como Unavailable no hub.
	// nesse contexto o lease é pq o hub precisa saber se o agent está rodando
	// lease usa pull modal (agente atualiza um recurso local e o registration agent observa e reporta status pro hub via API spoke->hub)
	// geralmente +utilizado em aplicações distribuídas (nesse caso os addons) 
	leaseUpdater := lease.NewLeaseUpdater(spokeClient, o.AddonName, o.AddonNamespace)
	go leaseUpdater.Start(ctx)

	// Loop de sincronização
	ticker := time.NewTicker(SyncInterval)
	defer ticker.Stop()

	// Sync imediato na inicialização
	o.sync(ctx, spokeClient, hubClient)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			o.sync(ctx, spokeClient, hubClient)
		}
	}
}

// sync coleta pods do spoke e envia relatório para o hub.
//
// Fluxo:
// 1. Lista todos os pods do spoke
// 2. Monta o relatório (PodReport)
// 3. Cria/atualiza o ConfigMap no hub (namespace = nome do spoke cluster)
//
// O ConfigMap é criado no namespace do spoke no hub. Isso permite que
// o hub tenha visibilidade dos pods de cada spoke.
func (o *AgentOptions) sync(ctx context.Context, spokeClient, hubClient kubernetes.Interface) {
	// busca os pods usando o client k8s
	// interessante que aqui temos acesso tanto ao spoke quanto hub. 
	// livre para implementarmos qualquer tipo de integração, lógica, etc.
	pods, err := spokeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Falha ao listar pods: %v", err)
		return
	}

	// montamos o report através de um método
	report := o.buildReport(pods.Items)
	// parseamos o dado
	data, _ := json.Marshal(report)

	// montamos o configmap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: o.SpokeClusterName, // Namespace no hub = nome do spoke
		},
		Data: map[string]string{"report": string(data)},
	}

	// Tenta obter o ConfigMap existente para fazer update (precisa do ResourceVersion)
	// fazemos um "upsert" aqui.
	existing, err := hubClient.CoreV1().ConfigMaps(o.SpokeClusterName).Get(ctx, ConfigMapName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = hubClient.CoreV1().ConfigMaps(o.SpokeClusterName).Create(ctx, cm, metav1.CreateOptions{})
	} else if err == nil {
		cm.ResourceVersion = existing.ResourceVersion
		_, err = hubClient.CoreV1().ConfigMaps(o.SpokeClusterName).Update(ctx, cm, metav1.UpdateOptions{})
	}

	if err != nil {
		klog.Errorf("Falha ao sincronizar relatório: %v", err)
		return
	}
	klog.Infof("Relatório sincronizado: %d pods", report.TotalPods)
}

// buildReport cria um PodReport a partir da lista de pods.
func (o *AgentOptions) buildReport(pods []corev1.Pod) PodReport {
	infos := make([]PodInfo, len(pods))
	for i, p := range pods {
		infos[i] = PodInfo{
			Name:      p.Name,
			Namespace: p.Namespace,
			Status:    string(p.Status.Phase),
		}
	}
	return PodReport{
		ClusterName: o.SpokeClusterName,
		Timestamp:   time.Now().UTC(),
		TotalPods:   len(pods),
		Pods:        infos,
	}
}
