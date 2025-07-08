/*
Copyright 2025 The Flux authors

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

package auth_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/url"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/fluxcd/pkg/auth"
	"github.com/fluxcd/pkg/cache"
)

func TestGetAccessToken(t *testing.T) {
	g := NewWithT(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	// Create test env.
	testEnv := &envtest.Environment{}
	conf, err := testEnv.Start()
	g.Expect(err).NotTo(HaveOccurred())
	t.Cleanup(func() { testEnv.Stop() })
	kubeClient, err := client.New(conf, client.Options{})
	g.Expect(err).NotTo(HaveOccurred())

	// Create HTTP client for OIDC verification.
	clusterCAPool := x509.NewCertPool()
	ok := clusterCAPool.AppendCertsFromPEM(conf.TLSClientConfig.CAData)
	g.Expect(ok).To(BeTrue())
	oidcClient := &http.Client{}
	oidcClient.Transport = http.DefaultTransport.(*http.Transport).Clone()
	oidcClient.Transport.(*http.Transport).TLSClientConfig = &tls.Config{
		RootCAs: clusterCAPool,
	}

	// Grant anonymous access to service account issuer discovery.
	err = kubeClient.Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "anonymous-service-account-issuer-discovery",
		},
		Subjects: []rbacv1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "User",
				Name:     "system:anonymous",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:service-account-issuer-discovery",
		},
	})
	g.Expect(err).NotTo(HaveOccurred())

	// Create a default service account.
	defaultServiceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "default",
		},
	}
	err = kubeClient.Create(ctx, defaultServiceAccount)
	g.Expect(err).NotTo(HaveOccurred())
	saRef := client.ObjectKey{
		Name:      defaultServiceAccount.Name,
		Namespace: defaultServiceAccount.Namespace,
	}

	for _, tt := range []struct {
		name               string
		provider           *mockProvider
		opts               []auth.Option
		disableObjectLevel bool
		expectedToken      auth.Token
		expectedErr        string
	}{
		{
			name: "controller access token",
			provider: &mockProvider{
				returnControllerToken: &mockToken{token: "mock-default-token"},
			},
			opts: []auth.Option{
				auth.WithScopes("scope1", "scope2"),
				auth.WithSTSRegion("us-east-1"),
				auth.WithSTSEndpoint("https://sts.some-cloud.io"),
				auth.WithProxyURL(url.URL{Scheme: "http", Host: "proxy.io:8080"}),
			},
			expectedToken: &mockToken{token: "mock-default-token"},
		},
		{
			name: "controller access token allowing shell out",
			provider: &mockProvider{
				returnControllerToken: &mockToken{token: "mock-default-token"},
				paramAllowShellOut:    true,
			},
			opts: []auth.Option{
				auth.WithScopes("scope1", "scope2"),
				auth.WithSTSRegion("us-east-1"),
				auth.WithSTSEndpoint("https://sts.some-cloud.io"),
				auth.WithProxyURL(url.URL{Scheme: "http", Host: "proxy.io:8080"}),
				auth.WithAllowShellOut(),
			},
			expectedToken: &mockToken{token: "mock-default-token"},
		},
		{
			name: "access token from service account",
			provider: &mockProvider{
				returnName:           "mock-provider",
				returnAudience:       "mock-audience",
				returnAccessToken:    &mockToken{token: "mock-access-token"},
				paramServiceAccount:  *defaultServiceAccount,
				paramOIDCTokenClient: oidcClient,
			},
			opts: []auth.Option{
				auth.WithServiceAccount(saRef, kubeClient),
				auth.WithScopes("scope1", "scope2"),
				auth.WithSTSRegion("us-east-1"),
				auth.WithSTSEndpoint("https://sts.some-cloud.io"),
				auth.WithProxyURL(url.URL{Scheme: "http", Host: "proxy.io:8080"}),
				// Exercise the code path where a cache is set but no token is
				// available in the cache.
				func(o *auth.Options) {
					tokenCache, err := cache.NewTokenCache(1)
					g.Expect(err).NotTo(HaveOccurred())
					o.Cache = tokenCache
				},
			},
			expectedToken: &mockToken{token: "mock-access-token"},
		},
		{
			name: "all the options are taken into account in the cache key",
			provider: &mockProvider{
				returnName:          "mock-provider",
				returnAudience:      "mock-audience",
				returnIdentity:      "mock-identity",
				paramServiceAccount: *defaultServiceAccount,
			},
			opts: []auth.Option{
				auth.WithServiceAccount(saRef, kubeClient),
				auth.WithScopes("scope1", "scope2"),
				auth.WithSTSRegion("us-east-1"),
				auth.WithSTSEndpoint("https://sts.some-cloud.io"),
				auth.WithProxyURL(url.URL{Scheme: "http", Host: "proxy.io:8080"}),
				func(o *auth.Options) {
					tokenCache, err := cache.NewTokenCache(1)
					g.Expect(err).NotTo(HaveOccurred())

					const key = "ae7db4da77f53fba1d2acd49027e818c2146be91de80051177d93b9e6da48dca"
					token := &mockToken{token: "cached-token"}
					cachedToken, ok, err := tokenCache.GetOrSet(ctx, key, func(ctx context.Context) (cache.Token, error) {
						return token, nil
					})
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(ok).To(BeFalse())
					g.Expect(cachedToken).To(Equal(token))

					o.Cache = tokenCache
				},
			},
			expectedToken: &mockToken{token: "cached-token"},
		},
		{
			name: "error getting identity",
			provider: &mockProvider{
				returnIdentityErr:   "mock error",
				paramServiceAccount: *defaultServiceAccount,
			},
			opts: []auth.Option{
				auth.WithServiceAccount(saRef, kubeClient),
			},
			expectedErr: "failed to get provider identity from service account 'default/default' annotations: mock error",
		},
		{
			name: "error getting identity using cache",
			provider: &mockProvider{
				returnIdentityErr:   "mock error",
				paramServiceAccount: *defaultServiceAccount,
			},
			opts: []auth.Option{
				auth.WithServiceAccount(saRef, kubeClient),
				func(o *auth.Options) {
					tokenCache, err := cache.NewTokenCache(1)
					g.Expect(err).NotTo(HaveOccurred())
					o.Cache = tokenCache
				},
			},
			expectedErr: "failed to get provider identity from service account 'default/default' annotations: mock error",
		},
		{
			name: "disable object level workload identity",
			provider: &mockProvider{
				paramServiceAccount: *defaultServiceAccount,
			},
			opts: []auth.Option{
				auth.WithServiceAccount(saRef, kubeClient),
			},
			disableObjectLevel: true,
			expectedErr:        "ObjectLevelWorkloadIdentity feature gate is not enabled",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			tt.provider.t = t

			if !tt.disableObjectLevel {
				t.Setenv(auth.EnvVarEnableObjectLevelWorkloadIdentity, "true")
			}

			token, err := auth.GetAccessToken(ctx, tt.provider, tt.opts...)

			if tt.expectedErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.expectedErr))
				g.Expect(token).To(BeNil())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(token).To(Equal(tt.expectedToken))
			}
		})
	}
}
