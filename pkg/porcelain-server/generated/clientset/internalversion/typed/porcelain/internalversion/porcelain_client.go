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
	"github.com/operator-framework/operator-lifecycle-manager/pkg/porcelain-server/generated/clientset/internalversion/scheme"
	rest "k8s.io/client-go/rest"
)

type PorcelainInterface interface {
	RESTClient() rest.Interface
	InstalledOperatorsGetter
}

// PorcelainClient is used to interact with features provided by the porcelain.operators.coreos.com group.
type PorcelainClient struct {
	restClient rest.Interface
}

func (c *PorcelainClient) InstalledOperators(namespace string) InstalledOperatorInterface {
	return newInstalledOperators(c, namespace)
}

// NewForConfig creates a new PorcelainClient for the given config.
func NewForConfig(c *rest.Config) (*PorcelainClient, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}
	return &PorcelainClient{client}, nil
}

// NewForConfigOrDie creates a new PorcelainClient for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *PorcelainClient {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new PorcelainClient for the given RESTClient.
func New(c rest.Interface) *PorcelainClient {
	return &PorcelainClient{c}
}

func setConfigDefaults(config *rest.Config) error {
	config.APIPath = "/apis"
	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	if config.GroupVersion == nil || config.GroupVersion.Group != scheme.Scheme.PrioritizedVersionsForGroup("porcelain.operators.coreos.com")[0].Group {
		gv := scheme.Scheme.PrioritizedVersionsForGroup("porcelain.operators.coreos.com")[0]
		config.GroupVersion = &gv
	}
	config.NegotiatedSerializer = scheme.Codecs

	if config.QPS == 0 {
		config.QPS = 5
	}
	if config.Burst == 0 {
		config.Burst = 10
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *PorcelainClient) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
