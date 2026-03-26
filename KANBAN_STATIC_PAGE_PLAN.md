# Kanban Static Page Plan for `bv --export-kanban`

## 1. Objective

Build a simplified static export experience as a completely separate fork-only command:

1. `bv --export-kanban <output-dir>`

The output should be a single-page Kanban board focused on `task` issues, with columns:

1. `draft`
2. `blocked`
3. `open`
4. `in_progress`
5. `review`
6. `closed`

Behavior requirements:

1. Cards shown in columns are only top-level `task` issues.
2. Opening a task card reveals its child issues of type `subtask`.
3. Page-level filter uses exact matching (not fuzzy/token search) on:
   1. task title
   2. parent epic title(s) of that task
   3. parent epic ID(s) of that task
4. When a parent epic title or epic ID is matched, all tasks under that epic appear.
5. Cards inside each column are sorted by `updated_at` descending (most recently updated first).
6. Blocked-task cards show the specific Kanban-blocking task(s) with blocker ID, title, and raw status.
7. `blocked` column includes:
   1. `task` issues whose raw status is `blocked`
   2. `task` issues whose raw status is `open` and that are blocked by `task` dependencies whose blocker status is in the Kanban blocking-status set `{draft, open, in_progress, blocked, deferred, pinned, hooked}`

Non-goal for this fork:

1. Do not rework `--pages`.
2. Do not rework `--export-pages`.
3. Do not add template plumbing.
4. Do not add `--pages-template`.
5. Do not make Kanban integrate cleanly with commands this fork does not use.

## 2. Current-State Findings

Key implementation facts from the current code:

1. `--pages`, `--preview-pages`, and `--export-pages` are all dispatched from [`cmd/bv/main.go`](beads_viewer/cmd/bv/main.go).
2. The existing `--export-pages` implementation is not just an asset copy. The branch in `main.go` currently:
   1. reuses the already-loaded `issues []model.Issue`
   2. optionally filters closed issues
   3. optionally runs pre/post export hooks
   4. computes graph analysis and triage
   5. writes a SQLite bundle through [`pkg/export/sqlite_export.go`](beads_viewer/pkg/export/sqlite_export.go)
   6. copies viewer assets through the local `copyViewerAssets(...)` helper in `main.go`
   7. optionally writes history data and supports watch mode
3. `copyViewerAssets(...)` in [`cmd/bv/main.go`](beads_viewer/cmd/bv/main.go) prefers embedded assets from [`pkg/export/viewer_embed.go`](beads_viewer/pkg/export/viewer_embed.go), but also has a filesystem fallback for development/test builds.
4. Making Kanban a first-class variant of the existing pages system would create unnecessary merge conflict surface for a personal fork, especially around:
   1. flag wiring
   2. wizard wiring
   3. asset-template selection
   4. existing export-path filtering and dependency preparation
5. Parent/subtask relationships still depend on raw issue/dependency data, especially `parent-child` relationships from [`pkg/model/types.go`](beads_viewer/pkg/model/types.go).
6. The loader already normalizes status strings to lowercase on load, but the builder should still be defensive when tests construct in-memory issues directly.
7. `model.IssueType` intentionally allows unknown non-empty values, so matching `"subtask"` by normalized string is valid even though upstream does not define a dedicated enum constant for it.
8. The current `--export-pages` branch only filters out `status == closed` when `--pages-include-closed=false`; it does not also exclude `tombstone`. The Kanban export must implement its own explicit closed-like predicate instead of copying that behavior blindly.
9. `main.go` already loads issues before the static-export branches run, so the Kanban path should reuse the existing `issues` slice rather than inventing a second loading path.

Conclusion:

1. The fork should avoid extending the current pages architecture.
2. The lowest-maintenance approach is a new standalone export path that lives mostly in new files.
3. The implementation should copy the shape of the current `--export-pages` driver where convenient, then prune away everything Kanban does not need.
4. Some duplication is desirable if it prevents repeated rebasing pain in `cmd/bv/main.go` and `pkg/export/viewer_embed.go`.

## 3. Design Principles (Fork Maintenance First)

1. Optimize for smallest rebase footprint, not for architectural elegance.
2. Treat the existing pages/viewer system as upstream-owned and mostly off-limits.
3. Put almost all Kanban behavior in new fork-only files.
4. Touch `cmd/bv/main.go` only enough to register a new CLI flag and dispatch to a new helper.
5. Avoid modifying `pkg/export/viewer_embed.go` at all if possible.
6. Prefer copy/paste from the current `--export-pages` path over extracting shared abstractions if copying keeps the diff local to new files.
7. Prefer duplication over integration when duplication reduces conflict risk.
8. Use a dedicated JSON dataset (`data/kanban_board.json`) so the frontend stays simple and decoupled from SQLite/viewer internals.

## 4. Recommended Architecture

### 4.1 CLI Strategy

Add one new command-line flag:

1. `--export-kanban <output-dir>`

Recommended behavior:

1. If `--export-kanban` is set, run a dedicated Kanban export path and exit.
2. Leave `--pages` and `--export-pages` unchanged.
3. Reuse `--pages-title` and `--pages-include-closed` if convenient, rather than adding more Kanban-specific flags.
4. Do not add wizard support.
5. Do not add environment-variable template selection.
6. Add the new dispatch branch in the same static-export section that currently handles `--preview-pages` and `--export-pages`.
7. Reuse the already-loaded `issues` slice from `main.go`; do not add custom file discovery or a second issue-loading path for Kanban.

Rationale:

1. One extra branch in `main.go` is much cheaper than threading a new template through the current pages flow.
2. Reusing existing title/include-closed flags is slightly awkward but reduces new surface area.
3. Reusing the already-loaded `issues` slice avoids another place where loader behavior can drift from upstream.
4. Since this is a personal fork, awkward-but-local is preferable to elegant-but-invasive.

### 4.2 Package/Layout Strategy

Keep the implementation in new files:

1. [`cmd/bv/export_kanban.go`](beads_viewer/cmd/bv/export_kanban.go) for the CLI-side helper
2. [`pkg/export/kanban_export.go`](beads_viewer/pkg/export/kanban_export.go) for data building
3. [`pkg/export/kanban_bundle.go`](beads_viewer/pkg/export/kanban_bundle.go) for writing files/output structure
4. [`pkg/export/kanban_embed.go`](beads_viewer/pkg/export/kanban_embed.go) for Kanban-only embedded assets
5. [`pkg/export/kanban_assets/*`](beads_viewer/pkg/export/kanban_assets) for the standalone frontend

Implication:

1. `pkg/export/viewer_embed.go` does not need to know Kanban exists.
2. `viewer_assets/*` stays untouched.
3. The Kanban export path can evolve independently from upstream viewer changes.
4. The new code should feel like a forked mini-exporter, not a new branch of the shared pages subsystem.

### 4.3 Data Strategy

Generate a dedicated JSON file for Kanban:

1. `data/kanban_board.json`

Do not depend on client-side SQLite querying for this view. Benefits:

1. simpler JS
2. faster page load
3. fewer moving parts
4. less dependency on upstream viewer internals
5. no need to copy the SQLite/chunking/graph-layout/history machinery from `--export-pages`

### 4.4 UI Strategy

The Kanban frontend should be standalone, not template-driven.

Rules:

1. Copy visual ideas from existing viewer assets if useful.
2. Do not make the Kanban page share the upstream viewer asset pipeline.
3. It is acceptable to duplicate a small amount of CSS/HTML structure into `kanban_assets/*`.
4. Keep the frontend tiny and boring.
5. Prioritize readability and maintainability over reuse.

Practical recommendation:

1. Put everything Kanban needs in:
   1. `index.html`
   2. `kanban.js`
   3. `kanban.css`
2. If a few utility styles from `viewer_assets/styles.css` are helpful, copy the relevant rules into `kanban.css` instead of introducing shared-copy logic.
3. Avoid depending on `viewer.js`, `graph.js`, `charts.js`, `hybrid_scorer.js`, or any other viewer-specific runtime.
4. Avoid vendor assets entirely unless the Kanban page later proves it truly needs them.

## 5. Functional Specification

### 5.1 Column Mapping

Map raw issue states to Kanban columns:

1. `draft` -> `draft`
2. `blocked` column includes:
   1. any `task` with raw `status == blocked`
   2. any `task` with raw `status == open` and at least one blocking dependency on another `task` whose status is in `{draft, open, in_progress, blocked, deferred, pinned, hooked}`
   3. include `blocked_by_tasks[]` payload for UI rendering
3. `open` -> `open`, `pinned`, `hooked`, `deferred` (excluding tasks promoted to computed `blocked` column)
4. `in_progress` -> `in_progress`
5. `review` -> `review`
6. `closed` -> `closed`, `tombstone`

Unknown statuses fallback: `open`.

`--pages-include-closed=false` must exclude both `closed` and `tombstone` tasks before Kanban data generation.

Implementation note:

1. Do not reuse `model.Status.IsClosed()` for this filter because it only covers `closed`, not `tombstone`.

Kanban blocker predicate for task-to-task dependencies:

1. Only dependencies where `dep.Type.IsBlocking()` are considered.
2. Only blocker issues with `issue_type == task` are considered for `blocked` column promotion and `blocked_by_tasks[]`.
3. Blocking statuses for Kanban are exactly `{draft, open, in_progress, blocked, deferred, pinned, hooked}`.
4. Non-blocking statuses for Kanban are exactly `{review, closed, tombstone}`.
5. Unknown/custom blocker statuses are treated as blocking for safety, and the export should append a warning entry in `meta.warnings[]`.
6. Missing blocker issues are ignored and do not block the dependent task.
7. Raw `status == blocked` tasks always remain in the `blocked` column even if `blocked_by_tasks[]` is empty after applying the Kanban blocker predicate.
8. `blocked_by_tasks[]` is populated for both raw-blocked and computed-blocked tasks, contains only direct blockers from the task's own dependency list that satisfy rules 1-5 above, is de-duplicated by blocker ID, and is sorted by blocker ID ascending for deterministic output.

### 5.2 Task and Subtask Rules

Card inclusion rule (important to avoid duplicates):

1. include issue if `issue_type == task`
2. exclude if it is a child of another `task` via `parent-child` dependency
3. include if parent is `epic` or no parent task exists

Subtask rule:

1. subtasks are issues where `issue_type == "subtask"` and direct parent is the card task (`parent-child` edge child->task)
2. subtasks render only in expanded card body (not as top-level cards)
3. because `subtask` is a custom issue type in this fork workflow, match issue type by normalized string value (case-insensitive), not by hardcoded upstream enum constants

### 5.3 Parent Epic Resolution

For each card task, compute parent epics from ancestor chain:

1. follow `parent-child` links upward
2. collect ancestors with `issue_type == epic`
3. stop on cycles and mark warning in export log/metadata

### 5.4 Filter Behavior (Exact Match)

Single page-level filter input:

1. case-insensitive exact match after normalization (`trim`, collapse inner spaces)
2. no substring/fuzzy/token matching
3. card matches if normalized query equals any one of:
   1. normalized task title
   2. normalized parent epic title
   3. normalized parent epic ID

Recommended precomputed fields per task card:

1. `match_task_title`
2. `match_parent_epic_titles[]`
3. `match_parent_epic_ids[]`

Epic match expansion rule:

1. if query equals a parent epic title or parent epic ID, include all tasks that reference that epic in `match_parent_epic_*`

Subtasks do not drive filter results; filter target is task cards only.

### 5.5 Sorting and Ordering Rules (Explicit)

Top-level task card order within each column:

1. primary: `updated_at` descending
2. tie-breaker #1: `created_at` descending
3. tie-breaker #2: `id` ascending (stable deterministic output)

Subtask order inside expanded task:

1. primary: validated linear blocking dependency chain among sibling subtasks (prerequisite first)
2. scope assumption for this fork: sibling subtasks under one parent task form a single linear chain in real data
3. if that invariant is violated, log an export warning and fall back to deterministic `id` ascending for the ambiguous siblings
4. tie-breaker: `id` ascending when dependency order does not decide between otherwise valid positions

Missing timestamps:

1. if `updated_at` is zero-value, fallback to `created_at`
2. if both are zero-value, treat as Unix epoch and rely on ID tie-breaker

### 5.6 Column Order Invariant

Kanban column order is fixed and must never be auto-reordered:

1. `draft`
2. `blocked`
3. `open`
4. `in_progress`
5. `review`
6. `closed`

Rules:

1. render columns in this exact left-to-right order even if some columns are empty
2. do not sort columns by issue count or status name
3. use the `columns` array from exported JSON as the single source of truth

## 6. Data Contract for `data/kanban_board.json`

Suggested shape:

```json
{
  "meta": {
    "generated_at": "2026-03-08T00:00:00Z",
    "template": "kanban",
    "title": "Project Name",
    "include_closed": true,
    "source_issue_count": 123,
    "task_card_count": 40,
    "kanban_blocking_statuses": ["draft", "open", "in_progress", "blocked", "deferred", "pinned", "hooked"],
    "kanban_non_blocking_statuses": ["review", "closed", "tombstone"],
    "warnings": []
  },
  "columns": ["draft", "blocked", "open", "in_progress", "review", "closed"],
  "tasks": [
    {
      "id": "bv-123",
      "title": "Implement auth flow",
      "status": "open",
      "column": "blocked",
      "updated_at": "2026-03-08T11:22:33Z",
      "created_at": "2026-03-01T09:00:00Z",
      "priority": 1,
      "assignee": "alice",
      "parent_epics": [
        {"id": "epic-9", "title": "Authentication Revamp"}
      ],
      "match_task_title": "implement auth flow",
      "match_parent_epic_titles": ["authentication revamp"],
      "match_parent_epic_ids": ["epic-9"],
      "blocked_by_task_ids": ["bv-110"],
      "blocked_by_tasks": [
        {"id": "bv-110", "title": "Finalize auth data model", "status": "in_progress"}
      ],
      "subtasks": [
        {
          "id": "bv-124",
          "title": "Add OAuth callback validation",
          "status": "review",
          "updated_at": "2026-03-08T10:00:00Z",
          "priority": 2,
          "subtask_order": 1
        }
      ]
    }
  ]
}
```

Contract notes:

1. `meta.kanban_blocking_statuses` and `meta.kanban_non_blocking_statuses` are the exported source of truth for blocker semantics used by this bundle.
2. `meta.warnings[]` contains deterministic human-readable strings for non-fatal data-quality issues; append warnings in first-seen order during the stable issue-ID traversal used by the builder.
3. Recommended warning format: `issue <ID>: unknown blocker status <status>; treated as blocking`.
4. `blocked_by_task_ids[]` must mirror `blocked_by_tasks[].id` in the exact same sorted order.
5. `blocked_by_tasks[]` may be empty for raw `status == blocked` tasks that have no current Kanban-blocking task dependencies.
6. Timestamps should be emitted through normal Go JSON marshaling of `time.Time`, which yields RFC3339 strings consistent with existing export JSON files in this repo.

## 7. Implementation Plan (Phased)

### Phase 0: Add a Tiny CLI Entry Point

Files:

1. [`cmd/bv/main.go`](beads_viewer/cmd/bv/main.go)
2. [`cmd/bv/export_kanban.go`](beads_viewer/cmd/bv/export_kanban.go)

Changes:

1. Add one new flag: `--export-kanban`.
2. Add one new dispatch branch in `main.go`:
   1. if the flag is non-empty, call a dedicated helper and return
3. Place the new branch next to the existing static-export branches, after `--preview-pages` and before `--export-pages`.
4. Put the real logic in `cmd/bv/export_kanban.go`, not in `main.go`.
5. Reuse `--pages-title` and `--pages-include-closed`.
6. Reuse the already-loaded `issues` slice from `main.go`; do not re-read `.beads` inside the helper.
7. Copy the broad structure of the existing `--export-pages` `doExport(...)` closure, but keep it local to Kanban rather than abstracting shared helpers.
8. Keep pre/post export hook support by copying the existing hook invocation block into the Kanban helper, using `BV_EXPORT_FORMAT=html` for compatibility with existing hook expectations.
9. Intentionally omit from the Kanban helper:
   1. watch mode
   2. preview integration
   3. README generation
   4. history export
   5. graph analysis
   6. triage generation
   7. SQLite export
10. Do not touch:
   1. `--pages`
   2. `--export-pages`
   3. `runPagesWizard(...)`
   4. template-selection logic
   5. viewer-asset copy helpers

Concrete helper signature:

1. `func doExportKanban(allIssues []model.Issue, outputDir, title string, includeClosed bool, noHooks bool) error`

### Phase 1: Create Kanban Export Data Builder

New file:

1. [`pkg/export/kanban_export.go`](beads_viewer/pkg/export/kanban_export.go)

Changes:

1. Implement pure Go builder:
   1. build issue lookup
   2. extract `parent-child` graph
   3. classify top-level task cards
   4. derive subtasks (`issue_type == "subtask"`)
   5. resolve parent epics
   6. map statuses to columns including computed `blocked`
   7. sort top-level cards by `updated_at` descending with deterministic tie-breakers
   8. sort subtasks by validated linear blocking dependency chain
   9. populate `meta.kanban_blocking_statuses`, `meta.kanban_non_blocking_statuses`, and `meta.warnings`
2. Build directly from raw issues so `parent-child` and non-viewer-specific metadata stay available.
3. Ensure `includeClosed=false` excludes both `closed` and `tombstone`.
4. Normalize `issue_type` comparisons with `strings.EqualFold` or equivalent for `task`, `epic`, and `subtask` checks so custom fork data remains robust.
5. Normalize status comparisons defensively inside the builder for synthetic test fixtures, even though loader-normalized data from disk will already be lowercase.

Concrete types/functions to add in [`pkg/export/kanban_export.go`](beads_viewer/pkg/export/kanban_export.go):

1. `type KanbanBoardData struct { ... }`
2. `type KanbanBoardMeta struct { ... }`
3. `type KanbanTask struct { ... }`
4. `type KanbanSubtask struct { ... }`
5. `type KanbanEpicRef struct { ... }`
6. `type KanbanBlockingTask struct { ... }`
7. `func BuildKanbanBoardData(issues []model.Issue, includeClosed bool) KanbanBoardData`
8. `func normalizeKanbanIssueType(t model.IssueType) string`
9. `func statusToKanbanColumn(s model.Status) string`
10. `func isKanbanBlockingStatus(s model.Status) bool`
11. `func isKanbanClosedLikeStatus(s model.Status) bool`
12. `func hasUnresolvedTaskBlocker(issue model.Issue, issueByID map[string]*model.Issue) bool`
13. `func sortTasksByUpdatedDesc(tasks []KanbanTask)`
14. `func orderSubtasksByLinearBlockChain(subtasks []KanbanSubtask, deps []*model.Dependency) []KanbanSubtask`
15. `func collectBlockingTasks(issue model.Issue, issueByID map[string]*model.Issue) []KanbanBlockingTask`
16. `func appendKanbanWarning(data *KanbanBoardData, warning string)`

### Phase 2: Create a Standalone Kanban Bundle Writer

New file:

1. [`pkg/export/kanban_bundle.go`](beads_viewer/pkg/export/kanban_bundle.go)

Changes:

1. Write the output directory structure.
2. Write `data/kanban_board.json`.
3. Copy/embed only the Kanban frontend assets.
4. Keep this path completely separate from the existing viewer export path.
5. Mirror the current `copyViewerAssets(...)` strategy:
   1. embedded assets first
   2. filesystem fallback second for development/test builds
6. Do not generate:
   1. `beads.sqlite3`
   2. `beads.sqlite3.config.json`
   3. `data/meta.json`
   4. `data/project_health.json`
   5. `data/triage.json`
   6. `data/history.json`
   7. chunk files
   8. graph layout JSON

Concrete functions:

1. `func ExportKanbanBundle(outputDir string, data KanbanBoardData, title string) error`
2. `func WriteKanbanBoardData(path string, data KanbanBoardData) error`
3. `func CopyKanbanAssets(outputDir, title string) error`

### Phase 3: Add Minimal Standalone Frontend Assets

New directory:

1. [`pkg/export/kanban_assets`](beads_viewer/pkg/export/kanban_assets)

New files:

1. [`pkg/export/kanban_assets/index.html`](beads_viewer/pkg/export/kanban_assets/index.html)
2. [`pkg/export/kanban_assets/kanban.js`](beads_viewer/pkg/export/kanban_assets/kanban.js)
3. [`pkg/export/kanban_assets/kanban.css`](beads_viewer/pkg/export/kanban_assets/kanban.css)
4. [`pkg/export/kanban_embed.go`](beads_viewer/pkg/export/kanban_embed.go)

Behavior:

1. Fetch `data/kanban_board.json`.
2. Render 6-column board (`draft`, `blocked`, `open`, `in_progress`, `review`, `closed`).
3. Render only task cards.
4. Use `<details>` or controlled accordion to reveal subtasks.
5. Implement exact-match filter behavior as defined in section 5.4.
6. Show empty column and empty filter states explicitly.
7. Render task cards already pre-sorted; do not re-sort differently in JS.
8. For cards in `blocked` column, render a compact blocker list showing blocking task ID, title, and raw status.

Asset strategy:

1. Keep Kanban assets self-contained.
2. Do not route through `viewer_embed.go`.
3. Do not depend on `viewer_assets/*` at runtime.
4. If a little styling duplication is needed, accept it.
5. Keep runtime dependencies to zero unless a concrete UI requirement proves otherwise.

### Phase 4: Tests

Unit tests:

1. [`pkg/export/kanban_export_test.go`](beads_viewer/pkg/export/kanban_export_test.go)
2. [`pkg/export/kanban_bundle_test.go`](beads_viewer/pkg/export/kanban_bundle_test.go)

E2E tests:

1. [`tests/e2e/export_kanban_test.go`](beads_viewer/tests/e2e/export_kanban_test.go)

Test cases:

1. only tasks become cards
2. only `issue_type == "subtask"` child issues become subtasks in expansion data
3. task->epic linkage contributes parent epic filter text
4. status mapping correctness for raw `blocked`, computed blocked, deferred, pinned, hooked, review, and tombstone
5. `--pages-include-closed=false` omits both `closed` and `tombstone` tasks from Kanban data
6. `--export-kanban` writes the expected standalone bundle files
7. top-level card order is `updated_at` descending inside each column
8. exact-match filter semantics for:
   1. task title
   2. parent epic title
   3. parent epic ID
9. epic title/ID match returns all tasks in that epic
10. `blocked` column includes:
   1. raw `status == blocked` tasks
   2. `status == open` tasks blocked by dependencies whose blocker status is in `draft|open|in_progress|blocked|deferred|pinned|hooked`
11. `review` blockers do not promote dependent tasks into the `blocked` column
12. blocked task payload includes blocking task list sorted by blocker ID ascending
13. raw `status == blocked` tasks remain in `blocked` even when the blocking-task list is empty
14. blocked task payload includes only direct blockers, not transitive blocker closure
15. unknown/custom blocker statuses generate `meta.warnings[]`
16. blocked column UI renders blocking task list for each blocked card
17. subtask order follows the validated linear blocking chain used in this fork
18. tie-breaker determinism for equal timestamps (ID ascending for top-level cards)
19. column render order is exactly `draft -> blocked -> open -> in_progress -> review -> closed` regardless of emptiness
20. Kanban export runs pre/post hooks the same way as `--export-pages` when hooks exist, unless `--no-hooks` is set
21. Kanban export does not create SQLite/pages-only artifacts (`beads.sqlite3`, `data/meta.json`, `data/history.json`, etc.)
22. Development/E2E builds can load standalone Kanban assets via the same embedded-assets-first / staged-files fallback pattern used by existing pages export tests

E2E implementation note:

1. Existing pages E2E tests use `stageViewerAssets(...)` because test binaries may not have filesystem access to source assets in the expected relative location.
2. Add a Kanban equivalent helper, likely `stageKanbanAssets(...)`, rather than trying to generalize the existing helper.
3. Since this is a fork-only path, local duplication in the test helper is preferable to refactoring shared test infrastructure.

### Phase 5: Documentation and DX

Files:

1. [`KANBAN_README.md`](beads_viewer/KANBAN_README.md)

Changes:

1. document standalone usage:
   1. `bv --export-kanban ./out`
   2. `bv --export-kanban ./out --pages-title "Sprint Board"`
2. document status mapping and filtering semantics, including that `review` is non-blocking for Kanban blocker evaluation.
3. document sorting semantics explicitly:
   1. top-level cards: `updated_at DESC` within each column
   2. subtasks: validated linear blocking dependency chain order
4. document intentional divergences from `--export-pages`:
   1. no SQLite database
   2. no preview/wizard/watch mode
   3. no history export
   4. no README generation

### Phase 6: Rebase-Focused Guardrails

1. Add a short comment next to the new `--export-kanban` branch in `cmd/bv/main.go` indicating that the heavy logic lives outside `main.go`.
2. Keep all non-trivial Kanban logic in new files under `cmd/bv/` and `pkg/export/`.
3. Do not edit `pkg/export/viewer_embed.go` unless absolutely forced by implementation reality.
4. If upstream changes `--export-pages`, prefer re-copying any small useful snippets into the Kanban helper instead of trying to retroactively unify the two paths.

### Phase 7: Exact Edit Map (As of 2026-03-09)

`cmd/bv/main.go` anchor lines to touch:

1. Flag declaration area, to add `--export-kanban`.
2. Main command dispatch area, to add the new early-return branch.

Rebase guidance for these anchors:

1. Prefer adding one new flag variable and one new branch only.
2. Do not modify existing pages branches unless a compiler error forces it.
3. If line numbers drift after upstream changes, use the flag declaration block and top-level command-dispatch block as semantic anchors.

Minimal-diff edit recipe:

1. In `main.go`, add one new flag variable and one new early-return branch.
2. Reuse the already-loaded `issues` slice instead of adding a second load path.
3. Put all implementation details in new files.
4. Do not thread Kanban through existing function signatures.
5. Do not alter viewer asset helpers.
6. Copy code into the Kanban path instead of extracting shared abstractions if a choice must be made.

## 8. Build, Lint, and Validation Checklist

After implementation, run:

```bash
rch exec -- go build ./...
rch exec -- go vet ./...
gofmt -l .
rch exec -- go test ./pkg/export/... -v
rch exec -- go test ./tests/e2e -run 'Export.*Kanban' -v
```

If `rch` is unavailable, run commands locally without `rch exec --`.

## 9. Fork Maintenance Strategy (Lowest Effort)

### 9.1 Keep Diff Surface Small

Rules:

1. Add new behavior in new files (`export_kanban.go`, `kanban_export.go`, `kanban_bundle.go`, `kanban_assets/*`, `kanban_embed.go`).
2. Keep edits to high-churn files minimal and localized:
   1. `cmd/bv/main.go` only
3. Do not modify upstream heavy viewer files unless unavoidable.
4. When in doubt, duplicate a small helper into the Kanban path instead of broadening shared pages code.

### 9.2 Preserve Upstream Path by Non-Interference

1. Leave `--pages` and `--export-pages` alone.
2. Let upstream continue to own that path completely.
3. This avoids emergency breakage from fork changes bleeding into commands you do not use.

### 9.3 Rebase Workflow

Suggested cadence: weekly or biweekly.

```bash
git remote add upstream https://github.com/Dicklesworthstone/beads_viewer.git
git fetch upstream
git checkout main
git rebase upstream/main
rch exec -- go test ./pkg/export/... ./tests/e2e -run 'Export.*Kanban' -v
```

Enable conflict reuse once:

```bash
git config rerere.enabled true
```

### 9.4 Conflict Hotspots to Expect

1. `cmd/bv/main.go` (frequent upstream flag additions)

Mitigation:

1. Keep fork changes there as tiny wrappers/delegators.
2. Put most logic in stable new files.
3. Keep all upstream `viewer_assets/*` and `viewer_embed.go` untouched.

### 9.5 Intentional Duplication Policy

Allowed in this fork:

1. Duplicating a small amount of CSS/HTML/asset-copy logic is acceptable.
2. Duplicating a small output-writer path is acceptable.
3. Duplicating small pieces of the current `--export-pages` driver into `cmd/bv/export_kanban.go` is acceptable.
4. Duplicating a test asset-staging helper is acceptable.
5. Avoiding integration with the upstream viewer is the point, not a code smell.

## 10. Rollout Plan

1. Land Phase 0-1 with tests.
2. Land Phase 2-3 (bundle + frontend) and smoke test manually on real `.beads` data.
3. Land Phase 4 and run the Kanban-specific export-related test suite.
4. Update docs.

## 11. Definition of Done

The work is done when:

1. `bv --export-kanban <dir>` exports a single-page Kanban board.
2. Board has exactly six required columns (`draft`, `blocked`, `open`, `in_progress`, `review`, `closed`).
3. Only top-level task cards appear.
4. Expanding a task reveals only child issues of type `subtask`.
5. Filter is exact-match and works for task title, parent epic title, and parent epic ID (with epic match expanding to all tasks in that epic).
6. Blocked cards display their Kanban-blocking task list, with `review` tasks excluded from that list.
7. Go build/vet/tests pass for changed areas.
8. Existing `--pages` and `--export-pages` behavior remains unchanged because the Kanban fork does not route through them.

## 12. Post-Implementation Changes (2026-03-10)

The following changes were made after the initial implementation, superseding parts of the original plan.

### 12.1 Standalone Binary — Separate from `bv`

The kanban export was extracted from `bv` into a standalone binary `bv-kanban` (`cmd/bv-kanban/main.go`). The kanban code was moved from `pkg/export/` to its own package `pkg/kanban/` with zero dependency on `pkg/export`.

**Motivation:** The full `bv` binary is ~48MB due to embedded viewer assets, SQLite, gonum, and TUI dependencies. The standalone `bv-kanban` binary is ~4MB (stripped) since it only imports `pkg/kanban`, `pkg/loader`, `pkg/model`, and `pkg/hooks`. This is better for CI/CD pipelines that only need kanban export.

**Key consequence:** `cmd/bv/main.go` has **zero kanban-related changes**, completely eliminating the #1 rebase conflict hotspot identified in section 9.4. This supersedes sections 4.1, 7.0, and 9.4's mitigation advice about keeping `main.go` changes small — there are none.

**Structural changes:**
- `pkg/export/kanban_export.go` → `pkg/kanban/kanban_export.go`
- `pkg/export/kanban_bundle.go` → `pkg/kanban/kanban_bundle.go`
- `pkg/export/kanban_embed.go` → `pkg/kanban/kanban_embed.go`
- `pkg/export/kanban_assets/` → `pkg/kanban/kanban_assets/`
- `pkg/export/kanban_export_test.go` → `pkg/kanban/kanban_export_test.go`
- `pkg/export/kanban_bundle_test.go` → `pkg/kanban/kanban_bundle_test.go`
- `cmd/bv/export_kanban.go` → removed (replaced by `cmd/bv-kanban/main.go`)
- `cmd/bv/main.go` → all kanban changes reverted

**`marked.min.js` handling:** Copied into `pkg/kanban/kanban_assets/vendor/` so the kanban package is fully self-contained and does not reference `ViewerAssetsFS` from `pkg/export/`. This eliminates the dependency that would have pulled all viewer assets into the binary.

**CLI changes:**
- Old: `bv --export-kanban ./out --pages-title "Title" --pages-include-closed=false`
- New: `bv-kanban -output ./out -title "Title" -include-closed=false`
- Default output directory is `kanban/` (no `-output` flag needed for basic usage)

### 12.2 Substring Search (Contains) Instead of Exact Match

Section 5.4 specified exact-match filtering. The implementation was changed to **substring (contains) matching** for better usability. Typing "login" now matches a card titled "Implement Login Flow". Matching remains case-insensitive after normalization.

This supersedes section 5.4 rules 2-3. The precomputed match fields (`match_task_title`, `match_parent_epic_titles`, `match_parent_epic_ids`) are still used, but compared with `indexOf` instead of `===`.

### 12.3 Task and Subtask Detail Panel (Side Drawer)

A right-side detail panel was added, opened by clicking any card or subtask item. The panel shows:
- Header: ID, priority badge, status badge
- Title, metadata (assignee, updated/created dates)
- Labels and parent epic tags
- Blocked-by list (for blocked cards)
- Content sections rendered as markdown via `marked.js`: Description, Design, Acceptance Criteria, Notes
- Comments with author, date, and markdown body

**Subtask handling:** Clicking a subtask item in a card's expandable `<details>` section opens the subtask's own detail panel. The parent task's detail panel does not include subtasks — each item opens independently. Both tasks and subtasks use the same detail layout.

**Data changes:** `KanbanTask` gained: `Description`, `Design`, `AcceptanceCriteria`, `Notes`, `Labels`, `Comments` fields. `KanbanSubtask` gained the same fields plus `CreatedAt` and `Assignee` for full parity. New `KanbanComment` struct with `Author`, `Text`, `CreatedAt`.

Close via ✕ button, clicking the overlay backdrop, or pressing Escape.

### 12.4 Auto-Scroll and Board Spacer for Detail Panel Visibility

When opening the detail panel, the board auto-scrolls to keep the clicked card visible to the left of the panel. If the card is already visible, no scroll occurs.

When there isn't enough scrollable space (e.g., the card is in the rightmost column), a temporary spacer element is appended to the board to extend the scrollable area. The spacer is removed and scroll position restored when the panel closes.

### 12.5 Column Label Change

The "Review" column header label was changed to **"In Review"** for clarity. The internal column key remains `review`.

### 12.6 Updated Build and Validation Checklist

This supersedes section 8:

```bash
go build ./cmd/bv-kanban/
go build ./cmd/bv/
go vet ./...
go test ./pkg/kanban/... -v
```

For a minimal CI binary:

```bash
go build -ldflags="-s -w" -o bv-kanban ./cmd/bv-kanban/
```

### 12.7 Updated File Layout

This supersedes sections 4.2 and the implementation files table:

```
cmd/bv-kanban/main.go              # Standalone CLI binary
pkg/kanban/
├── kanban_export.go               # Data builder
├── kanban_export_test.go          # Data builder tests (20 tests)
├── kanban_bundle.go               # Bundle writer
├── kanban_bundle_test.go          # Bundle writer tests (9 tests)
├── kanban_embed.go                # go:embed directive
└── kanban_assets/
    ├── index.html                 # Page structure + detail panel overlay
    ├── kanban.js                  # Board rendering, detail panel, filtering, auto-scroll
    ├── kanban.css                 # Dark theme, prose typography, detail panel styles
    └── vendor/
        └── marked.min.js          # Markdown renderer (copied from viewer_assets)
```

No files in `cmd/bv/` or `pkg/export/` are modified by the kanban feature.
