# gl-pipe

A blazingly fast, single-binary, keyboard-first terminal UI for managing, searching, inspecting, and batch-triggering GitLab CI/CD pipelines across multiple repositories at once. Works against GitLab.com and self-hosted/managed instances, with instant switching between profiles.

Built with [Bubbletea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles), and [Lipgloss](https://github.com/charmbracelet/lipgloss), against GitLab's official [client-go](https://gitlab.com/gitlab-org/api/client-go) SDK.

## Build

Requires Go 1.23+.

```powershell
go build -o gl-pipe.exe .\cmd\gl-pipe
```

```bash
go build -o gl-pipe ./cmd/gl-pipe
```

Run the test suite:

```bash
go test ./...
```

### Cross-compilation

```bash
GOOS=linux   GOARCH=amd64 go build -o gl-pipe-linux   ./cmd/gl-pipe
GOOS=darwin  GOARCH=arm64 go build -o gl-pipe-macos   ./cmd/gl-pipe
GOOS=windows GOARCH=amd64 go build -o gl-pipe.exe     ./cmd/gl-pipe
```

No CGO is used anywhere in the build, so these cross-compiles work from any host without a C toolchain.

## First run

On first launch (no `config.yaml` found under the OS config directory — `~/.config/gl-pipe/` on Linux/macOS, `%APPDATA%\gl-pipe\` on Windows), gl-pipe opens an onboarding form: enter your instance URL (defaults to `https://gitlab.com`) and a personal access token. The token is validated against the instance's `/api/v4/user` endpoint before anything is written to disk.

### CLI flags

| Flag | Purpose |
|---|---|
| `--url` | Override the active profile's instance URL for this run |
| `--token` | Override the active profile's token for this run |
| `--config` | Use a config file at a non-default path |
| `--profile` | Activate a specific instance profile for this run |

### Config file

```yaml
current_instance: work
instances:
  work:
    url: "https://gitlab.internal.mycompany.com"
    token: "glpat-xxxxxxxxxxxxxxxxxxxx"
    default_groups:
      - "core-services"
      - "infrastructure"
  personal:
    url: "https://gitlab.com"
    token: "${GITLAB_TOKEN}"       # expanded from the environment
cache:
  ttl_minutes: 60
presets:
  deploy_dev:
    variables:
      DEPLOY_ENV: "development"
      SKIP_SMOKE_TESTS: "false"
```

A token can be a literal PAT, an `${ENV_VAR}` reference expanded at load time, or a `token_command: <shell command>` whose stdout is used as the token (e.g. to pull from a password manager). `token_command` takes precedence over `token` when both are set. The config file itself is written with owner-only (`0600`) permissions.

The project explorer only syncs projects from an instance's `default_groups` — there's no unscoped "list every project on the instance" mode, to keep sync fast and predictable on large instances. You don't have to hand-type these: press `<Space> g` to fetch every group you're a member of and multi-select the ones you want (`x` to toggle, `Enter` on the `[ Save ]` row) — selections are merged into `default_groups` and gl-pipe resyncs immediately. If an instance has no `default_groups` configured yet, the status bar says so explicitly and points you at `<Space> g`.

## Keyboard shortcuts

### Global navigation

| Key | Action |
|---|---|
| `j` / `↓` | Down |
| `k` / `↑` | Up |
| `g` | Top |
| `G` | Bottom |
| `Tab` | Next pane (Explorer ↔ Pipelines) |
| `Shift+Tab` | Previous pane |
| `Ctrl+d` | Half-page down |
| `Ctrl+u` | Half-page up |
| `Esc` | Dismiss modal / go back |
| `Ctrl+c` | Quit immediately |
| `Space` | Open the leader menu (only when no text input is focused) |

### Leader menu (`Space` then...)

| Key | Action |
|---|---|
| `p` | Open the pipeline trigger modal for the staged (or highlighted) repo(s) |
| `f` | Open the blob (code) search filter, scoped to a group |
| `v` | Pick a variable preset to prefill the next trigger |
| `g` | Discover groups you belong to and add some to `default_groups` |
| `s` | Open settings — switch instance profile, view TTL/presets |
| `o` | Open the focused project, pipeline, or job in your browser |
| `r` | Force refresh — re-sync the project cache from GitLab |
| `q` | Quit |

### Project explorer

| Key | Action |
|---|---|
| `/` | Fuzzy-filter projects by path |
| `x` | Toggle multi-select on the highlighted project |
| `T` | Lock the highlighted project's ref to its latest SemVer tag |
| `Enter` | Drill into that project's pipelines |

### Group discovery modal

| Key | Action |
|---|---|
| `/` | Fuzzy-filter the group list |
| `x` / `Enter` | Toggle the highlighted group |
| `Enter` on `[ Save ]` | Merge selected groups into `default_groups` and resync |
| `Esc` | Cancel without saving |

### Pipeline matrix

| Key | Action |
|---|---|
| `x` | Stage/unstage a pipeline for a bulk action |
| `Enter` | View the job matrix for the highlighted pipeline |
| `R` | Bulk retry the staged (or highlighted) pipeline(s) |
| `K` | Bulk cancel the staged (or highlighted) pipeline(s) |
| `s` | Cycle the sort column (Date → Project → Status → Ref → SHA → Author → Duration) |
| `S` | Reverse sort direction without changing column |

Sorted by Date, newest first, by default.

### Job matrix

| Key | Action |
|---|---|
| `x` | Stage/unstage a job for a bulk action |
| `Enter` | Stream that job's live log |
| `R` | Bulk retry the staged (or highlighted) job(s) |
| `K` | Bulk cancel the staged (or highlighted) job(s) |
| `Esc` | Back to the pipeline matrix |

### Pipeline trigger modal

| Key | Action |
|---|---|
| `Tab` | Switch focus between the ref field and the variable table |
| `a` | Add a variable row |
| `d` | Delete the highlighted variable row |
| `t` | Toggle the row's type (`env_var` / `file`) |
| `m` | Toggle `masked` |
| `p` | Toggle `protected` |
| `Enter` | Edit a row's key/value, or dispatch on the `[ Dispatch ]` row |
| `Esc` | Cancel without dispatching |

### Log viewer

| Key | Action |
|---|---|
| `/` | Search the buffered log |
| `n` / `N` | Next / previous search match |
| `E` | Jump to the first error/failure/panic line |
| `G` | Jump to the live tail |
| `Esc` | Back to the job matrix |

## Architecture

```
cmd/gl-pipe/          Kong CLI flags, first-run routing, tea.NewProgram
internal/api/         client-go wrapper — returns domain types, never gitlab.* structs
internal/cache/       flat JSON project index, TTL + fuzzy ranking
internal/config/      YAML config, multi-instance profiles, token resolution
internal/ui/          root Bubbletea model, key dispatch, Cmd factories
internal/ui/components/  wizard, project list, pipeline/job matrix, variable
                          editor, log viewer, leader menu, settings
```

Every GitLab API call is wrapped in a `tea.Cmd` closure so it runs on its own goroutine and reports back as a `tea.Msg` — the Bubbletea `Update` loop is the only place model state is ever mutated, which is what keeps 60fps rendering non-blocking and free of races (`go test ./... -race`, given a C toolchain, exercises this directly against the log-streaming goroutine). Async responses carry a monotonic request ID; `Update` drops any reply whose ID is older than the current generation for that pane, so an abandoned filter or stale project sync can never clobber fresher state.
