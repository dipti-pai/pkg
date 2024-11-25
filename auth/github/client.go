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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"golang.org/x/net/http/httpproxy"
)

const (
	AppIDKey             = "githubAppID"
	AppInstallationIDKey = "githubAppInstallationID"
	AppPrivateKey        = "githubAppPrivateKey"
	ApiURLKey            = "githubApiURL"
)

// Client is an authentication provider for GitHub Apps.
type Client struct {
	appID          string
	installationID string
	privateKey     []byte
	apiURL         string
	proxyURL       *url.URL
	ghTransport    *ghinstallation.Transport
}

// OptFunc enables specifying options for the provider.
type OptFunc func(*Client)

// New returns a new authentication provider for GitHub Apps.
func New(opts ...OptFunc) (*Client, error) {
	var err error

	p := &Client{}
	for _, opt := range opts {
		opt(p)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if p.proxyURL != nil {
		proxyStr := p.proxyURL.String()
		proxyConfig := &httpproxy.Config{
			HTTPProxy:  proxyStr,
			HTTPSProxy: proxyStr,
		}
		proxyFunc := func(req *http.Request) (*url.URL, error) {
			return proxyConfig.ProxyFunc()(req.URL)
		}
		transport.Proxy = proxyFunc
	}

	appID, err := strconv.Atoi(p.appID)
	if err != nil {
		return nil, fmt.Errorf("invalid app id, err: %v", err)
	}
	installationID, err := strconv.Atoi(p.installationID)
	if err != nil {
		return nil, fmt.Errorf("invalid app installation id, err: %v", err)
	}

	p.ghTransport, err = ghinstallation.New(transport, int64(appID), int64(installationID), p.privateKey)
	if err != nil {
		return nil, err
	}

	if p.apiURL != "" {
		p.ghTransport.BaseURL = p.apiURL
	}

	return p, nil
}

// WithInstallationID configures the installation ID of the GitHub App.
func WithInstllationID(installationID string) OptFunc {
	return func(p *Client) {
		p.installationID = installationID
	}
}

// WithAppID configures the app ID of the GitHub App.
func WithAppID(appID string) OptFunc {
	return func(p *Client) {
		p.appID = appID
	}
}

// WithPrivateKey configures the private key of the GitHub App.
func WithPrivateKey(pk []byte) OptFunc {
	return func(p *Client) {
		p.privateKey = pk
	}
}

// WithApiURL configures the GitHub API endpoint to use to fetch GitHub App
// installation token.
func WithApiURL(apiURL string) OptFunc {
	return func(p *Client) {
		p.apiURL = apiURL
	}
}

// WithAppData configures the client using data from a map
func WithAppData(appData map[string][]byte) OptFunc {
	return func(p *Client) {
		val, ok := appData[AppIDKey]
		if ok {
			p.appID = string(val)
		}
		val, ok = appData[AppInstallationIDKey]
		if ok {
			p.installationID = string(val)
		}
		val, ok = appData[AppPrivateKey]
		if ok {
			p.privateKey = val
		}
		val, ok = appData[ApiURLKey]
		if ok {
			p.apiURL = string(val)
		}
	}
}

// WithProxyURL sets the proxy URL to use with the transport.
func WithProxyURL(proxyURL *url.URL) OptFunc {
	return func(p *Client) {
		p.proxyURL = proxyURL
	}
}

// AppToken contains a GitHub App installation token and its expiry.
type AppToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GetToken returns the token that can be used to authenticate
// as a GitHub App installation.
// Ref: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation
func (p *Client) GetToken(ctx context.Context) (*AppToken, error) {
	token, err := p.ghTransport.Token(ctx)
	if err != nil {
		return nil, err
	}

	expiresAt, _, err := p.ghTransport.Expiry()
	if err != nil {
		return nil, err
	}

	return &AppToken{
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}
