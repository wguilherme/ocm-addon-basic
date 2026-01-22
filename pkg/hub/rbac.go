// Package hub contém a configuração de RBAC no hub para o agent.
// Define as permissões que o agent terá para escrever dados no hub.
package hub

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"open-cluster-management.io/addon-framework/pkg/agent"
	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// AddonRBAC cria Role e RoleBinding no namespace do spoke (no hub).
// Isso permite que o agent escreva ConfigMaps no hub.
//
// Fluxo:
// 1. Esta função é chamada pelo controller quando o ManagedClusterAddOn é criado
// 2. Cria Role com permissão para get/create/update ConfigMaps
// 3. Cria RoleBinding associando o grupo do agent à Role
// 4. O grupo do agent segue o padrão: system:open-cluster-management:cluster:<cluster>:addon:<addon>
//
// Por que Role e não ClusterRole?
// - Role é namespace-scoped, limita as permissões ao namespace do spoke
// - Cada spoke tem seu próprio namespace no hub (mesmo nome do cluster)
// - Isso isola os dados de cada spoke
func AddonRBAC(kubeConfig *rest.Config) agent.PermissionConfigFunc {
	return func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
		// Se não tiver kubeConfig, não faz nada (útil para testes)
		if kubeConfig == nil {
			return nil
		}

		client, err := kubernetes.NewForConfig(kubeConfig)
		if err != nil {
			return err
		}

		// Nome da Role segue convenção OCM
		roleName := fmt.Sprintf("open-cluster-management:%s:agent", addon.Name)

		// Grupos do agent (DefaultGroups retorna os grupos padrão do OCM)
		// Formato: system:open-cluster-management:cluster:<cluster>:addon:<addon>
		groups := agent.DefaultGroups(cluster.Name, addon.Name)

		// Role com permissão para manipular ConfigMaps
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: cluster.Name, // Namespace = nome do cluster spoke
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:     []string{"get", "create", "update"},
					Resources: []string{"configmaps"},
					APIGroups: []string{""},
				},
			},
		}

		// RoleBinding associa o grupo do agent à Role
		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: cluster.Name,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     roleName,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:     "Group",
					APIGroup: "rbac.authorization.k8s.io",
					Name:     groups[0], // Primeiro grupo é o mais específico
				},
			},
		}

		// Cria ou atualiza Role
		if _, err := client.RbacV1().Roles(cluster.Name).Get(context.TODO(), roleName, metav1.GetOptions{}); errors.IsNotFound(err) {
			_, err = client.RbacV1().Roles(cluster.Name).Create(context.TODO(), role, metav1.CreateOptions{})
		} else if err == nil {
			_, err = client.RbacV1().Roles(cluster.Name).Update(context.TODO(), role, metav1.UpdateOptions{})
		}
		if err != nil {
			return err
		}

		// Cria ou atualiza RoleBinding
		if _, err := client.RbacV1().RoleBindings(cluster.Name).Get(context.TODO(), roleName, metav1.GetOptions{}); errors.IsNotFound(err) {
			_, err = client.RbacV1().RoleBindings(cluster.Name).Create(context.TODO(), binding, metav1.CreateOptions{})
		} else if err == nil {
			_, err = client.RbacV1().RoleBindings(cluster.Name).Update(context.TODO(), binding, metav1.UpdateOptions{})
		}
		return err
	}
}
