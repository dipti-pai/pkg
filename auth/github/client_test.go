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

package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

type accessToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func TestClient_Options(t *testing.T) {
	appID := 123
	installationID := 456
	pk, _ := createPrivateKey()
	gitHubDefaultURL := "https://api.github.com"
	gitHubEnterpriseURL := "https://github.example.com/api/v3"
	proxy, _ := url.Parse("http://localhost:8080")

	tests := []struct {
		name         string
		opts         []OptFunc
		useProxy     bool
		customApiUrl bool
		wantErr      error
	}{
		{
			name: "Create new client",
			opts: []OptFunc{WithInstllationID(installationID), WithAppID(appID), WithPrivateKey(pk)},
		},
		{
			name:     "Create new client with proxy",
			opts:     []OptFunc{WithInstllationID(installationID), WithAppID(appID), WithPrivateKey(pk)},
			useProxy: true,
		},
		{
			name:         "Create new client with custom api url",
			opts:         []OptFunc{WithApiURL(gitHubEnterpriseURL), WithInstllationID(installationID), WithAppID(appID), WithPrivateKey(pk)},
			customApiUrl: true,
		},
		{
			name: "Create new client with secret",
			opts: []OptFunc{WithSecret(corev1.Secret{
				Data: map[string][]byte{
					AppIDKey:             []byte(fmt.Sprintf("%d", appID)),
					AppInstallationIDKey: []byte(fmt.Sprintf("%d", installationID)),
					AppPrivateKey:        pk,
				},
			})},
		},
		{
			name: "Create new client with invalid appID in secret",
			opts: []OptFunc{WithSecret(corev1.Secret{
				Data: map[string][]byte{
					AppIDKey:             []byte("abc"),
					AppInstallationIDKey: []byte(fmt.Sprintf("%d", installationID)),
					AppPrivateKey:        pk,
				},
			})},
			wantErr: &strconv.NumError{Func: "Atoi", Num: "abc", Err: strconv.ErrSyntax},
		},
		{
			name: "Create new client with invalid installationID in secret",
			opts: []OptFunc{WithSecret(corev1.Secret{
				Data: map[string][]byte{
					AppIDKey:             []byte(fmt.Sprintf("%d", appID)),
					AppInstallationIDKey: []byte("abc"),
					AppPrivateKey:        pk,
				},
			})},
			wantErr: &strconv.NumError{Func: "Atoi", Num: "abc", Err: strconv.ErrSyntax},
		},
		{
			name: "Create new client with invalid private key in secret",
			opts: []OptFunc{WithSecret(corev1.Secret{
				Data: map[string][]byte{
					AppIDKey:             []byte(fmt.Sprintf("%d", appID)),
					AppInstallationIDKey: []byte(fmt.Sprintf("%d", installationID)),
				},
			})},
			wantErr: errors.New("could not parse private key: invalid key: Key must be a PEM encoded PKCS1 or PKCS8 key"),
		},
		{
			name:    "Create new client with no private key option",
			opts:    []OptFunc{WithInstllationID(installationID), WithAppID(appID)},
			wantErr: errors.New("could not parse private key: invalid key: Key must be a PEM encoded PKCS1 or PKCS8 key"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			opts := tt.opts
			if tt.useProxy {
				opts = append(opts, WithProxyURL(proxy))
			}

			client, err := New(opts...)
			if tt.wantErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tt.wantErr))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(client.appID).To(Equal(appID))
				g.Expect(client.installationID).To(Equal(installationID))
				g.Expect(client.privateKey).To(Equal(pk))

				if tt.customApiUrl {
					g.Expect(client.apiURL).To(Equal(gitHubEnterpriseURL))
					g.Expect(client.ghTransport.BaseURL).To(Equal(gitHubEnterpriseURL))
				} else {
					g.Expect(client.ghTransport.BaseURL).To(Equal(gitHubDefaultURL))
				}
			}
		})
	}
}

func TestClient_GetToken(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	tests := []struct {
		name         string
		accessToken  *accessToken
		statusCode   int
		wantErr      bool
		wantAppToken *AppToken
	}{
		{
			name: "Get valid token",
			accessToken: &accessToken{
				Token:     "access-token",
				ExpiresAt: expiresAt,
			},
			statusCode: http.StatusOK,
			wantAppToken: &AppToken{
				Token:     "access-token",
				ExpiresAt: expiresAt,
			},
		},
		{
			name:       "Failure in getting token",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			handler := func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				var response []byte
				var err error
				if tt.accessToken != nil {
					response, err = json.Marshal(tt.accessToken)
					g.Expect(err).ToNot(HaveOccurred())
				}
				w.Write(response)
			}
			srv := httptest.NewServer(http.HandlerFunc(handler))
			t.Cleanup(func() {
				srv.Close()
			})

			pk, err := createPrivateKey()
			g.Expect(err).ToNot(HaveOccurred())
			opts := []OptFunc{
				WithApiURL(srv.URL), WithInstllationID(123), WithAppID(456), WithPrivateKey(pk),
			}

			provider, err := New(opts...)
			g.Expect(err).ToNot(HaveOccurred())

			appToken, err := provider.GetToken(context.TODO())
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(appToken.Token).To(Equal(tt.wantAppToken.Token))
				g.Expect(appToken.ExpiresAt).To(Equal(tt.wantAppToken.ExpiresAt))
			}
		})
	}
}

func createPrivateKey() ([]byte, error) {
	privatekey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	var privateKeyBytes []byte = x509.MarshalPKCS1PrivateKey(privatekey)
	privateKeyBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	pk := pem.EncodeToMemory(privateKeyBlock)
	return pk, nil
}
