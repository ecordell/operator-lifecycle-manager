package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1alpha1 "github.com/operator-framework/operator-lifecycle-manager/pkg/api/apis/operators/v1alpha1"
)

// OperatorList is a list of Operator objects.
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OperatorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Operator `json:"items"`
}

// Operator holds information about a running operator that is watching the given namespace
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Operator struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OperatorSpec   `json:"spec,omitempty"`
	Status OperatorStatus `json:"status,omitempty"`
}

// OperatorSpec defines the desired state of Operator
type OperatorSpec struct {
	operatorv1alpha1.ClusterServiceVersionSpec
}

// OperatorStatus represents the current status of the Operator
type OperatorStatus struct {
	operatorv1alpha1.ClusterServiceVersionStatus
}
