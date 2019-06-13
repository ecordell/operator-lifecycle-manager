// +build !ignore_autogenerated

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

// Code generated by conversion-gen. DO NOT EDIT.

package v1

import (
	unsafe "unsafe"

	operators "github.com/operator-framework/operator-lifecycle-manager/pkg/operator-server/apis/operators"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func init() {
	localSchemeBuilder.Register(RegisterConversions)
}

// RegisterConversions adds conversion functions to the given scheme.
// Public to allow building arbitrary schemes.
func RegisterConversions(s *runtime.Scheme) error {
	if err := s.AddGeneratedConversionFunc((*Operator)(nil), (*operators.Operator)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_Operator_To_operators_Operator(a.(*Operator), b.(*operators.Operator), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*operators.Operator)(nil), (*Operator)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_operators_Operator_To_v1_Operator(a.(*operators.Operator), b.(*Operator), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*OperatorList)(nil), (*operators.OperatorList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_OperatorList_To_operators_OperatorList(a.(*OperatorList), b.(*operators.OperatorList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*operators.OperatorList)(nil), (*OperatorList)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_operators_OperatorList_To_v1_OperatorList(a.(*operators.OperatorList), b.(*OperatorList), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*OperatorSpec)(nil), (*operators.OperatorSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_OperatorSpec_To_operators_OperatorSpec(a.(*OperatorSpec), b.(*operators.OperatorSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*operators.OperatorSpec)(nil), (*OperatorSpec)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_operators_OperatorSpec_To_v1_OperatorSpec(a.(*operators.OperatorSpec), b.(*OperatorSpec), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*OperatorStatus)(nil), (*operators.OperatorStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_v1_OperatorStatus_To_operators_OperatorStatus(a.(*OperatorStatus), b.(*operators.OperatorStatus), scope)
	}); err != nil {
		return err
	}
	if err := s.AddGeneratedConversionFunc((*operators.OperatorStatus)(nil), (*OperatorStatus)(nil), func(a, b interface{}, scope conversion.Scope) error {
		return Convert_operators_OperatorStatus_To_v1_OperatorStatus(a.(*operators.OperatorStatus), b.(*OperatorStatus), scope)
	}); err != nil {
		return err
	}
	return nil
}

func autoConvert_v1_Operator_To_operators_Operator(in *Operator, out *operators.Operator, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_v1_OperatorSpec_To_operators_OperatorSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_v1_OperatorStatus_To_operators_OperatorStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_v1_Operator_To_operators_Operator is an autogenerated conversion function.
func Convert_v1_Operator_To_operators_Operator(in *Operator, out *operators.Operator, s conversion.Scope) error {
	return autoConvert_v1_Operator_To_operators_Operator(in, out, s)
}

func autoConvert_operators_Operator_To_v1_Operator(in *operators.Operator, out *Operator, s conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if err := Convert_operators_OperatorSpec_To_v1_OperatorSpec(&in.Spec, &out.Spec, s); err != nil {
		return err
	}
	if err := Convert_operators_OperatorStatus_To_v1_OperatorStatus(&in.Status, &out.Status, s); err != nil {
		return err
	}
	return nil
}

// Convert_operators_Operator_To_v1_Operator is an autogenerated conversion function.
func Convert_operators_Operator_To_v1_Operator(in *operators.Operator, out *Operator, s conversion.Scope) error {
	return autoConvert_operators_Operator_To_v1_Operator(in, out, s)
}

func autoConvert_v1_OperatorList_To_operators_OperatorList(in *OperatorList, out *operators.OperatorList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	out.Items = *(*[]operators.Operator)(unsafe.Pointer(&in.Items))
	return nil
}

// Convert_v1_OperatorList_To_operators_OperatorList is an autogenerated conversion function.
func Convert_v1_OperatorList_To_operators_OperatorList(in *OperatorList, out *operators.OperatorList, s conversion.Scope) error {
	return autoConvert_v1_OperatorList_To_operators_OperatorList(in, out, s)
}

func autoConvert_operators_OperatorList_To_v1_OperatorList(in *operators.OperatorList, out *OperatorList, s conversion.Scope) error {
	out.ListMeta = in.ListMeta
	out.Items = *(*[]Operator)(unsafe.Pointer(&in.Items))
	return nil
}

// Convert_operators_OperatorList_To_v1_OperatorList is an autogenerated conversion function.
func Convert_operators_OperatorList_To_v1_OperatorList(in *operators.OperatorList, out *OperatorList, s conversion.Scope) error {
	return autoConvert_operators_OperatorList_To_v1_OperatorList(in, out, s)
}

func autoConvert_v1_OperatorSpec_To_operators_OperatorSpec(in *OperatorSpec, out *operators.OperatorSpec, s conversion.Scope) error {
	out.ClusterServiceVersionSpec = in.ClusterServiceVersionSpec
	return nil
}

// Convert_v1_OperatorSpec_To_operators_OperatorSpec is an autogenerated conversion function.
func Convert_v1_OperatorSpec_To_operators_OperatorSpec(in *OperatorSpec, out *operators.OperatorSpec, s conversion.Scope) error {
	return autoConvert_v1_OperatorSpec_To_operators_OperatorSpec(in, out, s)
}

func autoConvert_operators_OperatorSpec_To_v1_OperatorSpec(in *operators.OperatorSpec, out *OperatorSpec, s conversion.Scope) error {
	out.ClusterServiceVersionSpec = in.ClusterServiceVersionSpec
	return nil
}

// Convert_operators_OperatorSpec_To_v1_OperatorSpec is an autogenerated conversion function.
func Convert_operators_OperatorSpec_To_v1_OperatorSpec(in *operators.OperatorSpec, out *OperatorSpec, s conversion.Scope) error {
	return autoConvert_operators_OperatorSpec_To_v1_OperatorSpec(in, out, s)
}

func autoConvert_v1_OperatorStatus_To_operators_OperatorStatus(in *OperatorStatus, out *operators.OperatorStatus, s conversion.Scope) error {
	out.ClusterServiceVersionStatus = in.ClusterServiceVersionStatus
	return nil
}

// Convert_v1_OperatorStatus_To_operators_OperatorStatus is an autogenerated conversion function.
func Convert_v1_OperatorStatus_To_operators_OperatorStatus(in *OperatorStatus, out *operators.OperatorStatus, s conversion.Scope) error {
	return autoConvert_v1_OperatorStatus_To_operators_OperatorStatus(in, out, s)
}

func autoConvert_operators_OperatorStatus_To_v1_OperatorStatus(in *operators.OperatorStatus, out *OperatorStatus, s conversion.Scope) error {
	out.ClusterServiceVersionStatus = in.ClusterServiceVersionStatus
	return nil
}

// Convert_operators_OperatorStatus_To_v1_OperatorStatus is an autogenerated conversion function.
func Convert_operators_OperatorStatus_To_v1_OperatorStatus(in *operators.OperatorStatus, out *OperatorStatus, s conversion.Scope) error {
	return autoConvert_operators_OperatorStatus_To_v1_OperatorStatus(in, out, s)
}
