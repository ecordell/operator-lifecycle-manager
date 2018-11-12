package resolver

import (
	"reflect"
	"testing"

	opregistry "github.com/operator-framework/operator-registry/pkg/registry"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
	"github.com/operator-framework/operator-registry/pkg/registry"
)

func TestNewGenerationFromCSVs(t *testing.T) {
	type args struct {
		csvs []*v1alpha1.ClusterServiceVersion
	}
	tests := []struct {
		name    string
		args    args
		want    *NamespaceGeneration
		wantErr error
	}{
		{
			name: "SingleCSV/NoProvided/NoRequired",
			args: args{
				csvs: []*v1alpha1.ClusterServiceVersion{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "operator.v1",
						},
					},
				},
			},
			want: &NamespaceGeneration{
				providedAPIs:  EmptyAPIOwnerSet(),
				requiredAPIs:  EmptyAPIMultiOwnerSet(),
				uncheckedAPIs: EmptyAPISet(),
				missingAPIs:   EmptyAPIMultiOwnerSet(),
			},
		},
		{
			name: "SingleCSV/Provided/NoRequired",
			args: args{
				csvs: []*v1alpha1.ClusterServiceVersion{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "operator.v1",
						},
						Spec: v1alpha1.ClusterServiceVersionSpec{
							CustomResourceDefinitions: v1alpha1.CustomResourceDefinitions{
								Owned: []v1alpha1.CRDDescription{
									{
										Name:    "crdkinds.g",
										Version: "v1",
										Kind:    "CRDKind",
									},
								},
							},
							APIServiceDefinitions: v1alpha1.APIServiceDefinitions{
								Owned: []v1alpha1.APIServiceDescription{
									{
										Name:    "apikinds",
										Group:   "g",
										Version: "v1",
										Kind:    "APIKind",
									},
								},
							},
						},
					},
				},
			},
			want: &NamespaceGeneration{
				providedAPIs: map[opregistry.APIKey]OperatorSurface{
					{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: &Operator{
						name: "operator.v1",
						providedAPIs: map[registry.APIKey]struct{}{
							{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: {},
							{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: {},
						},
						requiredAPIs: EmptyAPISet(),
						sourceInfo:   &ExistingOperator,
					},
					{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: &Operator{
						name: "operator.v1",
						providedAPIs: map[registry.APIKey]struct{}{
							{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: {},
							{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: {},
						},
						requiredAPIs: EmptyAPISet(),
						sourceInfo:   &ExistingOperator,
					},
				},
				requiredAPIs:  EmptyAPIMultiOwnerSet(),
				uncheckedAPIs: EmptyAPISet(),
				missingAPIs:   EmptyAPIMultiOwnerSet(),
			},
		},
		{
			name: "SingleCSV/NoProvided/Required",
			args: args{
				csvs: []*v1alpha1.ClusterServiceVersion{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "operator.v1",
						},
						Spec: v1alpha1.ClusterServiceVersionSpec{
							CustomResourceDefinitions: v1alpha1.CustomResourceDefinitions{
								Required: []v1alpha1.CRDDescription{
									{
										Name:    "crdkinds.g",
										Version: "v1",
										Kind:    "CRDKind",
									},
								},
							},
							APIServiceDefinitions: v1alpha1.APIServiceDefinitions{
								Required: []v1alpha1.APIServiceDescription{
									{
										Name:    "apikinds",
										Group:   "g",
										Version: "v1",
										Kind:    "APIKind",
									},
								},
							},
						},
					},
				},
			},
			want: &NamespaceGeneration{
				providedAPIs: EmptyAPIOwnerSet(),
				requiredAPIs: map[opregistry.APIKey]OperatorSet{
					{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: map[string]OperatorSurface{
						"operator.v1": &Operator{
							name:         "operator.v1",
							providedAPIs: EmptyAPISet(),
							requiredAPIs: map[registry.APIKey]struct{}{
								{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: {},
								{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: {},
							},
							sourceInfo: &ExistingOperator,
						},
					},
					{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: map[string]OperatorSurface{
						"operator.v1": &Operator{
							name:         "operator.v1",
							providedAPIs: EmptyAPISet(),
							requiredAPIs: map[registry.APIKey]struct{}{
								{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: {},
								{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: {},
							},
							sourceInfo: &ExistingOperator,
						},
					},
				},
				uncheckedAPIs: map[registry.APIKey]struct{}{
					{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: {},
					{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: {},
				},
				missingAPIs: map[opregistry.APIKey]OperatorSet{
					{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: map[string]OperatorSurface{
						"operator.v1": &Operator{
							name:         "operator.v1",
							providedAPIs: EmptyAPISet(),
							requiredAPIs: map[registry.APIKey]struct{}{
								{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: {},
								{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: {},
							},
							sourceInfo: &ExistingOperator,
						},
					},
					{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: map[string]OperatorSurface{
						"operator.v1": &Operator{
							name:         "operator.v1",
							providedAPIs: EmptyAPISet(),
							requiredAPIs: map[registry.APIKey]struct{}{
								{Group: "g", Version: "v1", Kind: "APIKind", Plural: "apikinds"}: {},
								{Group: "g", Version: "v1", Kind: "CRDKind", Plural: "crdkinds"}: {},
							},
							sourceInfo: &ExistingOperator,
						},
					},
				},
			},
		},
		{
			name: "SingleCSV/Provided/Required/Missing",
			args: args{
				csvs: []*v1alpha1.ClusterServiceVersion{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "operator.v1",
						},
						Spec: v1alpha1.ClusterServiceVersionSpec{
							CustomResourceDefinitions: v1alpha1.CustomResourceDefinitions{
								Owned: []v1alpha1.CRDDescription{
									{
										Name:    "crdownedkinds.g",
										Version: "v1",
										Kind:    "CRDOwnedKind",
									},
								},
								Required: []v1alpha1.CRDDescription{
									{
										Name:    "crdreqkinds.g2",
										Version: "v1",
										Kind:    "CRDReqKind",
									},
								},
							},
							APIServiceDefinitions: v1alpha1.APIServiceDefinitions{
								Owned: []v1alpha1.APIServiceDescription{
									{
										Name:    "apiownedkinds",
										Group:   "g",
										Version: "v1",
										Kind:    "APIOwnedKind",
									},
								},
								Required: []v1alpha1.APIServiceDescription{
									{
										Name:    "apireqkinds",
										Group:   "g2",
										Version: "v1",
										Kind:    "APIReqKind",
									},
								},
							},
						},
					},
				},
			},
			want: &NamespaceGeneration{
				providedAPIs: map[opregistry.APIKey]OperatorSurface{
					{Group: "g", Version: "v1", Kind: "APIOwnedKind", Plural: "apiownedkinds"}: &Operator{
						name: "operator.v1",
						providedAPIs: map[opregistry.APIKey]struct{}{
							{Group: "g", Version: "v1", Kind: "APIOwnedKind", Plural: "apiownedkinds"}: {},
							{Group: "g", Version: "v1", Kind: "CRDOwnedKind", Plural: "crdownedkinds"}: {},
						},
						requiredAPIs: map[opregistry.APIKey]struct{}{
							{Group: "g2", Version: "v1", Kind: "APIReqKind", Plural: "apireqkinds"}: {},
							{Group: "g2", Version: "v1", Kind: "CRDReqKind", Plural: "crdreqkinds"}: {},
						},
						sourceInfo: &ExistingOperator,
					},
					{Group: "g", Version: "v1", Kind: "CRDOwnedKind", Plural: "crdownedkinds"}: &Operator{
						name: "operator.v1",
						providedAPIs: map[opregistry.APIKey]struct{}{
							{Group: "g", Version: "v1", Kind: "APIOwnedKind", Plural: "apiownedkinds"}: {},
							{Group: "g", Version: "v1", Kind: "CRDOwnedKind", Plural: "crdownedkinds"}: {},
						},
						requiredAPIs: map[opregistry.APIKey]struct{}{
							{Group: "g2", Version: "v1", Kind: "APIReqKind", Plural: "apireqkinds"}: {},
							{Group: "g2", Version: "v1", Kind: "CRDReqKind", Plural: "crdreqkinds"}: {},
						},
						sourceInfo: &ExistingOperator,
					},
				},
				requiredAPIs: map[opregistry.APIKey]OperatorSet{
					{Group: "g", Version: "v1", Kind: "APReqIKind", Plural: "apireqkinds"}: map[string]OperatorSurface{
						"operator.v1": &Operator{
							name: "operator.v1",
							providedAPIs: map[opregistry.APIKey]struct{}{
								{Group: "g", Version: "v1", Kind: "APIOwnedKind", Plural: "apiownedkinds"}: {},
								{Group: "g", Version: "v1", Kind: "CRDOwnedKind", Plural: "crdownedkinds"}: {},
							},
							requiredAPIs: map[opregistry.APIKey]struct{}{
								{Group: "g2", Version: "v1", Kind: "APIReqKind", Plural: "apireqkinds"}: {},
								{Group: "g2", Version: "v1", Kind: "CRDReqKind", Plural: "crdreqkinds"}: {},
							},
							sourceInfo: &ExistingOperator,
						},
					},
					{Group: "g", Version: "v1", Kind: "CRDReqKind", Plural: "crdreqkinds"}: map[string]OperatorSurface{
						"operator.v1": &Operator{
							name: "operator.v1",
							providedAPIs: map[opregistry.APIKey]struct{}{
								{Group: "g", Version: "v1", Kind: "APIOwnedKind", Plural: "apiownedkinds"}: {},
								{Group: "g", Version: "v1", Kind: "CRDOwnedKind", Plural: "crdownedkinds"}: {},
							},
							requiredAPIs: map[opregistry.APIKey]struct{}{
								{Group: "g2", Version: "v1", Kind: "APIReqKind", Plural: "apireqkinds"}: {},
								{Group: "g2", Version: "v1", Kind: "CRDReqKind", Plural: "crdreqkinds"}: {},
							},
							sourceInfo: &ExistingOperator,
						},
					},
				},
				uncheckedAPIs: map[opregistry.APIKey]struct{}{
					{Group: "g2", Version: "v1", Kind: "APIReqKind", Plural: "apireqkinds"}: {},
					{Group: "g2", Version: "v1", Kind: "CRDReqKind", Plural: "crdreqkinds"}: {},
				},
				missingAPIs: map[opregistry.APIKey]OperatorSet{
					{Group: "g", Version: "v1", Kind: "APIReqKind", Plural: "apireqkinds"}: map[string]OperatorSurface{
						"operator.v1": &Operator{
							name: "operator.v1",
							providedAPIs: map[opregistry.APIKey]struct{}{
								{Group: "g", Version: "v1", Kind: "APIOwnedKind", Plural: "apiownedkinds"}: {},
								{Group: "g", Version: "v1", Kind: "CRDOwnedKind", Plural: "crdownedkinds"}: {},
							},
							requiredAPIs: map[opregistry.APIKey]struct{}{
								{Group: "g2", Version: "v1", Kind: "APIReqKind", Plural: "apireqkinds"}: {},
								{Group: "g2", Version: "v1", Kind: "CRDReqKind", Plural: "crdreqkinds"}: {},
							},
							sourceInfo: &ExistingOperator,
						},
					},
					{Group: "g", Version: "v1", Kind: "CRDReqKind", Plural: "crdreqkinds"}: map[string]OperatorSurface{
						"operator.v1": &Operator{
							name: "operator.v1",
							providedAPIs: map[opregistry.APIKey]struct{}{
								{Group: "g", Version: "v1", Kind: "APIOwnedKind", Plural: "apiownedkinds"}: {},
								{Group: "g", Version: "v1", Kind: "CRDOwnedKind", Plural: "crdownedkinds"}: {},
							},
							requiredAPIs: map[opregistry.APIKey]struct{}{
								{Group: "g2", Version: "v1", Kind: "APIReqKind", Plural: "apireqkinds"}: {},
								{Group: "g2", Version: "v1", Kind: "CRDReqKind", Plural: "crdreqkinds"}: {},
							},
							sourceInfo: &ExistingOperator,
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			// calculate expected operator set from input csvs
			operatorSet := EmptyOperatorSet()
			for _, csv := range tt.args.csvs {
				// there's a separate unit test for this constructor
				op, err := NewOperatorFromCSV(csv)
				require.NoError(t, err)
				operatorSet[op.Identifier()] = op
			}
			tt.want.operators = operatorSet

			got, err := NewGenerationFromCSVs(tt.args.csvs)
			require.Equal(t, tt.wantErr, err)
			require.EqualValues(t, tt.want, got)
		})
	}
}

func TestNamespaceGeneration_AddOperator(t *testing.T) {
	type fields struct {
		providedAPIs  APIOwnerSet
		requiredAPIs  APIMultiOwnerSet
		uncheckedAPIs APISet
		missingAPIs   APIMultiOwnerSet
		operators     OperatorSet
	}
	type args struct {
		o OperatorSurface
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &NamespaceGeneration{
				providedAPIs:  tt.fields.providedAPIs,
				requiredAPIs:  tt.fields.requiredAPIs,
				uncheckedAPIs: tt.fields.uncheckedAPIs,
				missingAPIs:   tt.fields.missingAPIs,
				operators:     tt.fields.operators,
			}
			if err := g.AddOperator(tt.args.o); (err != nil) != tt.wantErr {
				t.Errorf("NamespaceGeneration.AddOperator() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNamespaceGeneration_RemoveOperator(t *testing.T) {
	type fields struct {
		providedAPIs  APIOwnerSet
		requiredAPIs  APIMultiOwnerSet
		uncheckedAPIs APISet
		missingAPIs   APIMultiOwnerSet
		operators     OperatorSet
	}
	type args struct {
		o OperatorSurface
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &NamespaceGeneration{
				providedAPIs:  tt.fields.providedAPIs,
				requiredAPIs:  tt.fields.requiredAPIs,
				uncheckedAPIs: tt.fields.uncheckedAPIs,
				missingAPIs:   tt.fields.missingAPIs,
				operators:     tt.fields.operators,
			}
			g.RemoveOperator(tt.args.o)
		})
	}
}

func TestNamespaceGeneration_MarkAPIChecked(t *testing.T) {
	type fields struct {
		providedAPIs  APIOwnerSet
		requiredAPIs  APIMultiOwnerSet
		uncheckedAPIs APISet
		missingAPIs   APIMultiOwnerSet
		operators     OperatorSet
	}
	type args struct {
		key registry.APIKey
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &NamespaceGeneration{
				providedAPIs:  tt.fields.providedAPIs,
				requiredAPIs:  tt.fields.requiredAPIs,
				uncheckedAPIs: tt.fields.uncheckedAPIs,
				missingAPIs:   tt.fields.missingAPIs,
				operators:     tt.fields.operators,
			}
			g.MarkAPIChecked(tt.args.key)
		})
	}
}

func TestNamespaceGeneration_ResetUnchecked(t *testing.T) {
	type fields struct {
		providedAPIs  APIOwnerSet
		requiredAPIs  APIMultiOwnerSet
		uncheckedAPIs APISet
		missingAPIs   APIMultiOwnerSet
		operators     OperatorSet
	}
	tests := []struct {
		name   string
		fields fields
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &NamespaceGeneration{
				providedAPIs:  tt.fields.providedAPIs,
				requiredAPIs:  tt.fields.requiredAPIs,
				uncheckedAPIs: tt.fields.uncheckedAPIs,
				missingAPIs:   tt.fields.missingAPIs,
				operators:     tt.fields.operators,
			}
			g.ResetUnchecked()
		})
	}
}

func TestNamespaceGeneration_MissingAPIs(t *testing.T) {
	type fields struct {
		providedAPIs  APIOwnerSet
		requiredAPIs  APIMultiOwnerSet
		uncheckedAPIs APISet
		missingAPIs   APIMultiOwnerSet
		operators     OperatorSet
	}
	tests := []struct {
		name   string
		fields fields
		want   APIMultiOwnerSet
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &NamespaceGeneration{
				providedAPIs:  tt.fields.providedAPIs,
				requiredAPIs:  tt.fields.requiredAPIs,
				uncheckedAPIs: tt.fields.uncheckedAPIs,
				missingAPIs:   tt.fields.missingAPIs,
				operators:     tt.fields.operators,
			}
			if got := g.MissingAPIs(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamespaceGeneration.MissingAPIs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNamespaceGeneration_UncheckedAPIs(t *testing.T) {
	type fields struct {
		providedAPIs  APIOwnerSet
		requiredAPIs  APIMultiOwnerSet
		uncheckedAPIs APISet
		missingAPIs   APIMultiOwnerSet
		operators     OperatorSet
	}
	tests := []struct {
		name   string
		fields fields
		want   APISet
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &NamespaceGeneration{
				providedAPIs:  tt.fields.providedAPIs,
				requiredAPIs:  tt.fields.requiredAPIs,
				uncheckedAPIs: tt.fields.uncheckedAPIs,
				missingAPIs:   tt.fields.missingAPIs,
				operators:     tt.fields.operators,
			}
			if got := g.UncheckedAPIs(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamespaceGeneration.UncheckedAPIs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNamespaceGeneration_Operators(t *testing.T) {
	type fields struct {
		providedAPIs  APIOwnerSet
		requiredAPIs  APIMultiOwnerSet
		uncheckedAPIs APISet
		missingAPIs   APIMultiOwnerSet
		operators     OperatorSet
	}
	tests := []struct {
		name   string
		fields fields
		want   OperatorSet
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &NamespaceGeneration{
				providedAPIs:  tt.fields.providedAPIs,
				requiredAPIs:  tt.fields.requiredAPIs,
				uncheckedAPIs: tt.fields.uncheckedAPIs,
				missingAPIs:   tt.fields.missingAPIs,
				operators:     tt.fields.operators,
			}
			if got := g.Operators(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamespaceGeneration.Operators() = %v, want %v", got, tt.want)
			}
		})
	}
}
