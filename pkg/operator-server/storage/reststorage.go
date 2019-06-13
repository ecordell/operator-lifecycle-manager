package storage

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/kubernetes/pkg/printers"
	printerstorage "k8s.io/kubernetes/pkg/printers/storage"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apis/operators"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/provider"
)

type OperatorStorage struct {
	groupResource schema.GroupResource
	prov          provider.PackageManifestProvider
	scheme        *runtime.Scheme
	rest.TableConvertor
}

var _ rest.Storage = &OperatorStorage{}
var _ rest.KindProvider = &OperatorStorage{}
var _ rest.Lister = &OperatorStorage{}
var _ rest.Getter = &OperatorStorage{}
var _ rest.Scoper = &OperatorStorage{}
var _ rest.TableConvertor = &OperatorStorage{}

// NewStorage returns a struct that implements methods needed for Kubernetes to satisfy API requests for the `PackageManifest` resource
func NewStorage(groupResource schema.GroupResource, prov provider.PackageManifestProvider, scheme *runtime.Scheme) *OperatorStorage {
	return &OperatorStorage{
		groupResource:  groupResource,
		prov:           prov,
		scheme:         scheme,
		TableConvertor: printerstorage.TableConvertor{TablePrinter: printers.NewTablePrinter().With(addTableHandlers)},
	}
}

// New satisfies the Storage interface
func (m *OperatorStorage) New() runtime.Object {
	return &operators.PackageManifest{}
}

// Kind satisfies the KindProvider interface
func (m *OperatorStorage) Kind() string {
	return "PackageManifest"
}

// NewList satisfies part of the Lister interface
func (m *OperatorStorage) NewList() runtime.Object {
	return &operators.PackageManifestList{}
}

// List satisfies part of the Lister interface
func (m *OperatorStorage) List(ctx context.Context, options *metainternalversion.ListOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)

	labelSelector := labels.Everything()
	if options != nil && options.LabelSelector != nil {
		labelSelector = options.LabelSelector
	}

	name, err := nameFor(options.FieldSelector)
	if err != nil {
		return nil, err
	}

	res, err := m.prov.List(namespace)
	if err != nil {
		return nil, k8serrors.NewInternalError(err)
	}

	// Filter by label selector
	filtered := []operators.PackageManifest{}
	for _, manifest := range res.Items {
		if matches(manifest, name, labelSelector) {
			filtered = append(filtered, manifest)
		}
	}
	res.Items = filtered

	return res, nil
}

// Get satisfies the Getter interface
func (m *OperatorStorage) Get(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)
	manifest, err := m.prov.Get(namespace, name)
	if err != nil || manifest == nil {
		return nil, k8serrors.NewNotFound(m.groupResource, name)
	}

	return manifest, nil
}

// NamespaceScoped satisfies the Scoper interface
func (m *OperatorStorage) NamespaceScoped() bool {
	return true
}

func nameFor(fs fields.Selector) (string, error) {
	if fs == nil {
		fs = fields.Everything()
	}
	name := ""
	if value, found := fs.RequiresExactMatch("metadata.name"); found {
		name = value
	} else if !fs.Empty() {
		return "", fmt.Errorf("field label not supported: %s", fs.Requirements()[0].Field)
	}
	return name, nil
}

func matches(pm operators.PackageManifest, name string, ls labels.Selector) bool {
	if name == "" {
		name = pm.GetName()
	}
	return ls.Matches(labels.Set(pm.GetLabels())) && pm.GetName() == name
}
