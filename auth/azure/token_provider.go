/*
Copyright 2024 The Flux authors

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

package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

const (
	// 499b84ac-1321-427f-aa17-267ca6975798 is the UUID of Azure Devops
	// resource. The scope defined below provides access to Azure DevOps
	// Services REST API. The Microsoft Entra ID access token with this scope
	// can be used to call Azure DevOps API by passing it in the headers as a
	// Bearer Token : https://learn.microsoft.com/en-us/azure/devops/integrate/get-started/authentication/service-principal-managed-identity?view=azure-devops#q-can-i-use-a-service-principal-or-managed-identity-with-azure-cli
	AzureDevOpsRestApiScope = "499b84ac-1321-427f-aa17-267ca6975798/.default"
)

// Provider is an authentication provider for Azure.
type Provider struct {
	credential azcore.TokenCredential
	scopes     []string
}

// ProviderOptFunc enables specifying options for the provider.
type ProviderOptFunc func(*Provider)

// NewProvider returns a new authentication provider for Azure.
func NewProvider(opts ...ProviderOptFunc) *Provider {
	p := &Provider{}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// WithCredential configures the credential to use to fetch the resource manager
// token.
func WithCredential(cred azcore.TokenCredential) ProviderOptFunc {
	return func(p *Provider) {
		p.credential = cred
	}
}

// WithAzureDevOpsScope() configures the scope to access Azure DevOps Rest API
// needed to access Azure DevOps Repositories
func WithAzureDevOpsScope() ProviderOptFunc {
	return func(p *Provider) {
		p.scopes = []string{AzureDevOpsRestApiScope}
	}
}

// GetToken requests an access token from Microsoft Entra ID by using a default
// credential chain.
// https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/azidentity#NewDefaultAzureCredential
// The default scope is to ARM endpoint in Azure Cloud. The scope is overridden
// using ProviderOptFunc.
func (p *Provider) GetToken(ctx context.Context) (azcore.AccessToken, error) {
	var accessToken azcore.AccessToken
	if p.credential == nil {
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return accessToken, err
		}
		p.credential = cred
	}

	if len(p.scopes) == 0 {
		p.scopes = []string{cloud.AzurePublic.Services[cloud.ResourceManager].Endpoint + "/" + ".default"}
	}

	return p.credential.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: p.scopes,
	})
}
