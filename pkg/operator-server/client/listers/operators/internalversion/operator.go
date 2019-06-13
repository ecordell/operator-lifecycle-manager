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

// Code generated by lister-gen. DO NOT EDIT.

package internalversion

import (
	operators "github.com/operator-framework/operator-lifecycle-manager/pkg/operator-server/apis/operators"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// OperatorLister helps list Operators.
type OperatorLister interface {
	// List lists all Operators in the indexer.
	List(selector labels.Selector) (ret []*operators.Operator, err error)
	// Operators returns an object that can list and get Operators.
	Operators(namespace string) OperatorNamespaceLister
	OperatorListerExpansion
}

// operatorLister implements the OperatorLister interface.
type operatorLister struct {
	indexer cache.Indexer
}

// NewOperatorLister returns a new OperatorLister.
func NewOperatorLister(indexer cache.Indexer) OperatorLister {
	return &operatorLister{indexer: indexer}
}

// List lists all Operators in the indexer.
func (s *operatorLister) List(selector labels.Selector) (ret []*operators.Operator, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*operators.Operator))
	})
	return ret, err
}

// Operators returns an object that can list and get Operators.
func (s *operatorLister) Operators(namespace string) OperatorNamespaceLister {
	return operatorNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// OperatorNamespaceLister helps list and get Operators.
type OperatorNamespaceLister interface {
	// List lists all Operators in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*operators.Operator, err error)
	// Get retrieves the Operator from the indexer for a given namespace and name.
	Get(name string) (*operators.Operator, error)
	OperatorNamespaceListerExpansion
}

// operatorNamespaceLister implements the OperatorNamespaceLister
// interface.
type operatorNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Operators in the indexer for a given namespace.
func (s operatorNamespaceLister) List(selector labels.Selector) (ret []*operators.Operator, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*operators.Operator))
	})
	return ret, err
}

// Get retrieves the Operator from the indexer for a given namespace and name.
func (s operatorNamespaceLister) Get(name string) (*operators.Operator, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(operators.Resource("operator"), name)
	}
	return obj.(*operators.Operator), nil
}
