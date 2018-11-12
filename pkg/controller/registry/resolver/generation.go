package resolver

import (
	"fmt"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-registry/pkg/registry"
)

// Generation represents a set of operators and their required/provided API surfaces at a point in time.
type Generation interface {
	AddOperator(o OperatorSurface) error
	RemoveOperator(o OperatorSurface)
	ResetUnchecked()
	MissingAPIs() APIMultiOwnerSet
	Operators() OperatorSet
	MarkAPIChecked(key registry.APIKey)
	UncheckedAPIs() APISet
}

// NamespaceGeneration represents a generation of operators in a single namespace with methods for managing api checks
type NamespaceGeneration struct {
	providedAPIs  APIOwnerSet      // only allow one provider of any api
	requiredAPIs  APIMultiOwnerSet // multiple operators may require the same api
	uncheckedAPIs APISet           // required apis that haven't been checked yet
	missingAPIs   APIMultiOwnerSet
	operators     OperatorSet
}

func NewGenerationFromCSVs(csvs []*v1alpha1.ClusterServiceVersion) (*NamespaceGeneration, error) {
	g := &NamespaceGeneration{
		providedAPIs:  EmptyAPIOwnerSet(),
		requiredAPIs:  EmptyAPIMultiOwnerSet(),
		uncheckedAPIs: EmptyAPISet(),
		missingAPIs:   EmptyAPIMultiOwnerSet(),
		operators:     EmptyOperatorSet(),
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
		if _, ok := g.requiredAPIs[api]; !ok {
			g.requiredAPIs[api] = EmptyOperatorSet()
		}
		g.requiredAPIs[api][o.Identifier()] = o
		if _, ok := g.providedAPIs[api]; !ok {
			if _, ok := g.missingAPIs[api]; !ok {
				g.missingAPIs[api] = EmptyOperatorSet()
			}
			g.missingAPIs[api][o.Identifier()] = o
			g.uncheckedAPIs[api] = struct{}{}
		}
	}
	g.operators[o.Identifier()] = o
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
			delete(g.missingAPIs, api)
			delete(g.uncheckedAPIs, api)
		}
	}
	delete(g.operators, o.Identifier())
}

func (g *NamespaceGeneration) MarkAPIChecked(key registry.APIKey) {
	delete(g.uncheckedAPIs, key)
}

func (g *NamespaceGeneration) ResetUnchecked() {
	g.uncheckedAPIs = EmptyAPISet()
	for api := range g.requiredAPIs {
		g.uncheckedAPIs[api] = struct{}{}
	}
}

func (g *NamespaceGeneration) MissingAPIs() APIMultiOwnerSet {
	return g.missingAPIs
}

func (g *NamespaceGeneration) UncheckedAPIs() APISet {
	return g.uncheckedAPIs
}

func (g *NamespaceGeneration) Operators() OperatorSet {
	return g.operators
}
