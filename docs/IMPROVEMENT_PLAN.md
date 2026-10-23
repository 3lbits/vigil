# Vigil Improvement Plan

Reviewed 2026-10-21 against local `main` (`eebc2dc`) and all five other local branches. Branch references below describe their tips, not files already available on `main`. No remote branches or PR state were consulted.

This is a source review, not an executed build or browser certification. Generated implementations, generated assets, and security findings are outside scope. `.env.example` was inaccessible under organization content policy, so the configuration handoff in the clone/run instructions could not be fully verified. Only this plan was changed.

## 1. Summary

- Keep Go, templ, HTMX, PostgreSQL, and direct sqlc-backed handlers; there is no case for a rewrite or a new service layer.
- Module boundaries are mostly sound: feature handlers import their own templates, with composition in `internal/app/router.go`.
- The highest-return maintenance fixes are a reliable generation/live-reload loop and explicit handling of failed database writes.
- Risk workflows currently combine multi-write operations with success redirects after failed writes; this matters more than their file length.
- `feat/ui-kit` supplies a substantial `internal/ebui` library, but its risk-register proof uses fixtures and no production module imports it yet.
- `feat/ui2-native-templ` is a separate, broad page migration; merging both UI approaches wholesale would leave competing systems.
- Tests cover many helpers, handlers, and template contracts, but do not exercise real PostgreSQL migrations or transactional workflows.
- Onboarding documentation is stale, while dependency use is mostly justified; remove dead paths and duplicated work rather than replace useful libraries.

## 2. Branches

Inventory used `git for-each-ref --sort=-committerdate refs/heads/`, then `git rev-list --left-right --count main...<branch>`, `git log main..<branch> --oneline`, and `git diff main...<branch> --stat` with generated outputs excluded from review. The command returns **behind, ahead**; the table presents **ahead / behind**.

Statuses are maintenance recommendations inferred from local history and source, not claims about the author's intent. "Stale" means a competing/superseded direction here, not merely an age threshold. "Abandoned" on a fully merged pointer means there is no outstanding branch work to retain. No branch can confidently be called **finished but unmerged** from this evidence alone.

| Branch | Status | Ahead / behind main | Last commit | What it is doing | Recommendation |
|---|---|---:|---|---|---|
| `main` | Active | 0 / 0 | 2026-10-19 | `eebc2dc`: templ/toolchain updates; production pages still use `internal/ui`. | Keep as integration baseline. |
| `feat/ui-kit` | Active | 13 / 1 | 2026-10-19 | `acd2d20`: ElBits semantic templ library in `internal/ebui`, CSS/fonts, forms, tables, navigation, risk components, localized showcases and tests. | **Finish** integration with current main and one production page, then **merge** the foundation; migrate remaining pages incrementally. |
| `feat/ui2-native-templ` | Stale, competing implementation | 3 / 1 | 2026-10-07 | `d457f3e`: broad native-CSS/`internal/ui2` migration across module pages, removes many old primitives and Tailwind generation, adds risk-assessment preview and rebuilds dashboard. Still wraps `internal/ui/layout`. | **Delete after reconciliation**: inspect unique page/behavior fixes and port only those needed to the chosen `ebui` path. Do not merge the entire alternative UI stack. |
| `test/ui-golden-baseline` | Stale, superseded primitive work | 9 / 1 | 2026-09-28 | `9956be3`: badge/button/card/table changes, semantic risk tables, module template updates and component tests; not merely a baseline-test branch. | **Delete after reconciliation**: retain unique regression cases and behavior fixes, not another redesign of `internal/ui`. |
| `jonmartinstorm-refactored-barnacle` | Abandoned pointer; fully merged | 0 / 5 | 2026-09-07 | `516db26`: last commit is a Go image update; no unique commits and no merge-base diff against main. | **Delete** local pointer; no code needs merging. |
| `component-library` | Abandoned pointer; fully merged | 0 / 22 | 2026-08-10 | `1c02990`: last commit is a Mermaid update; no unique commits or merge-base diff. Despite its name, this is not the replacement library branch. | **Delete** local pointer; no code needs merging. |

`feat/ui-kit` and `feat/ui2-native-templ` share base `2106475` but neither contains the other's work. The latter's three commits include a large page migration: commit count is not a measure of remaining work. Local evidence favors `feat/ui-kit` as the current replacement effort because its component work continues through October 19, after the alternative's October 7 tip. Confirm that direction with its owner before discarding unique commits; no branches were deleted during this review.

## 3. UI migration

### What exists on `feat/ui-kit`

| Surface | Concrete evidence | Readiness |
|---|---|---|
| Semantic document/layout primitives | `internal/ebui/foundations.templ`, `components.templ`, `cmd/server/public/css/elbits.css`, bundled fonts | Page shell, skip link, headings, cards, buttons, tags and icons exist independently of `internal/ui`; no scripts are added by the default shell. |
| Forms and feedback | `internal/ebui/forms.templ`, `feedback.templ`, `navigation.templ`, `disclosure.templ` | Named controls, field errors/error summary, navigation and native disclosures exist. Not yet demonstrated through a real application mutation. |
| Tables and progressive enhancement | `internal/ebui/data.templ`, `table_query.go`, `live_table.go`; `internal/app/uikit_assets.templ`, `uikit_test.go` | GET links/forms, query parsing, sorting, pagination and optional HTMX work are implemented. The showcase distinguishes history restoration from partial requests. |
| Risk-specific composition | `internal/ebui/risk_matrix.go`, `risk_matrix.templ`, `coverage_bar.go`, `showcase_risk_register.go`, `showcase_risk_register.templ` | The proof composes real `db.ListAllRisksRow` shapes, but supplies 125 deterministic fixture rows, in-memory filtering and default thresholds. It does not read application data or offer mutations. |
| Verification | Seven `internal/ebui/*_test.go` files plus `internal/app/uikit_test.go` and `scripts/test-ebui-showcase.mjs` | Component semantics, localization, query handling and HTTP full/partial behavior have tests. The JS script uses Node's test runner with a mocked DOM; it is not a real-browser test and is not wired into Make/CI. |

**Production migration is not started on this branch:** no authored `internal/modules` source imports `internal/ebui`. Handlers, `internal/httputil/errors.go`, and `internal/app/bootstrap.go` still use `internal/ui/layout`; module templates still use old components. The component library is ready to attempt a real page, not proven to replace the application shell.

### Smallest path to switching pages

1. **Reconcile main and establish one production shell.** Compose `ebui.Page` using its existing `Head`/`Nav` slots. Preserve locale selection, current-user/module navigation, login/logout, existing request metadata and required script loading. Decide deliberately how existing mobile navigation and theme behavior carry over. Keep app-specific wiring out of the component library; no second layout framework is needed.
2. **Move the read-only risk register first.** Adapt `internal/modules/risk/handler.go:RiskRegister` and `templates/risks.templ` to the new components. Keep the real query's visibility rules and current filtering; supply configured thresholds/labels and real detail links. Reuse the proof's component composition, not its fixture provider or a new parallel production query implementation. Preserve unscored-risk handling.
3. **Prove the HTTP contract on that page.** Ordinary GETs and history-restoration requests must return full pages; ordinary HTMX requests return the intended region. Retain filter state, keyboard focus, status announcements and visible request errors. The branch's table showcase has useful prior art in `internal/app/router.go` and `cmd/server/public/js/ebui-showcase.js`; factor out only the parts the production page actually needs.
4. **Move one editable page next, then the rest in small slices.** Activities or assets can prove POST validation, preserved form values, successful redirects, empty states, and all three locales. Adapt existing behavior tests instead of adding page-wide HTML snapshots. Include the JS consumer check in the existing gate using an available runtime, and manually check keyboard/mobile/Back navigation; no new browser framework is required.
5. **Delete only after the last consumer moves.** Include error pages, risk template helper types, app asset initialization and tests when finding remaining `internal/ui` imports. Then remove obsolete components, CSS and related build targets. Remove Tailwind only when no remaining page needs its utilities; remove Alpine only if its real behaviors (including Mermaid preview) have been replaced.

**Resolved by UI migration:** old badge/button/card variants, Tailwind-heavy presentation duplication, old table markup and `internal/ui` layout presentation. This means they belong in the migration, **not that main is already fixed**. Do not separately redesign `internal/ui`, port `ui2` wholesale, or build more speculative `ebui` primitives before a production page needs them.

## 4. Improvements

Ordered by expected maintenance value relative to effort, with data-loss prevention weighted highly. **S:** roughly half a day or less; **M:** one to three focused days; **L:** several incremental changes. Each item names a completion check, not a claim that it was executed in this review.

### 1. Make generation use the tools the project installs

- **Problem:** `make setup` installs Tailwind via mise, but `css-build`, `css-watch`, `dev` and `dev-op` invoke untracked `bin/tailwindcss`. `tailwind-install` is only a phony declaration, so running it succeeds without installing anything. Docker installs templ `v0.3.1001`, while `go.mod` and mise use `v0.3.1070`. `generate` also runs an unfrozen `bun install` on every invocation.
- **Affected files:** `Makefile`, `mise.toml`, `Dockerfile`, `go.mod`, `CONTRIBUTING.md`, `.gitignore`.
- **Proposed change:** Invoke `tailwindcss` from the mise-enabled `PATH`; remove the no-op target and references. Align the Docker templ CLI with the module version. Use the existing lockfile with `bun install --frozen-lockfile` for ordinary vendoring; dependency updates remain explicit. Do not add a tool-version management abstraction.
- **Effort:** S. **Risk:** Low; changes build behavior, not application behavior.
- **Completion check:** In a fresh checkout with mise active and no `bin/tailwindcss`, generation/build work; Docker and local templ versions agree; ordinary vendoring leaves the lockfile unchanged.

### 2. Make live reload regenerate the source being edited

- **Problem:** `.air.toml` watches `.templ` files but its build command only runs `go build`, so template edits rebuild stale output. Query and migration directories are explicitly excluded; their edits do not cause sqlc generation or a server restart. `make run` also does not run a CSS watcher.
- **Affected files:** `.air.toml`, `Makefile`, `README.md`.
- **Proposed change:** Run the required templ/sqlc generation before Air's build and watch their source directories, excluding generated outputs to avoid loops. Keep dependency installation out of that loop. Document `make dev` as the CSS-watching workflow while Tailwind remains in use; make migration edits restart the app so embedded migrations are refreshed.
- **Effort:** S. **Risk:** Low to medium; a poorly chosen watch set can create rebuild loops.
- **Completion check:** Editing one `.templ` file, SQL query and CSS source changes the running app without a manual `make generate`, and each edit settles rather than repeatedly rebuilding.

### 3. Make a risk save succeed completely or fail visibly

- **Problem:** `SaveStep2` calls helpers that only log failed writes, then ignores `UpdateRiskAssessmentStep` errors and redirects. `syncStep2RiskAssets` clears links before replacement writes without a transaction. `completeDecisionFlow` ignores its state-change result and reports completion. `persistDecision` writes decision and target score separately, allowing a half-applied change.
- **Affected files:** `internal/modules/risk/handler.go` (`saveParticipants`, `SaveStep2`, `updateStep2Risks`, `syncStep2RiskAssets`, `persistDecision`, `completeDecisionFlow`), `internal/modules/risk/routes.go`, `internal/app/router.go`, `internal/modules/risk/handler_test.go`.
- **Proposed change:** Return errors to the handler; stop before success redirects/events when persistence fails. Use `database/sql` transactions with sqlc for each logically atomic write group, following the existing local pattern in `internal/modules/avvik/handler.go:withTx`. Pass the DB handle through the module constructor as Avvik does; no repository/service layer or generic transaction framework.
- **Effort:** M. **Risk:** Medium; workflow persistence semantics change, so isolate this from UI migration.
- **Completion check:** Inject a failed replacement-link/target-score write and prove old data survives; failed submission/review must not return a success flash or emit the corresponding success event.

### 4. Stop treating database outages as missing or empty data

- **Problem:** Examples include `compliance.EditRequirement`/`DeleteRequirement` and `avvik.Show`, which turn any load error into 404; `avvik.Show` discards errors from numerous detail queries. Risk form loaders also ignore user/org/settings errors. These make broken data access look like normal empty screens and make diagnosis unnecessarily difficult.
- **Affected files:** `internal/modules/compliance/handler.go`, `internal/modules/avvik/handler.go`, `internal/modules/risk/handler.go`, `internal/httpresp/errors.go`, `internal/httputil/errors.go`, `internal/httputil/errors.templ`.
- **Proposed change:** Pick one rule: `errors.Is(err, sql.ErrNoRows)` means not found; other required-query errors are logged once with operation/entity context and return 500. Optional sections may degrade only with a visible unavailable state. Use the existing localized handler error helper instead of scattered plain-text 500s. Keep low-level middleware errors independent of the session/layout package to avoid import cycles; share a small leaf templ error fragment rather than maintaining both raw `fmt.Fprintf` markup and templ markup. Add a real localized 400 case before routing bad requests through these helpers, whose current text fallback is 500.
- **Effort:** M. **Risk:** Medium; status codes and visible failure states become more accurate.
- **Completion check:** Existing fake queriers return `sql.ErrNoRows` and a distinct database error; verify 404 versus 500 and a visible full-page/HTMX failure. Do not assume HTMX displays a non-2xx fragment automatically.

### 5. Make the onboarding instructions describe this repository

- **Problem:** README and AGENTS point to root `modules/` and `templates/layout/`, and README places registry composition in `cmd/server/main.go`. Actual paths are `internal/modules/`, `internal/ui/layout/` and `internal/app/router.go`. CONTRIBUTING omits mise as a prerequisite, invokes the no-op Tailwind target and suggests installing the old golangci-lint module path manually despite mise managing v2. README's suggested shell alias for Docker is not a reliable substitute inside noninteractive Make recipes.
- **Affected files:** `README.md`, `CONTRIBUTING.md`, `AGENTS.md`, `internal/modregistry/module.go` package comment, `Makefile`.
- **Proposed change:** Keep one copyable clone/setup/generate/database/run/test sequence, with explicit Docker commands alongside the Podman path rather than relying on aliases. Remove obsolete manual tool installation instructions. Document that `make run` automatically invokes 1Password when `op` is present, or make that behavior explicit via the already-existing `run-op` target. Correct the architecture map; distinguish page view-model structs already in use from the prohibited service/DTO layers.
- **Effort:** S. **Risk:** Low.
- **Completion check:** A new contributor follows the README without existing binaries or shell aliases. The `.env.example` step still requires an authorized human to verify; it was not accessible in this review.

### 6. Remove the unused flag and honor explicit empty configuration

- **Problem:** `config.Config.AvvikEnabled` reads `AVVIK_ENABLED` but no runtime consumer uses it; actual enablement comes from database settings through `ModuleFlagsCache`. `METRICS_PATH` is documented as disabled by an empty value, but `getEnvWithDefault` treats empty as absent, so it restores `/metrics`.
- **Affected files:** `internal/config/config.go`, `internal/config/config_test.go`, `internal/config/config_nais_test.go`, `internal/middleware/middleware.go`, `README.md`.
- **Proposed change:** Remove the unused environment-backed Avvik field and tests that only prove parsing of an ineffective knob; keep the existing DB-backed toggle. For metrics, use `os.LookupEnv` locally to distinguish unset from explicitly empty. Do not globally change defaults for every variable.
- **Effort:** S. **Risk:** Low; confirm deployments are not relying on the ineffective flag.
- **Completion check:** Tests cover unset/default, empty/disabled and custom metrics paths; the admin module toggle remains the only effective Avvik setting.

### 7. Replace asynchronous template test rendering with a buffer

- **Problem:** Three `renderToDoc` helpers create `io.Pipe` plus a goroutine, discard the templ render error, and then parse HTML. These are in-memory tests; concurrency obscures failures without buying anything.
- **Affected files:** `internal/modules/compliance/templates/compliance_test.go`, `internal/modules/measures/templates/measures_test.go`, `internal/modules/dashboard/templates/dashboard_test.go`.
- **Proposed change:** Render synchronously into `bytes.Buffer`, fail on the render error, then pass the buffer to goquery. Keep the tiny local helpers; a cross-module test framework is unnecessary. Retain goquery because the existing DOM/selector assertions use it meaningfully.
- **Effort:** S. **Risk:** Low.
- **Completion check:** The three existing template test packages pass and deliberately failing render components cannot be mistaken for successful HTML.

### 8. Delete inactive plumbing, not whole working subsystems

- **Problem:** `Registry.SeedDev` duplicates the seed iteration in `cmd/seed/main.go` and is called only by its own test. `SaveModuleSettings` fetches current settings solely to preserve `PlaygroundEnabled`, even though no Playground module is registered or has tracked implementation source.
- **Affected files:** `internal/modregistry/module.go`, `internal/modregistry/module_test.go`, `cmd/seed/main.go`, `internal/modules/admin/handler.go`, `db/queries/settings.sql`, related admin tests.
- **Proposed change:** Keep the explicit seed command and `DevSeeder` implementations; remove the unused registry method and its method-only test. Stop updating the dormant Playground column in `UpdateAppSettings`, then remove the extra read/preserve path. Leave the historical column/migration alone for now: dropping it is not needed to simplify the application.
- **Effort:** S. **Risk:** Low; removing the settings query also removes an unnecessary failure point.
- **Completion check:** Seeding still calls the same modules in the same order; saving active module flags does not call `GetAppSettings` just to preserve a dead field.

### 9. Add database checks where stubs cannot prove behavior

- **Problem:** CI has no PostgreSQL service. `internal/testutil/StubQuerier` defaults to successful zero-value results; `TestCompleteDecisionFlow` checks only redirect status, and Avvik handler tests construct the handler with a nil SQL DB, so its real transaction path is not covered. Existing fake SQL drivers test wiring/session code, not PostgreSQL semantics.
- **Affected files:** `.github/workflows/ci.yml`, `internal/modules/risk/handler_test.go`, `internal/modules/avvik/handler_test.go`, `internal/testutil/stub_querier.go`, `db/migrations/`, `db/queries/`.
- **Proposed change:** Add a small opt-in integration test using CI's PostgreSQL service and existing goose/pgx tooling, not testcontainers or a mocking framework. Apply migrations to a disposable DB, run a representative risk save and Avvik mutation, and assert stored state plus rollback on a constraint failure. Strengthen critical handler stubs to record expected writes/failures rather than rewriting the whole shared stub.
- **Effort:** M. **Risk:** Low to application behavior; CI needs an isolated database and bounded startup wait.
- **Completion check:** A broken migration, failed rollback or missing expected workflow write fails the test; ordinary unit tests remain runnable without PostgreSQL.

### 10. Finish one UI migration path and one HTTP representation contract

- **Problem:** The local branches implement competing UI directions. On main, list handlers branch solely on `HX-Request`; the UI-kit showcase additionally handles `HX-History-Restore-Request` and sets `Vary`. Copying components alone will not transfer navigation, error/focus handling or back-button behavior.
- **Affected files:** `feat/ui-kit:internal/ebui/`, `internal/app/router.go`, `internal/app/uikit_assets.templ`, `cmd/server/public/js/ebui-showcase.js`, `scripts/test-ebui-showcase.mjs`; main's `internal/modules/{risk,activities,measures,assets,avvik}/handler.go`, their templates, `internal/httputil/errors.go`.
- **Proposed change:** Follow section 3. Prefer full server-rendered GET pages with progressive HTMX enhancement. Carry the proven history/full-page distinction and `Vary` behavior into migrated routes; avoid a generic table-controller layer. Move demo-only risk fixture/composition code out of the reusable library if it impedes production use, rather than expanding its public API.
- **Effort:** L overall; mergeable page-sized changes. **Risk:** Medium; navigation, accessibility and mutation behavior must be preserved.
- **Completion check:** The first real page works with scripts disabled, with HTMX filters, and with Back/Forward; the next proves failed/successful POSTs. A final deletion check includes non-module layout consumers. Existing guards remain unchanged.

### 11. Replace repeated framework-count queries with one SQL summary

- **Problem:** Both `dashboard.Dashboard` and `compliance.listPage` loop over frameworks and issue two count queries each, then repeat coverage calculation. Compliance additionally queries requirements per framework and measures per requirement, producing a deeply nested loading path.
- **Affected files:** `internal/modules/dashboard/handler.go`, `internal/modules/compliance/handler.go`, `db/queries/compliance.sql`, `db/queries/dashboard.sql`, corresponding handler tests.
- **Proposed change:** Start with one shared sqlc summary query for framework totals/covered counts, consumed directly by both handlers. If simplifying compliance detail loading next, fetch requirement-measure rows in a bounded batch and group once in Go. Preserve distinct-count and not-relevant semantics; no cache, service or new dependency.
- **Effort:** M. **Risk:** Medium; joins can accidentally multiply coverage counts.
- **Completion check:** Frameworks with zero requirements, excluded requirements and multiple linked implemented measures produce the same totals, with a constant number of summary queries rather than two per framework.

### 12. Split the risk handler by existing workflow, not by layer

- **Problem:** `internal/modules/risk/handler.go` has 2,109 lines covering assessment creation, identification, linking, reassessment, review and per-risk decisions. Persistence helpers such as `persistDecision` also write HTTP responses, making it unclear which caller owns error handling.
- **Affected files:** `internal/modules/risk/handler.go`, `internal/modules/risk/handler_test.go`.
- **Proposed change:** After the persistence corrections, move existing methods into a few same-package workflow files, such as assessment/wizard, decision and linking. Keep the same `Handler` and direct sqlc access. Make persistence helpers return errors and leave response ownership to the calling handler. Do not split every method, or extract a service merely to reduce line counts.
- **Effort:** M. **Risk:** Low for file moves; medium for response-ownership changes, which should be separate.
- **Completion check:** Route registration is unchanged and focused workflow/error tests preserve behavior.

### 13. Use one localization convention for handler-owned text

- **Problem:** Assets already calls `locale.T` for flashes, while activities/measures/compliance/risk frequently send English literals. Layout callers mix translation keys such as `page_dashboard_title`, literal labels and user data; `layout.Layout` writes its `title` argument directly into the document title. This can expose raw message keys and makes translation ownership unclear.
- **Affected files:** `internal/modules/assets/handler.go` (existing pattern), `internal/modules/{activities,measures,compliance,risk,dashboard}/handler.go`, `internal/locale/locales/active.{en,nb,nn}.toml`; the production page-shell integration described in section 3.
- **Proposed change:** Translate fixed titles/flash messages in handlers, and pass display text to layouts. Never translate database titles as message IDs. Reuse existing locale keys and add only missing messages. Carry that rule into the new shell; old-shell presentation changes are **resolved by UI migration**, not a second layout project.
- **Effort:** M. **Risk:** Low.
- **Completion check:** A representative success/error workflow and document title render correctly in English, Bokmal and Nynorsk without exposing locale keys.

### Dependency decisions: keep the useful parts

There is no evidence-backed direct-library deletion worth making solely to shrink `go.mod`:

- `goquery` is used by DOM tests; `bytes.Buffer` replaces their concurrency, not their HTML parser.
- `go-i18n`, TOML and `x/text` serve the existing three-language bundles. Replacing them would create custom localization work.
- `godotenv` is used by both executable entrypoints; Go has no equivalent dotenv parser. Remove it only if the project deliberately moves all environment loading out of the executables.
- SCS, OIDC, OPA, goose, pgx and the observability libraries implement existing responsibilities. Keep them; particularly do not bundle policy changes into a maintainability cleanup.
- Mermaid is used by risk information-flow previews, and Alpine provides real page behavior in `cmd/server/public/js/alpine-data.js`. Neither is currently dead. Tailwind/Alpine removal is conditional on finishing the chosen migration, not on a preference for fewer dependencies.

The stdlib opportunities here are smaller and safer: buffers instead of goroutines/pipes, `os.LookupEnv` for empty-vs-unset values, and transactions/query batching using the database tooling already installed.

## 5. Quick wins

Each is a separate task that should fit under an hour; the full UI/DB work above does not.

| Task | Files / scope | Done when |
|---|---|---|
| Stop requiring a private Tailwind binary | `Makefile`, stale `tailwind-install` references | CSS targets use the mise-installed command; the no-op target is gone. |
| Align Docker's templ CLI | `Dockerfile` | Its pinned CLI matches `go.mod` and mise. |
| Make routine JS installs locked | `Makefile:js-vendor` | Existing lockfile is honored without resolution changes. |
| Replace template-test pipes | The three `templates/*_test.go` files in item 7 | Rendering is synchronous and errors are checked. |
| Honor an empty metrics path | `internal/config/config.go`, `config_test.go` | Unset, empty and custom values have explicit tests. |
| Remove the unused registry seed method | `internal/modregistry/module.go`, `module_test.go` | Real `cmd/seed` behavior remains unchanged. |
| Correct the architecture map | `README.md`, `AGENTS.md`, `internal/modregistry/module.go` | Paths and composition location match the tree. |
| Delete fully merged local pointers | `component-library`, `jonmartinstorm-refactored-barnacle` | Reconfirm ancestry, then use ordinary safe branch deletion. Leave the two divergent UI branches until reconciliation. |

Implement subsequent code changes in small independent patches and run the repository's `make pre-commit-fast` gate after fixing its tool-path issue. Do not run that mutating generation/tidy command merely to validate this documentation-only review.
