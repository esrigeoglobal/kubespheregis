/*
 * Copyright 2024 the KubeSphere Authors.
 * Please refer to the LICENSE file in the root directory of the project.
 * https://github.com/kubesphere/kubesphere/blob/master/LICENSE
 */

package options

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/Masterminds/semver/v3"
	"golang.org/x/net/context"
	corev1 "k8s.io/api/core/v1"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"

	"kubesphere.io/kubesphere/pkg/apiserver"
	"kubesphere.io/kubesphere/pkg/apiserver/authentication/identityprovider"
	"kubesphere.io/kubesphere/pkg/apiserver/options"
	"kubesphere.io/kubesphere/pkg/config"
	"kubesphere.io/kubesphere/pkg/models/auth"
	"kubesphere.io/kubesphere/pkg/models/registries/imagesearch"
	resourcev1beta1 "kubesphere.io/kubesphere/pkg/models/resources/v1beta1"
	"kubesphere.io/kubesphere/pkg/scheme"
	genericoptions "kubesphere.io/kubesphere/pkg/server/options"
	"kubesphere.io/kubesphere/pkg/simple/client/cache"
	"kubesphere.io/kubesphere/pkg/simple/client/k8s"
	"kubesphere.io/kubesphere/pkg/utils/clusterclient"
)

type APIServerOptions struct {
	options.Options
	GenericServerRunOptions *genericoptions.ServerRunOptions
	ConfigFile              string
	DebugMode               bool
}

func NewAPIServerOptions() *APIServerOptions {
	return &APIServerOptions{
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
	}
}

func (s *APIServerOptions) Flags() (fss cliflag.NamedFlagSets) {
	fs := fss.FlagSet("generic")
	fs.BoolVar(&s.DebugMode, "debug", false, "Don't enable this if you don't know what it means.")
	s.GenericServerRunOptions.AddFlags(fs, s.GenericServerRunOptions)
	s.KubernetesOptions.AddFlags(fss.FlagSet("kubernetes"), s.KubernetesOptions)
	s.AuthenticationOptions.AddFlags(fss.FlagSet("authentication"), s.AuthenticationOptions)
	s.AuthorizationOptions.AddFlags(fss.FlagSet("authorization"), s.AuthorizationOptions)
	s.MultiClusterOptions.AddFlags(fss.FlagSet("multicluster"), s.MultiClusterOptions)
	s.AuditingOptions.AddFlags(fss.FlagSet("auditing"), s.AuditingOptions)

	fs = fss.FlagSet("klog")
	local := flag.NewFlagSet("klog", flag.ExitOnError)
	klog.InitFlags(local)
	local.VisitAll(func(fl *flag.Flag) {
		fl.Name = strings.Replace(fl.Name, "_", "-", -1)
		fs.AddGoFlag(fl)
	})

	return fss
}

// NewAPIServer creates an APIServer instance using given options
func (s *APIServerOptions) NewAPIServer(ctx context.Context) (*apiserver.APIServer, error) {
	apiServer := &apiserver.APIServer{
		Options: s.Options,
	}

	ctrl.SetLogger(klog.NewKlogr())

	var err error
	if apiServer.K8sClient, err = k8s.NewKubernetesClient(s.KubernetesOptions); err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client, error: %v", err)
	}

	if apiServer.CacheClient, err = cache.New(s.CacheOptions, ctx.Done()); err != nil {
		return nil, fmt.Errorf("failed to create cache, error: %v", err)
	}

	if c, err := cluster.New(apiServer.K8sClient.Config(), func(options *cluster.Options) {
		options.Scheme = scheme.Scheme
	}); err != nil {
		return nil, fmt.Errorf("unable to create controller runtime cluster: %v", err)
	} else {
		apiServer.RuntimeCache = c.GetCache()
		key := "involvedObject.name"
		indexerFunc := func(obj client.Object) []string {
			e := obj.(*corev1.Event)
			return []string{e.InvolvedObject.Name}
		}
		if err = apiServer.RuntimeCache.IndexField(ctx, &corev1.Event{}, key, indexerFunc); err != nil {
			klog.Fatalf("unable to create index field: %v", err)
		}
		apiServer.RuntimeClient = c.GetClient()
	}
		return nil, fmt.Errorf("unable to create issuer: %v", err)
	}

