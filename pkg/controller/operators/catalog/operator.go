package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	registryclient "github.com/operator-framework/operator-registry/pkg/client"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1beta1ext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/operatorlister"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/client"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/informers/externalversions"
	olmerrors "github.com/operator-framework/operator-lifecycle-manager/pkg/controller/errors"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/controller/registry"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/controller/registry/resolver"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/ownerutil"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/queueinformer"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/metrics"
)

const (
	crdKind                = "CustomResourceDefinition"
	secretKind             = "Secret"
	clusterRoleKind        = "ClusterRole"
	clusterRoleBindingKind = "ClusterRoleBinding"
	serviceAccountKind     = "ServiceAccount"
	roleKind               = "Role"
	roleBindingKind        = "RoleBinding"
)

//for test stubbing and for ensuring standardization of timezones to UTC
var timeNow = func() metav1.Time { return metav1.NewTime(time.Now().UTC()) }

// Operator represents a Kubernetes operator that executes InstallPlans by
// resolving dependencies in a catalog.
type Operator struct {
	*queueinformer.Operator
	client                      versioned.Interface
	namespace                   string
	sources                     map[resolver.CatalogKey]registryclient.Interface
	sourcesLock                 sync.RWMutex
	sourcesLastUpdate           metav1.Time
	resolvers                   map[string]resolver.Resolver
	subQueue                    workqueue.RateLimitingInterface
	catSrcQueue                 workqueue.RateLimitingInterface
	lister                      operatorlister.OperatorLister
	configmapRegistryReconciler *registry.ConfigMapRegistryReconciler
}

// NewOperator creates a new Catalog Operator.
func NewOperator(kubeconfigPath string, wakeupInterval time.Duration, configmapRegistryImage string, operatorNamespace string, watchedNamespaces ...string) (*Operator, error) {
	// Default to watching all namespaces.
	if watchedNamespaces == nil {
		watchedNamespaces = []string{metav1.NamespaceAll}
	}

	// Create a new client for ALM types (CRs)
	crClient, err := client.NewClient(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	var namespaceResolvers = make(map[string]resolver.Resolver)
	// Create an informer for each watched namespace.
	ipSharedIndexInformers := []cache.SharedIndexInformer{}
	subSharedIndexInformers := []cache.SharedIndexInformer{}
	for _, namespace := range watchedNamespaces {
		nsInformerFactory := externalversions.NewSharedInformerFactoryWithOptions(crClient, wakeupInterval, externalversions.WithNamespace(namespace))
		ipSharedIndexInformers = append(ipSharedIndexInformers, nsInformerFactory.Operators().V1alpha1().InstallPlans().Informer())
		subSharedIndexInformers = append(subSharedIndexInformers, nsInformerFactory.Operators().V1alpha1().Subscriptions().Informer())
		namespaceResolvers[namespace] = resolver.NewNamespaceResolver(namespace, nsInformerFactory)
	}

	// Create an informer for each catalog namespace
	catsrcSharedIndexInformers := []cache.SharedIndexInformer{}
	for _, namespace := range []string{operatorNamespace} {
		nsInformerFactory := externalversions.NewSharedInformerFactoryWithOptions(crClient, wakeupInterval, externalversions.WithNamespace(namespace))
		catsrcSharedIndexInformers = append(catsrcSharedIndexInformers, nsInformerFactory.Operators().V1alpha1().CatalogSources().Informer())
	}

	// Create a new queueinformer-based operator.
	queueOperator, err := queueinformer.NewOperator(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	// Allocate the new instance of an Operator.
	op := &Operator{
		Operator:           queueOperator,
		client:             crClient,
		namespace:          operatorNamespace,
		lister:             operatorlister.NewLister(),
		sources:            make(map[resolver.CatalogKey]registryclient.Interface),
		resolvers:          namespaceResolvers,
	}

	// Register CatalogSource informers.
	catsrcQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "catalogsources")
	catsrcQueueInformer := queueinformer.New(
		catsrcQueue,
		catsrcSharedIndexInformers,
		op.syncCatalogSources,
		nil,
		"catsrc",
		metrics.NewMetricsCatalogSource(op.client),
	)
	for _, informer := range catsrcQueueInformer {
		op.RegisterQueueInformer(informer)
	}
	op.catSrcQueue = catsrcQueue

	// Register InstallPlan informers.
	ipQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "installplans")
	ipQueueInformers := queueinformer.New(
		ipQueue,
		ipSharedIndexInformers,
		op.syncInstallPlans,
		nil,
		"installplan",
		metrics.NewMetricsInstallPlan(op.client),
	)
	for _, informer := range ipQueueInformers {
		op.RegisterQueueInformer(informer)
	}

	// Register Subscription informers.
	subscriptionQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "subscriptions")
	subscriptionQueueInformers := queueinformer.New(
		subscriptionQueue,
		subSharedIndexInformers,
		op.syncSubscriptions,
		nil,
		"subscription",
		metrics.NewMetricsSubscription(op.client),
	)
	op.subQueue = subscriptionQueue
	for _, informer := range subscriptionQueueInformers {
		op.RegisterQueueInformer(informer)
	}

	handleDelete := &cache.ResourceEventHandlerFuncs{
		DeleteFunc: op.handleDeletion,
	}
	// Set up informers for requeuing catalogs
	for _, namespace := range watchedNamespaces {
		roleQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "role")
		roleBindingQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "rolebinding")
		serviceAccountQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "serviceaccount")
		serviceQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "service")
		podQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "pod")
		configmapQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "configmap")

		informers.NewSharedInformerFactoryWithOptions(op.OpClient.KubernetesInterface(), wakeupInterval, informers.WithNamespace(namespace))
		informerFactory := informers.NewSharedInformerFactory(op.OpClient.KubernetesInterface(), wakeupInterval)
		roleInformer := informerFactory.Rbac().V1().Roles()
		roleBindingInformer := informerFactory.Rbac().V1().RoleBindings()
		serviceAccountInformer := informerFactory.Core().V1().ServiceAccounts()
		serviceInformer := informerFactory.Core().V1().Services()
		podInformer := informerFactory.Core().V1().Pods()
		configMapInformer := informerFactory.Core().V1().ConfigMaps()

		queueInformers := []*queueinformer.QueueInformer{
			queueinformer.NewInformer(roleQueue, roleInformer.Informer(), op.syncObject, handleDelete, "role", metrics.NewMetricsNil()),
			queueinformer.NewInformer(roleBindingQueue, roleBindingInformer.Informer(), op.syncObject, handleDelete, "rolebinding", metrics.NewMetricsNil()),
			queueinformer.NewInformer(serviceAccountQueue, serviceAccountInformer.Informer(), op.syncObject, handleDelete, "serviceaccount", metrics.NewMetricsNil()),
			queueinformer.NewInformer(serviceQueue, serviceInformer.Informer(), op.syncObject, handleDelete, "service", metrics.NewMetricsNil()),
			queueinformer.NewInformer(podQueue, podInformer.Informer(), op.syncObject, handleDelete, "pod", metrics.NewMetricsNil()),
			queueinformer.NewInformer(configmapQueue, configMapInformer.Informer(), op.syncObject, handleDelete, "configmap", metrics.NewMetricsNil()),
		}
		for _, q := range queueInformers {
			op.RegisterQueueInformer(q)
		}

		op.lister.RbacV1().RegisterRoleLister(namespace, roleInformer.Lister())
		op.lister.RbacV1().RegisterRoleBindingLister(namespace, roleBindingInformer.Lister())
		op.lister.CoreV1().RegisterServiceAccountLister(namespace, serviceAccountInformer.Lister())
		op.lister.CoreV1().RegisterServiceLister(namespace, serviceInformer.Lister())
		op.lister.CoreV1().RegisterPodLister(namespace, podInformer.Lister())
		op.lister.CoreV1().RegisterConfigMapLister(namespace, configMapInformer.Lister())
	}
	op.configmapRegistryReconciler = &registry.ConfigMapRegistryReconciler{
		Image:    configmapRegistryImage,
		OpClient: op.OpClient,
		Lister:   op.lister,
	}
	return op, nil
}

func (o *Operator) syncObject(obj interface{}) (syncError error) {
	// Assert as runtime.Object
	runtimeObj, ok := obj.(runtime.Object)
	if !ok {
		syncError = errors.New("object sync: casting to runtime.Object failed")
		log.Warn(syncError.Error())
		return
	}

	gvk := runtimeObj.GetObjectKind().GroupVersionKind()
	logger := log.WithFields(log.Fields{
		"group":   gvk.Group,
		"version": gvk.Version,
		"kind":    gvk.Kind,
	})

	// Assert as metav1.Object
	metaObj, ok := obj.(metav1.Object)
	if !ok {
		syncError = errors.New("object sync: casting to metav1.Object failed")
		logger.Warn(syncError.Error())
		return
	}
	logger = logger.WithFields(log.Fields{
		"name":      metaObj.GetName(),
		"namespace": metaObj.GetNamespace(),
	})

	logger.Debug("syncing")

	if ownerutil.IsOwnedByKind(metaObj, v1alpha1.CatalogSourceKind) {
		logger.Debug("requeueing owner CatalogSource")
		owner := ownerutil.GetOwnerByKind(metaObj, v1alpha1.CatalogSourceKind)
		o.catSrcQueue.AddRateLimited(fmt.Sprintf("%s/%s", metaObj.GetNamespace(), owner.Name))
	}

	return nil
}

func (o *Operator) handleDeletion(obj interface{}) {
	ownee, ok := obj.(metav1.Object)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("Couldn't get object from tombstone %#v", obj))
			return
		}

		ownee, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("Tombstone contained object that is not a metav1 object %#v", obj))
			return
		}
	}

	if owner := ownerutil.GetOwnerByKind(ownee, v1alpha1.CatalogSourceKind); owner != nil {
		o.catSrcQueue.AddRateLimited(fmt.Sprintf("%s/%s", ownee.GetNamespace(), owner.Name))
	}
}

func (o *Operator) syncCatalogSources(obj interface{}) (syncError error) {
	catsrc, ok := obj.(*v1alpha1.CatalogSource)
	if !ok {
		log.Debugf("wrong type: %#v", obj)
		return fmt.Errorf("casting CatalogSource failed")
	}

	logger := log.WithFields(log.Fields{
		"source": catsrc.GetName(),
	})

	if catsrc.Spec.SourceType == v1alpha1.SourceTypeInternal || catsrc.Spec.SourceType == v1alpha1.SourceTypeConfigmap {
		return o.syncConfigMapSource(logger, catsrc)
	}

	logger.WithField("sourceType", catsrc.Spec.SourceType).Warn("unknown source type")

	// TODO: write status about invalid source type

	return nil
}

func (o *Operator) syncConfigMapSource(logger *log.Entry, catsrc *v1alpha1.CatalogSource) (syncError error) {

	// Get the catalog source's config map
	configMap, err := o.lister.CoreV1().ConfigMapLister().ConfigMaps(catsrc.GetNamespace()).Get(catsrc.Spec.ConfigMap)
	if err != nil {
		return fmt.Errorf("failed to get catalog config map %s: %s", catsrc.Spec.ConfigMap, err)
	}


	if catsrc.Status.ConfigMapResource == nil || catsrc.Status.ConfigMapResource.UID != configMap.GetUID() || catsrc.Status.ConfigMapResource.ResourceVersion != configMap.GetResourceVersion() {
		// configmap ref nonexistant or updated, write out the new configmap ref to status and exit
		out := catsrc.DeepCopy()
		out.Status.ConfigMapResource = &v1alpha1.ConfigMapResourceReference{
			Name:            configMap.GetName(),
			Namespace:       configMap.GetNamespace(),
			UID:             configMap.GetUID(),
			ResourceVersion: configMap.GetResourceVersion(),
		}
		out.Status.LastSync = timeNow()

		// update status
		if _, err = o.client.OperatorsV1alpha1().CatalogSources(out.GetNamespace()).UpdateStatus(out); err != nil {
			return err
		}
		return nil
	}

	// configmap ref is up to date, continue parsing
	if catsrc.Status.RegistryServiceStatus == nil || catsrc.Status.RegistryServiceStatus.CreatedAt.Before(&catsrc.Status.LastSync) {
		// if registry pod hasn't been created or hasn't been updated since the last configmap update, recreate it

		out := catsrc.DeepCopy()
		if err := o.configmapRegistryReconciler.EnsureRegistryServer(out); err != nil {
			logger.WithError(err).Warn("couldn't ensure registry server")
			return err
		}

		if !catsrc.Status.LastSync.Before(&out.Status.LastSync) {
			return nil
		}

		// update status
		if _, err = o.client.OperatorsV1alpha1().CatalogSources(out.GetNamespace()).UpdateStatus(out); err != nil {
			return err
		}
		o.sourcesLastUpdate = timeNow()
		return nil
	}

	// check if client exists
	sourceKey := resolver.CatalogKey{Name: catsrc.GetName(), Namespace: catsrc.GetNamespace()}
	var sourceFound bool
	func() {
		o.sourcesLock.RLock()
		defer o.sourcesLock.RUnlock()
		_, sourceFound = o.sources[sourceKey]
	}()

	// create client if it doesn't yet exist
	if !sourceFound {
		client, err := registryclient.NewClient(catsrc.Status.RegistryServiceStatus.Address())
		if err != nil {
			logger.WithError(err).WithField("address", catsrc.Status.RegistryServiceStatus.Address()).Warn("failed to connect to catalog source registry")
			return err
		}

		func() {
			o.sourcesLock.Lock()
			defer o.sourcesLock.Unlock()
			o.sources[sourceKey] = client
		}()
	}

	// TODO: health check source

	// no updates
	return nil
}

func (o *Operator) syncSubscriptions(obj interface{}) error {
	sub, ok := obj.(*v1alpha1.Subscription)
	if !ok {
		log.Debugf("wrong type: %#v", obj)
		return fmt.Errorf("casting Subscription failed")
	}
	namespace := sub.GetNamespace()

	if err := o.ensureSubscriptionCSVState(sub); err!=nil {
		return err
	}

	if !o.shouldUpdateSubscription(sub) {
		return nil
	}

	nsResolver := o.resolvers[namespace]

	//TODO: double check if this lock is still needed
	o.sourcesLock.RLock()
	defer o.sourcesLock.RUnlock()

	steps, subs, err := nsResolver.ResolveSteps(resolver.NewNamespaceSourceQuerier(o.sources))
	if err != nil {
		return err
	}

	installPlanApproval := v1alpha1.ApprovalAutomatic
	for _, sub := range subs {
		if sub.Spec.InstallPlanApproval == v1alpha1.ApprovalManual {
			installPlanApproval = v1alpha1.ApprovalManual
			break
		}
	}

	installplanReference, err := o.createInstallPlan(namespace, installPlanApproval, steps)
	if err!=nil {
		return err
	}

	if err := o.ensureSubscriptionInstallPlanState(namespace, subs, installplanReference); err!=nil {
		return err
	}
	return nil
}

func (o *Operator) shouldUpdateSubscription(sub *v1alpha1.Subscription) bool {
	// Only sync if catalog has been updated since last sync time
	if o.sourcesLastUpdate.Before(&sub.Status.LastUpdated) && sub.Status.State == v1alpha1.SubscriptionStateAtLatest {
		log.Infof("skipping update: no new updates to catalog since last sync at %s", sub.Status.LastUpdated.String())
		return false
	}
	if sub.Status.Install != nil && sub.Status.State == v1alpha1.SubscriptionStateUpgradePending {
		log.Infof("skipping update: installplan already created")
		return false
	}
	return true
}

func (o *Operator) ensureSubscriptionCSVState(sub *v1alpha1.Subscription) error {
	// TODO pull from cache
	csv, err := o.client.OperatorsV1alpha1().ClusterServiceVersions(sub.GetNamespace()).Get(sub.Status.CurrentCSV, metav1.GetOptions{})
	out := sub.DeepCopy()
	if err != nil || csv == nil {
		out.Status.State = v1alpha1.SubscriptionStateUpgradePending
	} else {
		out.Status.State = v1alpha1.SubscriptionStateAtLatest
	}

	if sub.Status == out.Status {
		// The subscription status represents the cluster state
		return nil
	}
	out.Status.LastUpdated = timeNow()

	// Update Subscription with status of transition. Log errors if we can't write them to the status.
	if sub, err = o.client.OperatorsV1alpha1().Subscriptions(out.GetNamespace()).UpdateStatus(out); err != nil {
		log.WithError(err).Info("error updating subscription status")
		return fmt.Errorf("error updating Subscription status: " + err.Error())
	}

	// subscription status represents cluster state
	return nil
}

func (o *Operator) ensureSubscriptionInstallPlanState(namespace string, subs []*v1alpha1.Subscription, installPlanRef *v1alpha1.InstallPlanReference) error {
	//TODO parallel, sync waitgroup
	for _, sub := range subs {
		sub.Status.Install = installPlanRef
		if _, err := o.client.OperatorsV1alpha1().Subscriptions(namespace).UpdateStatus(sub); err != nil {
			return err
		}
	}
	return nil
}

func (o *Operator) createInstallPlan(namespace string, installPlanApproval v1alpha1.Approval, steps []*v1alpha1.Step) (*v1alpha1.InstallPlanReference, error) {
	if len(steps) == 0 {
		return nil, nil
	}

	csvNames := []string{}
	catalogSourceMap := map[string]struct{}{}
	for _, s := range steps {
		if s.Resource.Kind == "ClusterServiceVersion" {
			csvNames = append(csvNames, s.Resource.Name)
		}
		catalogSourceMap[s.Resource.CatalogSource] = struct{}{}
	}
	catalogSources := []string{}
	for s := range catalogSourceMap {
		catalogSources = append(catalogSources, s)
	}

	phase := v1alpha1.InstallPlanPhaseInstalling
	if installPlanApproval == v1alpha1.ApprovalManual {
		phase = v1alpha1.InstallPlanPhaseRequiresApproval
	}
	ip := &v1alpha1.InstallPlan{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "install-",
			Namespace: namespace,
		},
		Spec: v1alpha1.InstallPlanSpec{
			ClusterServiceVersionNames: csvNames,
			Approval:                   installPlanApproval,
			Approved:                   installPlanApproval == v1alpha1.ApprovalAutomatic,
		},
	}

	res, err := o.client.OperatorsV1alpha1().InstallPlans(namespace).Create(ip)
	if err != nil {
		return nil, err
	}

	res.Status = v1alpha1.InstallPlanStatus{
		Phase: phase,
		Plan: steps,
		CatalogSources: catalogSources,
	}
	res, err = o.client.OperatorsV1alpha1().InstallPlans(namespace).UpdateStatus(res)
	if err != nil {
		return nil, err
	}
	return &v1alpha1.InstallPlanReference{
		UID:        res.GetUID(),
		Name:       res.GetName(),
		APIVersion: v1alpha1.SchemeGroupVersion.String(),
		Kind:       v1alpha1.InstallPlanKind,
	}, nil

}

func (o *Operator) requeueInstallPlan(name, namespace string) {
	// we can build the key directly, will need to change if queue uses different key scheme
	key := fmt.Sprintf("%s/%s", namespace, name)
	o.subQueue.AddRateLimited(key)
	return
}

func (o *Operator) syncInstallPlans(obj interface{}) (syncError error) {
	plan, ok := obj.(*v1alpha1.InstallPlan)
	if !ok {
		log.Debugf("wrong type: %#v", obj)
		return fmt.Errorf("casting InstallPlan failed")
	}

	logger := log.WithFields(log.Fields{
		"ip":        plan.GetName(),
		"namespace": plan.GetNamespace(),
		"phase":     plan.Status.Phase,
	})

	logger.Info("syncing")
	outInstallPlan, syncError := transitionInstallPlanState(o, *plan)

	if syncError != nil {
		logger = logger.WithField("syncError", syncError)
	}

	// no changes in status, don't update
	if outInstallPlan.Status.Phase == plan.Status.Phase {
		return
	}

	// notify subscription loop of installplan changes
	if ownerutil.IsOwnedByKind(outInstallPlan, v1alpha1.SubscriptionKind) {
		oref := ownerutil.GetOwnerByKind(outInstallPlan, v1alpha1.SubscriptionKind)
		o.requeueInstallPlan(oref.Name, outInstallPlan.GetNamespace())
	}

	// Update InstallPlan with status of transition. Log errors if we can't write them to the status.
	if _, err := o.client.OperatorsV1alpha1().InstallPlans(plan.GetNamespace()).UpdateStatus(outInstallPlan); err != nil {
		logger = logger.WithField("updateError", err.Error())
		updateErr := errors.New("error updating InstallPlan status: " + err.Error())
		if syncError == nil {
			logger.Info("error updating InstallPlan status")
			return updateErr
		}
		logger.Info("error transitioning InstallPlan")
		syncError = fmt.Errorf("error transitioning InstallPlan: %s and error updating InstallPlan status: %s", syncError, updateErr)
	}
	return
}

type installPlanTransitioner interface {
	ResolvePlan(*v1alpha1.InstallPlan) error
	ExecutePlan(*v1alpha1.InstallPlan) error
}

var _ installPlanTransitioner = &Operator{}

func transitionInstallPlanState(transitioner installPlanTransitioner, in v1alpha1.InstallPlan) (*v1alpha1.InstallPlan, error) {
	logger := log.WithFields(log.Fields{
		"ip":        in.GetName(),
		"namespace": in.GetNamespace(),
		"phase":     in.Status.Phase,
	})

	out := in.DeepCopy()

	switch in.Status.Phase {
	case v1alpha1.InstallPlanPhaseRequiresApproval:
		if out.Spec.Approved {
			logger.Debugf("approved, setting to %s", v1alpha1.InstallPlanPhasePlanning)
			out.Status.Phase = v1alpha1.InstallPlanPhaseInstalling
		} else {
			logger.Debug("not approved, skipping sync")
		}
		return out, nil

	case v1alpha1.InstallPlanPhaseInstalling:
		logger.Debug("attempting to install")
		if err := transitioner.ExecutePlan(out); err != nil {
			out.Status.SetCondition(v1alpha1.ConditionFailed(v1alpha1.InstallPlanInstalled,
				v1alpha1.InstallPlanReasonComponentFailed, err))
			out.Status.Phase = v1alpha1.InstallPlanPhaseFailed
			return out, err
		}
		out.Status.SetCondition(v1alpha1.ConditionMet(v1alpha1.InstallPlanInstalled))
		out.Status.Phase = v1alpha1.InstallPlanPhaseComplete
		return out, nil
	default:
		return out, nil
	}
}

// ResolvePlan modifies an InstallPlan to contain a Plan in its Status field.
func (o *Operator) ResolvePlan(plan *v1alpha1.InstallPlan) error {
	return nil
}

// ExecutePlan applies a planned InstallPlan to a namespace.
func (o *Operator) ExecutePlan(plan *v1alpha1.InstallPlan) error {
	if plan.Status.Phase != v1alpha1.InstallPlanPhaseInstalling {
		panic("attempted to install a plan that wasn't in the installing phase")
	}

	// Get the set of initial installplan csv names
	initialCSVNames := getCSVNameSet(plan)
	// Get pre-existing CRD owners to make decisions about applying resolved CSVs
	existingCRDOwners, err := o.getExistingApiOwners(plan.GetNamespace())
	if err != nil {
		return err
	}

	for i, step := range plan.Status.Plan {
		switch step.Status {
		case v1alpha1.StepStatusPresent, v1alpha1.StepStatusCreated:
			continue

		case v1alpha1.StepStatusUnknown, v1alpha1.StepStatusNotPresent:
			log.Debugf("resource kind: %s", step.Resource.Kind)
			log.Debugf("resource name: %s", step.Resource.Name)
			switch step.Resource.Kind {
			case crdKind:
				// Marshal the manifest into a CRD instance.
				var crd v1beta1ext.CustomResourceDefinition
				err := json.Unmarshal([]byte(step.Resource.Manifest), &crd)
				if err != nil {
					return err
				}

				// TODO: check that names are accepted
				// Attempt to create the CRD.
				_, err = o.OpClient.ApiextensionsV1beta1Interface().ApiextensionsV1beta1().CustomResourceDefinitions().Create(&crd)
				if k8serrors.IsAlreadyExists(err) {
					// If it already existed, mark the step as Present.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusPresent
					continue
				} else if err != nil {
					return err
				} else {
					// If no error occured, mark the step as Created.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusCreated
					continue
				}

			case v1alpha1.ClusterServiceVersionKind:
				// Marshal the manifest into a CSV instance.
				var csv v1alpha1.ClusterServiceVersion
				err := json.Unmarshal([]byte(step.Resource.Manifest), &csv)
				if err != nil {
					return err
				}

				// Check if the resolved CSV is in the initial set
				if _, ok := initialCSVNames[csv.GetName()]; !ok {
					// Check for pre-existing CSVs that own the same CRDs
					competingOwners, err := competingCRDOwnersExist(plan.GetNamespace(), &csv, existingCRDOwners)
					if err != nil {
						return err
					}

					// TODO: decide on fail/continue logic for pre-existing dependent CSVs that own the same CRD(s)
					if competingOwners {
						// For now, error out
						return fmt.Errorf("pre-existing CRD owners found for owned CRD(s) of dependent CSV %s", csv.GetName())
					}
				}

				// Attempt to create the CSV.
				_, err = o.client.OperatorsV1alpha1().ClusterServiceVersions(csv.GetNamespace()).Create(&csv)
				if k8serrors.IsAlreadyExists(err) {
					// If it already existed, mark the step as Present.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusPresent
				} else if err != nil {
					return err
				} else {
					// If no error occurred, mark the step as Created.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusCreated
				}

			case secretKind:
				// Get the pre-existing secret.
				secret, err := o.OpClient.KubernetesInterface().CoreV1().Secrets(o.namespace).Get(step.Resource.Name, metav1.GetOptions{})
				if k8serrors.IsNotFound(err) {
					return fmt.Errorf("secret %s does not exist", step.Resource.Name)
				} else if err != nil {
					return err
				}

				// Set the namespace to the InstallPlan's namespace and attempt to
				// create a new secret.
				secret.Namespace = plan.Namespace
				_, err = o.OpClient.KubernetesInterface().CoreV1().Secrets(plan.Namespace).Create(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secret.Name,
						Namespace: plan.Namespace,
					},
					Data: secret.Data,
					Type: secret.Type,
				})
				if k8serrors.IsAlreadyExists(err) {
					// If it already existed, mark the step as Present.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusPresent
				} else if err != nil {
					return err
				} else {
					// If no error occured, mark the step as Created.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusCreated
				}

			case clusterRoleKind:
				// Marshal the manifest into a ClusterRole instance.
				var cr rbacv1.ClusterRole
				err := json.Unmarshal([]byte(step.Resource.Manifest), &cr)
				if err != nil {
					return err
				}

				// Update UIDs on all CSV OwnerReferences
				updated, err := o.getUpdatedOwnerReferences(cr.OwnerReferences, plan.Namespace)
				if err != nil {
					return err
				}
				cr.OwnerReferences = updated

				// Attempt to create the ClusterRole.
				_, err = o.OpClient.KubernetesInterface().RbacV1().ClusterRoles().Create(&cr)
				if k8serrors.IsAlreadyExists(err) {
					// If it already existed, mark the step as Present.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusPresent
				} else if err != nil {
					return err
				} else {
					// If no error occurred, mark the step as Created.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusCreated
				}
			case clusterRoleBindingKind:
				// Marshal the manifest into a RoleBinding instance.
				var rb rbacv1.ClusterRoleBinding
				err := json.Unmarshal([]byte(step.Resource.Manifest), &rb)
				if err != nil {
					return err
				}

				// Update UIDs on all CSV OwnerReferences
				updated, err := o.getUpdatedOwnerReferences(rb.OwnerReferences, plan.Namespace)
				if err != nil {
					return err
				}
				rb.OwnerReferences = updated

				// Attempt to create the ClusterRoleBinding.
				_, err = o.OpClient.KubernetesInterface().RbacV1().ClusterRoleBindings().Create(&rb)
				if k8serrors.IsAlreadyExists(err) {
					rb.SetNamespace(plan.Namespace)
					_, err = o.OpClient.UpdateClusterRoleBinding(&rb)
					if err != nil {
						return err
					}

					// If it already existed, mark the step as Present.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusPresent
				} else if err != nil {
					return err
				} else {
					// If no error occurred, mark the step as Created.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusCreated
				}

			case roleKind:
				// Marshal the manifest into a Role instance.
				var r rbacv1.Role
				err := json.Unmarshal([]byte(step.Resource.Manifest), &r)
				if err != nil {
					return err
				}

				// Update UIDs on all CSV OwnerReferences
				updated, err := o.getUpdatedOwnerReferences(r.OwnerReferences, plan.Namespace)
				if err != nil {
					return err
				}
				r.OwnerReferences = updated

				// Attempt to create the Role.
				_, err = o.OpClient.KubernetesInterface().RbacV1().Roles(plan.Namespace).Create(&r)
				if k8serrors.IsAlreadyExists(err) {
					// If it already existed, mark the step as Present.
					r.SetNamespace(plan.Namespace)
					_, err = o.OpClient.UpdateRole(&r)
					if err != nil {
						return err
					}

					plan.Status.Plan[i].Status = v1alpha1.StepStatusPresent
				} else if err != nil {
					return err
				} else {
					// If no error occurred, mark the step as Created.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusCreated
				}

			case roleBindingKind:
				// Marshal the manifest into a RoleBinding instance.
				var rb rbacv1.RoleBinding
				err := json.Unmarshal([]byte(step.Resource.Manifest), &rb)
				if err != nil {
					return err
				}

				// Update UIDs on all CSV OwnerReferences
				updated, err := o.getUpdatedOwnerReferences(rb.OwnerReferences, plan.Namespace)
				if err != nil {
					return err
				}
				rb.OwnerReferences = updated

				// Attempt to create the RoleBinding.
				_, err = o.OpClient.KubernetesInterface().RbacV1().RoleBindings(plan.Namespace).Create(&rb)
				if k8serrors.IsAlreadyExists(err) {
					rb.SetNamespace(plan.Namespace)
					_, err = o.OpClient.UpdateRoleBinding(&rb)
					if err != nil {
						return err
					}

					// If it already existed, mark the step as Present.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusPresent
				} else if err != nil {
					return err
				} else {
					// If no error occurred, mark the step as Created.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusCreated
				}

			case serviceAccountKind:
				// Marshal the manifest into a ServiceAccount instance.
				var sa corev1.ServiceAccount
				err := json.Unmarshal([]byte(step.Resource.Manifest), &sa)
				if err != nil {
					return err
				}

				// Update UIDs on all CSV OwnerReferences
				updated, err := o.getUpdatedOwnerReferences(sa.OwnerReferences, plan.Namespace)
				if err != nil {
					return err
				}
				sa.OwnerReferences = updated

				// Attempt to create the ServiceAccount.
				_, err = o.OpClient.KubernetesInterface().CoreV1().ServiceAccounts(plan.Namespace).Create(&sa)
				if k8serrors.IsAlreadyExists(err) {
					// If it already exists we need to patch the existing SA with the new OwnerReferences
					sa.SetNamespace(plan.Namespace)
					_, err = o.OpClient.UpdateServiceAccount(&sa)
					if err != nil {
						return err
					}

					// Mark as present
					plan.Status.Plan[i].Status = v1alpha1.StepStatusPresent
				} else if err != nil {
					return err
				} else {
					// If no error occurred, mark the step as Created.
					plan.Status.Plan[i].Status = v1alpha1.StepStatusCreated
				}

			default:
				return v1alpha1.ErrInvalidInstallPlan
			}

		default:
			return v1alpha1.ErrInvalidInstallPlan
		}
	}

	// Loop over one final time to check and see if everything is good.
	for _, step := range plan.Status.Plan {
		switch step.Status {
		case v1alpha1.StepStatusCreated, v1alpha1.StepStatusPresent:
		default:
			return nil
		}
	}

	return nil
}

// getExistingApiOwners creates a map of CRD names to existing owner CSVs in the given namespace
func (o *Operator) getExistingApiOwners(namespace string) (map[string][]string, error) {
	// Get a list of CSV CRs in the namespace
	csvList, err := o.client.OperatorsV1alpha1().ClusterServiceVersions(namespace).List(metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	// Map CRD names to existing owner CSV CRs in the namespace
	owners := make(map[string][]string)
	for _, csv := range csvList.Items {
		for _, crd := range csv.Spec.CustomResourceDefinitions.Owned {
			owners[crd.Name] = append(owners[crd.Name], csv.GetName())
		}
		for _, api := range csv.Spec.APIServiceDefinitions.Owned {
			owners[api.Group] = append(owners[api.Group], csv.GetName())
		}
	}

	return owners, nil
}

func (o *Operator) getUpdatedOwnerReferences(refs []metav1.OwnerReference, namespace string) ([]metav1.OwnerReference, error) {
	updated := append([]metav1.OwnerReference(nil), refs...)

	for i, owner := range refs {
		if owner.Kind == v1alpha1.ClusterServiceVersionKind {
			csv, err := o.client.Operators().ClusterServiceVersions(namespace).Get(owner.Name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			owner.UID = csv.GetUID()
			updated[i] = owner
		}
	}
	return updated, nil
}

// competingCRDOwnersExist returns true if there exists a CSV that owns at least one of the given CSVs owned CRDs (that's not the given CSV)
func competingCRDOwnersExist(namespace string, csv *v1alpha1.ClusterServiceVersion, existingOwners map[string][]string) (bool, error) {
	// Attempt to find a pre-existing owner in the namespace for any owned crd
	for _, crdDesc := range csv.Spec.CustomResourceDefinitions.Owned {
		crdOwners := existingOwners[crdDesc.Name]
		l := len(crdOwners)
		switch {
		case l == 1:
			// One competing owner found
			if crdOwners[0] != csv.GetName() {
				return true, nil
			}
		case l > 1:
			return true, olmerrors.NewMultipleExistingCRDOwnersError(crdOwners, crdDesc.Name, namespace)
		}
	}

	return false, nil
}

// getCSVNameSet returns a set of the given installplan's csv names
func getCSVNameSet(plan *v1alpha1.InstallPlan) map[string]struct{} {
	csvNameSet := make(map[string]struct{})
	for _, name := range plan.Spec.ClusterServiceVersionNames {
		csvNameSet[name] = struct{}{}
	}

	return csvNameSet
}
