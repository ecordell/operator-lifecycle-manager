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

// Code generated by client-gen. DO NOT EDIT.

package internalversion

import (
	"time"

	porcelain "github.com/operator-framework/operator-lifecycle-manager/pkg/porcelain-server/apis/porcelain"
	scheme "github.com/operator-framework/operator-lifecycle-manager/pkg/porcelain-server/generated/clientset/internalversion/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// InstalledOperatorsGetter has a method to return a InstalledOperatorInterface.
// A group's client should implement this interface.
type InstalledOperatorsGetter interface {
	InstalledOperators(namespace string) InstalledOperatorInterface
}

// InstalledOperatorInterface has methods to work with InstalledOperator resources.
type InstalledOperatorInterface interface {
	Create(*porcelain.InstalledOperator) (*porcelain.InstalledOperator, error)
	Update(*porcelain.InstalledOperator) (*porcelain.InstalledOperator, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*porcelain.InstalledOperator, error)
	List(opts v1.ListOptions) (*porcelain.InstalledOperatorList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *porcelain.InstalledOperator, err error)
	InstalledOperatorExpansion
}

// installedOperators implements InstalledOperatorInterface
type installedOperators struct {
	client rest.Interface
	ns     string
}

// newInstalledOperators returns a InstalledOperators
func newInstalledOperators(c *PorcelainClient, namespace string) *installedOperators {
	return &installedOperators{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the installedOperator, and returns the corresponding installedOperator object, and an error if there is any.
func (c *installedOperators) Get(name string, options v1.GetOptions) (result *porcelain.InstalledOperator, err error) {
	result = &porcelain.InstalledOperator{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("installedoperators").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of InstalledOperators that match those selectors.
func (c *installedOperators) List(opts v1.ListOptions) (result *porcelain.InstalledOperatorList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &porcelain.InstalledOperatorList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("installedoperators").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested installedOperators.
func (c *installedOperators) Watch(opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("installedoperators").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch()
}

// Create takes the representation of a installedOperator and creates it.  Returns the server's representation of the installedOperator, and an error, if there is any.
func (c *installedOperators) Create(installedOperator *porcelain.InstalledOperator) (result *porcelain.InstalledOperator, err error) {
	result = &porcelain.InstalledOperator{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("installedoperators").
		Body(installedOperator).
		Do().
		Into(result)
	return
}

// Update takes the representation of a installedOperator and updates it. Returns the server's representation of the installedOperator, and an error, if there is any.
func (c *installedOperators) Update(installedOperator *porcelain.InstalledOperator) (result *porcelain.InstalledOperator, err error) {
	result = &porcelain.InstalledOperator{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("installedoperators").
		Name(installedOperator.Name).
		Body(installedOperator).
		Do().
		Into(result)
	return
}

// Delete takes name of the installedOperator and deletes it. Returns an error if one occurs.
func (c *installedOperators) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("installedoperators").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *installedOperators) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("installedoperators").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched installedOperator.
func (c *installedOperators) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *porcelain.InstalledOperator, err error) {
	result = &porcelain.InstalledOperator{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("installedoperators").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}
