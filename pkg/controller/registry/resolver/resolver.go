package resolver

import (
	"fmt"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/informers/externalversions"
	v1alpha1listers "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/listers/operators/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"time"
)

var timeNow = func() metav1.Time { return metav1.NewTime(time.Now().UTC()) }

type Resolver interface {
	ResolveSteps(sourceQuerier SourceQuerier) ([]*v1alpha1.Step, []*v1alpha1.Subscription, error)
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

func (r *NamespaceResolver) ResolveSteps(sourceQuerier SourceQuerier) ([]*v1alpha1.Step, []*v1alpha1.Subscription, error) {
	if err := sourceQuerier.Queryable(); err != nil {
		return nil, nil, err
	}

	csvs, err := r.csvLister.List(labels.Everything())
	if err != nil {
		return nil, nil, err
	}
	gen, err := NewGenerationFromCSVs(csvs)
	if err != nil {
		return nil, nil, err
	}

	subs, err := r.subLister.List(labels.Everything())
	subMap := r.sourceInfoToSubscriptions(subs)
	add := r.sourceInfoForNewSubscriptions(subMap)

	if err := NewNamespaceGenerationEvolver(sourceQuerier, gen).Evolve(add); err != nil {
		return nil, nil, err
	}


	steps := []*v1alpha1.Step{}
	updatedSubs := []*v1alpha1.Subscription{}
	for name, op := range gen.Operators() {
		if op.SourceInfo() == &ExistingOperator {
			continue
		}

		// add steps for any new bundle
		bundleSteps, err := NewStepResourceFromBundle(op.Bundle(), r.namespace)
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

		if sub, ok := subMap[*op.SourceInfo()]; ok {
			// update existing subscriptions status
			sub.Status.CurrentCSV = op.Identifier()
			updatedSubs = append(updatedSubs, sub)
			continue
		}

		// add steps for subscriptions for bundles that were added through resolution
		subStep, err := NewSubscriptionStepResource(r.namespace, *op.SourceInfo())
		if err != nil {
			return nil, nil, err
		}
		steps = append(steps, &v1alpha1.Step{
			Resolving: name,
			Resource:subStep,
			Status: v1alpha1.StepStatusUnknown,
		})
	}

	return steps, updatedSubs, nil
}

func (r *NamespaceResolver) sourceInfoForNewSubscriptions(subs map[OperatorSourceInfo]*v1alpha1.Subscription) (add map[OperatorSourceInfo]struct{}) {
	for key := range subs {
		add[key] = struct{}{}
	}
	return
}

func (r *NamespaceResolver) sourceInfoToSubscriptions(subs []*v1alpha1.Subscription) (add map[OperatorSourceInfo]*v1alpha1.Subscription) {
	for _, s := range subs {
		if s.Status.CurrentCSV == "" {
			add[OperatorSourceInfo{
				Package:s.Spec.Package,
				Channel:s.Spec.Channel,
				CatalogSource: s.Spec.CatalogSource,
				CatalogSourceNamespace: s.Spec.CatalogSourceNamespace,
			}] = s
		}
	}
	return
}

