# Vigil

Vigil is a Go-based GRC (governance, risk, compliance) toolbox for security
and compliance teams. It provides framework and requirement tracking, control
(measure) management, activity tracking, and a multi-step risk assessment
flow with treatment planning and risk register views.

Authorization is policy-based using embedded [OPA](https://www.openpolicyagent.org/)
Rego (`cmd/server/policies/authz.rego`), not hardcoded role checks in handlers.

> **Status:** Vigil is open-sourced for the common good under Apache 2.0.
> No commercial support, hosted offering, or maintenance commitments are
> provided. Parts of this codebase have been developed with the help of
> AI coding agents (Claude Code, GitHub Copilot CLI); see
> [AGENTS.md](AGENTS.md) for the project conventions agents follow.
> See [LICENSE](LICENSE) and the disclaimer at the bottom of this file.
> Bug reports, ideas, and pull requests are welcome — see
> [CONTRIBUTING.md](CONTRIBUTING.md).

## Features

| Area | What's in the box |
|---|---|
| Compliance | Framework + requirement CRUD, CSV requirement import, requirement/measure linking |
| Measures | Measure CRUD, owner + assignee handling, requirement linking, external links |
| Activities | Activity CRUD, status transitions (start/complete/reopen), deadlines, priorities |
| Risk | Assessment wizard, per-risk decision/treatment flow (criticality order), risk register and detail views, risk/measure/activity/asset linking, acceptance workflow |
| Assets | Asset inventory CRUD + search, linking to risk assessments and individual risks |
| Admin | User/org management, role assignment (`admin/editor/viewer/contributor`), module/risk settings, active sessions, audit log, CSV exports |
| Auth | Development stub auth plus GitHub, Entra ID, and generic OIDC login providers |
| Ops | Health and readiness endpoints, Prometheus metrics, OpenTelemetry tracing, structured redacted logs |

## Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.26+ (`net/http`) |
| HTML | [templ](https://templ.guide/) |
| CSS / JS | Tailwind CSS v4 CLI, htmx, Alpine.js (CSP build) |
| Data | PostgreSQL + pgx/v5 + sqlc |
| Migrations | goose |
| AuthZ | OPA / Rego |
| Sessions | scs (DB-backed) |
| Observability | OpenTelemetry + Prometheus |

## Quick start

Requirements: Go 1.26+, Docker or Podman, and the Tailwind standalone CLI
(installed by `make tailwind-install`).

```sh
git clone https://github.com/3lbits/vigil.git
cd vigil

make setup
cp .env.example .env

make tailwind-install
make generate
make db-up

make run
```

Open <http://localhost:8080>.

> The Makefile uses `podman compose` by default. If you run Docker, either
> alias `podman` to `docker` or run `docker compose up -d` directly in place
> of `make db-up`.

### Demo with published GHCR image

To run Vigil from the published container image with PostgreSQL:

```sh
docker compose -f demo-compose.yml up -d
```

Then open <http://localhost:8080>.

Useful commands:

```sh
docker compose -f demo-compose.yml logs -f app
docker compose -f demo-compose.yml down
# remove demo database volume too:
docker compose -f demo-compose.yml down -v
```

### Demo from source (quick local path)

If you prefer running the app from source with live reload:

```sh
cp .env.example .env
make setup tailwind-install db-reset-seed
DEV_STUB_AUTH=true make run
```

### Tooling

The `make pre-commit` target uses `golangci-lint`, `govulncheck`, and
`semgrep`. Install them if you plan to contribute:

```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
# semgrep: see https://semgrep.dev/docs/getting-started/
```

### Local auth modes

- **`DEV_STUB_AUTH=true`** (development only): stub user session, no OAuth
  app setup needed. This is the easiest way to explore Vigil locally.
- **`DEV_STUB_AUTH=false`**: enable one or more real providers via
  `AUTH_PROVIDERS=github,entra,oidc`.

Provider callbacks:

| Provider | Callback path |
|---|---|
| GitHub | `/auth/github/callback` |
| Entra ID | `/auth/entra/callback` |
| Generic OIDC | `/auth/oidc/callback` |

## Configuration

Key environment variables. See `.env.example` for the full list with comments.

| Variable | Purpose | Default |
|---|---|---|
| `PORT` | HTTP port | `8080` |
| `APP_ENV` | Environment mode (`development`, `staging`, `production`) | `development` |
| `DATABASE_URL` | PostgreSQL DSN | required |
| `ALLOW_INSECURE_DB_SSL` | Staging-only escape hatch for non-TLS DB (`true` allowed only when `APP_ENV=staging`) | `false` |
| `DEV_STUB_AUTH` | Enables stub auth | `false` |
| `DEV_SEED` | Enables `cmd/seed` development dataset seeding (development only) | `false` |
| `AUTH_PROVIDERS` | Comma-separated providers (`github,entra,oidc`) | falls back to `AUTH_PROVIDER` |
| `AUTH_ALLOWED_EMAIL_DOMAINS` | Comma-separated OAuth login allowlist domains (e.g. `example.com,subsidiary.org`) | empty (required when `github` is enabled) |
| `APP_BASE_URL` | Public app URL (used for OIDC redirects) | `http://localhost:8080` |
| `SESSION_COOKIE_NAME` | Session cookie name | `vigil_session` |
| `SESSION_COOKIE_SECURE` | Set `Secure` flag on session cookie | `true` (unless explicitly `false`) |
| `SESSION_IDLE_TIMEOUT` | Session inactivity timeout (`time.ParseDuration` format) | `45m` |
| `SESSION_HMAC_KEY` | HMAC key for session-token event hashing | empty (**required in production**) |
| `METRICS_PATH` | Prometheus path (empty disables) | `/metrics` |
| `TRUSTED_PROXY_CIDRS` | Trusted proxy CIDRs for source-IP extraction | empty |
| `GLOBAL_RATE_LIMIT_PER_WINDOW` | Global in-memory soft rate limit per key (`0` disables) | `240` |
| `GLOBAL_RATE_LIMIT_WINDOW` | Window duration for global soft rate limit | `1m` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Enables OpenTelemetry tracing when set | empty |

### OAuth registration model (important)

- OAuth callback first tries to claim a pre-provisioned pending user by email.
- If no pending user exists, Vigil creates a new `viewer` user for that identity.
- When `github` is enabled, `AUTH_ALLOWED_EMAIL_DOMAINS` must be set or startup fails.

For public deployments, do not expose GitHub login without a strict domain allowlist.

## Common Make targets

| Target | Description |
|---|---|
| `make generate` | Run sqlc + templ + CSS generation |
| `make run` | Start the dev server with live reload (air) |
| `make dev` | Tailwind watch + air in one terminal |
| `make build` | Build `bin/server` |
| `make test` | Run race-enabled tests |
| `make lint` | Run golangci-lint |
| `make pre-commit` | Full pre-commit suite (generate, lint, govulncheck, tests, semgrep) |
| `make pre-commit-fast` | Same without semgrep |
| `make db-up` / `db-reset` | Manage the local PostgreSQL container |
| `make dev-seed` / `db-reset-seed` | Seed coherent dev data (manual; dev only) |
| `make tailwind-install` | Download the Tailwind v4 standalone CLI |
| `make vendor-js` | Download and integrity-check htmx and Alpine.js |
| `make loadtest-browse` / `loadtest-rate-burst` | Manual ad-hoc k6 load checks (developer tooling) |

Run `make help` for the full list.

### Optional developer load checks

Ad-hoc load scripts live under [`scripts/loadtest`](scripts/loadtest). These are
for occasional manual validation by developers and are not part of CI.

### Optional developer data seeding

For local UI testing after `make db-reset`, you can seed a coherent development
dataset (5 users, 5 orgs in a hierarchy, 7 frameworks with 5 requirements each,
5 measures, 5 activities, 5 assets, and 7 risk assessments with linked
risks/participants):

```sh
make dev-seed
# or in one step:
make db-reset-seed
```

This is manual developer tooling and is intentionally gated to development.

## Deployment

The production image is published to the GitHub Container Registry:

```sh
docker pull ghcr.io/3lbits/vigil:latest
# or pin a specific release:
docker pull ghcr.io/3lbits/vigil:v1.0.0
```

The image is built from a distroless base, runs as non-root (UID 65532), and
is multi-arch (amd64 and arm64). Recommended runtime flags:

```sh
docker run --rm \
  --cap-drop all \
  --security-opt=no-new-privileges \
  --read-only --tmpfs /tmp \
  --user 65532:65532 \
  -e DATABASE_URL=... \
  -e SESSION_HMAC_KEY=... \
  -p 8080:8080 \
  ghcr.io/3lbits/vigil:latest
```

Vigil applies its database migrations at startup (via goose) — point it at a
PostgreSQL database and it will bring the schema up on first run.

**Data extraction:** use `pg_dump` or `psql \copy` against the database directly.

## Repository layout

```text
cmd/server/          app entrypoint, route wiring, embedded static assets/policy
db/migrations/       goose migrations (applied at startup)
db/queries/          sqlc query sources
internal/
  auth/              providers + session user middleware
  authz/             OPA engine + RequirePolicy middleware
  config/            env config loader/validation
  obs/               tracing, metrics, security event logging
  locale/            i18n bundle + language middleware
modules/
  about/
  dashboard/
  compliance/
  measures/
  activities/
  risk/
  admin/
  auth/
cmd/server/policies/authz.rego  source-of-truth authorization policy
templates/layout/    shared templ layout/components
```

## Architecture

Modules under `modules/*` implement the `module.Module` contract and are
composed via `Registry` in `cmd/server/main.go`. Module-specific
dependencies are passed to each module's constructor; the shared
`Dependencies` struct is kept deliberately minimal. See
[`internal/modregistry`](internal/modregistry) for the contract.
 
**Handler design.** Handlers are intentionally "fat" and query the database
directly via sqlc. There is no service layer, no DTOs, and no per-module
interfaces — these solve problems this stack does not have. Split a handler
only when concerns are genuinely mixed or logic is duplicated.
 
**Generated code is not edited by hand.** `**/*_templ.go` (templ), the
sqlc-generated Go, and `cmd/server/public/css/output.css` (Tailwind) are all
produced by `make generate`. Change the source (`.templ`, `db/queries/*.sql`,
or the Tailwind input) and regenerate — never edit the output.
 
The full set of conventions lives in [AGENTS.md](AGENTS.md); contributor-facing
highlights are summarised in [CONTRIBUTING.md](CONTRIBUTING.md).

## Authorization policy

`cmd/server/policies/authz.rego` is the single source of truth for
authorization and is embedded into the server binary by `cmd/server/main.go`.

Handlers must never role-check directly; all authorization goes through
`authz.RequirePolicy(engine, resource, action)`.

## Observability

| Endpoint | Purpose |
|---|---|
| `GET /healthz` | Liveness |
| `GET /readyz` | Readiness |
| `GET /metrics` | Prometheus metrics (protected by admin policy; path configurable via `METRICS_PATH`) |

Tracing is enabled when `OTEL_EXPORTER_OTLP_ENDPOINT` is set.

## Contributing

We welcome bug reports, feature suggestions, and pull requests. See
[CONTRIBUTING.md](CONTRIBUTING.md) for the development process and
[AGENTS.md](AGENTS.md) for guidance on using AI coding agents on this
codebase.

For security vulnerabilities, please follow the disclosure process in
[SECURITY.md](SECURITY.md) rather than opening a public issue.

Please do.

## License and disclaimer

Apache License 2.0 — see [LICENSE](LICENSE).

Vigil is open-sourced for the common good under the Apache License 2.0 and is
developed and supplied outside the course of any commercial activity. No paid
support, hosting, consulting, warranties, or maintenance commitments are
offered. Use at your own risk.
