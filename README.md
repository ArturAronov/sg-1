# SG-1

An internal application builder for a real-estate company. Next.js 16 / TypeScript frontend, Go API and workers, PostgreSQL, Docker, and private S3-compatible file storage.

**Status: functional local development build pipeline, not a production-ready company platform.** See [PLAN.md](PLAN.md) for the complete requirements. The default branch is `master`.

## Working locally

- Create a project: SG-1 immediately queues a real starter-container build, creates isolated application networks and persistent PostgreSQL resources, checks health, and opens its live preview. No prompt is required.
- Save personal OpenAI, Claude/Anthropic, GLM, and Kimi API keys with encrypted storage, replacement, and removal.
- Choose a provider, load its available models or enter a model ID, and submit a build request.
- The worker sends current source, recent chat, and the request to that provider. It accepts a complete source tree, builds the Docker image, validates a candidate against a separate preview database, starts the production candidate, and publishes only after health checks pass.
- Up to three generation/repair attempts are allowed per request. Your provider account pays for model usage. No model call is made merely by saving a key.
- Failed builds leave the current deployment in place. Restore earlier images from deployment history without replacing the project database.
- View applications at `/{project-name}`; build at `/build/{project-name}`. The viewer embeds a separate application origin.
- Applications default to locked/password mode. An admin sets a visitor password or explicitly enables public access. The gateway protects all paths and WebSocket upgrades; direct app addresses do not bypass the gate.
- Upload/download private project files, up to 10 MiB each. Local object storage is SeaweedFS using the real S3 protocol and a persistent volume. The adapter also supports AWS S3.
- Download the current application source as a ZIP. Source snapshots, chat, jobs, deployment history, and audit records persist in PostgreSQL.

## Not implemented / production blockers

- Employee Entra ID / OneLogin SSO, individual employee sessions, and membership-management screens/APIs.
- AWS infrastructure and CodeBuild/ECR/ECS/Fargate deployment. Local Docker is not an ECS implementation.
- Automatic generated-source synchronization into the GitHub monorepo. The current source of truth for generated applications is versioned database snapshots; ZIP export is available. A scoped GitHub App integration is still required.
- Production-grade untrusted-code isolation, egress controls, quotas, image policy, migration safeguards, malware scanning, high availability, and backup/recovery sign-off.
- Automatic parsing/sending of uploaded attachments to models, application-specific named-user authorization, and application access to uploaded builder files. Uploads are deliberately not exposed just because an app is public.

Production server startup is intentionally disabled until real authentication is implemented. **Do not expose this development stack publicly or use sensitive production data in it.** The Docker socket gives the trusted local worker host-level authority. The local PostgreSQL administrator uses development credentials. This arrangement is not suitable for mutually untrusted tenants or company production workloads.

## Start

Requires Docker with Compose and Node.js 22+ for the setup helper.

```sh
node scripts/setup.mjs
docker compose up --build -d --wait
```

Open **http://localhost:3000**. Sign in using `SG1_DEV_TOKEN` from the root `.env` file.

The helper generates separate random development-login, encryption, and object-storage credentials, preserves existing values, and appends newly required credentials to older setups. It never silently rotates an existing encryption key. Keep `.env` private; it is excluded from Git and Docker build contexts.

The development seed creates `developer@localhost` and a Property management department. **All local sign-ins currently share that development identity.** User-scoped credential/data authorization is implemented, but separate employee logins require the outstanding SSO work.

Use `localhost`, not `127.0.0.1`, in the browser unless `SG1_PUBLIC_URL` is adjusted. Browser mutation origins are checked exactly.

### Addresses and services

| Service | Address |
| --- | --- |
| SG-1 | `http://localhost:3000` |
| API liveness / readiness | `http://127.0.0.1:8080/healthz`, `/readyz` |
| Application gateway | `http://{project-name}.localhost:8081` |
| PostgreSQL | `127.0.0.1:55432` |
| Object storage | Private Docker network only; no host port |

Modern browsers resolve `*.localhost` to loopback. Application origins differ from SG-1 and from other projects. Protected embedded sessions use Secure, HTTP-only, partitioned cookies; production requires HTTPS and approved domains. Chrome/Chromium is the currently tested browser; validate company browsers before rollout.

Compose starts migrations, development seeding, object-store initialization, the API, frontend, gateway, and provisioning/build worker. The worker alone receives the Docker socket. Generated application containers have no host ports, Docker socket, provider keys, platform credentials, or control-plane database access. They run with memory/CPU/PID limits, dropped capabilities, and read-only root filesystems.

Each project receives distinct production and preview PostgreSQL databases and credentials. The databases live outside application containers. Preview validation uses the preview credentials; the live task receives production credentials at runtime. Credentials are not put in Docker build contexts, source snapshots, or AI prompts.

```sh
docker compose ps
docker compose logs -f api worker gateway web
docker ps --filter label=sg1.managed=true
```

The worker reconnects trusted gateway/database containers to existing project networks after platform container replacement. Provisioning/build progress is durable and polled by the browser. A disconnected browser does not cancel a job.

### Stop and persistence

```sh
docker compose down
```

This preserves the PostgreSQL and object-storage volumes. **Generated project containers/networks are independently managed and are not removed by Compose down.** They have no published host ports. Use their exact `sg1-app-...` container names if stopping individual local projects; do not run broad Docker prune/delete commands. Full project archival/deletion and automatic retention cleanup are not implemented yet.

Do not delete `sg1_postgres_data` or `sg1_object_data` unless you intend to discard those local records/files. Keep the encryption key backed up separately: losing it makes saved provider/project credentials unreadable.

## Use the builder

1. Open **AI providers** and save an API key. A chat subscription/account password is not an API key.
2. Create a project. Wait for **Container running** and the starter preview.
3. Choose a provider, click **Load models**, and select a compatible model. A provider may reject unavailable/deprecated models or account-specific endpoints; a model ID can be entered manually.
4. Describe a tool and click **Build application**. Source/chat/build-failure context is sent to that provider. Do not include secrets or sensitive production information.
5. Watch generation, Docker build, startup validation, and deployment progress. On failure, the old app remains active and the error appears in chat.
6. Under **Sharing & passwords**, set a visitor password or explicitly confirm public access. Public means no visitor authentication; this local stack is still loopback-only.
7. Use **Deployment history** to restore an earlier image, **Download source** for a ZIP, and **Private project files** for uploads/downloads.

Generated stacks must implement the runtime contract: listen on `0.0.0.0:8080`, provide `/healthz`, define Docker HEALTHCHECK, run non-root, tolerate a read-only filesystem with writable `/tmp`, and use `DATABASE_URL` for persistent business data. Dependencies must be installed during build. Runtime networks have no external internet access. Additional services/integrations need explicit platform support.

Schema changes in generated applications are not a substitute for a reviewed migration system. Image rollback does not undo data/schema changes. Do not entrust production records to this prototype.

## Provider and object-storage credentials

API keys are AES-256-GCM encrypted before SQL receives them. Associated data binds each encrypted credential to its user and provider. Status endpoints return no key, suffix, or ciphertext. Secrets are not injected into generated application containers. Removing a key from SG-1 does not revoke it at the provider.

`SG1_CREDENTIALS_KEY` is a base64-encoded 32-byte master key, injected into trusted backend services only. Production must use approved secrets injection and a reviewed rotation/recovery process. Rotating/re-encrypting existing records is not implemented.

The local S3 store uses a separately generated `SG1_S3_SECRET`. Buckets are private; downloads require project authorization and are served as attachments. Startup validates actual object upload/read/delete, not just bucket metadata. The development store uses bounded, small data volumes; adjust its capacity or use AWS S3 for larger workloads. Files are never executed or automatically included in model context. Malware scanning and production file-retention policy remain outstanding.

The Go S3 adapter can target AWS using `SG1_S3_BUCKET`, `AWS_REGION`, and the standard AWS SDK credential chain when `SG1_S3_ENDPOINT` is unset. In AWS, use a narrowly scoped workload role and a pre-provisioned private/encrypted bucket. Do not run the development bucket initializer against production. Endpoint/access-key overrides are for the local S3-compatible service.

## Tests

Backend, from `sg-1/backend`:

```sh
go test -race ./...
go vet ./...
SG1_TEST_DATABASE_URL='postgres://sg1:sg1_local_only@127.0.0.1:55432/sg1?sslmode=disable' go test -race -count=1 ./...
```

SQL tests use uniquely named test schemas. The Docker integration test additionally creates its own random project databases, roles, network, and containers, then cleans only those exact test resources:

```sh
node --env-file=.env scripts/test-runtime.mjs
```

Run that helper from the repository root. It inherits local configuration without printing secrets, authenticates independent HTTP probes, and discovers the Docker socket using `docker context inspect`. The worker in Compose uses the explicitly mounted `/var/run/docker.sock`.

The real Docker test covers starter provisioning, generated-image build/deploy, failed-build isolation, image rollback, database persistence, and denial of control-plane database access. Provider protocol tests use synthetic HTTP responses; automated tests do not use your paid provider keys. Real model availability/output quality must be tested separately with an approved key and model.

Frontend, from `sg-1/frontend`:

```sh
npm ci
npm run typecheck
npm run lint
npm run format:check
npm test
npm run build
npx playwright install chromium
npm run test:e2e
```

Browser tests require the running stack, load root `.env` without printing secrets, and create visibly named `Browser test ...` projects. They exercise real container startup, authenticated preview, protected/public viewing, private upload/download, source ZIP export, and credential management. Existing provider credentials are skipped, never overwritten. Synthetic keys created by tests are removed. Browser traces are disabled.

CI runs SQL tests, frontend checks, image builds, the real Docker pipeline test, and browser tests. No AWS resources or paid model calls are needed.

## Repository layout

```text
sg-1/frontend/             Next.js UI and browser tests
sg-1/backend/cmd/sg1/      API / worker / gateway / migration commands
sg-1/backend/internal/    Domain, store, encryption, runtime, S3, and tests
scripts/                  Local setup helper
.github/workflows/        CI
compose.yaml              Development infrastructure only
PLAN.md                   Complete product plan and remaining production work
```

Generated source currently lives in deployment snapshots, not a checked-out `projects/` directory. Do not confuse the completed local execution pipeline with the outstanding GitHub monorepo synchronization or AWS production infrastructure.
