//go:build e2e
// +build e2e

/*
// Copyright 2024 The Flux authors

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/fluxcd/go-git-providers/github"
	"github.com/fluxcd/go-git-providers/gitprovider"
	authgithub "github.com/fluxcd/pkg/auth/github"
	"github.com/fluxcd/pkg/cache"
	"github.com/fluxcd/pkg/git"
	"github.com/fluxcd/pkg/git/gogit"
	gogithub "github.com/google/go-github/v64/github"

	. "github.com/onsi/gomega"
)

func TestGitHubE2EWithCache(t *testing.T) {
	g := NewWithT(t)
	var err error
	githubOAuth2Token = os.Getenv(github.TokenVariable)
	if githubOAuth2Token == "" {
		t.Fatalf("could not read github oauth2 token")
	}
	githubUsername = os.Getenv(githubUser)
	if githubUsername == "" {
		t.Fatalf("could not read github username")
	}
	githubOrgname = os.Getenv(githubOrg)
	if githubOrgname == "" {
		t.Fatalf("could not read github org name")
	}
	githubAppID := os.Getenv(githubAppIDEnv)
	if githubAppID == "" {
		t.Fatalf("could not read github app id")
	}

	githubAppInstallID := os.Getenv(githubAppInstallIDEnv)
	if githubAppInstallID == "" {
		t.Fatalf("could not read github app installation id")
	}

	githubAppPrivateKey := []byte(os.Getenv(githubAppPKEnv))
	if len(githubAppPrivateKey) == 0 {
		t.Fatalf("could not read github app private key")
	}

	c, err := github.NewClient(gitprovider.WithDestructiveAPICalls(true), gitprovider.WithOAuth2Token(githubOAuth2Token))
	g.Expect(err).ToNot(HaveOccurred())
	orgClient := c.OrgRepositories()

	cache, err := cache.New[*git.Credentials](5, cache.WithCleanupInterval(1*time.Second))
	g.Expect(err).ToNot(HaveOccurred())

	grantPermissionsToApp := func(repo gitprovider.OrgRepository) error {
		ctx := context.Background()
		githubClient := c.Raw().(*gogithub.Client)
		ghRepo, _, err := githubClient.Repositories.Get(ctx, githubOrgname, repo.Repository().GetRepository())
		if err != nil {
			return err
		}
		installID, err := strconv.Atoi(githubAppInstallID)
		if err != nil {
			return err
		}
		_, _, err = githubClient.Apps.AddRepository(ctx, int64(installID), ghRepo.GetID())
		if err != nil {
			return err
		}

		return nil
	}

	repoName := fmt.Sprintf("github-e2e-checkout-%s", randStringRunes(5))
	upstreamRepoURL := githubHTTPHost + "/" + githubOrgname + "/" + repoName

	ref, err := gitprovider.ParseOrgRepositoryURL(upstreamRepoURL)
	g.Expect(err).ToNot(HaveOccurred())
	repo, err := orgClient.Create(context.TODO(), *ref, gitprovider.RepositoryInfo{})
	g.Expect(err).ToNot(HaveOccurred())

	defer repo.Delete(context.TODO())

	err = grantPermissionsToApp(repo)
	g.Expect(err).ToNot(HaveOccurred())

	err = initRepo(t.TempDir(), upstreamRepoURL, "main", "../../testdata/git/repo", githubUsername, githubOAuth2Token)
	g.Expect(err).ToNot(HaveOccurred())

	repoURL, err := url.Parse(githubHTTPHost + "/" + githubOrgname + "/" + repo.Repository().GetRepository())
	g.Expect(err).ToNot(HaveOccurred())

	var data map[string][]byte
	authOptions, err := git.NewAuthOptions(*repoURL, data)
	authOptions.ProviderOpts = &git.ProviderOptions{
		Name: git.ProviderGitHub,
		GitHubOpts: []authgithub.OptFunc{
			authgithub.WithAppID(githubAppID),
			authgithub.WithInstllationID(githubAppInstallID),
			authgithub.WithPrivateKey(githubAppPrivateKey),
		},
	}
	authOptions.Cache = cache

	client, err := gogit.NewClient(t.TempDir(), authOptions, gogit.WithDiskStorage(), gogit.WithCredentialCacheKey(repoURL.String()))
	g.Expect(err).ToNot(HaveOccurred())
	defer client.Close()

	output := testUsingClone(g, client, repoURL, upstreamRepoInfo{
		url:      upstreamRepoURL,
		username: githubUsername,
		password: githubOAuth2Token,
	})

	t.Logf("Test output : %s", output)
}
