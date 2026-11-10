# AWS production deployment

**Deployment implementation, not a production certification.** Terraform validation and local tests cannot prove live AWS/IAM/OIDC behavior. Do not admit employees or sensitive data until the acceptance checklist below passes in your account. No AWS resources are created by local development or by CI validation.

## LLM-assisted setup

Start with [docs/SETUP.md](docs/SETUP.md) for the operator instructions, required inputs, approval gates, deployment sequence and handoff format. Its companion references cover [detailed IAM permissions and policy examples](docs/IAM.md) and [exact security groups, routing, endpoints and DNS rules](docs/NETWORK.md).

**Known pre-deployment blocker:** `network.tf` currently lacks ALB outbound HTTPS for Cognito's authentication endpoints. Review and approve the [required correction](docs/NETWORK.md#required-correction-alb-to-cognito-https) before deployment; these docs do not apply it or authorize cloud changes.

## Architecture

- **EC2:** trusted Next.js frontend, Go API, gateway, and controller worker, using immutable platform images from **ECR**. Private subnet, no public IP or SSH, administration through SSM. Only the ALB can reach ports 42069/8081. The API has no host port. No production container receives the Docker socket.
- **Authentication:** HTTPS ALB → Cognito → **Microsoft Entra ID / OneLogin**. Both federated providers are configured. The backend verifies ES256 ALB claim signatures, expiry, signer ARN, issuer and client. Users bind by immutable issuer + subject, never by matching email. Self-service local passwords are not enabled. Enforce MFA and app assignment in both company identity providers.
- **RDS PostgreSQL 17:** separate private, encrypted, Multi-AZ instances for SG-1 control data and generated-application data. Verified TLS with the bundled official RDS CA. The runtime control-plane role is not a database administrator. Only the worker receives the application-database provisioner credential. Project production/preview databases and roles are distinct.
- **S3:** private, encrypted, versioned bucket; uploads are authorized by project membership and limited to 10 MiB per file. Uploads use `uploads/`. GuardDuty Malware Protection scans them; both the application and bucket policy deny download until a clean scan tag exists. Files are attachments, not public assets or automatically included in AI prompts. Build sources and release artifacts use separate prefixes.
- **Generated builds:** dedicated CodeBuild project and boundary-constrained IAM role per project. Each build role can read only its project's build-source prefix and push only its project's immutable ECR repository. No provider keys or database credentials enter builds. A completed ECR scan with zero HIGH/CRITICAL findings is required before deployment.
- **Generated runtime:** private ECS/Fargate services, no public IP or task role, read-only root, non-root application user, dropped capabilities, and a writable ephemeral `/tmp` prepared by a trusted init container. The execution role injects only the application's database secret. Task egress is limited to the application RDS instance, private AWS endpoints and S3 image-layer storage. A restricted S3 endpoint policy blocks public-bucket exfiltration; a fail-closed Resolver DNS Firewall limits DNS destinations. Tasks cannot initiate connections to SG-1, its RDS instance, or other apps. The trusted gateway reaches apps through private Cloud Map DNS; public/password access remains enforced there. Employee routes require ALB sign-in, while `/{project-slug}` viewers and static assets remain public so visitor access modes still work.
- Candidate HTTP health is checked through the gateway before publication. Rollback preserves databases. Build cancellation requests cloud cleanup; incomplete cleanup keeps the project's build slot occupied and is retried durably. The AWS workspace has a hard limit of 30 non-archived projects; one worker and one active build per project limit concurrency.

This is a **single-EC2 control plane**, not an HA control plane. RDS is Multi-AZ; an EC2 failure still requires recovery/redeployment. Workload/source/history retention and complete project deletion require an operator-reviewed lifecycle process. Do not use broad Docker/AWS deletion commands. Preserve images referenced by deployment history.

## Prerequisites

1. An approved AWS commercial region/account and a dedicated VPC with DNS support/hostnames enabled.
2. Two public ALB subnets and at least two private subnets in distinct AZs. Private control-plane routing must provide HTTPS egress through NAT for identity key retrieval, AI providers, ECR login and initial package installation. Supply the private route-table IDs for the S3 gateway endpoint. Import/reuse pre-existing endpoints instead of creating conflicting private-DNS endpoints.
3. Route 53 hosted zones for the platform and a **different registrable application domain**, for example `sg1.example.com` and `*.apps.example-apps.com`. Sibling subdomains under one registrable domain are rejected in production to prevent parent-domain cookie attacks from generated JavaScript. Set both hosted-zone IDs. ACM and DNS records are provisioned here.
4. An **existing encrypted, versioned Terraform state bucket** and restricted state-access role. State contains Cognito/IdP client secrets even though Terraform masks them in output. Use native S3 state locking. Never commit state, plans, tfvars, deployment JSON, or secrets.
5. Entra tenant-specific and OneLogin OIDC clients with app assignment and MFA enforced. Do not use Entra `common`/`organizations` issuer endpoints.
6. Secrets Manager entries for each IdP containing `{"issuer":"https://...","client_id":"...","client_secret":"..."}`. Configure their redirect URI as `https://<name>-<account>.auth.<region>.amazoncognito.com/oauth2/idpresponse`. Cognito's separate ALB callback is `https://<platform-domain>/oauth2/idpresponse` and is configured by Terraform.
7. Docker Buildx, AWS CLI v2, jq, Terraform >=1.10, and reviewed deployment permissions. ECR **basic scanning** must be enabled for the generated repositories; an account using enhanced scanning needs an Inspector-aware policy implementation before this runtime can be accepted. Unsupported/unavailable scans fail closed.
8. Review cost/quotas: two Multi-AZ RDS instances, ALB, EC2, NAT, interface endpoints, Cognito Plus, GuardDuty Malware Protection, Resolver DNS Firewall, CloudWatch, ECR, per-project CodeBuild, and Fargate services incur charges. Configure AWS Budgets and operational notifications before apply. The DNS firewall is VPC-wide; use a dedicated VPC and review approved domains before adding integrations.

## Provision infrastructure (operator action)

```sh
cp infrastructure/aws/terraform.tfvars.example infrastructure/aws/production.tfvars
cp infrastructure/aws/backend.hcl.example infrastructure/aws/backend.hcl
# Fill your region, VPC/subnets/routes, DNS names and IdP secret ARNs.
terraform -chdir=infrastructure/aws init -backend-config=backend.hcl
terraform -chdir=infrastructure/aws fmt -check
terraform -chdir=infrastructure/aws validate
terraform -chdir=infrastructure/aws plan -var-file=production.tfvars -out=production.tfplan
# Review IAM, network boundaries, cost, and deletion protection before applying.
terraform -chdir=infrastructure/aws apply production.tfplan
terraform -chdir=infrastructure/aws output -json deployment > infrastructure/aws/deployment.json
```

Terraform provisions existing-VPC integration; it does not create your VPC/NAT or configure your Entra/OneLogin tenants. IdP issuer/client-secret validity, DNS ownership, AWS service quotas and permission propagation require real-account verification. Existing ECS service-linked roles can be reused; the controller may create only the ECS service-linked role if it is missing.

## Configure secrets and the first administrator

After infrastructure exists, initialize the application settings secret ARN from `deployment.json` using a protected file and `aws secretsmanager put-secret-value --secret-id <ARN> --secret-string file://<private-file>`. Use `scripts/aws/settings.json.example` as the structure, not as actual secret values.

- Generate `SG1_CREDENTIALS_KEY` once with `openssl rand -base64 32`. Back it up separately. **Never rotate it during deployment:** existing provider/project credentials would become unreadable.
- Set `SG1_BOOTSTRAP_SUBJECTS` to the exact Cognito `sub` of an approved initial administrator (comma-separated for more than one). Complete company federation once and retrieve that user's immutable subject from Cognito using an administrator account. The ALB can complete the federation flow before the application target is available; its final target response will be unavailable until bootstrapping is complete. Do not guess the subject or use an email address.
- The bootstrap flag applies only when an identity is first created in SG-1; it does not silently re-elevate a previously deactivated/demoted employee.
- Newly federated employees get their own SG-1 identity and **no department membership by default**. An operator must provision memberships deliberately. Membership-management UI is not implemented yet; treat controlled onboarding/offboarding as a release prerequisite. Disable the SG-1 user to revoke backend access; IdP offboarding alone may leave an already issued ALB session valid until its expiry.

RDS administrator passwords are managed by RDS/Secrets Manager. `configure.py` generates and persists a separate runtime database password only if absent. It applies runtime table grants after migrations; the API does not run as the RDS master. Only migration execution temporarily receives the control master password. Generated apps have no access to either administrator credential.

## Publish and install a release

Commit reviewed code first; publishing refuses a dirty checkout so the revision matches its artifacts.

```sh
bash scripts/aws/check.sh infrastructure/aws/deployment.json
bash scripts/aws/publish.sh infrastructure/aws/deployment.json
```

The publisher builds Linux/amd64 platform images, pushes/reuses immutable commit tags, requires completed clean ECR basic scans, resolves ECR digests, and uploads a source-free release bundle containing the Compose file, public RDS CA, configuration tools, non-secret Terraform outputs, and digest-pinned image references. It never uploads `.env`, provider keys, or Git credentials. It prints a SHA-256 checksum; record and verify it before extracting the bundle.

Use **SSM Session Manager**, not a public SSH port:

```sh
aws ssm start-session --region '<REGION>' --target '<SG1_INSTANCE_ID>'
sudo -i
cd /opt/sg1
aws s3 cp --region '<REGION>' 's3://<BUCKET>/releases/<COMMIT>/release.tar.gz' release.tar.gz
# Verify this bundle against your reviewed release artifact/checksum before extracting.
tar --no-same-owner -xzf release.tar.gz
bash boot.sh
```

The host bootstrap installs Docker/Compose. `boot.sh` fetches secrets through the instance role, writes root-only mode-0600 environment files, pulls digest-pinned images, applies migrations, grants the runtime role, removes the temporary migration credential file, and waits for service health. Environment files must stay private and never be copied into Git or build artifacts. Root access to the trusted EC2 host is privileged access to control-plane secrets.

The public listener is **HTTPS 443**. Local development remains `http://localhost:42069`; port 42069 on EC2 is reachable only from the ALB. Do not expose ports 8080, 8081, 42069, PostgreSQL, SSM agents, or container runtime sockets directly to the internet.

## Mandatory acceptance checklist — do not skip

Local tests/`terraform validate` are not substitutes for these checks:

- [ ] `check.sh` passes under the correct account/region; all RDS/S3 resources are private/encrypted and deletion-protected where applicable. GuardDuty protection plan status is ACTIVE and tagging is enabled.
- [ ] Both Entra ID and OneLogin complete federation with the approved tenant, MFA and app assignment. Unassigned users cannot enter. Forged/expired/wrong-ALB claims, unsigned email headers and the development token are rejected.
- [ ] Initial admin has intended permissions; a new employee sees no projects until membership is granted. Test read/write/admin roles and immediate SG-1 user deactivation.
- [ ] HTTPS and wildcard app certificate/DNS work; every app has a distinct origin. Authenticated preview, password-protected/public viewers, WebSockets and direct app-address protection work.
- [ ] Upload a benign file through SG-1. It is private, initially quarantined, becomes downloadable only after a clean GuardDuty tag, and preserves its contents. Wrong-project/read-only upload/download attempts fail. Confirm a test malware sample in a disposable security-test project cannot be downloaded. Never use real malware or production data for testing.
- [ ] ECR basic scans complete for both platform images and generated images; HIGH/CRITICAL findings block launch. Verify the trusted init image, RDS CA and the actual Fargate task definition: no task IAM role, non-root app, read-only root, only its database secret, private networking, and writable `/tmp`.
- [ ] Create a disposable project in AWS; real CodeBuild build, ECR push, scan, private Fargate start, independent HTTP health and publication succeed. Failed build/start/scan leaves the old app running. Rollback preserves database records.
- [ ] Cancel during generation, CodeBuild and Fargate validation. Confirm no publication, completed cloud cleanup, occupied build slot until cleanup finishes, and recovery after controller restart. Exercise permission propagation, resource-creation partial failures and orphan reconciliation; inspect CloudWatch and cloud resources for leaks.
- [ ] Generated code cannot reach EC2/IMDS, the control RDS instance, other app tasks, provider keys, S3 uploads, or another project's build source/ECR write scope. Confirm the restricted S3 endpoint blocks third-party buckets, DNS Firewall blocks unapproved domains/encoded DNS exfiltration, and required provider/AWS DNS names still resolve. Review IAM Access Analyzer and security-group rules in the deployed account.
- [ ] Check RDS connection counts/storage and the 30-project cap. Configure budgets, alarms/notifications, log retention, incident ownership, image/source/workload cleanup and employee offboarding.
- [ ] Restore RDS and S3 versions in a recovery rehearsal, redeploy EC2 from the release, and restore the encryption key. Document RPO/RTO and accept the single-EC2 availability limitation.

**Do not label the deployment production-ready until these real-account checks and security review are signed off.** The implementation deliberately fails closed for missing production configuration, unverified identity, unclean/unscanned uploads and unsafe image scans.

ECR basic scans are OS-package vulnerability checks, not a proof that arbitrary generated code is benign or that all language dependencies are safe. Review the generated-code/base-image policy and add the required language-specific SCA for your accepted stacks. Platform CI runs `govulncheck` and npm production dependency auditing; the Go toolchain is pinned to the patched 1.26.10 release or newer within the 1.26 line.
