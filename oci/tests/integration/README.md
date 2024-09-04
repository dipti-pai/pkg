# Integration tests

Integration tests uses a test application(`testapp/`) to test the
oci and git package against each of the supported cloud providers.

**NOTE:** Tests in this package aren't run automatically by the `test-*` make
target at the root of `fluxcd/pkg` repo. These tests are more complicated than
the regular go tests as it involves cloud infrastructure and have to be run
explicitly.

Before running the tests, build the test app with `make docker-build` and use
the built container application in the integration test.

The integration test provisions cloud infrastructure in a target provider and
runs the test app as a batch job which tries to log in and list tags from the
test registry repository. A successful job indicates successful test. If the job
fails, the test fails.

Logs of a successful job run for oci:
```console
$ kubectl logs test-job-93tbl-4jp2r
2022/07/28 21:59:06 repo: xxx.dkr.ecr.us-east-2.amazonaws.com/test-repo-flux-test-heroic-ram
1.659045546831094e+09   INFO    logging in to AWS ECR for xxx.dkr.ecr.us-east-2.amazonaws.com/test-repo-flux-test-heroic-ram
2022/07/28 21:59:06 logged in
2022/07/28 21:59:06 tags: [v0.1.4 v0.1.3 v0.1.0 v0.1.2]
```

Logs of a successful job run for git:
```console
$ kubectl logs test-job-dzful-jrcqw
2024/08/27 22:28:22 Successfully cloned repository
2024/08/27 22:28:22 apiVersion: v1
kind: ConfigMap
metadata:
  name: foobar
2024/08/27 22:28:22 Keys in cache  0 [https://dev.azure.com/xxx/fluxProjpopularosheepdog/_git/fluxRepopopularosheepdog]
2024/08/27 22:28:22 Cache entry expiration  2024-08-28 22:28:21.335223377 +0000 UTC <nil>
2024/08/27 22:28:22 Successfully cloned repository
2024/08/27 22:28:22 apiVersion: v1
kind: ConfigMap
metadata:
  name: foobar
2024/08/27 22:28:22 Keys in cache  1 [https://dev.azure.com/xxx/fluxProjpopularosheepdog/_git/fluxRepopopularosheepdog]
2024/08/27 22:28:22 Cache entry expiration  2024-08-28 22:28:21.335223377 +0000 UTC <nil>
```

## Requirements

### Amazon Web Services

- AWS account with access key ID and secret access key with permissions to
    create EKS cluster and ECR repository.
- AWS CLI v2.x, does not need to be configured with the AWS account.
- Docker CLI for registry login.
- kubectl for applying certain install manifests.
- jq for parsing JSON response from AWS.

#### Permissions

The following policy document can be used to create an IAM Policy for
provisioning the infrastructure and running the tests:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "testinfra",
            "Effect": "Allow",
            "Action": [
                "ec2:AllocateAddress",
                "ec2:AssociateRouteTable",
                "ec2:AttachInternetGateway",
                "ec2:AuthorizeSecurityGroupEgress",
                "ec2:AuthorizeSecurityGroupIngress",
                "ec2:CreateInternetGateway",
                "ec2:CreateLaunchTemplate",
                "ec2:CreateLaunchTemplateVersion",
                "ec2:CreateNatGateway",
                "ec2:CreateNetworkAcl",
                "ec2:CreateNetworkAclEntry",
                "ec2:CreateRoute",
                "ec2:CreateRouteTable",
                "ec2:CreateSecurityGroup",
                "ec2:CreateSubnet",
                "ec2:CreateTags",
                "ec2:CreateVpc",
                "ec2:DeleteInternetGateway",
                "ec2:DeleteLaunchTemplate",
                "ec2:DeleteNatGateway",
                "ec2:DeleteNetworkAcl",
                "ec2:DeleteNetworkAclEntry",
                "ec2:DeleteRoute",
                "ec2:DeleteRouteTable",
                "ec2:DeleteSecurityGroup",
                "ec2:DeleteSubnet",
                "ec2:DeleteTags",
                "ec2:DeleteVolume",
                "ec2:DeleteVpc",
                "ec2:DescribeAddresses",
                "ec2:DescribeAddressesAttribute",
                "ec2:DescribeAvailabilityZones",
                "ec2:DescribeInternetGateways",
                "ec2:DescribeLaunchTemplates",
                "ec2:DescribeLaunchTemplateVersions",
                "ec2:DescribeNatGateways",
                "ec2:DescribeNetworkAcls",
                "ec2:DescribeNetworkInterfaces",
                "ec2:DescribeRouteTables",
                "ec2:DescribeSecurityGroupRules",
                "ec2:DescribeSecurityGroups",
                "ec2:DescribeSubnets",
                "ec2:DescribeTags",
                "ec2:DescribeVpcAttribute",
                "ec2:DescribeVpcs",
                "ec2:DetachInternetGateway",
                "ec2:DisassociateAddress",
                "ec2:DisassociateRouteTable",
                "ec2:ModifyVpcAttribute",
                "ec2:ReleaseAddress",
                "ec2:RevokeSecurityGroupEgress",
                "ec2:RevokeSecurityGroupIngress",
                "ec2:RunInstances",
                "ecr:BatchGetImage",
                "ecr:BatchCheckLayerAvailability",
                "ecr:CreateRepository",
                "ecr:CompleteLayerUpload",
                "ecr:DeleteRepository",
                "ecr:DescribeRepositories",
                "ecr:GetAuthorizationToken",
                "ecr:InitiateLayerUpload",
                "ecr:ListTagsForResource",
                "ecr:PutImage",
                "ecr:TagResource",
                "ecr:UploadLayerPart",
                "eks:AssociateAccessPolicy",
                "eks:CreateAccessEntry",
                "eks:CreateAddon",
                "eks:CreateCluster",
                "eks:CreateNodegroup",
                "eks:DeleteAccessEntry",
                "eks:DeleteAddon",
                "eks:DeleteCluster",
                "eks:DeleteNodegroup",
                "eks:DescribeAccessEntry",
                "eks:DescribeAddon",
                "eks:DescribeAddonVersions",
                "eks:DescribeCluster",
                "eks:DescribeNodegroup",
                "eks:DisassociateAccessPolicy",
                "eks:ListAssociatedAccessPolicies",
                "eks:ListNodegroups",
                "eks:TagResource",
                "eks:UpdateNodegroupConfig",
                "eks:UpdateNodegroupVersion",
                "iam:AttachRolePolicy",
                "iam:CreateOpenIDConnectProvider",
                "iam:CreateRole",
                "iam:DeleteOpenIDConnectProvider",
                "iam:DeleteRole",
                "iam:DetachRolePolicy",
                "iam:GetOpenIDConnectProvider",
                "iam:GetRole",
                "iam:ListAttachedRolePolicies",
                "iam:ListInstanceProfilesForRole",
                "iam:ListRolePolicies",
                "iam:TagOpenIDConnectProvider",
                "iam:TagRole",
                "ssm:GetParameters"
            ],
            "Resource": "*"
        },
        {
            "Sid": "clusterperms",
            "Effect": "Allow",
            "Action": [
                "iam:PassRole"
            ],
            "Resource": [
                "arn:aws:iam::<account-id>:role/flux-test-*",
                "arn:aws:iam::<account-id>:role/blue-eks-node-group-*",
                "arn:aws:iam::<account-id>:role/green-eks-node-group-*"
            ]
        }
    ]
}
```

#### IAM and CI Setup

To create all the necessary IAM role and policy with all the permissions, set up
CI secrets and variables using
[aws-gh-actions](https://github.com/fluxcd/test-infra/tree/main/tf-modules/aws/github-actions)
with the terraform configuration below. Please make sure all the requirements of
aws-gh-actions are followed before running it, especially registering GitHub
OIDC as an identity provider in the AWS account.

**NOTE:** When running the following for a repo under an organization, set the
environment variable `GITHUB_ORGANIZATION` if setting the `owner` in the
`github` provider doesn't work.

```hcl
module "aws_gh_actions" {
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/aws/github-actions"

  aws_region             = "us-east-2"
  aws_policy_name        = "oci-e2e"
  aws_policy_description = "policy for OCI e2e tests"
  aws_provision_perms = [
    "ec2:AllocateAddress",
    "ec2:AssociateRouteTable",
    "ec2:AttachInternetGateway",
    "ec2:AuthorizeSecurityGroupEgress",
    "ec2:AuthorizeSecurityGroupIngress",
    "ec2:CreateInternetGateway",
    "ec2:CreateLaunchTemplate",
    "ec2:CreateLaunchTemplateVersion",
    "ec2:CreateNatGateway",
    "ec2:CreateNetworkAcl",
    "ec2:CreateNetworkAclEntry",
    "ec2:CreateRoute",
    "ec2:CreateRouteTable",
    "ec2:CreateSecurityGroup",
    "ec2:CreateSubnet",
    "ec2:CreateTags",
    "ec2:CreateVpc",
    "ec2:DeleteInternetGateway",
    "ec2:DeleteLaunchTemplate",
    "ec2:DeleteNatGateway",
    "ec2:DeleteNetworkAcl",
    "ec2:DeleteNetworkAclEntry",
    "ec2:DeleteRoute",
    "ec2:DeleteRouteTable",
    "ec2:DeleteSecurityGroup",
    "ec2:DeleteSubnet",
    "ec2:DeleteTags",
    "ec2:DeleteVolume",
    "ec2:DeleteVpc",
    "ec2:DescribeAddresses",
    "ec2:DescribeAddressesAttribute",
    "ec2:DescribeAvailabilityZones",
    "ec2:DescribeInternetGateways",
    "ec2:DescribeLaunchTemplates",
    "ec2:DescribeLaunchTemplateVersions",
    "ec2:DescribeNatGateways",
    "ec2:DescribeNetworkAcls",
    "ec2:DescribeNetworkInterfaces",
    "ec2:DescribeRouteTables",
    "ec2:DescribeSecurityGroupRules",
    "ec2:DescribeSecurityGroups",
    "ec2:DescribeSubnets",
    "ec2:DescribeTags",
    "ec2:DescribeVpcAttribute",
    "ec2:DescribeVpcs",
    "ec2:DetachInternetGateway",
    "ec2:DisassociateAddress",
    "ec2:DisassociateRouteTable",
    "ec2:ModifyVpcAttribute",
    "ec2:ReleaseAddress",
    "ec2:RevokeSecurityGroupEgress",
    "ec2:RevokeSecurityGroupIngress",
    "ec2:RunInstances",
    "ecr:BatchGetImage",
    "ecr:BatchCheckLayerAvailability",
    "ecr:CreateRepository",
    "ecr:CompleteLayerUpload",
    "ecr:DeleteRepository",
    "ecr:DescribeRepositories",
    "ecr:GetAuthorizationToken",
    "ecr:InitiateLayerUpload",
    "ecr:ListTagsForResource",
    "ecr:PutImage",
    "ecr:TagResource",
    "ecr:UploadLayerPart",
    "eks:AssociateAccessPolicy",
    "eks:CreateAccessEntry",
    "eks:CreateAddon",
    "eks:CreateCluster",
    "eks:CreateNodegroup",
    "eks:DeleteAccessEntry",
    "eks:DeleteAddon",
    "eks:DeleteCluster",
    "eks:DeleteNodegroup",
    "eks:DescribeAccessEntry",
    "eks:DescribeAddon",
    "eks:DescribeAddonVersions",
    "eks:DescribeCluster",
    "eks:DescribeNodegroup",
    "eks:DisassociateAccessPolicy",
    "eks:ListAssociatedAccessPolicies",
    "eks:ListNodegroups",
    "eks:TagResource",
    "eks:UpdateNodegroupConfig",
    "eks:UpdateNodegroupVersion",
    "iam:AttachRolePolicy",
    "iam:CreateOpenIDConnectProvider",
    "iam:CreateRole",
    "iam:DeleteOpenIDConnectProvider",
    "iam:DeleteRole",
    "iam:DetachRolePolicy",
    "iam:GetOpenIDConnectProvider",
    "iam:GetRole",
    "iam:ListAttachedRolePolicies",
    "iam:ListInstanceProfilesForRole",
    "iam:ListRolePolicies",
    "iam:TagOpenIDConnectProvider",
    "iam:TagRole",
    "ssm:GetParameters"
  ]
  aws_cluster_role_prefix = [
    "flux-test-",
    "blue-eks-node-group-",
    "green-eks-node-group-"
  ]
  aws_role_name          = "oci-e2e"
  aws_role_description   = "role to assume in OCI e2e test"
  github_repo_owner      = "fluxcd"
  github_project         = "pkg"
  github_repo_branch_ref = "*"

  github_secret_assume_role_name = "OCI_E2E_AWS_ASSUME_ROLE_NAME"

  github_variable_custom = {
    "OCI_E2E_TF_VAR_cross_region" = "us-east-1"
  }
}
```

**NOTE:** Change the various names and environment variables above as necessary.

### Microsoft Azure

- Azure account with an active subscription to be able to create AKS and ACR,
    and permission to assign roles. Role assignment is required for allowing AKS
    workloads to access ACR.
- Azure CLI, need to be logged in using `az login` as a User (not a Service
  Principal).
- An Azure DevOps organization [connected to Microsoft
  Entra](https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/connect-organization-to-azure-ad?view=azure-devops),
  personal access token for accessing repositories within the organization. The
  scope required for the personal access token is:
  - Project and Team - read, write and manage access
  - Member Entitlement Management (Read & Write)
  - Code - Full
  - Please take a look at the [terraform
    provider](https://registry.terraform.io/providers/microsoft/azuredevops/latest/docs/guides/authenticating_using_the_personal_access_token#create-a-personal-access-token)
    for more explanation.
  - A valid Azure devops configuration is needed even if git is not being
    tested. 

  **NOTE:** To use Service Principal (for example in CI environment), set the
  `ARM-*` variables in `.env`, source it and authenticate Azure CLI with:
  ```console
  $ az login --service-principal -u $ARM_CLIENT_ID -p $ARM_CLIENT_SECRET --tenant $ARM_TENANT_ID
  ```
  In this case, the AzureRM client in terraform uses the Service Principal to
  authenticate and the Azure CLI is used only for authenticating with ACR for
  logging in and pushing container images. Attempting to authenticate terraform
  using Azure CLI with Service Principal results in the following error:
  > Authenticating using the Azure CLI is only supported as a User (not a Service Principal).
- Docker CLI for registry login.
- kubectl for applying certain install manifests.

#### Permissions

Following permissions are needed for provisioning the infrastructure and running
the tests:
- `Microsoft.Kubernetes/*`
- `Microsoft.Resources/*`
- `Microsoft.Authorization/roleAssignments/{read,write,delete}`
- `Microsoft.ContainerRegistry/*`
- `Microsoft.ContainerService/*`

Additional permissions needed when Workload Identity is enabled:

- `Microsoft.ManagedIdentity/userAssignedIdentities/{read,write,delete}`
- `Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials/{read,write,delete}`

#### IAM and CI setup

To create the necessary IAM role with all the permissions, set up CI secrets and
variables using
[azure-gh-actions](https://github.com/fluxcd/test-infra/tree/main/tf-modules/azure/github-actions)
use the terraform configuration below. Please make sure all the requirements of
azure-gh-actions are followed before running it.

**NOTE:** When running the following for a repo under an organization, set the
environment variable `GITHUB_ORGANIZATION` if setting the `owner` in the
`github` provider doesn't work.

```hcl
provider "github" {
  owner = "fluxcd"
}

module "azure_gh_actions" {
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/azure/github-actions"

  azure_owners          = ["owner-id-1", "owner-id-2"]
  azure_app_name        = "pkg-oci-e2e"
  azure_app_description = "pkg oci e2e"
  azure_permissions = [
    "Microsoft.Kubernetes/*",
    "Microsoft.Resources/*",
    "Microsoft.Authorization/roleAssignments/read",
    "Microsoft.Authorization/roleAssignments/write",
    "Microsoft.Authorization/roleAssignments/delete",
    "Microsoft.ContainerRegistry/*",
    "Microsoft.ContainerService/*",
    "Microsoft.ManagedIdentity/userAssignedIdentities/read",
    "Microsoft.ManagedIdentity/userAssignedIdentities/write",
    "Microsoft.ManagedIdentity/userAssignedIdentities/delete",
    "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials/read",
    "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials/write",
    "Microsoft.ManagedIdentity/userAssignedIdentities/federatedIdentityCredentials/delete"
  ]
  azure_location = "eastus"

  github_project = "pkg"

  github_secret_client_id_name       = "OCI_E2E_AZ_ARM_CLIENT_ID"
  github_secret_client_secret_name   = "OCI_E2E_AZ_ARM_CLIENT_SECRET"
  github_secret_subscription_id_name = "OCI_E2E_AZ_ARM_SUBSCRIPTION_ID"
  github_secret_tenant_id_name       = "OCI_E2E_AZ_ARM_TENANT_ID"
}
```

**NOTE:** The environment variables used above are for the GitHub workflow that
runs the tests. Change the variable names if needed accordingly.

### Google Cloud Platform

- GCP account with project and GKE, GCR and Artifact Registry services enabled
    in the project.
- gcloud CLI, need to be logged in using `gcloud auth login` as a User (not a
  Service Account), configure application default credentials with `gcloud auth
  application-default login` and docker credential helper with `gcloud auth configure-docker`.

  **NOTE:** To use Service Account (for example in CI environment), set
  `GOOGLE_APPLICATION_CREDENTIALS` variable in `.env` with the path to the JSON
  key file, source it and authenticate gcloud CLI with:
  ```console
  $ gcloud auth activate-service-account --key-file=$GOOGLE_APPLICATION_CREDENTIALS
  ```
  Depending on the Container/Artifact Registry host used in the test, authenticate
  docker accordingly
  ```console
  $ gcloud auth print-access-token | docker login -u oauth2accesstoken --password-stdin https://us-central1-docker.pkg.dev
  $ gcloud auth print-access-token | docker login -u oauth2accesstoken --password-stdin https://gcr.io
  ```
  In this case, the GCP client in terraform uses the Service Account to
  authenticate and the gcloud CLI is used only to authenticate with Google
  Container Registry and Google Artifact Registry.

  **NOTE FOR CI USAGE:** When saving the JSON key file as a CI secret, compress
  the file content with
  ```console
  $ cat key.json | jq -r tostring
  ```
  to prevent aggressive masking in the logs. Refer
  [aggressive replacement in logs](https://github.com/google-github-actions/auth/blob/v1.1.0/docs/TROUBLESHOOTING.md#aggressive--replacement-in-logs)
  for more details.
- Docker CLI for registry login.
- kubectl for applying certain install manifests.

**NOTE:** Unlike ECR, AKS and Google Artifact Registry, Google Container
Registry tests don't create a new registry. It pushes to an existing registry
host in a project, for example `gcr.io`. Due to this, the test images pushed to
GCR aren't cleaned up automatically at the end of the test and have to be
deleted manually. [`gcrgc`](https://github.com/graillus/gcrgc) can be used to
automatically delete all the GCR images.
```console
$ gcrgc gcr.io/<project-name>
```

#### Permissions

Following roles are needed for provisioning the infrastructure and running the
tests:
- Artifact Registry Administrator - `roles/artifactregistry.admin`
- Compute Instance Admin (v1) - `roles/compute.instanceAdmin.v1`
- Compute Storage Admin - `roles/compute.storageAdmin`
- Kubernetes Engine Admin - `roles/container.admin`
- Service Account Admin - `roles/iam.serviceAccountAdmin`
- Service Account Token Creator - `roles/iam.serviceAccountTokenCreator`
- Service Account User - `roles/iam.serviceAccountUser`
- Storage Admin - `roles/storage.admin`

If workload identity is enabled, the following role is also needed:

- Project IAM Admin - `roles/resourcemanager.projectIamAdmin`

#### IAM and CI setup

To create the necessary IAM role with all the permissions, set up CI secrets and
variables using
[gcp-gh-actions](https://github.com/fluxcd/test-infra/tree/main/tf-modules/gcp/github-actions)
use the terraform configuration below. Please make sure all the requirements of
gcp-gh-actions are followed before running it.

**NOTE:** When running the following for a repo under an organization, set the
environment variable `GITHUB_ORGANIZATION` if setting the `owner` in the
`github` provider doesn't work.

```hcl
provider "google" {}

provider "github" {
  owner = "fluxcd"
}

module "gcp_gh_actions" {
  source = "git::https://github.com/fluxcd/test-infra.git//tf-modules/gcp/github-actions"

  gcp_service_account_id   = "pkg-oci-e2e"
  gcp_service_account_name = "pkg-oci-e2e"
  gcp_roles = [
    "roles/artifactregistry.admin",
    "roles/compute.instanceAdmin.v1",
    "roles/compute.storageAdmin",
    "roles/container.admin",
    "roles/iam.serviceAccountAdmin",
    "roles/iam.serviceAccountTokenCreator",
    "roles/iam.serviceAccountUser",
    "roles/storage.admin",
    "roles/resourcemanager.projectIamAdmin"
  ]

  github_project = "pkg"

  github_secret_credentials_name = "OCI_E2E_GOOGLE_CREDENTIALS"
}
```

**NOTE:** The environment variables used above are for the GitHub workflow that
runs the tests. Change the variable names if needed accordingly.

## Test setup

Copy `.env.sample` to `.env`, put the respective provider configurations in the
environment variables and source it, `source .env`.

Ensure the test app container image is built and ready for testing. Test app
container image can be built with make target `docker-build`.

Run the test with `make test-*`, setting the test app image with variable
`TEST_IMG`. By default, the default test app container image,
`fluxcd/testapp:test`, will be used.

```console
$ make test-azure
make test PROVIDER_ARG="-provider azure"
docker image inspect fluxcd/testapp:test >/dev/null
TEST_IMG=fluxcd/testapp:test go test -timeout 30m -v ./ -run "^.*" -provider azure --tags=integration
2024/08/26 23:39:13 Terraform binary:  /snap/bin/terraform
2024/08/26 23:39:13 Init Terraform
2024/08/26 23:39:15 Applying Terraform
...
```

If not configured explicitly to retain the infrastructure, at the end of the
test, the test infrastructure is deleted. In case of any failure due to which
the resources don't get deleted, the `make destroy-*` commands can be run for
the respective provider. This will run terraform destroy in the respective
provider's terraform configuration directory. This can be used to quickly
destroy the infrastructure without going through the provision-test-destroy
steps. There is a known issue with Azure user not getting cleaned up if the
infrastructure is retained and destroy is used for cleanup. The workaround is to
manually delete the user from Azure DevOps Organization
Settings->Users page. 

## Workload Identity

By default, the tests use node identity for authentication. To run the integration tests on clusters with workload identity
enabled for any of the providers. The following terraform variables need to be set.

```shell
export TF_VAR_wi_k8s_sa_name=
export TF_VAR_wi_k8s_sa_ns=
export TF_VAR_enable_wi=
```

They have been included in the `.env.sample` and you can simply uncomment it.

The git integration tests require workload identity to be enabled.

## Debugging the tests

For debugging environment provisioning, enable verbose output with `-verbose`
test flag.

```console
$ make test-aws GO_TEST_ARGS="-verbose"
```

The test environment is destroyed at the end by default. Run the tests with
`-retain` flag to retain the created test infrastructure.

```console
$ make test-aws GO_TEST_ARGS="-retain"
```

The tests require the infrastructure state to be clean. For re-running the tests
with a retained infrastructure, set `-existing` flag.

```console
$ make test-aws GO_TEST_ARGS="-retain -existing"
```

To delete an existing infrastructure created with `-retain` flag:

```console
$ make test-aws GO_TEST_ARGS="-existing"
```
