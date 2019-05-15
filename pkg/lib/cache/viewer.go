package cache

import (
	"fmt"
	"reflect"
	"sync"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/controller/registry/reconciler"
)

type ViewKind string

const (
	CatalogStatusViewKind ViewKind = "catalogstatus"
)

type Keyable interface {
	// Kind() ViewKind
	Key() string
	// Name() string
	// Namespace() string
}

// TODO: have an interface e.g. GetViewKind
type ViewMeta struct {
	Kind ViewKind

	Name string

	Namespace string

	// resourceVersion (?)
}

var _ Keyable = &ViewMeta{}

func (m *ViewMeta) Key() string {
	return fmt.Sprintf("%s/%s/%s", CatalogStatusViewKind, m.Namespace, m.Name)
}

// Create a key for any internal view that implements Keyable
func DefaultViewMetaKeyFunc(obj interface{}) (string, error) {
	meta, ok := obj.(ViewMeta)
	if !ok {
		return "", fmt.Errorf("invalid view object")
	}
	return meta.Key(), nil
}

type SubscriptionCatalogStatusView struct {
	ViewMeta

	// CatalogSourceRef is a reference to a CatalogSource.
	CatalogSourceRef *v1.ObjectReference `json:"catalogSourceRef"`

	// LastUpdated represents the last time that the CatalogSourceHealth changed
	LastUpdated metav1.Time `json:"lastUpdated"`

	// Healthy is true if the CatalogSource is healthy; false otherwise.
	Healthy bool `json:"healthy"`
}

// Constructors for the View; can be generated or at least scaffolded
func SubscriptionCatalogStatusViewForV1alpha1CatalogSource(in *v1alpha1.CatalogSource) (*SubscriptionCatalogStatusView, error) {
	ref, err := operators.GetReference(in)
	if err != nil {
		return nil, err
	}

	return &SubscriptionCatalogStatusView{
		ViewMeta: ViewMeta{
			Kind:      CatalogStatusViewKind,
			Name:      in.GetName(),
			Namespace: in.GetNamespace(),
		},
		CatalogSourceRef: ref,
		// TODO: derive healthy differently
		Healthy: !in.Status.LastSync.IsZero(),
	}, nil
}

// Generate indices into subscriptioncatalogstatusview cache by key: subscriptions/namespace/name
func SubscriptionIndex(subscriptionCache cache.Indexer, globalNamespace string) cache.IndexFunc {
	return func(obj interface{}) ([]string, error) {
		view, ok := obj.(*SubscriptionCatalogStatusView)
		if !ok {
			return nil, fmt.Errorf("invalid view object")
		}

		if view.Namespace == globalNamespace {
			return subscriptionCache.ListKeys(), nil
		}

		return subscriptionCache.IndexKeys(cache.NamespaceIndex, view.Namespace)
	}
}

func syncCatalogSource(cat *v1alpha1.CatalogSource) error {
	// todo kick to another queue?

	// these would exist on the operator already
	subscriptionIndex := cache.NewIndexer(nil, nil)
	globalNamespace := "operator-lifecycle-manager"
	catSrcCache := cache.NewIndexer(DefaultViewMetaKeyFunc, cache.Indexers{
		"bySubscription": SubscriptionIndex(subscriptionIndex, globalNamespace),
	})
	recFact := reconciler.NewRegistryReconcilerFactory(nil, nil, "")

	// check status
	healthy, err := recFact.ReconcilerForSource(cat).CheckRegistryServer(cat)
	if err != nil {
		return err
	}
	cat.Status.Healthy = healthy

	// store/update cache - this whole section should have an abstraction for updating the cache and returning if something changed
	last, exists, err := catSrcCache.GetByKey(currentCatalogStatus.Key())
	if err != nil {
		return err
	}
	lastCatalogStatus, ok := last.(*SubscriptionCatalogStatusView)
	if !ok {
		return fmt.Errorf("got wrong type from cache")
	}

	updated := false
	if !exists {
		if err := catSrcCache.Add(currentCatalogStatus); err != nil {
			return err
		}
		updated = true
	} else {
		if currentCatalogStatus.Healthy != lastCatalogStatus.Healthy {
			currentCatalogStatus.LastUpdated = metav1.Now()
			if err := catSrcCache.Update(currentCatalogStatus); err != nil {
				return err
			}
			updated = true
		}
	}

	// make necessary changes to the cluster
	if updated {
		// find relevant subscriptions - ideally we do this with an index.
		// should we share this status with the catalog source itself? i.e. can the healthy part be from another view?
	}
	return nil
}

func syncSubscription(sub *v1alpha1.Subscription) error {
	// get from operator

	// currentCatalogStatus, err := SubscriptionCatalogStatusViewForV1alpha1CatalogSource(cat) // Alternately
	// if err != nil {
	// 	return err
	// }
	catSrcStatusViewCache := cache.NewIndexer(nil, nil)

	currentCatSrcStatus, err := catSrcStatusViewCache.ByIndex("bySubscription", fmt.Sprintf("subscription/%s/%s", sub.GetNamespace(), sub.GetName()))
	if err != nil {
		// no status
		// condition = no available catalogs
		return fmt.Errorf("no catalogs")
	}

	if !sub.Status.CatalogStatus.Equal(currentCatSrcStatus) {
		// update and return
		return nil
	}
	return nil
}

// Viewer is a type that knows how to create and key an alternate view of a given resource.
type Viewer interface {
	// Key returns the resulting view key for the given object.
	Key(obj interface{}) (key string, err error)

	// KeyByView returns the view key for the given view object.
	KeyByView(view interface{}) (key string, err error)

	// View returns the view for the given object.
	View(obj interface{}) (value interface{}, err error)
}

// ViewerKey relates a Viewer name to the type of resource it transforms.
type ViewerKey string

// Viewers maps ViewerKeys to a Viewers.
type Viewers map[ViewerKey]Viewer

// ViewIndexer is a storage interface that supports building and indexing alternate views of given data.
type ViewIndexer interface {
	cache.Indexer

	// View returns the stored view of the given object generated by the viewer of the given key if one exists.
	View(viewerKey ViewerKey, obj interface{}) (value interface{}, err error)

	// AddViewer adds a Viewer to the ViewIndexer with the given key. The results of calling this after data has been added to the indexer are undefined.
	AddViewer(viewerKey ViewerKey, viewer Viewer) error

	// AddViewers adds the given set of Viewers to the ViewIndexer. If you call this after you already have data in the indexer, the results are undefined.
	AddViewers(viewers Viewers) error

	// AddIndex adds an index func to the indexer with the given name.
	AddIndex(name string, indexFunc cache.IndexFunc) error

	// TODO: Include Add, Update, Delete methods that can specify a single view to use?
}

// ViewIndexerSet provides thread-safe methods for storing and retrieving ViewIndexers.
type ViewIndexerSet struct {
	lock           sync.RWMutex
	viewIndexerSet map[string]ViewIndexer
}

// Get returns the ViewIndexer associated with the given namespace.
func (v *ViewIndexerSet) Get(namespace string) ViewIndexer {
	v.lock.RLock()
	defer v.lock.RUnlock()

	if viewIndexer, ok := v.viewIndexerSet[metav1.NamespaceAll]; ok {
		return viewIndexer
	}

	return v.viewIndexerSet[namespace]
}

// Set sets the given ViewIndexer at the given namespace.
func (v *ViewIndexerSet) Set(namespace string, viewIndexer ViewIndexer) {
	v.lock.Lock()
	defer v.lock.Unlock()

	v.viewIndexerSet[namespace] = viewIndexer
}

// NewViewIndexerSet returns a newly initialized
func NewViewIndexerSet() *ViewIndexerSet {
	return &ViewIndexerSet{viewIndexerSet: map[string]ViewIndexer{}}
}

// IndexerSet provides thread-safe methods for storing and retrieving .
type IndexerSet struct {
	lock       sync.RWMutex
	indexerSet map[string]cache.Indexer
}

// Get returns the Indexer associated with the given namespace.
func (i *IndexerSet) Get(namespace string) cache.Indexer {
	i.lock.RLock()
	defer i.lock.RUnlock()

	if indexer, ok := i.indexerSet[metav1.NamespaceAll]; ok {
		return indexer
	}

	return i.indexerSet[namespace]
}

// Set sets the given Indexer at the given namespace.
func (i *IndexerSet) Set(namespace string, indexer cache.Indexer) {
	i.lock.Lock()
	defer i.lock.Unlock()

	i.indexerSet[namespace] = indexer
}

// NewIndexerSet returns a newly initialized
func NewIndexerSet() *IndexerSet {
	return &IndexerSet{indexerSet: map[string]cache.Indexer{}}
}

// ------------------------------------------------------------------------------------------------------------------

type viewCache struct {
	cache.Indexer

	lock    sync.RWMutex
	viewers Viewers

	// TODO: use key pointers internally if it will reduce overhead.
	// viewerTypeToKey maps ViewerTypes to existing ViewerKeys.
	// Used to prevent the same viewer from being registered multiple times.
	viewerTypeToKey map[reflect.Type]ViewerKey

	// transformsToKeys maps transform types to sets of ViewerKeys.
	// Used to quickly look up the set of applicable views for an added resource.
	transformsToKeys map[reflect.Type][]ViewerKey

	// intoToKey maps a view type to the key of the viewer that produces it.
	intoToKey map[reflect.Type]ViewerKey
}

var _ ViewIndexer = &viewCache{}

// NewViewIndexer returns a zeroed ViewIndexer.
func NewViewIndexer() ViewIndexer {
	c := &viewCache{
		viewers:          Viewers{},
		viewerTypeToKey:  map[reflect.Type]ViewerKey{},
		transformsToKeys: map[reflect.Type][]ViewerKey{},
		intoToKey:        map[reflect.Type]ViewerKey{},
	}
	c.Indexer = cache.NewIndexer(c.sharedViewKey, cache.Indexers{})
	return c
}

func (c *viewCache) View(viewerKey ViewerKey, obj interface{}) (value interface{}, err error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	viewer, ok := c.viewers[viewerKey]
	if !ok {
		return nil, fmt.Errorf("view %v not found", viewerKey)
	}

	key, err := viewer.Key(obj)
	if err != nil {
		return nil, err
	}

	stored, _, err := c.GetByKey(key)

	return stored, err
}

func (c *viewCache) AddViewer(viewerKey ViewerKey, viewer Viewer) error {
	return c.AddViewers(Viewers{viewerKey: viewer})
}

func (c *viewCache) AddViewers(viewers Viewers) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if len(c.viewers) > 0 {
		return fmt.Errorf("cannot add views to running viewer")
	}

	for vk, viewer := range viewers {
		vt := reflect.TypeOf(viewer)
		if vk, ok := c.viewerTypeToKey[vt]; ok {
			return fmt.Errorf("viewer of type %T already registered with key %v", viewer, vk)
		}

		c.viewers[vk] = viewer
		c.viewerTypeToKey[vt] = vk
	}

	return nil
}

func (c *viewCache) AddIndex(name string, indexFunc cache.IndexFunc) error {
	return c.Indexer.AddIndexers(cache.Indexers{name: indexFunc})
}

// sharedViewKey invokes the key function matching the viewer type of the given view.
// This allows a store expecting a single key function to support multiple types.
func (c *viewCache) sharedViewKey(view interface{}) (string, error) {
	// Find a viewer that matches the dynamic type
	if vk, ok := c.intoToKey[reflect.TypeOf(view)]; ok {
		if viewer, ok := c.viewers[vk]; ok {
			return viewer.KeyByView(view)
		}
	}

	// TODO: have a default key function if a view doesn't exist? This could be useful for storing runtime.Objects in the same cache.
	return "", fmt.Errorf("no viewer of type %T registered", view)
}

type modifierFunc func(obj interface{}) error

func (c *viewCache) modifyViews(obj interface{}, modify modifierFunc) error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	errs := []error{}
	for _, vk := range c.transformsToKeys[reflect.TypeOf(obj)] {
		viewer, ok := c.viewers[vk]
		if !ok {
			return fmt.Errorf("no view found for key: %v", vk)
		}

		view, err := viewer.View(obj)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if err := modify(view); err != nil {
			errs = append(errs, err)
		}
	}

	return utilerrors.NewAggregate(errs)
}

// Add sets an item in the cache.
func (c *viewCache) Add(obj interface{}) error {
	return c.modifyViews(obj, c.Indexer.Add)
}

// Update sets an item in the cache to its updated state.
func (c *viewCache) Update(obj interface{}) error {
	return c.modifyViews(obj, c.Indexer.Update)
}

// Delete removes an item from the cache.
func (c *viewCache) Delete(obj interface{}) error {
	return c.modifyViews(obj, c.Indexer.Delete)
}

// ------------------------------------------------------------------------------------------------------------------

// EnqueuingIndexBuilder builds index funcs that enqueue their indices on a set of queues as a side effect.
type EnqueuingIndexBuilder interface {
	// Build builds a new function from the given index function that enqueues its indices on the given queues as a side effect.
	Build(cache.IndexFunc, ...workqueue.RateLimitingInterface) cache.IndexFunc
}

// EnqueuingIndexBuilderFunc implements EnqueuingIndexBuilder.
type EnqueuingIndexBuilderFunc func(cache.IndexFunc, ...workqueue.RateLimitingInterface) cache.IndexFunc

// Build builds a new function from the given index function that enqueues its indices on the given queues as a side effect.
func (f EnqueuingIndexBuilderFunc) Build(index cache.IndexFunc, queues ...workqueue.RateLimitingInterface) cache.IndexFunc {
	return f(index, queues...)
}

// BuildEnqueuingIndex builds a new function from the given index function that enqueues its indices on the given queues synchronously as a side effect.
func BuildEnqueuingIndex(index cache.IndexFunc, queues ...workqueue.RateLimitingInterface) cache.IndexFunc {
	return func(obj interface{}) ([]string, error) {
		indices, err := index(obj)
		if err != nil {
			return nil, err
		}

		// Enqueue on every queue before returning
		for _, queue := range queues {
			for _, idx := range indices {
				queue.Add(idx)
			}
		}

		return indices, nil
	}
}
