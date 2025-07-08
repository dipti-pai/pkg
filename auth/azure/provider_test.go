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

package azure_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/go-containerregistry/pkg/authn"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fluxcd/pkg/auth"
	"github.com/fluxcd/pkg/auth/azure"
)

func TestProvider_NewControllerToken(t *testing.T) {
	for _, tt := range []struct {
		name     string
		shellOut bool
	}{
		{
			name:     "without shell out",
			shellOut: false,
		},
		{
			name:     "with shell out",
			shellOut: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			impl := &mockImplementation{
				t:           t,
				shellOut:    tt.shellOut,
				argProxyURL: &url.URL{Scheme: "http", Host: "proxy.example.com"},
				argScopes:   []string{"scope1", "scope2"},
				returnToken: "access-token",
			}

			opts := []auth.Option{
				auth.WithProxyURL(url.URL{Scheme: "http", Host: "proxy.example.com"}),
				auth.WithScopes("scope1", "scope2"),
			}

			if tt.shellOut {
				opts = append(opts, auth.WithAllowShellOut())
			}

			provider := azure.Provider{Implementation: impl}
			token, err := provider.NewControllerToken(context.Background(), opts...)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(token).To(Equal(&azure.Token{AccessToken: azcore.AccessToken{
				Token:     "access-token",
				ExpiresOn: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			}}))
		})
	}
}

func TestProvider_NewTokenForServiceAccount(t *testing.T) {
	for _, tt := range []struct {
		name        string
		annotations map[string]string
		err         string
	}{
		{
			name: "valid",
			annotations: map[string]string{
				"azure.workload.identity/tenant-id": "tenant-id",
				"azure.workload.identity/client-id": "client-id",
			},
		},
		{
			name: "tenant id missing",
			annotations: map[string]string{
				"azure.workload.identity/client-id": "client-id",
			},
			err: "azure tenant ID is not set in the service account annotation azure.workload.identity/tenant-id",
		},
		{
			name: "client id missing",
			annotations: map[string]string{
				"azure.workload.identity/tenant-id": "tenant-id",
			},
			err: "azure client ID is not set in the service account annotation azure.workload.identity/client-id",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			impl := &mockImplementation{
				t:            t,
				argTenantID:  "tenant-id",
				argClientID:  "client-id",
				argOIDCToken: "oidc-token",
				argProxyURL:  &url.URL{Scheme: "http", Host: "proxy.example.com"},
				argScopes:    []string{"scope1", "scope2"},
				returnToken:  "access-token",
			}

			oidcToken := "oidc-token"
			serviceAccount := corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			opts := []auth.Option{
				auth.WithProxyURL(url.URL{Scheme: "http", Host: "proxy.example.com"}),
				auth.WithScopes("scope1", "scope2"),
			}

			provider := azure.Provider{Implementation: impl}
			token, err := provider.NewTokenForServiceAccount(context.Background(), oidcToken, serviceAccount, opts...)

			if tt.err == "" {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(token).To(Equal(&azure.Token{AccessToken: azcore.AccessToken{
					Token:     "access-token",
					ExpiresOn: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				}}))
			} else {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(Equal(tt.err))
				g.Expect(token).To(BeNil())
			}
		})
	}
}

func TestProvider_GetAudience(t *testing.T) {
	g := NewWithT(t)
	aud, err := azure.Provider{}.GetAudience(context.Background(), corev1.ServiceAccount{})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(aud).To(Equal("api://AzureADTokenExchange"))
}

func TestProvider_GetIdentity(t *testing.T) {
	g := NewWithT(t)

	identity, err := azure.Provider{}.GetIdentity(corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"azure.workload.identity/client-id": "client-id",
				"azure.workload.identity/tenant-id": "tenant-id",
			},
		},
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(identity).To(Equal("tenant-id/client-id"))
}

func TestProvider_NewArtifactRegistryCredentials(t *testing.T) {
	g := NewWithT(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	g.Expect(err).NotTo(HaveOccurred())
	exp := time.Now().Add(time.Hour).Unix()
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"exp": exp,
	}).SignedString(privateKey)
	g.Expect(err).NotTo(HaveOccurred())

	for _, tt := range []struct {
		registry      string
		expectedScope string
	}{
		{
			registry:      "foo.azurecr.io",
			expectedScope: "https://management.azure.com/.default",
		},
		{
			registry:      "foo.azurecr.cn",
			expectedScope: "https://management.chinacloudapi.cn/.default",
		},
		{
			registry:      "foo.azurecr.us",
			expectedScope: "https://management.usgovcloudapi.net/.default",
		},
	} {
		t.Run(tt.registry, func(t *testing.T) {
			g := NewWithT(t)

			impl := &mockImplementation{
				t:              t,
				argRegistry:    tt.registry,
				argToken:       "access-token",
				argProxyURL:    &url.URL{Scheme: "http", Host: "proxy.example.com"},
				argScopes:      []string{tt.expectedScope},
				returnToken:    "access-token",
				returnACRToken: refreshToken,
			}
			provider := azure.Provider{Implementation: impl}

			artifactRepository := fmt.Sprintf("%s/repo", tt.registry)
			opts := []auth.Option{
				auth.WithProxyURL(url.URL{Scheme: "http", Host: "proxy.example.com"}),
			}

			creds, err := auth.GetArtifactRegistryCredentials(context.Background(), provider, artifactRepository, opts...)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(creds).To(Equal(&auth.ArtifactRegistryCredentials{
				Authenticator: authn.FromConfig(authn.AuthConfig{
					Username: "00000000-0000-0000-0000-000000000000",
					Password: refreshToken,
				}),
				ExpiresAt: time.Unix(exp, 0),
			}))
		})
	}
}

func TestProvider_ParseArtifactRegistry(t *testing.T) {
	for _, tt := range []struct {
		artifactRepository  string
		expectedRegistryURL string
		expectValid         bool
	}{
		{
			artifactRepository:  "foo.azurecr.io/repo",
			expectedRegistryURL: "foo.azurecr.io",
			expectValid:         true,
		},
		{
			artifactRepository:  "foo.azurecr.cn/repo",
			expectedRegistryURL: "foo.azurecr.cn",
			expectValid:         true,
		},
		{
			artifactRepository:  "foo.azurecr.de/repo",
			expectedRegistryURL: "foo.azurecr.de",
			expectValid:         true,
		},
		{
			artifactRepository:  "foo.azurecr.us/repo",
			expectedRegistryURL: "foo.azurecr.us",
			expectValid:         true,
		},
		{
			artifactRepository: "foo.azurecr.com/repo",
			expectValid:        false,
		},
		{
			artifactRepository: ".azurecr.io/repo",
			expectValid:        false,
		},
		{
			artifactRepository: "012345678901.dkr.ecr.us-east-1.amazonaws.com",
			expectValid:        false,
		},
	} {
		t.Run(tt.artifactRepository, func(t *testing.T) {
			g := NewWithT(t)

			registryURL, err := azure.Provider{}.ParseArtifactRepository(tt.artifactRepository)

			if tt.expectValid {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(registryURL).To(Equal(tt.expectedRegistryURL))
			} else {
				g.Expect(err).To(HaveOccurred())
				g.Expect(registryURL).To(BeEmpty())
			}
		})
	}
}

func TestProvider_NewRESTConfig(t *testing.T) {
	for _, tt := range []struct {
		name           string
		cluster        string
		clusterAddress string
		aadProfile     *armcontainerservice.ManagedClusterAADProfile
		kubeconfigs    []*armcontainerservice.CredentialResult
		err            string
	}{
		{
			name:    "valid AKS cluster",
			cluster: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ContainerService/managedClusters/test-cluster",
			aadProfile: &armcontainerservice.ManagedClusterAADProfile{
				Managed: &[]bool{true}[0],
			},
			kubeconfigs: []*armcontainerservice.CredentialResult{
				{
					Name:  &[]string{"clusterUser"}[0],
					Value: createKubeconfig("test-cluster", "https://test-cluster-12345678.hcp.eastus.azmk8s.io:443"),
				},
				{
					Name:  &[]string{"clusterUser-secondary"}[0],
					Value: createKubeconfig("test-cluster-secondary", "https://test-cluster-secondary-87654321.hcp.westus.azmk8s.io:443"),
				},
			},
		},
		{
			name:    "valid AKS cluster - lowercase",
			cluster: "/subscriptions/12345678-1234-1234-1234-123456789012/resourcegroups/test-rg/providers/Microsoft.ContainerService/managedClusters/test-cluster",
			aadProfile: &armcontainerservice.ManagedClusterAADProfile{
				Managed: &[]bool{true}[0],
			},
			kubeconfigs: []*armcontainerservice.CredentialResult{
				{
					Name:  &[]string{"clusterUser"}[0],
					Value: createKubeconfig("test-cluster", "https://test-cluster-12345678.hcp.eastus.azmk8s.io:443"),
				},
				{
					Name:  &[]string{"clusterUser-secondary"}[0],
					Value: createKubeconfig("test-cluster-secondary", "https://test-cluster-secondary-87654321.hcp.westus.azmk8s.io:443"),
				},
			},
		},
		{
			name:           "valid AKS cluster with address match",
			cluster:        "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ContainerService/managedClusters/test-cluster",
			clusterAddress: "https://test-cluster-secondary-87654321.hcp.westus.azmk8s.io:443",
			aadProfile: &armcontainerservice.ManagedClusterAADProfile{
				Managed: &[]bool{true}[0],
			},
			kubeconfigs: []*armcontainerservice.CredentialResult{
				{
					Name:  &[]string{"clusterUser"}[0],
					Value: createKubeconfig("test-cluster", "https://test-cluster-12345678.hcp.eastus.azmk8s.io:443"),
				},
				{
					Name:  &[]string{"clusterUser-secondary"}[0],
					Value: createKubeconfig("test-cluster-secondary", "https://test-cluster-secondary-87654321.hcp.westus.azmk8s.io:443"),
				},
			},
		},
		{
			name:           "cluster address mismatch",
			cluster:        "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ContainerService/managedClusters/test-cluster",
			clusterAddress: "https://different-cluster.hcp.eastus.azmk8s.io:443",
			aadProfile: &armcontainerservice.ManagedClusterAADProfile{
				Managed: &[]bool{true}[0],
			},
			kubeconfigs: []*armcontainerservice.CredentialResult{
				{
					Name:  &[]string{"clusterUser"}[0],
					Value: createKubeconfig("test-cluster", "https://test-cluster-12345678.hcp.eastus.azmk8s.io:443"),
				},
			},
			err: "AKS cluster /subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ContainerService/managedClusters/test-cluster does not match specified address 'https://different-cluster.hcp.eastus.azmk8s.io:443'. cluster addresses: ['https://test-cluster-12345678.hcp.eastus.azmk8s.io:443']",
		},
		{
			name:    "cluster without AAD integration",
			cluster: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ContainerService/managedClusters/test-cluster",
			err:     "AKS cluster /subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ContainerService/managedClusters/test-cluster does not have Microsoft Entra ID integration enabled. See docs for enabling: https://learn.microsoft.com/en-us/azure/aks/enable-authentication-microsoft-entra-id",
		},
		{
			name:    "invalid cluster ID",
			cluster: "invalid-cluster-id",
			err:     `invalid AKS cluster ID: 'invalid-cluster-id'. must match (?i)^/subscriptions/([^/]{36})/resourceGroups/([^/]{1,200})/providers/Microsoft\.ContainerService/managedClusters/([^/]{1,200})$`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			impl := &mockImplementation{
				t:                t,
				argToken:         "access-token",
				argFirstScopes:   []string{"https://management.core.windows.net//.default"},
				argSecondScopes:  []string{"6dae42f8-4368-4678-94ff-3960e28e3630/.default"},
				argSubscription:  "12345678-1234-1234-1234-123456789012",
				argResourceGroup: "test-rg",
				argClusterName:   "test-cluster",
				argProxyURL:      &url.URL{Scheme: "http", Host: "proxy.example.com"},
				returnToken:      "access-token",
				returnCluster: armcontainerservice.ManagedCluster{
					Properties: &armcontainerservice.ManagedClusterProperties{
						AADProfile: tt.aadProfile,
					},
				},
				returnKubeconfigs: tt.kubeconfigs,
			}

			opts := []auth.Option{
				auth.WithProxyURL(url.URL{Scheme: "http", Host: "proxy.example.com"}),
			}

			if tt.clusterAddress != "" {
				opts = append(opts, auth.WithClusterAddress(tt.clusterAddress))
			}

			provider := azure.Provider{Implementation: impl}
			restConfig, err := auth.GetRESTConfig(context.Background(), provider, tt.cluster, opts...)

			if tt.err == "" {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(restConfig).NotTo(BeNil())
				expectedHost := "https://test-cluster-12345678.hcp.eastus.azmk8s.io:443"
				if tt.clusterAddress != "" {
					expectedHost = tt.clusterAddress
				}
				g.Expect(restConfig.Host).To(Equal(expectedHost))
				g.Expect(restConfig.BearerToken).To(Equal("access-token"))
				g.Expect(restConfig.CAData).To(Equal([]byte("-----BEGIN CERTIFICATE-----")))
				g.Expect(restConfig.ExpiresAt).To(Equal(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))
			} else {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.err))
				g.Expect(restConfig).To(BeNil())
			}
		})
	}
}

func TestProvider_GetAccessTokenOptionsForCluster(t *testing.T) {
	g := NewWithT(t)

	opts, err := azure.Provider{}.GetAccessTokenOptionsForCluster("/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/test-rg/providers/Microsoft.ContainerService/managedClusters/test-cluster")
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(opts).To(HaveLen(2))

	// ARM token options
	var armOptions auth.Options
	armOptions.Apply(opts[0]...)
	g.Expect(armOptions.Scopes).To(Equal([]string{"https://management.core.windows.net//.default"}))

	// AKS token options
	var aksOptions auth.Options
	aksOptions.Apply(opts[1]...)
	g.Expect(aksOptions.Scopes).To(Equal([]string{"6dae42f8-4368-4678-94ff-3960e28e3630/.default"}))
}

func createKubeconfig(clusterName, serverURL string) []byte {
	return []byte(fmt.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t
    server: %s
  name: %s
contexts:
- context:
    cluster: %s
    user: clusterUser_test-rg_%s
  name: %s
current-context: %s
kind: Config
users:
- name: clusterUser_test-rg_%s
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: kubelogin
      env: null
`, serverURL, clusterName, clusterName, clusterName, clusterName, clusterName, clusterName))
}
