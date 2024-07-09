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

package git

import (
	"context"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/fluxcd/pkg/auth/azure"
	. "github.com/onsi/gomega"
)

func TestGetCredentials(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	tests := []struct {
		name            string
		url             string
		provider        *ProviderOptions
		wantCredentials *Credentials
		wantScope       string
	}{
		{
			name: "get credentials from azure",
			url:  "https://dev.azure.com/foo/bar/_git/baz",
			provider: &ProviderOptions{
				Name: ProviderAzure,
				AzureOpts: []azure.ProviderOptFunc{
					azure.WithCredential(&azure.FakeTokenCredential{
						Token:     "ado-token",
						ExpiresOn: expiresAt,
					}),
					azure.WithAzureDevOpsScope(),
				},
			},
			wantCredentials: &Credentials{
				BearerToken: "ado-token",
			},
			wantScope: azure.AzureDevOpsRestApiScope,
		},
		{
			name: "get credentials from azure without scope",
			url:  "https://dev.azure.com/foo/bar/_git/baz",
			provider: &ProviderOptions{
				Name: ProviderAzure,
				AzureOpts: []azure.ProviderOptFunc{
					azure.WithCredential(&azure.FakeTokenCredential{
						Token:     "ado-token",
						ExpiresOn: expiresAt,
					}),
				},
			},
			wantCredentials: &Credentials{
				BearerToken: "ado-token",
			},
			wantScope: cloud.AzurePublic.Services[cloud.ResourceManager].Endpoint + "/" + ".default",
		},
		{
			name: "get credentials from azure",
			url:  "https://dev.azure.com/foo/bar/_git/baz",
			provider: &ProviderOptions{
				Name: "dummy",
			},
			wantCredentials: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			str := ""
			ctx := context.WithValue(context.TODO(), "scope", &str)
			creds, expiry, err := GetCredentials(ctx, tt.url, tt.provider)

			if tt.wantCredentials != nil {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(*creds).To(Equal(*tt.wantCredentials))
				val := ctx.Value("scope").(*string)
				g.Expect(*val).ToNot(BeEmpty())
				g.Expect(*val).To(Equal((tt.wantScope)))

				expectedCredBytes := []byte(tt.wantCredentials.BearerToken)
				receivedCredBytes := []byte(creds.BearerToken)
				g.Expect(receivedCredBytes).To(Equal(expectedCredBytes))
				g.Expect(expiry).To(Equal(expiresAt))
			} else {
				g.Expect(creds).To(BeNil())
				g.Expect(err).To(HaveOccurred())
			}
		})
	}
}
