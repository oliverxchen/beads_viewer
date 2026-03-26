# Kanban Board Export (`bv-kanban`)

A standalone binary that exports a single-page Kanban board from your Beads project. It's a separate binary from `bv`, producing a much smaller build (~4MB vs ~48MB) since it doesn't include the TUI, graph analysis, SQLite, or viewer assets.

## Usage

```bash
# Basic export (outputs to ./.kanban/ by default)
bv-kanban

# Custom output directory
bv-kanban -output ./sprint-board

# With custom title
bv-kanban -title "Sprint 42 Board"

# With custom tab icon from a local file (copied into output as favicon.<ext>)
bv-kanban -favicon ./assets/sprint-42.ico

# Exclude closed/tombstone tasks
bv-kanban -include-closed=false

# Point to a specific beads file or directory
bv-kanban -db /path/to/.beads

# Skip hooks
bv-kanban -no-hooks

# Watch mode: rebuild on Beads file changes, serve locally, and auto-refresh browser
bv-kanban -watch

# Watch mode with explicit port and no browser auto-open
bv-kanban -watch -watch-port 9015 -no-browser
```

Open `kanban/index.html` in any browser — no server required.

Or use `-watch` to run a local preview server that auto-rebuilds and live-refreshes.

## Watch Mode

`bv-kanban -watch` performs an initial export, starts a local server, and then:

- Watches Beads JSONL files (including `.beads/issues.jsonl`) for changes
- Rebuilds `index.html` and board data when input files change
- Live-refreshes connected browser tabs when `index.html` changes

Flags:
- `-watch` enables watch mode
- `-watch-port` sets the preview port (`0` = auto-select in `9000-9100`)
- `-no-browser` disables browser auto-open

## Building from Source

### Prerequisites

Install Go 1.25+ (the project requires it via `go.mod`):

```bash
brew install go
```

### Build and install

```bash
# From the repo root
cd ~/src/beads_viewer

# Build and install to your GOPATH/bin (usually ~/go/bin)
GOTOOLCHAIN=auto go install ./cmd/bv-kanban/

# For a smaller binary (~4MB), strip debug symbols:
go build -ldflags="-s -w" -o ~/go/bin/bv-kanban ./cmd/bv-kanban/

# Make sure ~/go/bin is in your PATH (add to ~/.zshrc if not already there)
export PATH="$HOME/go/bin:$PATH"

# Verify
bv-kanban -help
```

## Publish A Rolling GitHub Release (Manual)

If you want a single public release endpoint (no version bumps), use:

```bash
# From repo root
./scripts/publish_bv_kanban_release.sh
```

This script:
- Builds a minimal binary (`CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, `-ldflags "-s -w -buildid="`)
- Targets `linux/amd64` by default (override with `GOOS`/`GOARCH`)
- Publishes to a fixed release tag: `bv-kanban-latest`
- Overwrites the asset on each run

Public URLs become stable:
- Release page: `https://github.com/<owner>/<repo>/releases/tag/bv-kanban-latest`
- Direct download (example for Linux AMD): `https://github.com/<owner>/<repo>/releases/download/bv-kanban-latest/bv-kanban_linux_amd64`

Optional flags:

```bash
./scripts/publish_bv_kanban_release.sh -repo oliverxchen/beads_viewer
./scripts/publish_bv_kanban_release.sh -tag my-kanban-latest -title "bv-kanban latest"
GOOS=linux GOARCH=amd64 ./scripts/publish_bv_kanban_release.sh
```

## Columns

The board renders exactly six columns in fixed left-to-right order:

| Column | Raw statuses mapped here |
|--------|--------------------------|
| **Draft** | `draft` |
| **Blocked** | `blocked` (raw), `open` with active blocking deps (computed) |
| **Open** | `open`, `pinned`, `hooked`, `deferred` |
| **In Progress** | `in_progress` |
| **In Review** | `review` |
| **Closed** | `closed`, `tombstone` |

Unknown/unrecognized statuses fall back to **Open**.

## Blocking Rules

A task is promoted from **Open** to **Blocked** when:

1. Its raw status is `open`, **and**
2. It has at least one `blocks`-type dependency on another **task** whose status is in the blocking set

**Kanban blocking statuses:** `draft`, `open`, `in_progress`, `blocked`, `deferred`, `pinned`, `hooked`

**Kanban non-blocking statuses:** `review`, `closed`, `tombstone`

Key behaviors:
- Only `task`-type blockers count — bugs, epics, etc. are ignored for blocking evaluation
- `review` status is **non-blocking** — a task blocked only by `review`-status tasks stays in Open
- Raw `status == blocked` tasks always remain in Blocked even if no active blockers are found
- Unknown/custom blocker statuses are treated as blocking (a warning is emitted in `meta.warnings`)
- Missing blocker issues are silently ignored

Each blocked card displays a compact list of its blocking tasks with ID, title, and raw status.

## Card Rules

- Only `issue_type == task` issues appear as top-level cards
- Tasks that are children of another task (via `parent-child` dependency) are excluded from top-level
- Child issues with `issue_type == subtask` appear inside their parent task's expandable section
- Bugs, features, epics, and other types never appear as cards

## Detail Panel

Clicking a card opens a right-side detail panel showing:
- Task header (ID, priority, status), title, metadata (assignee, dates)
- Labels and parent epic tags
- Blocked-by list (for blocked cards)
- Description, Design, Acceptance Criteria, Notes — rendered as markdown
- Comments with author, date, and markdown body

Clicking a subtask in the card's expandable section opens the subtask's own detail panel with the same layout.

The board auto-scrolls to keep the clicked card visible next to the panel. If there isn't enough room to scroll, extra space is added temporarily. Close the panel with ✕, clicking the backdrop, or pressing Escape.

## Sorting

**Top-level cards** in `draft`, `blocked`, `open`, `in_progress`, and `review`:
1. `priority` ascending (P0 first)
2. `blocking_tasks_count` descending (direct downstream tasks unblocked by this task)
3. `blocked_by_tasks_count` ascending
4. `created_at` ascending
5. Final tie-break: `id` ascending (deterministic)

**Top-level cards** in `closed`:
1. `updated_at` descending (most recently updated first)
2. Tie-break: `created_at` descending
3. Final tie-break: `id` ascending (deterministic)

**Subtasks** inside an expanded card:
1. Ordered by validated linear blocking dependency chain (prerequisite first)
2. If the chain invariant is violated, falls back to `id` ascending with a warning

## Filtering

The page-level filter uses **substring matching** (contains):

- Case-insensitive after normalization (trim whitespace, collapse inner spaces)
- A card matches if the normalized query is contained in any of:
  - The task's ID (lowercased)
  - The task's normalized title
  - Any parent epic's normalized title
  - Any parent epic's ID (lowercased)
- When an epic title or ID matches, **all** tasks under that epic appear
- Subtasks do not drive filter results

## Output Structure

```
.kanban/
├── index.html              # Standalone page (with inlined JSON data)
├── kanban.js               # Board rendering, detail panel, filtering
├── kanban.css              # Dark theme styling
├── vendor/
│   └── marked.min.js       # Markdown renderer
└── data/
    └── kanban_board.json   # Pre-built board data
```

No SQLite database, no chunked data.

## Intentional Divergences from `bv --export-pages`

| Feature | `--export-pages` | `bv-kanban` |
|---------|-------------------|-------------|
| Binary | Full `bv` (~48MB) | Standalone (~4MB) |
| Database | SQLite + chunked JSON | Single JSON file |
| Graph analysis | Full PageRank/betweenness | None |
| Triage | Computed | None |
| History/time-travel | Included | Not included |
| Preview server | `--preview-pages` | Built in via `-watch` |
| Wizard | `--pages` | Not supported |
| Watch mode | `--watch-export` | Built in via `-watch` |
| README generation | Auto-generated | Not included |
| Card types shown | All issues | Tasks only |
| `cmd/bv/main.go` changes | N/A | None (zero rebase friction) |

## Hooks

Pre-export and post-export hooks from `.bv/hooks.yaml` run the same way as `--export-pages`:
- `BV_EXPORT_FORMAT` is set to `html`
- `BV_EXPORT_PATH` points to the output directory
- Use `-no-hooks` to skip

## Implementation Files

| File | Role |
|------|------|
| `cmd/bv-kanban/main.go` | Standalone CLI binary |
| `pkg/kanban/kanban_export.go` | Data builder (status mapping, blocking, epics, sorting) |
| `pkg/kanban/kanban_bundle.go` | Bundle writer (JSON + asset copy) |
| `pkg/kanban/kanban_embed.go` | Embedded asset directive |
| `pkg/kanban/kanban_assets/` | Standalone HTML/JS/CSS frontend + vendor |
| `pkg/kanban/kanban_export_test.go` | Unit tests for data builder |
| `pkg/kanban/kanban_bundle_test.go` | Unit tests for bundle writer |
