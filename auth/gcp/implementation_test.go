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

package gcp_test

import (
	"context"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"unsafe"

	. "github.com/onsi/gomega"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google/externalaccount"
	"google.golang.org/api/container/v1"
)

type mockImplementation struct {
	t *testing.T

	argConfig   externalaccount.Config
	argProxyURL *url.URL
	argCluster  string

	returnToken   *oauth2.Token
	returnCluster *container.Cluster
}

func (m *mockImplementation) DefaultTokenSource(ctx context.Context, scope ...string) (oauth2.TokenSource, error) {
	m.t.Helper()
	g := NewWithT(m.t)
	g.Expect(ctx).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient)).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient).(*http.Client)).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient).(*http.Client).Transport).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient).(*http.Client).Transport.(*http.Transport)).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient).(*http.Client).Transport.(*http.Transport).Proxy).NotTo(BeNil())
	proxyURL, err := ctx.Value(oauth2.HTTPClient).(*http.Client).Transport.(*http.Transport).Proxy(nil)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(proxyURL).To(Equal(m.argProxyURL))
	g.Expect(scope).To(Equal([]string{
		"https://www.googleapis.com/auth/cloud-platform",
		"https://www.googleapis.com/auth/userinfo.email",
	}))
	return oauth2.StaticTokenSource(m.returnToken), nil
}

func (m *mockImplementation) NewTokenSource(ctx context.Context, conf externalaccount.Config) (oauth2.TokenSource, error) {
	m.t.Helper()
	g := NewWithT(m.t)
	g.Expect(ctx).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient)).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient).(*http.Client)).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient).(*http.Client).Transport).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient).(*http.Client).Transport.(*http.Transport)).NotTo(BeNil())
	g.Expect(ctx.Value(oauth2.HTTPClient).(*http.Client).Transport.(*http.Transport).Proxy).NotTo(BeNil())
	proxyURL, err := ctx.Value(oauth2.HTTPClient).(*http.Client).Transport.(*http.Transport).Proxy(nil)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(proxyURL).To(Equal(m.argProxyURL))
	g.Expect(conf).To(Equal(m.argConfig))
	return oauth2.StaticTokenSource(m.returnToken), nil
}

func (m *mockImplementation) GetCluster(ctx context.Context, cluster string, client *container.Service) (*container.Cluster, error) {
	m.t.Helper()
	g := NewWithT(m.t)
	g.Expect(ctx).NotTo(BeNil())
	g.Expect(cluster).To(Equal(m.argCluster))
	g.Expect(client).NotTo(BeNil())
	g.Expect(client.BasePath).To(Equal("https://container.googleapis.com/"))
	httpClientField := reflect.ValueOf(client).Elem().FieldByName("client")
	httpClientValue := reflect.NewAt(httpClientField.Type(), unsafe.Pointer(httpClientField.UnsafeAddr())).Elem().Interface().(*http.Client)
	g.Expect(httpClientValue).NotTo(BeNil())
	g.Expect(httpClientValue.Transport).NotTo(BeNil())
	g.Expect(httpClientValue.Transport.(*oauth2.Transport)).NotTo(BeNil())
	g.Expect(httpClientValue.Transport.(*oauth2.Transport).Source).NotTo(BeNil())
	token, err := httpClientValue.Transport.(*oauth2.Transport).Source.Token()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(token).To(Equal(m.returnToken))
	g.Expect(httpClientValue.Transport.(*oauth2.Transport).Base).NotTo(BeNil())
	g.Expect(httpClientValue.Transport.(*oauth2.Transport).Base.(*otelhttp.Transport)).NotTo(BeNil())
	otelRoundTripperField := reflect.ValueOf(httpClientValue.Transport.(*oauth2.Transport).Base.(*otelhttp.Transport)).Elem().FieldByName("rt")
	otelRoundTripperValue := reflect.NewAt(otelRoundTripperField.Type(), unsafe.Pointer(otelRoundTripperField.UnsafeAddr())).Elem().Interface()
	g.Expect(otelRoundTripperValue).NotTo(BeNil())
	parameterRoundTripperField := reflect.ValueOf(otelRoundTripperValue).Elem().FieldByName("base")
	parameterRoundTripperValue := reflect.NewAt(parameterRoundTripperField.Type(), unsafe.Pointer(parameterRoundTripperField.UnsafeAddr())).Elem().Interface()
	g.Expect(parameterRoundTripperValue.(*http.Transport)).NotTo(BeNil())
	g.Expect(parameterRoundTripperValue.(*http.Transport).Proxy).NotTo(BeNil())
	proxyURL, err := parameterRoundTripperValue.(*http.Transport).Proxy(nil)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(proxyURL).To(Equal(m.argProxyURL))
	return m.returnCluster, nil
}
