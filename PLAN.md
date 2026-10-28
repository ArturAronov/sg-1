# SG-1 — Product and Implementation Plan

Status: local foundation implemented; production integrations and AWS provisioning are not implemented yet.

This document records the agreed requirements, recommended architecture, implementation sequence, and decisions still needed. Items marked **proposed** are recommendations, not requirements already approved by stakeholders.

### Implementation progress

- [x] Next.js 16 / TypeScript dashboard and split-view builder shell.
- [x] Go API, PostgreSQL connection handling, migrations, and development seed command.
- [x] Persistent control-plane identities, departments, roles, projects, locked access defaults, audit events, and provisioning outbox.
- [x] Authorized project listing/details and transactional, idempotent project creation with globally unique slugs.
- [x] Explicitly development-only authentication; production access fails closed until SSO is implemented.
- [x] Local Docker Compose stack and non-root application images.
- [x] Unit/integration tests and CI definitions; browser smoke-test coverage.
- [ ] Entra ID / OneLogin and membership-management UI/API.
- [ ] AWS infrastructure and isolated project provisioning; no generated project containers run yet.
- [ ] Isolated application gateway, password enforcement, and public serving.
- [ ] S3 uploads and persistent per-project databases.
- [ ] AI orchestration, validated builds, deployment, and rollback.
- [ ] Production hardening, recovery rehearsal, and pilot approval.

The current create-project flow reserves metadata and immediately queues provisioning. It does not yet satisfy the final requirement to start a project container; the UI explicitly reports this missing integration. See README.md for runnable local setup and current limitations.

## 1. Product goal

SG-1 is an internal AI application builder for a non-technical real-estate company. Employees describe the tools they need in chat, see the application in a split-screen preview, and publish working applications without managing infrastructure themselves.

The builder is internal. Published applications can be public on the internet or protected by a project password. Because the company handles substantial sensitive information, access control, project isolation, persistent storage, and auditability are foundational requirements rather than later enhancements.

Initial audience: approximately 20–50 users. The expected number of projects and simultaneous builds is still to be established.

## 2. Confirmed requirements

### 2.1 SG-1 platform

- Frontend: TypeScript and Next.js 16.
- Backend: Go.
- Hosting: AWS.
- Authentication: support both Microsoft Entra ID and OneLogin.
- Department directors are department administrators.
- Administrators can add users and assign read, write, and admin access.
- Main workspace: chat beside a preview of the generated application.
- Source control: one private GitHub monorepo containing SG-1 and all generated projects.
- Generated projects live under `projects/{department-name}/{project-name}`.
- Users can upload files to S3.
- Projects have persistent PostgreSQL data independent of application containers.
- Container images are stored in ECR.
- Published application containers run on ECS Fargate.
- Hosting SG-1 itself on ECS versus EC2 was not originally decided. ECS Fargate is the current recommendation.

### 2.2 Generated applications

- Users may choose any application stack; Next.js and Go are requirements for SG-1, not for generated applications.
- Every project has its own application container workload.
- Creating a project starts a container before the user enters a prompt.
- The first container serves a standard placeholder because no application stack has been chosen yet.
- AI-generated changes must pass a Docker build before they can be deployed.
- A candidate must also start successfully and pass health checks.
- Successful changes publish automatically; no administrator approval is required for routine publishing by authorized editors.
- Failed builds or unhealthy candidates do not replace the working deployment.
- Application data and files survive container restarts and replacements.
- Previous working versions remain available for rollback.

### 2.3 URLs and application access

For a project named `maintenance-tracker`:

| URL | Purpose |
| --- | --- |
| `https://sg-1.ourcompany.net/build/maintenance-tracker` | Authenticated builder, chat, preview, and deployment controls |
| `https://sg-1.ourcompany.net/maintenance-tracker` | Published application viewer |

- The viewer embeds the application from an isolated application origin while retaining the requested viewer URL in the address bar.
- The build workspace always requires employee SSO and project authorization.
- Published applications have two agreed access modes: public and password-protected.
- Public applications are accessible to anyone on the internet without signing in.
- Password-protected applications are accessible to anyone with the project password; this is not equivalent to employee identity or per-user authorization.
- New projects default to password-protected.
- Making an application public requires an explicit action.
- Deployments preserve the existing access setting.
- Access protection covers the application, APIs, WebSockets, and underlying application address, not just the viewer page.
- Build previews remain employee-only, even for a public project.
- S3 storage remains private. Application access does not make an entire bucket or database public.

## 3. Architectural principles

1. **Separate the control plane from generated applications.** Generated code cannot access SG-1's database, credentials, source-management credentials, or deployment privileges.
2. **Separate builds from running applications.** The AI does not edit files inside the currently published container.
3. **Treat generated code and uploaded files as untrusted.** Containerization alone is not a complete security boundary.
4. **Deploy immutable images.** Record image digests, source commits, validation results, and deployment events.
5. **Keep durable state outside containers.** Use RDS for databases and S3 for files and artifacts.
6. **Preserve availability during failed changes.** A new candidate replaces the active version only after validation.
7. **Enforce permissions on the server.** UI visibility is not authorization.
8. **Use a constrained runtime contract, not a constrained programming language.** Any stack must satisfy the platform's deployment and embedding requirements.
9. **Do not equate code editing with business-data permission.** Builder roles and authorization inside an application are different concerns.
10. **Make sensitive operations visible.** Audit membership changes, access changes, deployments, credential changes, and destructive operations.

## 4. Recommended system architecture

```text
Employees
  |
  +--> SG-1 Next.js frontend
         |
         +--> Go control-plane API --> Control-plane PostgreSQL
         |          |
         |          +--> Entra ID / OneLogin
         |          +--> Private S3 uploads and artifacts
         |          +--> Durable job queue --> Go orchestration workers
         |                                      |
         |                                      +--> AI model provider
         |                                      +--> Isolated build workspace
         |                                      +--> AWS CodeBuild
         |                                      +--> GitHub integration
         |                                      +--> ECR / ECS deployment operations
         |
         +--> Authenticated preview gateway --> Candidate application tasks

Internet visitors
  |
  +--> SG-1 project viewer
         |
         +--> Isolated application origin / access gateway
                    |
                    +--> Public access or project-password check
                    +--> Private ECS Fargate application service
                                   |
                                   +--> Project PostgreSQL database
                                   +--> Authorized private S3 access
```

### 4.1 AWS components

| Component | Recommended service | Notes |
| --- | --- | --- |
| SG-1 frontend | ECS Fargate | Next.js service behind managed ingress |
| SG-1 backend | ECS Fargate | Go API service |
| Background orchestration | ECS Fargate workers | Separate from request-serving API |
| Job queue | SQS | Durable delivery; workers must tolerate duplicates |
| Container builds | CodeBuild | Docker-capable isolated build execution |
| Image registry | ECR | Immutable image references and vulnerability scanning |
| Published applications | ECS Fargate | Independently deployed service per project initially |
| Candidate previews | ECS Fargate | Temporary tasks/services, separately authenticated |
| Databases | RDS PostgreSQL | Separate control-plane and project credentials |
| Files and artifacts | S3 | Private, encrypted, lifecycle-managed storage |
| Secrets | Secrets Manager | Runtime injection; never committed to GitHub |
| DNS and TLS | Route 53 / ACM, or existing company equivalents | SG-1 and isolated application domains |
| Logs and alarms | CloudWatch | Redaction, retention limits, operational alerts |

ECS Fargate is recommended for SG-1 itself to avoid maintaining EC2 hosts. This is an architectural recommendation that still needs formal confirmation.

Fargate application tasks must not be used for privileged Docker-in-Docker builds. Use CodeBuild or another purpose-built sandbox for Docker builds.

### 4.2 Build workspace design

For the MVP, use a project-scoped CodeBuild job as the isolated execution environment for an AI build attempt. The Go orchestrator runs the agent loop, while sandbox commands execute inside the build environment through a narrowly scoped job protocol. CodeBuild performs the final Docker build.

Validate this command-execution and event-streaming approach in an early technical spike. If CodeBuild startup latency or its long-running command model is unsuitable, use a dedicated isolated sandbox service for editing and checks, retaining CodeBuild for image builds. Do not move untrusted command execution into the Go API service as a shortcut.

Each workspace starts from a known source revision, has a bounded lifetime, and is discarded after its outputs have been persisted. Persist source changes, logs, and job status outside the workspace so a worker or sandbox restart does not lose the project.

## 5. Repository design

- GitHub repository: `https://github.com/ArturAronov/sg-1.git`.
- Root/default branch: `master`.
- Automatic commits and pushes of completed, task-scoped changes are authorized. Do not force-push or overwrite unrelated work.

```text
/
├── PLAN.md
├── projects/
│   ├── finance/
│   │   ├── invoice-tracker/
│   │   └── budget-dashboard/
│   └── property-management/
│       └── maintenance-tracker/
├── sg-1/
│   ├── frontend/
│   └── backend/
├── templates/
├── infrastructure/
└── .github/
    └── workflows/
```

This is the target layout. SG-1's frontend/backend and CI directories are implemented; generated-project, shared-template, and AWS-infrastructure directories will be added with their real implementations.

### 5.1 Per-project contents

Each generated project should contain:

- Application source and dependency manifests/lockfiles.
- A Dockerfile and appropriately scoped `.dockerignore`.
- A platform project manifest, for example `sg1.yaml`.
- Database migrations where applicable.
- Automated checks and smoke tests where applicable.
- A short application README and non-sensitive environment-variable documentation.

The project manifest describes the build context, Dockerfile, application port, health endpoint, resource requirements, migration command, and requested platform resources. SG-1 validates it against an allowlisted schema. It is not permission to create arbitrary AWS resources or expand IAM privileges.

### 5.2 Monorepo safety and concurrency

- Use a GitHub App with the minimum repository permissions needed.
- Keep GitHub credentials in the trusted control plane, never in generated code or build scripts.
- Scope AI filesystem access, build contexts, and permitted diffs to the assigned project.
- Reject path traversal, symlink escapes, and changes to SG-1, infrastructure, workflows, templates, or other projects.
- Treat shared-template changes as platform-maintainer work, not normal application edits.
- Run generated dependency installation and build scripts only inside the sandbox.
- Serialize builds that modify the same project for the MVP.
- Allow different projects to build concurrently within a global capacity limit.
- Start each attempt from a recorded Git commit and validate the exact revision represented by the image.
- Serialize or safely retry Git merges when concurrent projects update the monorepo.
- Revalidate if a merge changes the candidate's actual build inputs.
- Build only affected projects; do not rebuild every application for every commit.

GitHub does not provide folder-level repository authorization for this layout. Stakeholders use SG-1's authorization layer and do not need direct repository access. Platform maintainers with repository access can see all project source; that access must be limited and audited separately.

## 6. Identity and permissions

### 6.1 SSO

- Support Entra ID and OneLogin, using OIDC where supported by the company's configuration.
- Confirm whether either identity-provider configuration requires SAML before implementation.
- Identify accounts using the identity provider/issuer and stable subject identifier, not an email address alone.
- Allow explicit, verified account linking when the same employee uses both providers.
- Do not automatically merge accounts solely because their email addresses match.
- Restrict accepted tenants and identity-provider configurations to the company.
- Use secure, HTTP-only, host-only session cookies and server-side authorization.
- Enforce CSRF protections, origin validation, session expiry, and revocation.
- Leverage identity-provider MFA and access policies.

### 6.2 Department and project roles

Proposed mapping of the agreed read/write/admin model:

| Scope / role | Permissions |
| --- | --- |
| Project read | View project status and read-only builder information; no code changes or deployment |
| Project write | Chat, request changes, upload authorized files, and publish validated builds |
| Project admin | Project-write rights plus project membership and access administration |
| Department admin | Manage department membership, create projects, and assign project access within the department |
| Platform admin | Manage SG-1 configuration, identity integrations, platform policies, and infrastructure operations |

- Department directors are department admins.
- Department authority must not extend into unrelated departments.
- Administrators add or invite existing corporate identities; SG-1 does not create Microsoft or OneLogin accounts unless a separate provisioning integration is explicitly approved.
- Define whether department membership grants any default project access; use no implicit project access until that rule is approved.
- Keep project deletion, public-access changes, credential resets, and other destructive actions separately authorized and audited.
- Proposed: only project/department admins can change public/password mode. Editors may still publish directly. Confirm this permission boundary before implementation.

Application business-data permissions are separate from these roles. Public or shared-password access does not provide individual-user identity. Applications requiring record-level or named-user authorization need an explicit application-authentication design.

## 7. Control-plane database

Use a master/control-plane PostgreSQL database as the authoritative store for identity, names, permissions, resource ownership, and deployment state.

### 7.1 Core records

| Record | Purpose |
| --- | --- |
| `users` | Internal user identity and lifecycle status |
| `identity_providers` | Trusted Entra ID / OneLogin configurations |
| `user_identities` | Provider/issuer and subject mappings |
| `departments` | Department identity, name, and slug |
| `department_memberships` | Department roles |
| `projects` | Immutable project ID, department, display name, globally unique slug, lifecycle status |
| `project_memberships` | Project-level read/write/admin roles |
| `project_resources` | Assigned databases, storage prefixes, services, and secret references |
| `project_access_settings` | Access mode, password hash, credential version |
| `conversations` / `messages` | Chat history and attachment references |
| `build_jobs` / `build_events` | Durable execution state and progress |
| `deployments` | Source revision, image digest, checks, status, previous deployment |
| `uploaded_files` | Ownership, purpose, object key, scan status, and retention |
| `audit_events` | Security-sensitive actions and actors |
| `outbox_events` | Reliable handoff of committed work to asynchronous processing |

### 7.2 Invariants

- Globally unique normalized project slugs, enforced by a database unique constraint.
- Unique provider/issuer/subject identity mappings.
- Unique memberships per user and scope.
- Immutable internal project IDs; display names may change independently.
- URL-slug changes are a separate, explicit operation with a defined redirect policy.
- Reserve platform routes including `build`, `api`, `auth`, `admin`, framework asset paths, and other system names.
- Use a DNS-safe slug format compatible with the isolated application hostname.
- Database constraints, transactions, and idempotency keys prevent duplicate project creation and duplicate provisioning.
- A project is not marked ready until its required resources and placeholder are available.
- Failed provisioning remains visible and can be retried safely.
- Do not immediately recycle deleted slugs while resources, links, or credentials could still refer to the old project.

Generated applications must have no access to this database or its credentials.

## 8. Application databases and persistent files

### 8.1 PostgreSQL

- Use RDS PostgreSQL rather than a PostgreSQL container attached to the application task.
- Initially, project databases may share an RDS instance to reduce operational overhead.
- Give every project a separate database and credentials; remove default cross-database access where necessary.
- Separate runtime credentials from migration/admin credentials where practical.
- Do not give generated applications cluster administration or control-plane database privileges.
- Store credentials in Secrets Manager and inject them at runtime.
- Apply connection limits and introduce pooling if required by observed usage.
- Enable backups and point-in-time recovery with documented retention.
- Test a project-level restore procedure; restoring one database may require restoring the instance to a temporary environment first.
- Confirm production availability requirements before selecting single-AZ versus Multi-AZ configuration.

### 8.2 Database changes

- Record migration version alongside deployments.
- Run migrations through a controlled deployment step, not arbitrary API request handling.
- Prefer backward-compatible, expand/contract migrations because old and new tasks may overlap during rollout.
- Automatically run validated, non-destructive migrations under policy.
- Require explicit confirmation for destructive operations; this is a data-loss safeguard, not routine administrator release approval.
- Do not assume an image rollback can reverse schema or data changes.
- Preview/build environments use separate databases and credentials by default.
- Do not copy production data into previews without an approved, audited sanitization/access process.

### 8.3 S3

- Block public bucket access and encrypt stored objects.
- Scope object ownership using immutable project IDs rather than user-provided paths.
- Distinguish chat attachments, application uploads, build artifacts, and operational exports.
- Issue short-lived signed upload/download URLs only after authorization.
- Validate size and type; quarantine and scan untrusted uploads before processing.
- Restrict cross-origin access to approved origins.
- Serve potentially active uploaded content from an isolated download origin or as attachments, not executable content on the SG-1 origin.
- Apply retention and deletion policies per file category.
- Never assume a public application means every project file should be downloadable.

## 9. URL routing, origins, and access gateway

### 9.1 Builder and viewer

- `/build/{slug}` is an authenticated Next.js workspace.
- `/{slug}` is a trusted viewer shell, not arbitrary generated HTML served as SG-1 code.
- The shell embeds an application hosted on a separate origin, such as `{slug}.<approved-app-domain>`.
- Prefer a separately registered application domain to strengthen isolation from SG-1. Exact DNS names remain to be chosen.
- Applications run at `/` on their own origin, reducing framework-specific subpath problems.
- Reserve SG-1 route names before accepting project slugs.
- The shell must not contain platform tokens or secrets accessible to the embedded application.
- Restrict iframe permissions and validate any `postMessage` communication by exact origin and message schema.
- Do not expose an untrusted HTML reverse proxy on the SG-1 origin as a fallback for incompatible frameworks.

### 9.2 Application gateway

- Application ECS tasks run in private networking and are reachable only through approved gateway paths.
- Route validated application hostnames to registered project services; reject unknown hosts.
- Enforce access before proxying HTTP requests and WebSocket upgrades.
- Protect all application paths and protocols consistently.
- Expose no alternate task, load-balancer, or preview address that bypasses authentication.
- Strip untrusted forwarding/identity headers and supply only trusted gateway metadata.
- Use HTTPS, appropriate request limits, rate limits, and security headers.
- Revoke existing password sessions when the password or access mode changes.
- Avoid shared-domain cookies; keep SG-1 and application sessions isolated.
- Do not rely on SameSite cookies alone for control-plane protection, particularly if app and platform domains share a registrable domain.

Proposed implementation: a shared gateway behind managed ingress, routing over private service discovery to project ECS services. Validate HTTP streaming, WebSockets, connection timeouts, service discovery, scale, and high availability in the infrastructure spike. A routing scheme based on one ALB rule per project must account for AWS quotas rather than assuming unlimited rules.

### 9.3 Password mode

- Generate or require a strong initial password; never commit it or display it in ordinary logs/chat.
- Store only a modern password hash, for example Argon2id with reviewed parameters.
- Use short-lived, revocable application access sessions after password verification.
- Rate-limit password attempts and provide administrative reset.
- Public mode skips the password gate, but it does not disable application-specific business authorization.
- Public/password changes take effect on the actual app origin and active access sessions, not just the viewer shell.
- Preview access requires the employee's project authorization, not the shared published password.

## 10. Application runtime contract

Any language or framework is allowed if the application:

1. Builds into an image supported by the selected Fargate runtime/CPU architecture.
2. Starts without privileged access, host mounts, or a Docker daemon.
3. Listens on the assigned interface and port.
4. Provides a configured health endpoint.
5. Keeps persistent business data in PostgreSQL/S3 rather than the container filesystem.
6. Obtains configuration and secrets through approved runtime injection.
7. Runs with constrained CPU, memory, storage, and network permissions.
8. Supports the platform's embedding, gateway, and proxy requirements.
9. Produces usable logs without deliberately exposing secrets or sensitive records.
10. Exits and handles termination predictably enough for rolling deployment.

Use non-root execution and read-only filesystems where compatible. Grant narrowly scoped temporary writable storage when required. Exceptions need a documented platform policy, not AI-controlled privilege escalation.

Arbitrary stacks do not mean arbitrary infrastructure. Additional services, privileged containers, GPUs, multiple cooperating services, and unrestricted outbound connectivity are not automatic MVP capabilities.

## 11. Project lifecycle

### 11.1 Immediate startup

```text
Create request
  -> authorize and reserve slug transactionally
  -> persist project and provisioning job
  -> provision project resources idempotently
  -> commit initial project files
  -> start placeholder container
  -> verify gateway and health
  -> mark ready and open chat/preview
```

- Trigger provisioning immediately, before any chat prompt.
- Display real provisioning progress and retryable failures.
- Starting a Fargate task is asynchronous; do not promise zero startup latency.
- The placeholder is password-protected on the published route.
- The builder preview is available to authorized employees without entering the shared project password.
- If the user later selects a different stack, build a new image rather than mutating the running placeholder.

### 11.2 Build and deployment

```text
Persist user request and attachments
  -> authorize and enqueue job
  -> acquire project build lease
  -> prepare isolated source workspace
  -> AI edits and runs project-scoped checks
  -> validate permitted diff and runtime manifest
  -> build Docker image
  -> run security policy checks
  -> launch candidate using non-production preview resources
  -> health checks and smoke tests
  -> finalize source revision and immutable image record
  -> execute approved production migration step if needed
  -> deploy the same validated image digest
  -> confirm production health
  -> mark successful and retain rollback reference
```

- Set bounded AI retries, tool budgets, execution timeouts, and build cost limits.
- Persist progress so browser reconnects do not interrupt the job.
- Treat queue delivery as at-least-once: operations must be idempotent.
- Use worker leases/heartbeats and recover abandoned jobs.
- Support cancellation without leaving partial public deployments.
- Never mark a build successful solely because the model says it is done.
- The current deployment remains available until the replacement is healthy.
- Rolling deployment can temporarily require more than one container for a project; “one project container” means an isolated project workload, not a prohibition on safe rollout overlap.
- If final deployment fails, retain the previous working image and show a clear failure state.
- Rollback must account for migration compatibility.

### 11.3 State models

Proposed project states:

`creating -> provisioning -> ready`, with explicit `failed`, `archiving`, and `archived` states.

Proposed build states:

`queued -> preparing -> generating -> checking -> building -> validating -> deploying -> succeeded`.

Terminal or exceptional states include `failed`, `cancelled`, and `timed_out`. Store an error category, safe user-facing explanation, and diagnostic reference rather than raw secrets.

### 11.4 Archive and deletion

- Archiving stops serving the app according to policy but does not silently erase data.
- Destructive deletion requires explicit authorization and confirmation.
- Define retention for source history, images, database backups, files, and audit records.
- Revoke credentials and gateway mappings during deprovisioning.
- Do not release a project slug until the deletion/reuse policy makes it safe.

## 12. AI orchestration

- Keep provider integration behind a Go-side abstraction; provider/model selection remains open.
- Stream user-visible status, command results, and build outcomes, not hidden model reasoning.
- Give the model project-scoped tools rather than unrestricted AWS, GitHub, database, or host access.
- Keep production secrets and data out of prompts and ordinary build environments.
- Treat instructions in uploads, repository files, web content, and command output as untrusted input.
- Require policy validation outside the model before privileged operations.
- Never expose broad IAM credentials in a sandbox. Any necessary artifact access must be narrowly scoped and short-lived.
- Do not bake credentials into images or Docker build arguments/layers.
- Record provider/model, token usage, tool calls, duration, and cost metadata without unnecessary sensitive content.
- Establish provider data retention, training-use restrictions, processing region, and contractual controls before sending company data.
- Warn clearly that source, prompts, and uploaded documents may contain sensitive data; log redaction alone does not sanitize model inputs.

Generated code can intentionally exfiltrate any data or secrets it is allowed to access at runtime. Restrict network access and credentials, scan and test generated changes, and make public exposure explicit; no architecture can make arbitrary code safe simply by calling it AI-generated.

## 13. SG-1 frontend and API

### 13.1 Core screens

- SSO login with approved providers.
- Department/project dashboard with role-aware actions.
- Create-project flow with unique-name validation and provisioning progress.
- Builder with chat, file attachments, streamed status, and split-view preview.
- Build/deployment history with errors and rollback controls.
- Project settings: membership, access mode, password reset, resources, archive/delete.
- Department user administration.
- Public/password-protected viewer and friendly unavailable/not-found states.

### 13.2 Go API responsibilities

- Authentication callbacks/session lifecycle and authorization.
- Department/project CRUD and uniqueness enforcement.
- Chat and attachment metadata.
- Build submission, status, cancellation, and deployment history.
- Signed file operations and upload ownership checks.
- Access settings and password verification/session management.
- Worker coordination, resource provisioning, audit events, and reconciliation.

Keep cloud provisioning and long-running AI work outside synchronous API requests. Proposed browser streaming transport: Server-Sent Events with resumable event IDs, authenticated access, and reconnect support. Use WebSockets only where bidirectional interaction requires them; app WebSocket proxying is a separate gateway capability.

Frontend permission checks improve usability but never replace API authorization. Public viewer routes must not accidentally expose internal APIs, chat history, preview credentials, or administrative metadata.

## 14. Security and operations baseline

### 14.1 Isolation and network policy

- Private application tasks and databases; no public database endpoints.
- Distinct control-plane, build, and project execution roles.
- Project-specific credentials and storage grants; do not reuse a broad role across untrusted apps.
- Least-privilege security groups and routing.
- Block access to control-plane internals and unrelated project services.
- Restrict outbound traffic where feasible, including SSRF-sensitive destinations and cloud credentials endpoints.
- Allow only required package/model destinations in build environments under a documented egress policy.
- Keep build execution isolated from production networks and data.
- Establish dependency, image, secret, and prohibited-change scanning.
- Set an explicit vulnerability policy and an audited exception process.

### 14.2 Sensitive data

- Inventory tenant, financial, employee, and other personal data before pilot launch.
- Select an AWS region and confirm data residency requirements.
- Encrypt transport and storage; restrict administrative access.
- Define retention for chat, uploads, build logs, application logs, backups, and audit data.
- Redact secrets and avoid logging request bodies or database records by default.
- Audit permission changes, public exposure, password resets, deployments, migrations, restores, and deletion.
- Do not present a shared project password as sufficient security for every category of sensitive information.

### 14.3 Reliability and cost

- Infrastructure as code for reproducible environments; tool choice remains open.
- Separate development/test and production AWS environments, preferably separate accounts.
- Automated SG-1 backups and tested restore procedures.
- Health checks, structured logs, request/build correlation IDs, and operational dashboards.
- Alerts for failed provisioning, stuck jobs, repeated password attempts, unhealthy services, storage/database pressure, and spend anomalies.
- Resource tagging by environment, department, and immutable project ID.
- Limits for concurrent builds, AI usage, application resources, uploads, and log retention.
- Expire candidate previews and build artifacts according to policy.
- Account for baseline Fargate cost per running project; user count alone does not predict hosting cost.
- Do not introduce scale-to-zero by default because it changes startup availability; evaluate later if cost justifies it.
- Define availability, recovery-time, and recovery-point targets before production sign-off.

## 15. Implementation phases and acceptance criteria

### Phase 0 — Decisions and technical spikes

Deliver:

- Approved AWS region, environments, networking, domains, and initial budget.
- Confirmed identity-provider configurations and user-linking policy.
- AI provider/model and data-processing approval.
- Proof of CodeBuild sandbox command execution, streaming, cancellation, and final Docker build.
- Proof of public/password gateway enforcement, iframe operation, and WebSocket proxying.
- Measured project startup/build times and a capacity estimate.

Acceptance: no unresolved infrastructure or identity limitation blocks the agreed user experience. Document startup latency, AWS quotas, security assumptions, and cost drivers.

### Phase 1 — Repository and platform foundation

Deliver:

- Monorepo layout, SG-1 Next.js/Go skeletons, configuration conventions.
- Infrastructure as code, ECR, ECS services, RDS, S3, queue, and Secrets Manager setup.
- SG-1 CI checks and deployment pipeline.
- Control-plane migrations, structured logging, health endpoints, and audit foundation.

Acceptance: SG-1 deploys reproducibly to a non-production environment without committed secrets; restarting services preserves control-plane data.

### Phase 2 — Identity, departments, and permissions

Deliver:

- Entra ID and OneLogin login flows.
- Session handling, trusted-provider restrictions, and explicit account linking.
- Department/project membership APIs and administration screens.
- Server-enforced read/write/admin permissions.

Acceptance: both providers work; unauthorized users cannot read or mutate other departments/projects; duplicate identities and memberships are prevented transactionally.

### Phase 3 — Project creation and secure serving

Deliver:

- Global slug reservation and reserved-route rules.
- Idempotent resource provisioning and placeholder container startup.
- Project database, private storage allocation, and secret assignment.
- Builder/viewer routes and isolated application-origin gateway.
- Password default, password reset, explicit public toggle, and employee-only preview.

Acceptance: creating a project without a prompt starts a healthy placeholder; public visitors and password holders see only permitted content; direct app URLs cannot bypass protection; restart does not erase project data.

### Phase 4 — Build, validation, and deployment engine

Deliver:

- Durable jobs, per-project leases, sandbox lifecycle, cancellation, retries, and recovery.
- GitHub App integration, scoped diffs, and concurrent monorepo handling.
- Docker build, scanning policy, candidate launch, health checks, and smoke tests.
- Immutable image deployment, migration handling, history, and rollback.

Acceptance: broken builds never replace a working version; worker retries do not create duplicate deployments; concurrent projects do not overwrite each other; generated code cannot change platform files or access production secrets during a build.

### Phase 5 — AI chat and builder experience

Deliver:

- Go-side model orchestration and project-scoped tools.
- Streaming chat/progress, attachments, reconnect, and useful failure messages.
- Split-view preview and deployment status/history.
- Bounded repair attempts and usage/cost accounting.
- At least two different generated stacks tested against the same runtime contract.

Acceptance: a non-technical employee can create, modify, and publish a small tool through chat; the UI distinguishes generating, building, validation, deployment, and failure states without claiming success prematurely.

### Phase 6 — Pilot hardening and launch

Deliver:

- Cross-project, cross-department, gateway-bypass, CSRF, upload, and session-revocation tests.
- Malicious generated-code and prompt-injection test cases.
- Backup/restore rehearsal, migration/rollback rehearsal, and incident runbooks.
- Retention policies, alarms, capacity tests, spend controls, and administrative documentation.
- Pilot with a small set of departments before wider rollout.

Acceptance: security and recovery checks pass; sensitive-data handling is approved; support owners and operational procedures are established.

## 16. Minimum end-to-end test scenarios

1. Sign in through each approved identity provider; verify account linking does not duplicate users or trust an unverified email match.
2. A department admin adds a user and assigns read, write, and admin access; every API enforces the resulting permissions.
3. Simultaneous requests for the same project slug produce exactly one project reservation.
4. Creating a project starts a password-protected placeholder without a chat prompt.
5. Restart/replacement of its container preserves PostgreSQL records and S3 files.
6. A failed Docker build, failed health check, or failed migration does not publish a broken replacement.
7. A successful build automatically publishes the validated image and preserves access mode.
8. Public apps work for an unauthenticated internet visitor; protected apps deny access without the password.
9. Direct app-origin access, alternate paths, APIs, and WebSocket upgrades cannot bypass the gateway.
10. Password reset and switching access modes revoke obsolete access sessions.
11. Public visitors cannot access builder pages, chat, private previews, platform tokens, or unrelated files.
12. Generated code cannot modify another project, SG-1 source, infrastructure, or workflows.
13. Concurrent changes to different projects preserve both histories and deploy the correct image/commit pair.
14. Duplicate queue deliveries, worker crashes, browser disconnects, cancellation, and job timeouts recover predictably.
15. Image rollback works for compatible schema versions; incompatible/destructive migration handling is explicit.
16. Two substantially different application stacks work through the gateway and embedded viewer.
17. Restore a project database and associated files using the documented recovery procedure.
18. Audit records identify the actor and outcome of membership, deployment, public-access, password, and deletion actions without recording plaintext secrets.

## 17. MVP boundaries

Included:

- Dual-provider SSO, departments, read/write/admin access.
- Immediate project container creation.
- Chat and split-view preview.
- One monorepo, scoped AI edits, build validation, automatic deployment, and rollback history.
- Public/password-protected serving through isolated application origins.
- Persistent PostgreSQL and private S3 integration.
- Essential audit, backup, retention, monitoring, and usage limits.

Not automatic MVP capabilities:

- Privileged containers, Docker daemon access, GPUs, or user-managed AWS infrastructure.
- Arbitrary multi-service application topologies or alternative database engines.
- Named-user authentication and record-level authorization for every generated application.
- Custom domains per project, collaborative live editing, or arbitrary shared libraries edited by application agents.
- Production data copied into AI prompts or preview databases.
- Unlimited AI retries, unrestricted outbound networking, or unrestricted compute.
- A guarantee that every framework works without satisfying the runtime/embedding contract.

Optional functionality should not displace the security and recovery requirements needed for sensitive data.

## 18. Remaining decisions

These do not overturn confirmed requirements, but must be resolved before the relevant implementation phase:

1. Confirm ECS Fargate for SG-1 itself, rather than EC2.
2. Choose AWS region, account/environment layout, and required data residency.
3. Choose the isolated application domain and DNS ownership model.
4. Choose infrastructure-as-code tooling and production CI/deployment ownership.
5. Confirm OIDC/SAML details, tenant restrictions, provisioning, and dual-provider account linking.
6. Choose AI provider(s), model(s), credentials, budgets, and approved sensitive-data processing terms.
7. Estimate project count, simultaneous builds, and acceptable creation/build latency.
8. Confirm who can create projects, change public/password access, delete projects, and grant department-wide access.
9. Establish data classifications and whether some classes must never be published publicly or under a shared password alone.
10. Set backup retention, availability targets, RTO/RPO, log retention, and deletion policies.
11. Decide how runtime external integrations and their secrets are requested and approved.
12. Validate the proposed CodeBuild-based sandbox and private application-gateway design with working spikes.

## 19. Definition of initial production readiness

SG-1 is ready for its initial production release when an authorized employee can create a project, immediately receive a running protected placeholder, build an application through chat, publish a validated container automatically, and use it at the agreed viewer URL—with durable data, enforced permissions, audited access changes, tested isolation, and a demonstrated recovery path.

Implementation should begin with Phase 0 and proceed through the acceptance gates. This document does not authorize unrelated infrastructure changes, deployment to an unapproved AWS account, or processing sensitive company data through an unapproved model provider.
