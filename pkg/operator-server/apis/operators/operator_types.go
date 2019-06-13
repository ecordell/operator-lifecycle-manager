package operators

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
)

// OperatorList is a list of Operator objects.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OperatorList struct {
	metav1.TypeMeta
	metav1.ListMeta

	Items []Operator
}

// Operator holds information about a running operator that is watching the given namespace
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Operator struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	Spec   OperatorSpec
	Status OperatorStatus
}

// OperatorSpec defines the desired state of Operator
type OperatorSpec struct{
	operatorv1alpha1.ClusterServiceVersionSpec
}

// OperatorStatus represents the current status of the Operator
type OperatorStatus struct {
	operatorv1alpha1.ClusterServiceVersionStatus
}

