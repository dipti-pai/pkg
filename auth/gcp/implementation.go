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

package gcp

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/google/externalaccount"
	"google.golang.org/api/container/v1"
)

// Implementation provides the required methods of the GCP libraries.
type Implementation interface {
	DefaultTokenSource(ctx context.Context, scope ...string) (oauth2.TokenSource, error)
	NewTokenSource(ctx context.Context, conf externalaccount.Config) (oauth2.TokenSource, error)
	GetCluster(ctx context.Context, cluster string, client *container.Service) (*container.Cluster, error)
}

type implementation struct{}

func (implementation) DefaultTokenSource(ctx context.Context, scope ...string) (oauth2.TokenSource, error) {
	return google.DefaultTokenSource(ctx, scope...)
}

func (implementation) NewTokenSource(ctx context.Context, conf externalaccount.Config) (oauth2.TokenSource, error) {
	return externalaccount.NewTokenSource(ctx, conf)
}

func (implementation) GetCluster(ctx context.Context, cluster string, client *container.Service) (*container.Cluster, error) {
	return client.Projects.Locations.Clusters.Get(cluster).Context(ctx).Do()
}
