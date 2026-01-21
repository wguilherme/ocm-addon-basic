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

// AddonRBAC returns a PermissionConfigFunc that creates Role and RoleBinding
// in the managed cluster namespace on the hub. This grants the agent permission
// to write ConfigMaps (pod reports) to the hub.
func AddonRBAC(kubeConfig *rest.Config) agent.PermissionConfigFunc {
	return func(cluster *clusterv1.ManagedCluster, addon *addonapiv1alpha1.ManagedClusterAddOn) error {
		if kubeConfig == nil {
			// Skip RBAC creation when kubeConfig is nil (e.g., in tests)
			return nil
		}

		kubeclient, err := kubernetes.NewForConfig(kubeConfig)
		if err != nil {
			return err
		}

		groups := agent.DefaultGroups(cluster.Name, addon.Name)

		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("open-cluster-management:%s:agent", addon.Name),
				Namespace: cluster.Name,
			},
			Rules: []rbacv1.PolicyRule{
				// Strategy 1: Allow agent to read/write ConfigMaps for pod reports
				{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
					Resources: []string{"configmaps"},
					APIGroups: []string{""},
				},
				// Strategy 2: Allow agent to read and update ManagedClusterAddOns status
				{
					Verbs:     []string{"get", "list", "watch", "update", "patch"},
					Resources: []string{"managedclusteraddons", "managedclusteraddons/status"},
					APIGroups: []string{"addon.open-cluster-management.io"},
				},
				// Strategy 3: Allow agent to create/update AddOnPlacementScores
				{
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch"},
					Resources: []string{"addonplacementscores", "addonplacementscores/status"},
					APIGroups: []string{"cluster.open-cluster-management.io"},
				},
			},
		}

		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("open-cluster-management:%s:agent", addon.Name),
				Namespace: cluster.Name,
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     fmt.Sprintf("open-cluster-management:%s:agent", addon.Name),
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:     "Group",
					APIGroup: "rbac.authorization.k8s.io",
					Name:     groups[0],
				},
			},
		}

		// Create or update Role
		_, err = kubeclient.RbacV1().Roles(cluster.Name).Get(context.TODO(), role.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, createErr := kubeclient.RbacV1().Roles(cluster.Name).Create(context.TODO(), role, metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
		case err != nil:
			return err
		default:
			_, updateErr := kubeclient.RbacV1().Roles(cluster.Name).Update(context.TODO(), role, metav1.UpdateOptions{})
			if updateErr != nil {
				return updateErr
			}
		}

		// Create or update RoleBinding
		_, err = kubeclient.RbacV1().RoleBindings(cluster.Name).Get(context.TODO(), binding.Name, metav1.GetOptions{})
		switch {
		case errors.IsNotFound(err):
			_, createErr := kubeclient.RbacV1().RoleBindings(cluster.Name).Create(context.TODO(), binding, metav1.CreateOptions{})
			if createErr != nil {
				return createErr
			}
		case err != nil:
			return err
		default:
			_, updateErr := kubeclient.RbacV1().RoleBindings(cluster.Name).Update(context.TODO(), binding, metav1.UpdateOptions{})
			if updateErr != nil {
				return updateErr
			}
		}

		return nil
	}
}
