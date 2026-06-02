# Contributing to Vigil

Thanks for your interest in contributing. Vigil is a GRC and risk-management
toolbox maintained by ElBits AS. We welcome bug reports, feature suggestions,
documentation improvements, and code contributions.

## Ways to contribute

- **Report a bug** — open an issue using the bug-report template.
- **Suggest a feature** — open an issue using the feature-request template.
  For larger changes, please open an issue to discuss before sending a PR.
- **Improve documentation** — PRs welcome directly.
- **Submit code** — see below.

## Reporting security issues

Please **do not** open a public issue for security vulnerabilities.
See [SECURITY.md](SECURITY.md) for the disclosure process.

## Development setup

Requirements: Go 1.26+, and Docker or Podman.

```sh
git clone https://github.com/3lbits/vigil.git
cd vigil

make setup
cp .env.example .env

make tailwind-install
make generate
make db-up

air
```

This serves the app on <http://localhost:8080> with live reload. The
database runs in a container (`make db-up`); the app applies its own
migrations on startup.

For quality gates before committing:

```sh
make pre-commit-fast
```

This regenerates templ/sqlc/CSS, tidies `go.mod`, lints, runs govulncheck,
and runs tests. Install `golangci-lint` and `govulncheck` first:

```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
```

`make pre-commit` adds a semgrep run on top — useful occasionally but not
required for everyday work.

## Submitting a pull request

External contributors:

1. Fork the repo and create a branch from `main`.
2. Make your change. Keep PRs focused — one logical change per PR.
3. Add or update tests. New code without tests will usually be asked for tests.
4. Run `make pre-commit-fast` locally and make sure it passes.
5. Sign off your commits (see DCO below).
6. Open a PR against `main`. Fill in the PR template.

Maintainers push to feature branches and open PRs against `main`. CI must
pass before merge.

### Definition of done

A change is ready for review when:

1. `make pre-commit-fast` exits 0 (regenerates code, tidies `go.mod`, lints,
   runs govulncheck, and runs tests).
2. Tests are added or updated for the changed behaviour.
3. Authorization stays policy-driven: `cmd/server/policies/authz.rego` remains
   the single source of truth, embedded via `cmd/server/main.go`. Authz changes
   are high-risk — flag them explicitly in the PR description and don't bundle
   them with unrelated work.
4. The change is focused: every modified line traces to the stated task. Don't
   refactor adjacent code, comments, or formatting unrelated to the change.

## Project conventions

These are the conventions a contributor most often needs. The complete,
authoritative set lives in [AGENTS.md] — it is written for AI agents but
applies equally to everyone, so reach for it when this summary isn't enough.

- **Fat handlers, no service layer.** Handlers query the database directly via
  sqlc. No service layer, no DTOs, no per-module interfaces. Split a handler
  only when concerns are genuinely mixed or logic is duplicated.
- **Authorization only in policy.** Never role-check in a handler
  (`if user.Role == "admin"` is wrong) — the check belongs in `authz.rego`,
  enforced through `authz.RequirePolicy(engine, resource, action)`.
- **Don't edit generated code.** `**/*_templ.go`, the sqlc-generated Go, and
  `cmd/server/public/css/output.css` are produced by `make generate`. Change
  the source and regenerate; the next generation overwrites hand edits.
- **Keep it simple.** No speculative abstractions or unrequested configuration
  knobs. If 200 lines could be 50, write 50. Match the existing style.

## Using AI coding agents

Agents like Claude Code, GitHub Copilot, Cursor, and Aider are welcome on
this project. Parts of this codebase have been developed with agent
assistance, and we don't treat agent-assisted contributions differently
from hand-written ones.

What we do expect:

- **You've read and understood what you're submitting.** The DCO sign-off
  on each commit certifies the contribution is yours to give — that
  applies regardless of how it was drafted. If you can't explain a line in
  review, don't ship it.
- **The same quality bar applies.** `make pre-commit-fast` must pass, and the
  [Project conventions](#project-conventions) must be followed — especially fat
  handlers / no service layer, and the authorization rules.
- **No required disclosure.** You don't need to flag agent use in the PR.
  If a change is substantially agent-generated and you're less confident
  in parts of it, saying so helps reviewers focus their attention, but
  it's optional.

If you're using an agent on this codebase, point it at [AGENTS.md] —
that's where the project's operational rules and architecture conventions
live, written for agents to read.

## Commit messages

We use [Conventional Commits]: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`,
`chore:`. Breaking changes go in the footer as `BREAKING CHANGE:`.

## Developer Certificate of Origin (DCO)

By contributing, you certify the [DCO]. Sign off each commit with:

```sh
git commit -s -m "feat: add risk scale validation"
```

This adds a `Signed-off-by:` line, attesting the contribution is yours to give.

## Code review

A maintainer will review your PR. We aim to respond within a week. We may
ask for changes — this is normal and not a rejection. Once approved and
CI is green, a maintainer will merge.

## License

By contributing, you agree your contributions are licensed under the
project's [LICENSE](LICENSE).

[AGENTS.md]: AGENTS.md
[Conventional Commits]: https://www.conventionalcommits.org
[DCO]: https://developercertificate.org
