package resolver

import (
	"bytes"
	"fmt"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/controller/install"
	"github.com/operator-framework/operator-registry/pkg/registry"
	"k8s.io/kubernetes/pkg/apis/rbac"

	rbacv1 "k8s.io/api/rbac/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extScheme "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/ownerutil"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	k8sscheme.AddToScheme(scheme)
	extScheme.AddToScheme(scheme)
	v1alpha1.AddToScheme(scheme)
}


// deprecated, remove when not used anywhere
// NewStepResourceFromCSV creates an unresolved Step for the provided CSV.
func NewStepResourceFromCSV(csv *v1alpha1.ClusterServiceVersion) (v1alpha1.StepResource, error) {
	return NewStepResourceFromObject(csv, csv.GetName())
}

// deprecated, remove when not used anywhere
// NewStepResourceFromCRD creates an unresolved Step for the provided CRD.
func NewStepResourcesFromCRD(crd *apiextensions.CustomResourceDefinition) ([]v1alpha1.StepResource, error) {
	steps := []v1alpha1.StepResource{}

	crdStep, err := NewStepResourceFromObject(crd, crd.GetName())
	if err != nil {
		return nil, err
	}
	steps = append(steps, crdStep)

	editRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("edit-%s-%s", crd.Name, crd.Spec.Version),
			Labels: map[string]string{
				"rbac.authorization.k8s.io/aggregate-to-admin": "true",
				"rbac.authorization.k8s.io/aggregate-to-edit":  "true",
			},
		},
		Rules: []rbacv1.PolicyRule{{Verbs: []string{"create", "update", "patch", "delete"}, APIGroups: []string{crd.Spec.Group}, Resources: []string{crd.Spec.Names.Plural}}},
	}
	editRoleStep, err := NewStepResourceFromObject(editRole, editRole.GetName())
	if err != nil {
		return nil, err
	}
	steps = append(steps, editRoleStep)

	viewRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("view-%s-%s", crd.Name, crd.Spec.Version),
			Labels: map[string]string{
				"rbac.authorization.k8s.io/aggregate-to-view": "true",
			},
		},
		Rules: []rbacv1.PolicyRule{{Verbs: []string{"get", "list", "watch"}, APIGroups: []string{crd.Spec.Group}, Resources: []string{crd.Spec.Names.Plural}}},
	}
	viewRoleStep, err := NewStepResourceFromObject(viewRole, viewRole.GetName())
	if err != nil {
		return nil, err
	}
	steps = append(steps, viewRoleStep)

	return steps, nil
}

// NewStepResourceForObject returns a new StepResource for the provided object
func NewStepResourceFromObject(obj runtime.Object, name string) (v1alpha1.StepResource, error) {
	var resource v1alpha1.StepResource

	// set up object serializer
	serializer := k8sjson.NewSerializer(k8sjson.DefaultMetaFactory, scheme, scheme, true)

	// create an object manifest
	var manifest bytes.Buffer
	err := serializer.Encode(obj, &manifest)
	if err != nil {
		return resource, err
	}

	if err := ownerutil.InferGroupVersionKind(obj); err != nil {
		return resource, err
	}

	gvk := obj.GetObjectKind().GroupVersionKind()

	// create the resource
	resource = v1alpha1.StepResource{
		Name:     name,
		Kind:     gvk.Kind,
		Group:    gvk.Group,
		Version:  gvk.Version,
		Manifest: manifest.String(),
	}

	return resource, nil
}

func NewStepResourceFromBundle(bundle *registry.Bundle, namespace string) ([]v1alpha1.StepResource, error) {
	steps := []v1alpha1.StepResource{}

	for _, object := range bundle.Objects {
		object.SetNamespace(namespace)
		step, err := NewStepResourceFromObject(object, bundle.Name)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}

	csv, err := bundle.ClusterServiceVersion()
	if err != nil {
		return nil, err
	}

	// TODO ownerrefs?
	csv.SetNamespace(namespace)
	operatorServiceAccountSteps, err := NewServiceAccountStepResources(csv)
	if err != nil {
		return nil, err
	}
	steps = append(steps, operatorServiceAccountSteps...)

	providedAPIs, err := bundle.ProvidedAPIs()
	if err != nil {
		return nil, err
	}
	providedAPIRbacSteps, err := NewStepResourcesForProvidedAPIs(providedAPIs)
	if err != nil {
		return nil, err
	}
	return append(steps, providedAPIRbacSteps...), nil
}

func NewStepResourcesForProvidedAPIs(apis map[registry.APIKey]struct{}) ([]v1alpha1.StepResource, error) {
	steps := []v1alpha1.StepResource{}

	for providedAPI := range apis {
		editRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("edit-%s-%s", providedAPI.Group, providedAPI.Version),
				Labels: map[string]string{
					"rbac.authorization.k8s.io/aggregate-to-admin": "true",
					"rbac.authorization.k8s.io/aggregate-to-edit":  "true",
				},
			},
			Rules: []rbacv1.PolicyRule{{Verbs: []string{"create", "update", "patch", "delete"}, APIGroups: []string{providedAPI.Group}, Resources: []string{providedAPI.Plural}}},
		}
		editRoleStep, err := NewStepResourceFromObject(editRole, editRole.GetName())
		if err != nil {
			return nil, err
		}
		steps = append(steps, editRoleStep)

		viewRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("view-%s-%s", providedAPI.Group, providedAPI.Version),
				Labels: map[string]string{
					"rbac.authorization.k8s.io/aggregate-to-view": "true",
				},
			},
			Rules: []rbacv1.PolicyRule{{Verbs: []string{"get", "list", "watch"}, APIGroups: []string{providedAPI.Group}, Resources: []string{providedAPI.Plural}}},
		}
		viewRoleStep, err := NewStepResourceFromObject(viewRole, viewRole.GetName())
		if err != nil {
			return nil, err
		}
		steps = append(steps, viewRoleStep)
	}
	return steps, nil
}

// NewServiceAccountStepResources returns a list of step resources required to satisfy the RBAC requirements of the given CSV's InstallStrategy
func NewServiceAccountStepResources(csv *v1alpha1.ClusterServiceVersion) ([]v1alpha1.StepResource, error) {
	var rbacSteps []v1alpha1.StepResource

	// User a StrategyResolver to get the strategy details
	strategyResolver := install.StrategyResolver{}
	strategy, err := strategyResolver.UnmarshalStrategy(csv.Spec.InstallStrategy)
	if err != nil {
		return nil, err
	}

	// Assume the strategy is for a deployment
	strategyDetailsDeployment, ok := strategy.(*install.StrategyDetailsDeployment)
	if !ok {
		return nil, fmt.Errorf("could not assert strategy implementation as deployment for CSV %s", csv.GetName())
	}

	// Track created ServiceAccount StepResources
	serviceaccounts := map[string]struct{}{}

	// Resolve Permissions as StepResources
	for i, permission := range strategyDetailsDeployment.Permissions {
		// Create ServiceAccount if necessary
		if _, ok := serviceaccounts[permission.ServiceAccountName]; !ok {
			serviceAccount := &corev1.ServiceAccount{}
			serviceAccount.SetName(permission.ServiceAccountName)
			ownerutil.AddNonBlockingOwner(serviceAccount, csv)
			step, err := NewStepResourceFromObject(serviceAccount, serviceAccount.GetName())
			if err != nil {
				return nil, err
			}
			rbacSteps = append(rbacSteps, step)

			// Mark that a StepResource has been resolved for this ServiceAccount
			serviceaccounts[permission.ServiceAccountName] = struct{}{}
		}

		// Create Role
		role := &rbacv1.Role{
			Rules: permission.Rules,
		}
		ownerutil.AddNonBlockingOwner(role, csv)
		role.SetName(fmt.Sprintf("%s-%d", csv.GetName(), i))
		step, err := NewStepResourceFromObject(role, role.GetName())
		if err != nil {
			return nil, err
		}
		rbacSteps = append(rbacSteps, step)

		// Create RoleBinding
		roleBinding := &rbacv1.RoleBinding{
			RoleRef: rbacv1.RoleRef{
				Kind:     "Role",
				Name:     role.GetName(),
				APIGroup: rbac.GroupName},
			Subjects: []rbacv1.Subject{{
				Kind:      "ServiceAccount",
				Name:      permission.ServiceAccountName,
				Namespace: csv.GetNamespace(),
			}},
		}
		ownerutil.AddNonBlockingOwner(roleBinding, csv)
		roleBinding.SetName(fmt.Sprintf("%s-%s", role.GetName(), permission.ServiceAccountName))
		step, err = NewStepResourceFromObject(roleBinding, roleBinding.GetName())
		if err != nil {
			return nil, err
		}
		rbacSteps = append(rbacSteps, step)
	}

	// Resolve ClusterPermissions as StepResources
	for i, permission := range strategyDetailsDeployment.ClusterPermissions {
		// Create ServiceAccount if necessary
		if _, ok := serviceaccounts[permission.ServiceAccountName]; !ok {
			serviceAccount := &corev1.ServiceAccount{}
			serviceAccount.SetName(permission.ServiceAccountName)
			ownerutil.AddNonBlockingOwner(serviceAccount, csv)
			step, err := NewStepResourceFromObject(serviceAccount, serviceAccount.GetName())
			if err != nil {
				return nil, err
			}
			rbacSteps = append(rbacSteps, step)

			// Mark that a StepResource has been resolved for this ServiceAccount
			serviceaccounts[permission.ServiceAccountName] = struct{}{}
		}

		// Create ClusterRole
		role := &rbacv1.ClusterRole{
			Rules: permission.Rules,
		}
		ownerutil.AddNonBlockingOwner(role, csv)
		role.SetName(fmt.Sprintf("%s-%d", csv.GetName(), i))
		step, err := NewStepResourceFromObject(role, role.GetName())
		if err != nil {
			return nil, err
		}
		rbacSteps = append(rbacSteps, step)

		// Create ClusterRoleBinding
		roleBinding := &rbacv1.ClusterRoleBinding{
			RoleRef: rbacv1.RoleRef{
				Kind:     "ClusterRole",
				Name:     role.GetName(),
				APIGroup: rbac.GroupName,
			},
			Subjects: []rbacv1.Subject{{
				Kind:      "ServiceAccount",
				Name:      permission.ServiceAccountName,
				Namespace: csv.GetNamespace(),
			}},
		}
		ownerutil.AddNonBlockingOwner(roleBinding, csv)
		roleBinding.SetName(fmt.Sprintf("%s-%s", role.GetName(), permission.ServiceAccountName))
		step, err = NewStepResourceFromObject(roleBinding, roleBinding.GetName())
		if err != nil {
			return nil, err
		}
		rbacSteps = append(rbacSteps, step)
	}

	return rbacSteps, nil
}
