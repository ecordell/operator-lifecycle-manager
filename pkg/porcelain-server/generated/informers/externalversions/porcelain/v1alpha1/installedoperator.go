/*
Copyright 2019 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	time "time"

	porcelainv1alpha1 "github.com/operator-framework/operator-lifecycle-manager/pkg/porcelain-server/apis/porcelain/v1alpha1"
	versioned "github.com/operator-framework/operator-lifecycle-manager/pkg/porcelain-server/generated/clientset/versioned"
	internalinterfaces "github.com/operator-framework/operator-lifecycle-manager/pkg/porcelain-server/generated/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/operator-framework/operator-lifecycle-manager/pkg/porcelain-server/generated/listers/porcelain/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// InstalledOperatorInformer provides access to a shared informer and lister for
// InstalledOperators.
type InstalledOperatorInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.InstalledOperatorLister
}

type installedOperatorInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewInstalledOperatorInformer constructs a new informer for InstalledOperator type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewInstalledOperatorInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredInstalledOperatorInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredInstalledOperatorInformer constructs a new informer for InstalledOperator type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredInstalledOperatorInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.PorcelainV1alpha1().InstalledOperators(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.PorcelainV1alpha1().InstalledOperators(namespace).Watch(options)
			},
		},
		&porcelainv1alpha1.InstalledOperator{},
		resyncPeriod,
		indexers,
	)
}

func (f *installedOperatorInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredInstalledOperatorInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *installedOperatorInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&porcelainv1alpha1.InstalledOperator{}, f.defaultInformer)
}

func (f *installedOperatorInformer) Lister() v1alpha1.InstalledOperatorLister {
	return v1alpha1.NewInstalledOperatorLister(f.Informer().GetIndexer())
}