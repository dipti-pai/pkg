module github.com/fluxcd/pkg/git/gogit

go 1.22.4

replace (
	github.com/fluxcd/pkg/auth => ../../auth
	github.com/fluxcd/pkg/git => ../../git
	github.com/fluxcd/pkg/gittestserver => ../../gittestserver
	github.com/fluxcd/pkg/ssh => ../../ssh
	github.com/fluxcd/pkg/version => ../../version
)

require (
	github.com/Masterminds/semver/v3 v3.3.0
	github.com/armon/go-socks5 v0.0.0-20160902184237-e75332964ef5
	github.com/elazarl/goproxy v0.0.0-20240909085733-6741dbfc16a1
	github.com/fluxcd/gitkit v0.6.0
	github.com/fluxcd/pkg/auth v0.0.1
	github.com/fluxcd/pkg/git v0.20.0
	github.com/fluxcd/pkg/gittestserver v0.13.1
	github.com/fluxcd/pkg/ssh v0.14.1
	github.com/fluxcd/pkg/version v0.4.1
	github.com/go-git/go-billy/v5 v5.5.0
	github.com/go-git/go-git/v5 v5.12.0
	github.com/onsi/gomega v1.34.2
	golang.org/x/crypto v0.27.0
)

require (
	dario.cat/mergo v1.0.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.14.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.10.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/ProtonMail/go-crypto v1.0.0 // indirect
	github.com/bradleyfalzon/ghinstallation/v2 v2.11.0 // indirect
	github.com/cloudflare/circl v1.4.0 // indirect
	github.com/cyphar/filepath-securejoin v0.3.2 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-git/gcfg v1.5.1-0.20230307220236-3a3c6141e376 // indirect
	github.com/gofrs/uuid v4.4.0+incompatible // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/go-github/v62 v62.0.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/pjbgf/sha1cd v0.3.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/sergi/go-diff v1.3.2-0.20230802210424-5b0b94c5c0d3 // indirect
	github.com/skeema/knownhosts v1.3.0 // indirect
	github.com/xanzy/ssh-agent v0.3.3 // indirect
	golang.org/x/mod v0.21.0 // indirect
	golang.org/x/net v0.29.0 // indirect
	golang.org/x/sync v0.8.0 // indirect
	golang.org/x/sys v0.25.0 // indirect
	golang.org/x/text v0.18.0 // indirect
	golang.org/x/tools v0.25.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
