module github.com/fluxcd/pkg/git

go 1.22.4

toolchain go1.22.5

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.12.0
	github.com/ProtonMail/go-crypto v1.0.0
	github.com/cyphar/filepath-securejoin v0.3.1
	github.com/fluxcd/pkg/auth v0.0.0-00010101000000-000000000000
	github.com/onsi/gomega v1.34.1
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.7.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.9.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.2 // indirect
	github.com/cloudflare/circl v1.3.9 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.1 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/pprof v0.0.0-20240525223248-4bfdf5a9a2af // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	golang.org/x/crypto v0.26.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sys v0.25.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/fluxcd/pkg/auth => ../auth
