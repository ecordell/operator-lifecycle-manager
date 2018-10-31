package resolver

import (
	"context"
	"fmt"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/informers/externalversions"
	v1alpha1listers "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/listers/operators/v1alpha1"
	"github.com/operator-framework/operator-registry/pkg/client"
	opregistry "github.com/operator-framework/operator-registry/pkg/registry"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/labels"
	"strings"
)

type APISet map[opregistry.APIKey]struct{}

func EmptyAPISet() APISet {
	return map[opregistry.APIKey]struct{}{}
}

type APIOwnerSet map[opregistry.APIKey]OperatorSurface

func EmptyAPIOwnerSet() APIOwnerSet {
	return map[opregistry.APIKey]OperatorSurface{}
}

type OperatorSet map[string]OperatorSurface

func EmptyOperatorSet() OperatorSet {
	return map[string]OperatorSurface{}
}

type APIMultiOwnerSet map[opregistry.APIKey]OperatorSet

func EmptyAPIMultiOwnerSet() APIMultiOwnerSet {
	return map[opregistry.APIKey]OperatorSet{}
}

func (s APIMultiOwnerSet) PopAPIKey() *opregistry.APIKey {
	for a := range s {
		api := &opregistry.APIKey{
			Group: a.Group,
			Version: a.Version,
			Kind: a.Kind,
			Plural: a.Plural,
		}
		delete(s, a)
		return api
	}
	return nil
}

func (s APIMultiOwnerSet) PopAPIRequirers() OperatorSet {
	requirers := EmptyOperatorSet()
	for a := range s {
		for key, op := range s[a] {
			requirers[key] = NewOperatorFromOperatorSurface(op)
		}
		delete(s, a)
		return requirers
	}
	return nil
}

// OperatorSurface describes the API surfaces provided and required by an Operator.
type OperatorSurface interface {
	ProvidedAPIs() APISet
	RequiredAPIs() APISet
	Identifier() string
}

type Operator struct {
	name         string
	providedAPIs APISet
	requiredAPIs APISet
	bundle       *opregistry.Bundle
}

var _ OperatorSurface = &Operator{}


func NewOperatorFromOperatorSurface(in OperatorSurface) *Operator {
	return &Operator{
		name: in.Identifier(),
		providedAPIs: in.ProvidedAPIs(),
		requiredAPIs: in.RequiredAPIs(),
	}
}
func NewOperatorFromBundle(bundle *opregistry.Bundle) (*Operator, error) {
	csv, err := bundle.ClusterServiceVersion()
	if err != nil {
		return nil, err
	}
	providedAPIs, err := bundle.ProvidedAPIs()
	if err != nil {
		return nil, err
	}
	requiredAPIs, err := bundle.RequiredAPIs()
	if err != nil {
		return nil, err
	}
	return &Operator{
		name:         csv.GetName(),
		providedAPIs: providedAPIs,
		requiredAPIs: requiredAPIs,
		bundle:       bundle,
	}, nil
}

func NewOperatorFromCSV(csv *v1alpha1.ClusterServiceVersion) (*Operator, error) {
	providedAPIs := EmptyAPISet()
	for _, crdDef := range csv.Spec.CustomResourceDefinitions.Owned {
		parts := strings.SplitAfterN(crdDef.Name, ".", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("error parsing crd name: %s", crdDef.Name)
		}
		providedAPIs[opregistry.APIKey{Plural: parts[0], Group: parts[1], Version: crdDef.Version, Kind: crdDef.Kind}] = struct{}{}
	}
	for _, api := range csv.Spec.APIServiceDefinitions.Owned {
		providedAPIs[opregistry.APIKey{Group: api.Group, Version: api.Version, Kind: api.Kind, Plural: api.Name}] = struct{}{}
	}

	requiredAPIs := EmptyAPISet()
	for _, crdDef := range csv.Spec.CustomResourceDefinitions.Required {
		parts := strings.SplitAfterN(crdDef.Name, ".", 2)
		if len(parts) < 2 {
			return nil, fmt.Errorf("error parsing crd name: %s", crdDef.Name)
		}
		requiredAPIs[opregistry.APIKey{Plural: parts[0], Group: parts[1], Version: crdDef.Version, Kind: crdDef.Kind}] = struct{}{}
	}
	for _, api := range csv.Spec.APIServiceDefinitions.Required {
		requiredAPIs[opregistry.APIKey{Group: api.Group, Version: api.Version, Kind: api.Kind, Plural: api.Name}] = struct{}{}
	}

	return &Operator{
		name:         csv.GetName(),
		providedAPIs: providedAPIs,
		requiredAPIs: requiredAPIs,
	}, nil
}

func (o *Operator) ProvidedAPIs() APISet {
	return o.providedAPIs
}

func (o *Operator) RequiredAPIs() APISet {
	return o.requiredAPIs
}

func (o *Operator) Identifier() string {
	return o.name
}

// Generation represents a set of operators and their required/provided API surfaces at a point in time.
type Generation interface {
	AddOperator(o OperatorSurface) error
	RemoveOperator(o OperatorSurface)
	MissingAPIs() APIMultiOwnerSet
}

// NamespaceGeneration represents a generation of operators in a single namespace
type NamespaceGeneration struct {
	providedAPIs APIOwnerSet      // only allow one provider of any api
	requiredAPIs APIMultiOwnerSet // multiple operators may require the same api
}

func NewNamespaceGenerationFromCSVs(csvs []*v1alpha1.ClusterServiceVersion) (*NamespaceGeneration, error) {
	g := &NamespaceGeneration{
		providedAPIs: EmptyAPIOwnerSet(),
		requiredAPIs: EmptyAPIMultiOwnerSet(),
	}

	for _, csv := range csvs {
		op, err := NewOperatorFromCSV(csv)
		if err != nil {
			return nil, err
		}
		if err := g.AddOperator(op); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func (g *NamespaceGeneration) AddOperator(o OperatorSurface) error {
	// add provided apis, error if two owners
	for api := range o.ProvidedAPIs() {
		if provider, ok := g.providedAPIs[api]; ok && provider.Identifier() != o.Identifier() {
			return fmt.Errorf("%v already provided by %s", api, provider.Identifier())
		}
		g.providedAPIs[api] = o
	}

	// add all requirers of apis
	for api := range o.RequiredAPIs() {
		g.requiredAPIs[api][o.Identifier()] = o
	}
	return nil
}

func (g *NamespaceGeneration) RemoveOperator(o OperatorSurface) {
	for api := range o.ProvidedAPIs() {
		delete(g.providedAPIs, api)
	}
	for api := range o.RequiredAPIs() {
		delete(g.requiredAPIs[api], o.Identifier())
		if len(g.requiredAPIs[api]) == 0 {
			delete(g.requiredAPIs, api)
		}
	}
}

func (g *NamespaceGeneration) MissingAPIs() APIMultiOwnerSet {
	missing := EmptyAPIMultiOwnerSet()
	for requiredAPI, requirers := range g.requiredAPIs {

		if _, ok := g.providedAPIs[requiredAPI]; !ok {
			missing[requiredAPI] = requirers
		}
	}
	return missing
}

type CatalogKey struct {
	Name      string
	Namespace string
}

func (k *CatalogKey) String() string {
	return fmt.Sprintf("%s/%s", k.Name, k.Namespace)
}

type OperatorSteps struct {
	Subscription *v1alpha1.Subscription
	CSV          *v1alpha1.ClusterServiceVersion
}

type Resolver interface {
	ResolveSteps(sources map[CatalogKey]client.Interface) ([]*v1alpha1.Step, []*v1alpha1.Subscription, error)
}

type NamespaceResolver struct {
	subLister v1alpha1listers.SubscriptionLister
	csvLister v1alpha1listers.ClusterServiceVersionLister
	namespace string
}

var _ Resolver = &NamespaceResolver{}

func NewNamespaceResolver(namespace string, factory externalversions.SharedInformerFactory) *NamespaceResolver {
	return &NamespaceResolver{
		namespace: namespace,
		subLister: factory.Operators().V1alpha1().Subscriptions().Lister(),
		csvLister: factory.Operators().V1alpha1().ClusterServiceVersions().Lister(),
	}
}

//TODO: sources should be ordered
func (r *NamespaceResolver) ResolveSteps(sources map[CatalogKey]client.Interface) ([]*v1alpha1.Step, []*v1alpha1.Subscription, error) {
	if len(sources) == 0 {
		return nil, nil, fmt.Errorf("no catalog sources available")
	}

	csvs, err := r.csvLister.List(labels.Everything())
	if err != nil {
		return nil, nil, err
	}
	gen, err := NewNamespaceGenerationFromCSVs(csvs)
	if err != nil {
		return nil, nil, err
	}

	subs, err := r.subLister.List(labels.Everything())

	// fetch any subscriptions that don't yet have Operators installed
	resolvedBundles := map[string]*opregistry.Bundle{}
	modifiedSubscriptions := []*v1alpha1.Subscription{}
	for _, s := range subs {
		if s.Status.State != v1alpha1.SubscriptionStateAtLatest {
			bundle, err := r.findInSources(sources, s.Spec.Package, s.Spec.Channel, CatalogKey{Name: s.Spec.CatalogSource, Namespace: s.Spec.CatalogSourceNamespace})
			if err != nil {
				return nil, nil, errors.Wrapf(err, "%s/%s not found", s.Spec.Package, s.Spec.Channel)
			}
			resolvedBundles[bundle.Name] = bundle
			s.Status.CurrentCSV = bundle.Name
			modifiedSubscriptions = append(modifiedSubscriptions, s)
		}
	}

	// adjust the provided/required APIs of the current generation based on the new operators
	for _, b := range resolvedBundles {
		o, err := NewOperatorFromBundle(b)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error parsing bundle")
		}
		if err := gen.AddOperator(o); err != nil {
			if err != nil {
				return nil, nil, errors.Wrap(err, "error calculating generation changes due to new bundle")
			}
		}
	}

	// if we're still missing apis, the first thing we do is try to resolve other providers to attempt to remove missing apis
	netNewBundles := map[string]*opregistry.Bundle{}
	missingAPIs := gen.MissingAPIs()
	for len(missingAPIs) > 0 {
		api := missingAPIs.PopAPIKey()
		if api == nil {
			continue
		}

		bundle, err := r.findProviderInSources(sources, *api)
		if err != nil {
			netNewBundles[bundle.Name] = bundle

			o, err := NewOperatorFromBundle(bundle)
			if err != nil {
				return nil, nil, errors.Wrap(err, "error parsing bundle")
			}
			if err := gen.AddOperator(o); err != nil {
				if err != nil {
					return nil, nil, errors.Wrap(err, "error calculating generation changes due to new bundle")
				}
			}
		}
	}

	// if we're still missing apis, we attempt to downgrade operators until the generation is valid
	for missingAPIs := gen.MissingAPIs(); len(missingAPIs) > 0; {
		requirers := missingAPIs.PopAPIRequirers()
		for opName, op := range requirers {
			gen.RemoveOperator(op)
			delete(resolvedBundles, opName)
			delete(netNewBundles, opName)
		}
	}

	steps := []*v1alpha1.Step{}
	for name, b := range resolvedBundles {
		bundleSteps, err := NewStepResourceFromBundle(b, r.namespace)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to turn bundle into steps")
		}
		for _, s := range bundleSteps {
			steps = append(steps, &v1alpha1.Step{
				Resolving: name,
				Resource:  s,
				Status:    v1alpha1.StepStatusUnknown,
			})
		}
	}
	for name, b := range netNewBundles {
		bundleSteps, err := NewStepResourceFromBundle(b, r.namespace)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to turn bundle into steps")
		}
		for _, s := range bundleSteps {
			steps = append(steps, &v1alpha1.Step{
				Resolving: name,
				Resource:  s,
				Status:    v1alpha1.StepStatusUnknown,
			})
		}
	}

	// TODO add subscription steps for each net new bundle
	return steps, modifiedSubscriptions, nil
}

type SourceQuerier interface {
	findProvider(sources map[CatalogKey]client.Interface, api opregistry.APIKey) (*opregistry.Bundle, error)
}

func (r *NamespaceResolver) findProviderInSources(sources map[CatalogKey]client.Interface, api opregistry.APIKey) (*opregistry.Bundle, error) {
	for _, source := range sources {
		bundle, err := source.GetBundleThatProvides(context.TODO(), api.Group, api.Version, api.Kind)
		if err == nil {
			return bundle, nil
		}
	}
	return nil, fmt.Errorf("%s not provided by a package in any CatalogSource", api)
}

func (r *NamespaceResolver) findInSources(sources map[CatalogKey]client.Interface, pkgName, channelName string, sourceKey CatalogKey) (*opregistry.Bundle, error) {
	if sourceKey.Name != "" && sourceKey.Namespace != "" {
		source, ok := sources[sourceKey]
		if !ok {
			return nil, fmt.Errorf("CatalogSource %s not found", sourceKey.Name)
		}
		return source.GetBundleInPackageChannel(context.TODO(), pkgName, channelName)
	}

	for _, source := range sources {
		bundle, err := source.GetBundleInPackageChannel(context.TODO(), pkgName, channelName)
		if err == nil {
			return bundle, nil
		}
	}
	return nil, fmt.Errorf("%s/%s not found in any available CatalogSource", pkgName, channelName)
}
