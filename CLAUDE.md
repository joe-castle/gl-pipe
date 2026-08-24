# Project Context & AI Working Memory

## 1. Project Mission & Overview
- **Project Name**: gl-pipe
- **Mission**: A single-binary, keyboard-first TUI for managing, searching, inspecting, and batch-triggering GitLab CI/CD pipelines across multiple repositories at once, against both GitLab.com and self-hosted instances.

### Overview
Triggering, inspecting, retrying, and cancelling pipelines across many repos today means many browser tabs. gl-pipe collapses that into one Helix-style terminal interface: multi-select repos in a fuzzy-filterable explorer, fire a parameterized pipeline at all of them at once, then watch the resulting multi-project pipeline/job matrix and stream logs — all from the keyboard.

---

## 2. Tech Stack & Run Commands
- **Language & Runtime**: Go 1.26 (module requires 1.23+), no CGO anywhere in the build
- **Frameworks & Libraries**: `charmbracelet/bubbletea` v1.3.10, `bubbles` v1.0.0, `lipgloss` v1.1.0, `gitlab.com/gitlab-org/api/client-go` v1.46.0, `alecthomas/kong`, `gopkg.in/yaml.v3`, `sahilm/fuzzy`, `golang.org/x/mod/semver`
- **Testing Framework**: standard `testing` package + `httptest` for the API layer

### Quick Commands
- **Install Dependencies**: `go mod download`
- **Run Dev Build**: `go run ./cmd/gl-pipe`
- **Run All Tests**: `go test ./...`
- **Run With Race Detector**: `go test ./... -race` (requires a C toolchain — CGO_ENABLED=1 + gcc/clang; **not available in the sandbox this was built in**, so the race detector has not actually been run against this code yet)
- **Run Single Test**: `go test ./internal/ui/components/ -run TestVariables_DispatchEmitsProjectsRefAndVars -v`
- **Lint/Format**: `gofmt -l .` (should print nothing), `go vet ./...`
- **Build**: `go build -o gl-pipe.exe ./cmd/gl-pipe` (Windows) / `go build -o gl-pipe ./cmd/gl-pipe` (Unix)

### Local Config
First run writes `%APPDATA%\gl-pipe\config.yaml` (Windows) or `~/.config/gl-pipe/config.yaml` (Unix). Per-instance project caches live alongside it as `cache-<instance>.json`. Use `--config <path>` to point at an alternate file for testing without touching the real one.

---

## 3. Architecture & Invariants
- **Pattern**: Elm-architecture TUI (Bubbletea) over a thin GitLab API wrapper, feature-organized under `internal/`.
- **Rules & Invariants**:
   1. No goroutine ever touches model state directly. Every GitLab call is a `tea.Cmd` closure returning a `tea.Msg`; all mutation happens in `Model.Update`. This is the entire race-freedom story — see `internal/ui/commands.go`.
   2. Async responses carry a monotonic `reqID`. `Update` drops any message whose `reqID` is older than the pane's current generation (`genProjects`, `genPipelines`, `genJobs`, `genLogs` in `internal/ui/model.go`), so an abandoned request can't clobber fresher state. Tested in `internal/ui/model_test.go`.
   3. Log streaming is a channel + self-rescheduling `tea.Cmd`: `startLogStreamCmd` launches a producer goroutine (`api.Client.StreamJobTrace`) writing to a channel, and `waitForLogChunkCmd` blocks on one receive per call, re-issuing itself until the job reaches a terminal status or the request is canceled. GitLab's trace endpoint has no incremental API — each poll returns the full trace, and the diff-to-suffix happens client-side in `StreamJobTrace`.
   4. Key dispatch precedence is strict: active modal (`variables` / `settings` / `presets` / `logViewer`) → leader menu → focused pane → global. `<Space>` only opens the leader menu when the focused pane reports no text input focused (`HasTextFocus()` on `ProjectList`, `Variables`, `LogViewer`). See `internal/ui/dispatch.go`.
   5. `internal/api` returns domain structs (`api.Project`, `api.Pipeline`, `api.Job`, ...) — never `gitlab.*` SDK types — so the UI layer stays decoupled and the API layer is unit-testable against `httptest` without touching Bubbletea.
   6. Business logic lives in `internal/`; `cmd/gl-pipe/main.go` only parses flags and starts the program.
   7. The project explorer only ever syncs from an instance's configured `default_groups` — there is no "list every project on the instance" path.

### Known simplifications (intentional, not oversights)
- `gx` as an alternate open-in-browser chord was dropped: bubbletea delivers `g` and `x` as two independent `KeyMsg`s, and `g` is already bound to "jump to top" per the nav spec, so a chord would need pending-prefix state machinery not otherwise justified by scope. `<Space> o` is the only bound path to open-in-browser.
- Batch pipeline trigger uses one shared `ref` field in the trigger modal, overridden per-project only when that project has a `T`-locked tag (`ProjectList.LockedRef`). There's no per-project ref editing inside the trigger modal itself.
- Retry counts in the job matrix are computed client-side (position within same stage+name, since GitLab's job list API doesn't expose a retry count field directly) — see `api.Client.ListJobs`.
- Settings (`<Space> s`) supports switching instances; it does not yet support editing presets or TTL in-place (both are read-only display there — edit `config.yaml` directly).

---

## 4. Active Backlog

| Status | Effort | # | Title | Description |
|--------|--------|---|-------|-------------|
| [DONE] | [XL] | 001 | **Project scaffold, config, cache, API layer** | Go module, YAML config with token indirection, JSON project cache with TTL/fuzzy ranking, client-go wrapper with domain types. Full `httptest` coverage. |
| [DONE] | [XL] | 002 | **Bubbletea UI: wizard, explorer, pipeline/job matrix, log viewer, leader menu, settings** | All views wired end-to-end against the API layer with generation-counted async dispatch. |
| [DONE] | [S]  | 003 | **README + keybinding table + cross-compile docs** | — |
| [TODO] | [M] | 004 | **Live-instance smoke test** | Every flow was verified by unit test and by launching the built binary (wizard renders, explorer renders against a stubbed config), but never driven end-to-end against a real GitLab instance — no PAT was available in the build environment. Do this before relying on it for real triggers. |
| [TODO] | [S] | 005 | **Run `go test ./... -race`** | The sandbox this was built in has no C toolchain (`CGO_ENABLED=1` requires gcc/clang), so `-race` has never actually executed against invariant #1. Run it on a machine with a C toolchain before trusting the race-freedom claim beyond code review. |
| [TODO] | [S] | 006 | **Settings: in-place preset/TTL editing** | Currently read-only in the `<Space> s` screen; editing requires hand-editing `config.yaml`. |
| [TODO] | [XS] | 007 | **textinput cursor blink** | `Model.Init()` doesn't return a blink `tea.Cmd` for the wizard/filter/search inputs — cursors are static rather than blinking. Cosmetic only. |

---

## 5. Decision Log (ADRs)

| Date | Title | Context | Decision | Rationale |
|------|-------|---------|----------|-----------|
| 2026-08-24 | Use `gitlab.com/gitlab-org/api/client-go` instead of `xanzy/go-gitlab` | The original spec named `xanzy/go-gitlab`, which is archived and frozen at v0.115.0 (Dec 2024) | Use GitLab's official successor module, v1.46.0 | Actively maintained, near-identical API surface, current endpoint coverage (blob search, pipeline variables, job traces) |
| 2026-08-24 | Flat JSON project cache instead of SQLite | Spec allowed either | Plain JSON file loaded into memory, fuzzy-filtered in-process via `sahilm/fuzzy` | Zero extra deps, no CGO, instant for realistic project-list sizes (tens of thousands); `modernc.org/sqlite` would add real binary bloat for no benefit at this scale |
| 2026-08-24 | Token indirection beyond plaintext YAML | Spec's config schema stores PATs in plaintext | Support `token: literal`, `token: ${ENV_VAR}`, and `token_command: <cmd>` (highest precedence); config written 0600 | Plaintext-only was the spec default but a PAT sitting readable on disk was worth avoiding cheaply; OS keyring was considered but rejected as an added dependency + cross-platform failure surface not justified for a first pass |
| 2026-08-24 | Bubbletea v1.3.10, not v2 | bubbletea v2.0.9 exists on the proxy | Pin to the v1 line | `bubbles` v1.0.0 and the wider component ecosystem target bubbletea v1; v2 is new enough that component compatibility wasn't worth the risk for this build |
