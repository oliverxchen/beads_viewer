package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildBvKanbanBinary(t *testing.T) string {
	t.Helper()

	binName := "bv-kanban"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(t.TempDir(), binName)

	cmd := exec.Command("go", "build", "-o", binPath, "../../cmd/bv-kanban")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build bv-kanban: %v\n%s", err, out)
	}

	return binPath
}

func stageKanbanAssets(t *testing.T, bvPath string) {
	t.Helper()
	root := findRepoRoot(t)
	src := filepath.Join(root, "pkg", "kanban", "kanban_assets")
	dst := filepath.Join(filepath.Dir(bvPath), "pkg", "kanban", "kanban_assets")
	if err := copyDirRecursive(src, dst); err != nil {
		t.Fatalf("stage kanban assets: %v", err)
	}
}

func createKanbanRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	envDir := filepath.Join(dir, "env")
	if err := os.MkdirAll(filepath.Join(envDir, ".beads"), 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}

	// Create test data with epic, tasks (various statuses), and subtasks
	jsonl := strings.Join([]string{
		`{"id":"epic-1","title":"Auth Revamp","status":"open","priority":1,"issue_type":"epic","created_at":"2026-03-01T09:00:00Z","updated_at":"2026-03-02T09:00:00Z"}`,
		`{"id":"task-1","title":"Implement Login","status":"open","priority":1,"issue_type":"task","created_at":"2026-03-01T09:00:00Z","updated_at":"2026-03-03T09:00:00Z","dependencies":[{"issue_id":"task-1","depends_on_id":"epic-1","type":"parent-child"}]}`,
		`{"id":"task-2","title":"Setup DB","status":"in_progress","priority":2,"issue_type":"task","created_at":"2026-03-01T09:00:00Z","updated_at":"2026-03-04T09:00:00Z"}`,
		`{"id":"task-3","title":"Blocked Task","status":"open","priority":1,"issue_type":"task","created_at":"2026-03-01T09:00:00Z","updated_at":"2026-03-02T09:00:00Z","dependencies":[{"issue_id":"task-3","depends_on_id":"task-2","type":"blocks"}]}`,
		`{"id":"task-4","title":"Done Task","status":"closed","priority":3,"issue_type":"task","created_at":"2026-03-01T09:00:00Z","updated_at":"2026-03-05T09:00:00Z"}`,
		`{"id":"task-5","title":"Draft Task","status":"draft","priority":2,"issue_type":"task","created_at":"2026-03-01T09:00:00Z","updated_at":"2026-03-02T09:00:00Z"}`,
		`{"id":"task-6","title":"Review Task","status":"review","priority":2,"issue_type":"task","created_at":"2026-03-01T09:00:00Z","updated_at":"2026-03-02T09:00:00Z"}`,
		`{"id":"sub-1","title":"Sub Step 1","status":"open","priority":2,"issue_type":"subtask","created_at":"2026-03-01T09:00:00Z","updated_at":"2026-03-02T09:00:00Z","dependencies":[{"issue_id":"sub-1","depends_on_id":"task-1","type":"parent-child"}]}`,
		`{"id":"bug-1","title":"Random Bug","status":"open","priority":1,"issue_type":"bug","created_at":"2026-03-01T09:00:00Z","updated_at":"2026-03-02T09:00:00Z"}`,
	}, "\n")

	if err := os.WriteFile(filepath.Join(envDir, ".beads", "beads.jsonl"), []byte(jsonl), 0644); err != nil {
		t.Fatalf("write beads.jsonl: %v", err)
	}
	return envDir
}

func TestExportKanban_CreatesBundle(t *testing.T) {
	bv := buildBvKanbanBinary(t)
	stageKanbanAssets(t, bv)
	repoDir := createKanbanRepo(t)
	exportDir := filepath.Join(repoDir, "kanban-out")

	cmd := exec.Command(bv, "-output", exportDir)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bv-kanban export failed: %v\n%s", err, out)
	}

	// Verify expected files exist
	for _, p := range []string{
		filepath.Join(exportDir, "index.html"),
		filepath.Join(exportDir, "kanban.js"),
		filepath.Join(exportDir, "kanban.css"),
		filepath.Join(exportDir, "data", "kanban_board.json"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing expected file %s: %v", p, err)
		}
	}

	// Verify SQLite/pages artifacts do NOT exist
	for _, p := range []string{
		filepath.Join(exportDir, "beads.sqlite3"),
		filepath.Join(exportDir, "data", "meta.json"),
		filepath.Join(exportDir, "data", "history.json"),
		filepath.Join(exportDir, "data", "triage.json"),
	} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("unexpected pages artifact exists: %s", p)
		}
	}
}

func TestExportKanban_JSONStructure(t *testing.T) {
	bv := buildBvKanbanBinary(t)
	stageKanbanAssets(t, bv)
	repoDir := createKanbanRepo(t)
	exportDir := filepath.Join(repoDir, "kanban-out")

	cmd := exec.Command(bv, "-output", exportDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bv-kanban export failed: %v\n%s", err, out)
	}

	data, err := os.ReadFile(filepath.Join(exportDir, "data", "kanban_board.json"))
	if err != nil {
		t.Fatalf("read kanban_board.json: %v", err)
	}

	var board struct {
		Meta struct {
			Template      string   `json:"template"`
			Warnings      []string `json:"warnings"`
			TaskCardCount int      `json:"task_card_count"`
		} `json:"meta"`
		Columns []string `json:"columns"`
		Tasks   []struct {
			ID       string `json:"id"`
			Column   string `json:"column"`
			Subtasks []struct {
				ID string `json:"id"`
			} `json:"subtasks"`
			BlockedByTasks []struct {
				ID string `json:"id"`
			} `json:"blocked_by_tasks"`
			ParentEpics []struct {
				ID string `json:"id"`
			} `json:"parent_epics"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &board); err != nil {
		t.Fatalf("decode kanban_board.json: %v", err)
	}

	if board.Meta.Template != "kanban" {
		t.Errorf("expected template=kanban, got %q", board.Meta.Template)
	}

	// Verify columns
	expectedCols := []string{"draft", "blocked", "open", "in_progress", "review", "closed"}
	if len(board.Columns) != len(expectedCols) {
		t.Fatalf("expected %d columns, got %d", len(expectedCols), len(board.Columns))
	}
	for i, col := range expectedCols {
		if board.Columns[i] != col {
			t.Errorf("column[%d]: expected %q, got %q", i, col, board.Columns[i])
		}
	}

	// Verify only tasks appear (not bugs, epics, subtasks)
	taskIDs := make(map[string]bool)
	for _, tk := range board.Tasks {
		taskIDs[tk.ID] = true
	}
	if taskIDs["bug-1"] {
		t.Error("bug-1 should not be a card")
	}
	if taskIDs["epic-1"] {
		t.Error("epic-1 should not be a card")
	}
	if taskIDs["sub-1"] {
		t.Error("sub-1 should not be a top-level card")
	}

	// Verify task-3 is in blocked column (open + blocked by in_progress task-2)
	for _, tk := range board.Tasks {
		if tk.ID == "task-3" {
			if tk.Column != "blocked" {
				t.Errorf("task-3 expected blocked column, got %q", tk.Column)
			}
			if len(tk.BlockedByTasks) != 1 || tk.BlockedByTasks[0].ID != "task-2" {
				t.Errorf("task-3 expected blocked_by=[task-2], got %v", tk.BlockedByTasks)
			}
		}
		// task-1 should have parent epic
		if tk.ID == "task-1" {
			if len(tk.ParentEpics) != 1 || tk.ParentEpics[0].ID != "epic-1" {
				t.Errorf("task-1 expected parent epic-1, got %v", tk.ParentEpics)
			}
			if len(tk.Subtasks) != 1 || tk.Subtasks[0].ID != "sub-1" {
				t.Errorf("task-1 expected subtask sub-1, got %v", tk.Subtasks)
			}
		}
	}
}

func TestExportKanban_ExcludeClosed(t *testing.T) {
	bv := buildBvKanbanBinary(t)
	stageKanbanAssets(t, bv)
	repoDir := createKanbanRepo(t)
	exportDir := filepath.Join(repoDir, "kanban-out")

	cmd := exec.Command(bv, "-output", exportDir, "-include-closed=false")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bv-kanban export failed: %v\n%s", err, out)
	}

	data, err := os.ReadFile(filepath.Join(exportDir, "data", "kanban_board.json"))
	if err != nil {
		t.Fatalf("read kanban_board.json: %v", err)
	}

	var board struct {
		Tasks []struct {
			ID     string `json:"id"`
			Column string `json:"column"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(data, &board); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, tk := range board.Tasks {
		if tk.ID == "task-4" {
			t.Error("task-4 (closed) should be excluded with -include-closed=false")
		}
	}
}

func TestExportKanban_CustomTitle(t *testing.T) {
	bv := buildBvKanbanBinary(t)
	stageKanbanAssets(t, bv)
	repoDir := createKanbanRepo(t)
	exportDir := filepath.Join(repoDir, "kanban-out")

	cmd := exec.Command(bv, "-output", exportDir, "-title", "Sprint 42")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bv-kanban export failed: %v\n%s", err, out)
	}

	indexBytes, err := os.ReadFile(filepath.Join(exportDir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	if !strings.Contains(string(indexBytes), "Sprint 42") {
		t.Error("index.html should contain custom title 'Sprint 42'")
	}
}

func TestExportKanban_CustomFavicon(t *testing.T) {
	bv := buildBvKanbanBinary(t)
	stageKanbanAssets(t, bv)
	repoDir := createKanbanRepo(t)
	exportDir := filepath.Join(repoDir, "kanban-out")
	sourceFavicon := filepath.Join(repoDir, "my-tab-icon.ico")
	if err := os.WriteFile(sourceFavicon, []byte("ico-bytes"), 0644); err != nil {
		t.Fatalf("write source favicon: %v", err)
	}

	cmd := exec.Command(bv, "-output", exportDir, "-favicon", sourceFavicon)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bv-kanban export failed: %v\n%s", err, out)
	}

	indexBytes, err := os.ReadFile(filepath.Join(exportDir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}

	if !strings.Contains(string(indexBytes), `<link rel="icon" href="favicon.ico">`) {
		t.Fatalf("index.html should reference copied favicon")
	}
	if _, err := os.Stat(filepath.Join(exportDir, "favicon.ico")); err != nil {
		t.Fatalf("expected copied favicon in export directory: %v", err)
	}
}

func TestExportKanban_FaviconURLRejected(t *testing.T) {
	bv := buildBvKanbanBinary(t)
	stageKanbanAssets(t, bv)
	repoDir := createKanbanRepo(t)
	exportDir := filepath.Join(repoDir, "kanban-out")

	cmd := exec.Command(bv, "-output", exportDir, "-favicon", "https://example.com/icon.ico")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected bv-kanban export to fail for URL favicon\n%s", out)
	}
	if !strings.Contains(string(out), "local file path") {
		t.Fatalf("expected URL rejection message, got:\n%s", out)
	}
}

func TestExportKanban_InlineData(t *testing.T) {
	bv := buildBvKanbanBinary(t)
	stageKanbanAssets(t, bv)
	repoDir := createKanbanRepo(t)
	exportDir := filepath.Join(repoDir, "kanban-out")

	cmd := exec.Command(bv, "-output", exportDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bv-kanban export failed: %v\n%s", err, out)
	}

	indexBytes, err := os.ReadFile(filepath.Join(exportDir, "index.html"))
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(indexBytes)

	if !strings.Contains(html, "window.__KANBAN_DATA=") {
		t.Error("index.html should contain inlined __KANBAN_DATA for file:// support")
	}
	if strings.Contains(html, "KANBAN_DATA_PLACEHOLDER") {
		t.Error("index.html should not contain the raw placeholder comment")
	}
}
