package server

import (
	"fmt"
	"io"
	"net"

	"github.com/spf13/cobra"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	//"k8s.io/sample-apiserver/pkg/admission/plugin/banflunder"
	//"k8s.io/sample-apiserver/pkg/admission/packageinitializer"

	"github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apis/packagemanifest/v1alpha1"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/apiserver"
	//clientset "k8s.io/sample-apiserver/pkg/client/clientset/internalversion"
	//informers "k8s.io/sample-apiserver/pkg/client/informers/internalversion"
	"github.com/operator-framework/operator-lifecycle-manager/pkg/package-server/options"
)

type PackageServerOptions struct {
	RecommendedOptions *options.RecommendedOptions

	//SharedInformerFactory informers.SharedInformerFactory
	StdOut io.Writer
	StdErr io.Writer
}

func NewPackageServerOptions(out, errOut io.Writer) *PackageServerOptions {
	o := &PackageServerOptions{
		RecommendedOptions: options.NewRecommendedOptions("packageserver", apiserver.Codecs.LegacyCodec(v1alpha1.SchemeGroupVersion)),
		StdOut:             out,
		StdErr:             errOut,
	}

	return o
}

// NewCommandStartPackageServer provides a CLI handler for 'start master' command
// with a default PackageServerOptions.
func NewCommandStartPackageServer(defaults *PackageServerOptions, stopCh <-chan struct{}) *cobra.Command {
	o := *defaults
	cmd := &cobra.Command{
		Short: "Launch a package API server",
		Long:  "Launch a package API server",
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			if err := o.RunPackageServer(stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	o.RecommendedOptions.AddFlags(flags)

	return cmd
}

func (o PackageServerOptions) Validate(args []string) error {
	errors := []error{}
	errors = append(errors, o.RecommendedOptions.Validate()...)
	return utilerrors.NewAggregate(errors)
}

func (o *PackageServerOptions) Complete() error {
	return nil
}

func (o *PackageServerOptions) Config() (*apiserver.Config, error) {
	// register admission plugins
	//banflunder.Register(o.RecommendedOptions.Admission.Plugins)

	// TODO have a "real" external address
	if err := o.RecommendedOptions.SecureServing.MaybeDefaultWithSelfSignedCerts("localhost", nil, []net.IP{net.ParseIP("127.0.0.1")}); err != nil {
		return nil, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	//o.RecommendedOptions.ExtraAdmissionInitializers = func(c *genericapiserver.RecommendedConfig) ([]admission.PluginInitializer, error) {
	//	client, err := clientset.NewForConfig(c.LoopbackClientConfig)
	//	if err != nil {
	//		return nil, err
	//	}
	//	informerFactory := informers.NewSharedInformerFactory(client, c.LoopbackClientConfig.Timeout)
	//	o.SharedInformerFactory = informerFactory
	//	return []admission.PluginInitializer{packageinitializer.New(informerFactory)}, nil
	//}

	serverConfig := genericapiserver.NewRecommendedConfig(apiserver.Codecs)
	if err := o.RecommendedOptions.ApplyTo(serverConfig, apiserver.Scheme); err != nil {
		return nil, err
	}

	config := &apiserver.Config{
		GenericConfig: serverConfig,
		ExtraConfig:   apiserver.ExtraConfig{},
	}
	return config, nil
}

func (o PackageServerOptions) RunPackageServer(stopCh <-chan struct{}) error {
	config, err := o.Config()
	if err != nil {
		return err
	}

	server, err := config.Complete().New()
	if err != nil {
		return err
	}

	//server.GenericAPIServer.AddPostStartHook("start-sample-server-informers", func(context genericapiserver.PostStartHookContext) error {
	//	config.GenericConfig.SharedInformerFactory.Start(context.StopCh)
	//	o.SharedInformerFactory.Start(context.StopCh)
	//	return nil
	//})

	return server.GenericAPIServer.PrepareRun().Run(stopCh)
}
