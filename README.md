# SG-1

An internal AI application studio for a real-estate company. SG-1 uses a Next.js 16 / TypeScript frontend, a Go control-plane API, and PostgreSQL.

**Status: runnable local foundation, not production-ready.** The full product plan is in [PLAN.md](PLAN.md). The repository's default branch is `master`.

## What works today

- Local development authentication with an explicitly configured secret token.
- Persistent users, identities, departments, roles, and projects in PostgreSQL.
- Server-enforced project visibility and department-admin project creation.
- Globally unique project URLs and idempotent project creation.
- Atomic project creation, locked/password-default access settings, audit record, provisioning job, and transactional outbox event.
- Project dashboard, search, department filtering, create dialog, and split-view builder shell.
- Local Docker Compose stack, versioned migrations, non-root application images, and automated tests.

## Not connected yet

Entra ID / OneLogin, AI orchestration, project-container provisioning, ECR/ECS deployment, S3 uploads, project databases, password verification for published apps, and the isolated application gateway.

Creating a project currently reserves its name and **queues** a provisioning request. It does **not** start a generated application container. The interface makes this explicit. Chat is disabled rather than pretending to send requests to an AI. Viewer URLs return 404 until real application serving and access enforcement are implemented.

Production API startup is intentionally disabled until SSO is implemented. Never expose this development stack publicly or put real sensitive company data into it.

## Start locally

Requires Docker with Compose and Node.js 22+ for the setup helper. Go 1.26+ is needed only for running Go tools outside Docker.

```sh
node scripts/setup.mjs
docker compose up --build -d
```

Open **http://localhost:3000**. Sign in with `SG1_DEV_TOKEN` from the generated root `.env` file. The setup helper creates a random token, gives the file restrictive permissions, and never overwrites an existing `.env`.

The local seed creates one platform administrator (`developer@localhost`) and a Property management department. This is a development identity, not employee SSO.

Use `localhost`, not `127.0.0.1`, in the browser unless you update `SG1_PUBLIC_URL`; mutation endpoints verify the exact configured origin.

### Local endpoints

| Service | Address |
| --- | --- |
| Frontend | `http://localhost:3000` |
| API liveness | `http://127.0.0.1:8080/healthz` |
| API/database readiness | `http://127.0.0.1:8080/readyz` |
| PostgreSQL | `127.0.0.1:55432`, local credentials in `.env.example` |

All published host ports bind to loopback. PostgreSQL data is stored in the named Docker volume `sg1_postgres_data`, not the API container.

```sh
docker compose ps
docker compose logs -f api web
docker compose down
```

`docker compose down` preserves database data. Do not delete the volume unless you deliberately intend to discard local records.

## Run outside Docker

Start the database first:

```sh
docker compose up -d postgres
```

Export root `.env` variables in each terminal before running the API or frontend. These are local-only secrets; never commit the file.

```sh
set -a
. ./.env
set +a
```

Backend, from `sg-1/backend`:

```sh
go run ./cmd/sg1 migrate
go run ./cmd/sg1 seed-dev
go run ./cmd/sg1 serve
```

Frontend, from `sg-1/frontend` in a separate terminal with the same exported environment:

```sh
npm ci
npm run dev
```

Do not run host services on ports already occupied by Compose's `api` and `web` services.

## Checks

From `sg-1/backend`:

```sh
go test -race ./...
go vet ./...
```

PostgreSQL integration tests require a local test connection. They create and drop only uniquely named test schemas; they do not truncate application tables. The supplied connection must permit schema creation.

```sh
SG1_TEST_DATABASE_URL='postgres://sg1:sg1_local_only@127.0.0.1:55432/sg1?sslmode=disable' go test -race -count=1 ./...
```

From `sg-1/frontend`:

```sh
npm ci
npm run typecheck
npm run lint
npm test
npm run build
```

From the root:

```sh
docker compose build
```

Unit tests run without Docker. Without `SG1_TEST_DATABASE_URL`, Go integration tests explicitly skip instead of substituting an in-memory database.

Browser smoke tests require the running Compose stack. From `sg-1/frontend`:

```sh
npx playwright install chromium
npm run test:e2e
```

The browser-test command loads the root `.env` without printing secrets. Tests cover authentication, rejected cross-origin writes, project creation, persistence across page reload, logout, and fail-closed viewer URLs. They create visibly named `Browser test ...` projects in the local database. Browser traces are disabled to avoid recording the development token.

## API surface

All `/v1/*` endpoints currently require `Authorization: Bearer <SG1_DEV_TOKEN>`. Development auth is allowed only with `SG1_ENV=development` and a token of at least 32 characters.

| Method | Endpoint | Purpose |
| --- | --- | --- |
| GET | `/healthz` | Process liveness |
| GET | `/readyz` | Database connectivity |
| GET | `/v1/me` | Current user and explicitly unavailable capabilities |
| GET | `/v1/departments` | Authorized departments |
| GET | `/v1/projects` | Authorized project list |
| GET | `/v1/projects/{slug}` | Authorized project details |
| POST | `/v1/projects` | Reserve a project and queue provisioning |

Project creation accepts `department_id`, `name`, `slug`, and `description`. Supply a unique `Idempotency-Key` containing 16–128 letters, numbers, hyphens, or underscores. Retrying the same payload and key returns the original project; reusing the key with different details returns 409. Unknown JSON fields, reserved URLs, oversized bodies, and unauthorized departments are rejected.

Browser mutations go through narrowly scoped Next.js endpoints. The backend URL and auth token are server-side only; cross-origin mutations are rejected and responses containing internal records are not cached.

## Repository layout

```text
sg-1/frontend/       Next.js application
sg-1/backend/        Go API, domain rules, SQL migrations, tests
scripts/            Local setup helpers
.github/workflows/  Continuous integration
compose.yaml        Development stack only
PLAN.md             Product and implementation plan
```

Generated `projects/`, shared templates, and AWS infrastructure will be added when their actual implementations are introduced, rather than represented by empty scaffolding.

## Next implementation slice

1. Confirm AWS region, app domain, identity-provider configuration, and AI provider.
2. Implement employee SSO and real membership administration before production access.
3. Connect the provisioning queue to isolated project resources and a real placeholder container.
4. Implement the isolated app gateway, password enforcement, and employee-only previews.
5. Add scoped AI execution, Docker validation, deployment, and rollback.

See the acceptance gates and unresolved decisions in [PLAN.md](PLAN.md) before provisioning AWS resources or sending company information to an AI provider.
