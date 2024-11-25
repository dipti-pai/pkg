//go:build e2e
// +build e2e

/*
// Copyright 2022 The Flux authors

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

	. "github.com/onsi/gomega"

	"github.com/fluxcd/go-git-providers/github"
	"github.com/fluxcd/go-git-providers/gitprovider"
	authgithub "github.com/fluxcd/pkg/auth/github"
	"github.com/fluxcd/pkg/git"
	"github.com/fluxcd/pkg/git/gogit"
	gogithub "github.com/google/go-github/v64/github"
)

const (
	githubSSHHost         = "ssh://" + git.DefaultPublicKeyAuthUser + "@" + github.DefaultDomain
	githubHTTPHost        = "https://" + github.DefaultDomain
	githubUser            = "GITHUB_USER"
	githubOrg             = "GITHUB_ORG"
	githubAppIDEnv        = "GHAPP_ID"
	githubAppInstallIDEnv = "GHAPP_INSTALL_ID"
	githubAppPKEnv        = "GHAPP_PRIVATE_KEY"
)

var (
	githubOAuth2Token       string
	githubUsername          string
	githubOrgname           string
	githubAppID             int
	githubAppInstallationID int
	githubAppPrivateKey     []byte
)

func TestGitHubE2E(t *testing.T) {
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

	repoInfo := func(proto git.TransportType, repo gitprovider.OrgRepository, githubApp bool) (*url.URL, *git.AuthOptions, error) {
		var repoURL *url.URL
		var authOptions *git.AuthOptions
		var err error

		if proto == git.SSH {
			repoURL, err = url.Parse(githubSSHHost + "/" + githubOrgname + "/" + repo.Repository().GetRepository())
			if err != nil {
				return nil, nil, err
			}

			sshAuth, err := createSSHIdentitySecret(*repoURL)
			if err != nil {
				return nil, nil, err
			}
			dkClient := repo.DeployKeys()
			var readOnly bool
			_, err = dkClient.Create(context.TODO(), gitprovider.DeployKeyInfo{
				Name:     "git-e2e-deploy-key" + randStringRunes(5),
				Key:      sshAuth["identity.pub"],
				ReadOnly: &readOnly,
			})
			if err != nil {
				return nil, nil, err
			}

			authOptions, err = git.NewAuthOptions(*repoURL, sshAuth)
			if err != nil {
				return nil, nil, err
			}
		} else {
			repoURL, err = url.Parse(githubHTTPHost + "/" + githubOrgname + "/" + repo.Repository().GetRepository())
			if err != nil {
				return nil, nil, err
			}

			if githubApp {
				var data map[string][]byte
				authOptions, err = git.NewAuthOptions(*repoURL, data)
				authOptions.ProviderOpts = &git.ProviderOptions{
					Name: git.ProviderGitHub,
					GitHubOpts: []authgithub.OptFunc{
						authgithub.WithAppID(githubAppID),
						authgithub.WithInstllationID(githubAppInstallID),
						authgithub.WithPrivateKey(githubAppPrivateKey),
					},
				}
			} else {
				authOptions, err = git.NewAuthOptions(*repoURL, map[string][]byte{
					"username": []byte(githubUsername),
					"password": []byte(githubOAuth2Token),
				})
			}
			if err != nil {
				return nil, nil, err
			}
		}
		return repoURL, authOptions, nil
	}

	protocols := []git.TransportType{git.HTTP, git.SSH}
	clients := []string{gogit.ClientName}

	testFunc := func(t *testing.T, proto git.TransportType, gitClient string, githubApp bool) {
		t.Run(fmt.Sprintf("repo created using Clone/%s/%s", gitClient, proto), func(t *testing.T) {
			g := NewWithT(t)

			repoName := fmt.Sprintf("github-e2e-checkout-%s-%s-%s-%s", string(proto), string(gitClient), strconv.FormatBool(githubApp), randStringRunes(5))
			upstreamRepoURL := githubHTTPHost + "/" + githubOrgname + "/" + repoName

			ref, err := gitprovider.ParseOrgRepositoryURL(upstreamRepoURL)
			g.Expect(err).ToNot(HaveOccurred())
			repo, err := orgClient.Create(context.TODO(), *ref, gitprovider.RepositoryInfo{})
			g.Expect(err).ToNot(HaveOccurred())

			defer repo.Delete(context.TODO())

			if githubApp {
				err := grantPermissionsToApp(repo)
				g.Expect(err).ToNot(HaveOccurred())
			}

			err = initRepo(t.TempDir(), upstreamRepoURL, "main", "../../testdata/git/repo", githubUsername, githubOAuth2Token)
			g.Expect(err).ToNot(HaveOccurred())
			repoURL, authOptions, err := repoInfo(proto, repo, githubApp)
			g.Expect(err).ToNot(HaveOccurred())

			client, err := newClient(gitClient, t.TempDir(), authOptions, false)
			g.Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			testUsingClone(g, client, repoURL, upstreamRepoInfo{
				url:      upstreamRepoURL,
				username: githubUsername,
				password: githubOAuth2Token,
			})
		})

		t.Run(fmt.Sprintf("repo created using Init/%s/%s", gitClient, proto), func(t *testing.T) {
			g := NewWithT(t)

			repoName := fmt.Sprintf("github-e2e-checkout-%s-%s-%s", string(proto), string(gitClient), randStringRunes(5))
			upstreamRepoURL := githubHTTPHost + "/" + githubOrgname + "/" + repoName

			ref, err := gitprovider.ParseOrgRepositoryURL(upstreamRepoURL)
			g.Expect(err).ToNot(HaveOccurred())
			repo, err := orgClient.Create(context.TODO(), *ref, gitprovider.RepositoryInfo{})
			g.Expect(err).ToNot(HaveOccurred())

			defer repo.Delete(context.TODO())

			if githubApp {
				err := grantPermissionsToApp(repo)
				g.Expect(err).ToNot(HaveOccurred())
			}

			repoURL, authOptions, err := repoInfo(proto, repo, githubApp)
			g.Expect(err).ToNot(HaveOccurred())

			client, err := newClient(gitClient, t.TempDir(), authOptions, false)
			g.Expect(err).ToNot(HaveOccurred())
			defer client.Close()

			testUsingInit(g, client, repoURL, upstreamRepoInfo{
				url:      upstreamRepoURL,
				username: githubUsername,
				password: githubOAuth2Token,
			})
		})
	}

	for _, client := range clients {
		for _, protocol := range protocols {
			// test client with all protocols without githubApp authentication
			testFunc(t, protocol, client, false)
		}
		// test client with HTTPS protocol with githubApp authentication
		testFunc(t, git.HTTP, client, true)
	}
}
