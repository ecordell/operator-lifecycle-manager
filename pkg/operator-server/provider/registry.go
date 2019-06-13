package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/operator-framework/operator-registry/pkg/api"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/pkg/util/labels"

	operatorsv1alpha1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/informers/externalversions"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/lib/queueinformer"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/metrics"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apis/operators"
)

// OperatorProvider aggregates several `CatalogSources` and establishes gRPC connections to their registry servers.
type OperatorProvider struct {
	*queueinformer.Operator

	globalNamespace string
}

var _ PackageManifestProvider = &OperatorProvider{}

func NewOperatorProvider(crClient versioned.Interface, operator *queueinformer.Operator, wakeupInterval time.Duration, watchedNamespaces []string, globalNamespace string) *OperatorProvider {
	p := &OperatorProvider{
		Operator: operator,

		globalNamespace: globalNamespace,
	}

	for _, namespace := range watchedNamespaces {
		factory := externalversions.NewSharedInformerFactoryWithOptions(crClient, wakeupInterval, externalversions.WithNamespace(namespace))
		sourceInformer := factory.Operators().V1alpha1().CatalogSources()

		// Register queue and QueueInformer
		logrus.WithField("namespace", namespace).Info("watching operatorgroups")
		queueName := fmt.Sprintf("%s/operatorgroups", namespace)
		sourceQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), queueName)
		sourceQueueInformer := queueinformer.NewInformer(sourceQueue, sourceInformer.Informer(), p.syncCatalogSource, sourceHandlers, queueName, metrics.NewMetricsNil(), logrus.New())
		p.RegisterQueueInformer(sourceQueueInformer)
	}

	// Watch OperatorGroups

	// Watch CSVs

	// Get OperatorGroups that Include Namespace, Get Operators In those operatorgroups
	// Indexes?

	// sourceHandlers := &cache.ResourceEventHandlerFuncs{
	// 	DeleteFunc: p.catalogSourceDeleted,
	// }
	// for _, namespace := range watchedNamespaces {
	// 	factory := externalversions.NewSharedInformerFactoryWithOptions(crClient, wakeupInterval, externalversions.WithNamespace(namespace))
	// 	sourceInformer := factory.Operators().V1alpha1().CatalogSources()
	//
	// 	// Register queue and QueueInformer
	// 	logrus.WithField("namespace", namespace).Info("watching catalogsources")
	// 	queueName := fmt.Sprintf("%s/catalogsources", namespace)
	// 	sourceQueue := workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), queueName)
	// 	sourceQueueInformer := queueinformer.NewInformer(sourceQueue, sourceInformer.Informer(), p.syncCatalogSource, sourceHandlers, queueName, metrics.NewMetricsNil(), logrus.New())
	// 	p.RegisterQueueInformer(sourceQueueInformer)
	// }

	return p
}

func (p *OperatorProvider) syncCatalogSource(obj interface{}) (syncError error) {
	source, ok := obj.(*operatorsv1alpha1.CatalogSource)
	if !ok {
		logrus.Errorf("catalogsource type assertion failed: wrong type: %#v", obj)
	}

	logger := logrus.WithFields(logrus.Fields{
		"action":    "sync catalogsource",
		"name":      source.GetName(),
		"namespace": source.GetNamespace(),
	})

	if source.Status.RegistryServiceStatus == nil {
		logger.Debug("registry service is not ready for grpc connection")
		return
	}

	key := sourceKey{source.GetName(), source.GetNamespace()}
	client, ok := p.getClient(key)
	if ok && source.Status.RegistryServiceStatus.ServiceName != "" {
		logger.Info("update detected, attempting to reset grpc connection")
		client.conn.ResetConnectBackoff()

		ctx, cancel := context.WithTimeout(context.TODO(), defaultConnectionTimeout)
		defer cancel()

		changed := client.conn.WaitForStateChange(ctx, connectivity.TransientFailure)
		if !changed {
			logger.Debugf("grpc connection reset timeout")
			syncError = fmt.Errorf("grpc connection reset timeout")
			return
		}

		logger.Info("grpc connection reset")
		return
	} else if ok {
		// Address type grpc CatalogSource, drop the connection dial in to the new address
		client.conn.Close()
	}

	logger.Info("attempting to add a new grpc connection")
	conn, err := grpc.Dial(source.Address(), grpc.WithInsecure())
	if err != nil {
		logger.WithField("err", err.Error()).Errorf("could not connect to registry service")
		syncError = err
		return
	}

	p.setClient(newRegistryClient(source, conn), key)
	logger.Info("new grpc connection added")

	return
}

func (p *OperatorProvider) catalogSourceDeleted(obj interface{}) {
	catsrc, ok := obj.(metav1.Object)
	if !ok {
		if !ok {
			tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
			if !ok {
				utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %#v", obj))
				return
			}

			catsrc, ok = tombstone.Obj.(metav1.Object)
			if !ok {
				utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a Namespace %#v", obj))
				return
			}
		}
	}

	logger := logrus.WithFields(logrus.Fields{
		"action":    "CatalogSource Deleted",
		"name":      catsrc.GetName(),
		"namespace": catsrc.GetNamespace(),
	})
	logger.Debugf("attempting to remove grpc connection")

	key := sourceKey{catsrc.GetName(), catsrc.GetNamespace()}
	client, removed := p.removeClient(key)
	if removed {
		err := client.conn.Close()
		if err != nil {
			logger.WithField("err", err.Error()).Error("error closing connection")
			utilruntime.HandleError(fmt.Errorf("error closing connection %s", err.Error()))
			return
		}
		logger.Debug("grpc connection removed")
		return
	}

	logger.Debugf("no gRPC connection to remove")

}

func (p *OperatorProvider) Get(namespace, name string) (*operators.PackageManifest, error) {
	logger := logrus.WithFields(logrus.Fields{
		"action":    "Get PackageManifest",
		"name":      name,
		"namespace": namespace,
	})

	pkgs, err := p.List(namespace)
	if err != nil {
		return nil, fmt.Errorf("could not list packages in namespace %s", namespace)
	}

	for _, pkg := range pkgs.Items {
		if pkg.GetName() == name {
			return &pkg, nil
		}
	}

	logger.Info("package not found")
	return nil, nil
}

func (p *OperatorProvider) List(namespace string) (*operators.PackageManifestList, error) {
	logger := logrus.WithFields(logrus.Fields{
		"action":    "List PackageManifests",
		"namespace": namespace,
	})

	p.mu.RLock()
	defer p.mu.RUnlock()

	pkgs := []operators.PackageManifest{}
	for _, client := range p.clients {
		if client.source.GetNamespace() == namespace || client.source.GetNamespace() == p.globalNamespace || namespace == metav1.NamespaceAll {
			logger.Debugf("found CatalogSource %s", client.source.GetName())

			stream, err := client.ListPackages(context.Background(), &api.ListPackageRequest{})
			if err != nil {
				logger.WithField("err", err.Error()).Warnf("error getting stream")
				continue
			}
			for {
				pkgName, err := stream.Recv()
				if err == io.EOF {
					break
				}

				if err != nil {
					logger.WithField("err", err.Error()).Warnf("error getting data")
					break
				}
				pkg, err := client.GetPackage(context.Background(), &api.GetPackageRequest{Name: pkgName.GetName()})
				if err != nil {
					logger.WithField("err", err.Error()).Warnf("error getting package")
					break
				}
				newPkg, err := toPackageManifest(pkg, client)
				if err != nil {
					logger.WithField("err", err.Error()).Warnf("error converting to packagemanifest")
					break
				}

				// Set request namespace to stop kube clients from complaining about global namespace mismatch.
				if namespace != metav1.NamespaceAll {
					newPkg.SetNamespace(namespace)
				}
				pkgs = append(pkgs, *newPkg)
			}
		}
	}

	return &operators.PackageManifestList{Items: pkgs}, nil
}

func toPackageManifest(pkg *api.Package, client registryClient) (*operators.PackageManifest, error) {
	pkgChannels := pkg.GetChannels()
	catsrc := client.source
	manifest := &operators.PackageManifest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pkg.GetName(),
			Namespace: catsrc.GetNamespace(),
			Labels: labels.CloneAndAddLabel(
				labels.CloneAndAddLabel(catsrc.GetLabels(),
					"catalog", catsrc.GetName()), "catalog-namespace", catsrc.GetNamespace()),
			CreationTimestamp: catsrc.GetCreationTimestamp(),
		},
		Status: operators.PackageManifestStatus{
			CatalogSource:            catsrc.GetName(),
			CatalogSourceDisplayName: catsrc.Spec.DisplayName,
			CatalogSourcePublisher:   catsrc.Spec.Publisher,
			CatalogSourceNamespace:   catsrc.GetNamespace(),
			PackageName:              pkg.Name,
			Channels:                 make([]operators.PackageChannel, len(pkgChannels)),
			DefaultChannel:           pkg.GetDefaultChannelName(),
		},
	}

	for i, pkgChannel := range pkgChannels {
		bundle, err := client.GetBundleForChannel(context.Background(), &api.GetBundleInChannelRequest{PkgName: pkg.GetName(), ChannelName: pkgChannel.GetName()})
		if err != nil {
			return nil, err
		}

		csv := operatorsv1alpha1.ClusterServiceVersion{}
		err = json.Unmarshal([]byte(bundle.GetCsvJson()), &csv)
		if err != nil {
			return nil, err
		}
		manifest.Status.Channels[i] = operators.PackageChannel{
			Name:           pkgChannel.GetName(),
			CurrentCSV:     csv.GetName(),
			CurrentCSVDesc: operators.CreateCSVDescription(&csv),
		}

		if manifest.Status.DefaultChannel != "" && pkgChannel.GetName() == manifest.Status.DefaultChannel || i == 0 {
			manifest.Status.Provider = operators.AppLink{
				Name: csv.Spec.Provider.Name,
				URL:  csv.Spec.Provider.URL,
			}
			manifest.ObjectMeta.Labels["provider"] = manifest.Status.Provider.Name
			manifest.ObjectMeta.Labels["provider-url"] = manifest.Status.Provider.URL
		}
	}

	return manifest, nil
}
