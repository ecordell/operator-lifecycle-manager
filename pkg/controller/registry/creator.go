package registry

import (
	"fmt"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/operatorclient"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	v1lister "k8s.io/client-go/listers/core/v1"
	rbacv1lister "k8s.io/client-go/listers/rbac/v1"
	"time"
)

//for test stubbing and for ensuring standardization of timezones to UTC
var timeNow = func() metav1.Time { return metav1.NewTime(time.Now().UTC()) }

// catalogsource wraps CatalogSource to add our derivation methods
type catalogSourceDeriver struct {
	v1alpha1.CatalogSource
}

func (s *catalogSourceDeriver) serviceAccountName() string {
	return s.GetName()+"-configmap-server"
}

func (s *catalogSourceDeriver) roleName() string {
	return s.GetName() + "configmap-reader"
}


func (s *catalogSourceDeriver) ConfigMapChanges(configMap *v1.ConfigMap) bool {
	if s.Status.ConfigMapResource == nil {
		return false
	}
	if s.Status.ConfigMapResource.Name != configMap.GetName() {
		return false
	}
	if s.Status.ConfigMapResource.ResourceVersion == configMap.GetResourceVersion() {
		return false
	}
	return true
}

func (s *catalogSourceDeriver) Service() *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.GetName(),
			Namespace: s.GetNamespace(),
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name: "grpc",
					Port: 50051,
					TargetPort: intstr.FromInt(50051),
				},
			},
			Selector: map[string]string{
				"catalogSourceDeriver": s.GetName(),
			},
		},
	}
}

func (s *catalogSourceDeriver) Pod(image string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: s.GetName(),
			Namespace: s.GetNamespace(),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "configmap-registry-server",
					Image: image,
					Args: []string{"-c", s.GetName(), "-n", s.GetNamespace()},
				},
			},
			ServiceAccountName: s.GetName()+"-configmap-server",
		},
	}
}

func (s *catalogSourceDeriver) ServiceAccount() *v1.ServiceAccount {
	return &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.serviceAccountName(),
			Namespace: s.GetNamespace(),
		},
	}
}

func (s *catalogSourceDeriver) Role() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.roleName(),
			Namespace: s.GetNamespace(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs: []string{"get"},
				APIGroups: []string{""},
				Resources: []string{"ConfigMap"},
				ResourceNames: []string{s.Spec.ConfigMap},
			},
		},
	}
}

func (s *catalogSourceDeriver) RoleBinding() *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.GetName() + "-server-configmap-reader",
			Namespace: s.GetNamespace(),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				APIGroup: "rbac.authorization.k8s.io",
				Name: s.serviceAccountName(),
				Namespace: s.GetNamespace(),
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind: "Role",
			Name: s.roleName(),
		},
	}
}

type RegistryCreator interface {
	EnsureRegistryServer(catalogSource *v1alpha1.CatalogSource) error
}

type ConfigMapRegistryCreator struct {
	ConfigMapLister      v1lister.ConfigMapLister
	ServiceLister        v1lister.ServiceLister
	RoleBindingLister    rbacv1lister.RoleBindingLister
	RoleLister           rbacv1lister.RoleLister
	PodLister            v1lister.PodLister
	ServiceAccountLister v1lister.ServiceAccountLister
	OpClient             operatorclient.ClientInterface
	Image                string
}

var _ RegistryCreator = &ConfigMapRegistryCreator{}

func (c *ConfigMapRegistryCreator) currentService(source catalogSourceDeriver) *v1.Service {
	serviceName := source.Service().GetName()
 	service, err := c.ServiceLister.Services(source.GetNamespace()).Get(serviceName)
 	if err!= nil {
 		logrus.WithField("service", serviceName).Warn("couldn't find service in cache")
 		return nil
	}
 	return service
}

func (c *ConfigMapRegistryCreator) currentServiceAccount(source catalogSourceDeriver) *v1.ServiceAccount {
	serviceAccountName := source.ServiceAccount().GetName()
	serviceAccount, err := c.ServiceAccountLister.ServiceAccounts(source.GetNamespace()).Get(serviceAccountName)
	if err!= nil {
		logrus.WithField("serviceAccouint", serviceAccountName).WithError(err).Warn("couldn't find service account in cache")
		return nil
	}
	return serviceAccount
}

func (c *ConfigMapRegistryCreator) currentRole(source catalogSourceDeriver) *rbacv1.Role {
	roleName := source.Role().GetName()
	role, err := c.RoleLister.Roles(source.GetNamespace()).Get(roleName)
	if err!= nil {
		logrus.WithField("role", roleName).WithError(err).Warn("couldn't find role in cache")
		return nil
	}
	return role
}

func (c *ConfigMapRegistryCreator) currentRoleBinding(source catalogSourceDeriver) *rbacv1.RoleBinding {
	roleBindingName := source.RoleBinding().GetName()
	roleBinding, err := c.RoleBindingLister.RoleBindings(source.GetNamespace()).Get(roleBindingName)
	if err!= nil {
		logrus.WithField("roleBinding", roleBindingName).WithError(err).Warn("couldn't find role binding in cache")
		return nil
	}
	return roleBinding
}

func (c *ConfigMapRegistryCreator) currentPod(source catalogSourceDeriver, image string) *v1.Pod {
	podName := source.Pod(image).GetName()
	pod, err := c.PodLister.Pods(source.GetNamespace()).Get(podName)
	if err!= nil {
		logrus.WithField("pod", podName).WithError(err).Warn("couldn't find pod in cache")
		return nil
	}
	return pod
}

// Ensure that all components of registry server are up to date.
func (c *ConfigMapRegistryCreator) EnsureRegistryServer(catalogSource *v1alpha1.CatalogSource) error {
	source := catalogSourceDeriver{*catalogSource}

	if source.Status.ConfigMapResource == nil || source.Status.ConfigMapResource.UID == "" {
		return fmt.Errorf("no configmap in catalogsource status")
	}

	// fetch configmap first, exit early if we can't find it
	configMap, err := c.ConfigMapLister.ConfigMaps(source.GetNamespace()).Get(source.Spec.ConfigMap)
	if err != nil {
		return err
	}

	// if service status is nil, we force create every object to ensure they're created the first time
	overwrite := source.Status.RegistryServiceStatus == nil

	// recreate the pod if there are configmap changes; this causes the db to be rebuilt
	overwritePod := overwrite || source.ConfigMapChanges(configMap)

	if err := c.ensureServiceAccount(source, overwrite); err != nil {
		return err
	}
	if err := c.ensureRole(source, overwrite); err != nil {
		return err
	}
	if err := c.ensureRoleBinding(source, overwrite); err != nil {
		return err
	}
	if err := c.ensurePod(source, overwritePod); err != nil {
		return err
	}
	if err := c.ensureService(source, overwrite); err!= nil {
		return err
	}
	catalogSource.Status.RegistryServiceStatus = &v1alpha1.RegistryServiceStatus{
		Protocol: "grpc",
		ServiceName: source.Service().GetName(),
		ServiceNamespace: source.GetNamespace(),
		Port: string(source.Service().Spec.Ports[0].Port),
	}
	catalogSource.Status.LastSync = timeNow()
	return nil
}

func (c *ConfigMapRegistryCreator) ensureServiceAccount(source catalogSourceDeriver, overwrite bool) error {
	serviceAccount := source.ServiceAccount()
	if c.currentServiceAccount(source) != nil {
		if !overwrite {
			return nil
		}
		if err := c.OpClient.DeleteServiceAccount(serviceAccount.GetNamespace(), serviceAccount.GetName(), metav1.NewDeleteOptions(0)); err!=nil {
			return err
		}
	}
	_, err := c.OpClient.CreateServiceAccount(serviceAccount)
	return err
}

func (c *ConfigMapRegistryCreator) ensureRole(source catalogSourceDeriver, overwrite bool) error {
	role := source.Role()
	if c.currentRole(source) != nil {
		if !overwrite {
			return nil
		}
		if err := c.OpClient.DeleteRole(role.GetNamespace(), role.GetName(), metav1.NewDeleteOptions(0)); err!=nil {
			return err
		}
	}
	_, err := c.OpClient.CreateRole(role)
	return err
}

func (c *ConfigMapRegistryCreator) ensureRoleBinding(source catalogSourceDeriver, overwrite bool) error {
	roleBinding := source.RoleBinding()
	if c.currentRoleBinding(source) != nil {
		if !overwrite {
			return nil
		}
		if err := c.OpClient.DeleteRoleBinding(roleBinding.GetNamespace(), roleBinding.GetName(), metav1.NewDeleteOptions(0)); err!=nil {
			return err
		}
	}
	_, err := c.OpClient.CreateRoleBinding(roleBinding)
	return err
}

func (c *ConfigMapRegistryCreator) ensurePod(source catalogSourceDeriver, overwrite bool) error {
	pod := source.Pod(c.Image)
	if c.currentPod(source, c.Image) != nil {
		if !overwrite {
			return nil
		}
		if err := c.OpClient.KubernetesInterface().CoreV1().Pods(pod.GetNamespace()).Delete(pod.GetName(), metav1.NewDeleteOptions(0)); err!=nil {
			return err
		}
	}
	_, err := c.OpClient.KubernetesInterface().CoreV1().Pods(pod.GetNamespace()).Create(pod)
	return err
}

func (c *ConfigMapRegistryCreator) ensureService(source catalogSourceDeriver, overwrite bool) error {
	service := source.Service()
	if c.currentService(source) != nil {
		if !overwrite {
			return nil
		}
		if err := c.OpClient.DeleteService(service.GetNamespace(), service.GetName(), metav1.NewDeleteOptions(0)); err!=nil {
			return err
		}
	}
	_, err := c.OpClient.CreateService(service)
	return err
}

