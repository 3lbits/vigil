# AGENTS.md

Instructions for AI coding agents working on this codebase. Read before
making changes.

---

## Project

Vigil — Go-based GRC (governance, risk, compliance) toolbox.

Stack: Go 1.26+, templ (server-side rendering), sqlc, goose, pgx/v5, htmx,
Alpine.js (CSP build), Tailwind CSS v4 CLI, air.

Authorization is policy-based using embedded OPA Rego.

---

## The only command you need

```sh
make pre-commit-fast
```

This runs everything: regenerates templ/sqlc/CSS, tidies `go.mod`, lints,
runs govulncheck, and runs tests. If it exits 0, the change is verified.

---

## Definition of done

A change is complete when:

1. `make pre-commit-fast` exits 0.
2. Tests added or updated for changed behaviour.
3. If `cmd/server/policies/authz.rego` changed, it remains the single source
   of truth and is embedded via `cmd/server/main.go`.
4. No unrelated changes — every modified line traces to the stated task.

---

## Repository layout

```
cmd/server/          app entrypoint, route wiring, embedded assets/policy
db/migrations/       goose migrations (applied at app startup)
db/queries/          sqlc query sources
internal/
  auth/              providers + session user middleware
  authz/             OPA engine + RequirePolicy middleware
  config/            env config loader/validation
  obs/               tracing, metrics, security event logging
  locale/            i18n bundle + language middleware
modules/             feature modules (about, dashboard, compliance,
                     measures, activities, risk, admin, auth)
cmd/server/policies/authz.rego  source-of-truth authorization policy
templates/layout/    shared templ layout and components
```

---

## Generated files

These regenerate automatically as part of `make pre-commit-fast`. Do not
edit them directly — the next generation overwrites the changes.

- `**/*_templ.go` — produced by `templ generate`
- sqlc-generated Go files — produced by `sqlc generate`
- `cmd/server/public/css/output.css` — produced by Tailwind CLI

If you need to change generated output, change the source (`.templ` file,
`db/queries/*.sql`, or Tailwind input) and re-run `make pre-commit-fast`.

---

## Architecture rules

### Authorization

- All authz goes through `authz.RequirePolicy(engine, resource, action)`.
  Never role-check in handlers (`if user.Role == "admin"` is wrong — the
  check belongs in `authz.rego`).
- `cmd/server/policies/authz.rego` is the source of truth and is embedded
  directly by `cmd/server/main.go`.
- Authz changes are high-risk. Flag them explicitly in the change
  description; do not bundle authz edits with unrelated work.

### Handler design (handler-direct-access)

- Fat handlers are correct. Handlers query the database directly via sqlc.
- No service layer, no DTOs, no per-module interfaces — these solve
  problems this stack does not have.
- Split a handler only when: unrelated concerns are genuinely mixed, or
  logic is duplicated across handlers.

### Simplicity

- No unasked features or speculative abstractions.
- No configuration knobs or "flexibility" that weren't requested.
- If 200 lines could be 50, write 50.

### Surgical changes

- Match existing style. Do not refactor adjacent code, comments, or
  formatting unrelated to the task.
- Unrelated dead code: mention it, do not delete it in the same change.
- Remove imports, variables, or functions only when your change made
  them unused.

---

## Before coding

- State assumptions. If uncertain, ask.
- Multiple plausible interpretations of the request → present them, do
  not pick silently.
- A simpler approach exists than what was asked → say so before
  proceeding.
- Unclear request → stop, name the confusion, ask.

For multi-step tasks, state a brief plan with a verification step per
stage before starting.
