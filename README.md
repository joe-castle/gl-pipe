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
| `f` | Open the blob (code) search filter (group + query in separate fields — see below) |
| `b` | Search for pipelines by an exact ref across every synced project — no need to know which repo it's in |
| `m` | Your merge requests (assigned + authored, open) across every synced project |
| `v` | Pick a variable preset to prefill the next trigger |
| `g` | Discover groups you belong to and add some to `default_groups` |
| `s` | Open settings — switch instance profile, view TTL/presets |
| `o` | Open the focused project, pipeline, or job in your browser |
| `r` | Force refresh — re-sync the project cache from GitLab |
| `q` | Quit |

### Project explorer

| Key | Action |
|---|---|
| `/` | Filter projects by path (fuzzy, with the extended syntax below) |
| `x` | Toggle multi-select on the highlighted project |
| `a` | Stage/unstage all — respects the current `/` filter |
| `T` | Lock the staged (or highlighted) project(s) to their latest SemVer tag — press again to unlock any of them that are currently locked |
| `t` | Same as `T`, but locks to the most recently *created* tag instead of the highest SemVer — for repos that don't tag with version numbers |
| `Ctrl+r` | Browse available refs (branches + tags) for the staged (or highlighted) project(s) and lock them all to whichever one you pick |
| `Enter` | View pipelines for the staged (`x`) projects together, or just the highlighted one if none are staged |
| `M` | Open merge requests for the staged (or highlighted) project(s) |

To lock every project matching a filter to its latest tag: `/` to filter, `a` to stage all of them, `T` (SemVer) or `t` (most recently created) to lock. Pressing the same key again unlocks — it targets whatever in the batch is currently locked, so one project with no qualifying tag (left on its default ref) won't block unlocking the rest.

`Ctrl+r` covers the case those two can't: an arbitrary ref, or a ref that isn't "latest" by either definition. It fetches branches+tags for the first staged (or highlighted) project and applies your pick to every project in the batch — so a common flow is `T` to lock most of a group to its latest tag, then stage just the few that need something else and `Ctrl+r` to point them at a specific branch or hotfix tag instead. Triggering a pipeline (`<Space> p`) already honors whatever's locked here, same as `T`/`t`.

#### Filter syntax

The `/` box is space-separated tokens, all of which must hold (AND). Most tokens are plain fuzzy terms (letters just need to appear in order somewhere in the path — same as before), but two prefixes carry special meaning, borrowed from fzf's extended search:

| Token | Meaning |
|---|---|
| `word` | Fuzzy subsequence match (default) — ranked, best match first |
| `group/` | Direct children of `group` only — excludes anything nested under a subgroup. This is an exact, non-fuzzy prefix check, since "not a subgroup" has no fuzzy equivalent |
| `!word` | Exclude any project whose path contains `word` |

They combine: `backend/ !legacy` finds direct children of `backend`, minus anything with "legacy" in the name — the pattern for "all the services in a group, not its subgroups, minus a few I don't want" without hand-picking each one. A query using only `group/`/`!word` tokens (no plain term) lists matches in cache order, since there's nothing to rank by.

### Blob (code) search modal

| Key | Action |
|---|---|
| `Tab` | Switch between the group and query fields |
| `Enter` | Run the search |
| `Esc` | Cancel |

Opened via `<Space> f`. Two separate fields — group and query — not one combined string, so the query can freely use GitLab's own search qualifiers without ambiguity. The group field starts prefilled from the active instance's first `default_groups` entry; clear or edit it to search elsewhere.

The search is scoped **only by the group field** — it calls GitLab's group-scoped blob search API directly and searches every project in that group (recursively through subgroups), regardless of anything filtered or staged in the project explorer. Pre-filtering the explorer with `/` before opening blob search has no effect on what gets searched.

The query is passed to GitLab's blob search unmodified, so its qualifiers work exactly as GitLab defines them:

- `path:src/main` — repo-location match (substring, not a glob: `path:src/main` matches anywhere `src/main` appears in the path, but `src/main/**/*.java` is not valid syntax)
- `filename:*.java` — filename match, `*` wildcard supported
- `extension:java` — exact extension match, no leading dot

These qualifiers are part of GitLab's **Advanced Search** (Elasticsearch), which requires a Premium/Ultimate self-hosted instance with Elasticsearch enabled, or GitLab.com. On a basic (non-Elasticsearch) instance they won't work as filters — GitLab will likely treat them as literal search text and return nothing. There's no way to detect this from the API response alone; if a qualified search comes back empty, try the same query without the qualifiers to check whether it's an Advanced Search gap rather than a "no matches" result.

### Group discovery modal

| Key | Action |
|---|---|
| `/` | Fuzzy-filter the group list |
| `x` / `Enter` | Toggle the highlighted group |
| `a` | Select/deselect all — respects the current `/` filter |
| `Enter` on `[ Save ]` | Merge selected groups into `default_groups` and resync |
| `Esc` | Cancel without saving |

### Merge request modal

| Key | Action |
|---|---|
| `/` | Fuzzy-filter by title, branch, project, or author |
| `x` | Stage/unstage the highlighted MR |
| `a` | Stage/unstage all — respects the current `/` filter |
| `Enter` | Jump to pipelines for the staged (or highlighted) MR(s) — feeds straight into the pipeline matrix below, so sort/filter/bulk-retry all apply |
| `Esc` | Cancel |

Opened via `<Space> m` ("my MRs" — assigned + authored, open, across every synced project) or `M` on the explorer (a specific project's open MRs). Either way, the point is retrying a pile of interrelated failed pipelines without leaving the keyboard: pull up the MRs, stage the ones you care about, `Enter` to see their pipelines together, `R` to bulk-retry the failed ones.

### Pipeline matrix

| Key | Action |
|---|---|
| `x` | Stage/unstage a pipeline for a bulk action |
| `a` | Stage/unstage all — respects the current `/` filter |
| `Enter` | View job matrices for the staged pipelines together, or just the highlighted one if none are staged |
| `R` | Bulk retry the staged (or highlighted) pipeline(s) |
| `K` | Bulk cancel the staged (or highlighted) pipeline(s) |
| `s` | Cycle the sort column (Date → Project → Status → Ref → SHA → Author → Duration) |
| `S` | Reverse sort direction without changing column |
| `/` | Filter by ref, status, project, author, or SHA (substring, case-insensitive) |
| `r` | Refresh now, without waiting for the next automatic poll |

Sorted by Date, newest first, by default. `/` narrows what's *already loaded* into the matrix — useful for narrowing to `/failed` or a ref you know is in there. Staging (`x`) is independent of the filter: pipelines you've staged stay staged even if a filter later hides them from view.

If you don't know which project a branch lives in, `/` won't find it — it only searches what's currently loaded. Use `<Space> b` instead: it queries every synced project directly by exact ref, so you don't need to stage (or even know) the right repo first.

The matrix polls automatically every 10s while any pipeline shown hasn't reached a final status (success/failed/canceled/skipped/manual) — no need to bounce back to the explorer and re-trigger `Enter` to see a running pipeline update. Polling pauses on its own once everything settles, and never runs while a fetch is already in flight. Press `r` any time for an immediate refresh instead of waiting for the next tick.

### Job matrix

| Key | Action |
|---|---|
| `x` | Stage/unstage a job for a bulk action |
| `a` | Stage/unstage all jobs currently loaded |
| `Enter` | Stream that job's live log |
| `R` | Bulk retry the staged (or highlighted) job(s) |
| `K` | Bulk cancel the staged (or highlighted) job(s) |
| `r` | Refresh now, without waiting for the next automatic poll |
| `Esc` | Back to the pipeline matrix |

Showing jobs from more than one pipeline adds `Project` and `Pipeline` (`#IID`) columns so you can tell which job belongs to which run. The same automatic polling as the pipeline matrix applies here, keyed off the jobs' own statuses rather than the parent pipeline's.

### Pipeline trigger modal

| Key | Action |
|---|---|
| `Tab` | Switch focus between the ref field and the variable table |
| `Ctrl+r` | Browse available refs (branches + tags) for the first staged project and pick one |
| `a` | Add a variable row |
| `d` | Delete the highlighted variable row |
| `t` | Toggle the row's type (`env_var` / `file`) |
| `m` | Toggle `masked` |
| `p` | Toggle `protected` |
| `Enter` | Edit a row's key/value, or dispatch on the `[ Dispatch ]` row |
| `Esc` | Cancel without dispatching (or, in the ref browser, close it without changing the ref) |

`Ctrl+r` fetches branches and tags for the *first* staged project — the ref field is shared across every project in a batch trigger, so with multiple projects staged the picker reflects only one of them; if the others differ, type the ref manually or lock each project's ref individually (`T`) before opening the trigger modal.

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
