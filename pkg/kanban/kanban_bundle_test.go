package kanban

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	json "github.com/goccy/go-json"
)

func newTestKanbanBoardData() KanbanBoardData {
	return KanbanBoardData{
		Meta: KanbanBoardMeta{
			GeneratedAt: time.Now().UTC(),
			Template:    "kanban",
			Title:       "Test Board",
		},
		Columns: []string{"draft", "blocked", "open", "in_progress", "review", "closed"},
		Tasks:   []KanbanTask{},
	}
}

func TestWriteKanbanBoardData_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	data := newTestKanbanBoardData()

	if err := WriteKanbanBoardData(dir, data); err != nil {
		t.Fatalf("WriteKanbanBoardData: %v", err)
	}

	path := filepath.Join(dir, "data", "kanban_board.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading kanban_board.json: %v", err)
	}

	var parsed KanbanBoardData
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("invalid JSON in kanban_board.json: %v", err)
	}
}

func TestWriteKanbanBoardData_ContainsColumns(t *testing.T) {
	dir := t.TempDir()
	data := newTestKanbanBoardData()

	if err := WriteKanbanBoardData(dir, data); err != nil {
		t.Fatalf("WriteKanbanBoardData: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "data", "kanban_board.json"))
	if err != nil {
		t.Fatalf("reading kanban_board.json: %v", err)
	}

	var parsed KanbanBoardData
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(parsed.Columns) != len(data.Columns) {
		t.Fatalf("expected %d columns, got %d", len(data.Columns), len(parsed.Columns))
	}
	for i, col := range data.Columns {
		if parsed.Columns[i] != col {
			t.Errorf("column[%d]: expected %q, got %q", i, col, parsed.Columns[i])
		}
	}
}

func TestExportKanbanBundle_CreatesAllFiles(t *testing.T) {
	dir := t.TempDir()
	data := newTestKanbanBoardData()

	if err := ExportKanbanBundle(dir, data, "Test Title", ""); err != nil {
		t.Fatalf("ExportKanbanBundle: %v", err)
	}

	expected := []string{
		"index.html",
		"kanban.js",
		"kanban.css",
		filepath.Join("data", "kanban_board.json"),
		filepath.Join("vendor", "marked.min.js"),
	}
	for _, rel := range expected {
		path := filepath.Join(dir, rel)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", rel)
		}
	}
}

func TestExportKanbanBundle_NoSQLiteArtifacts(t *testing.T) {
	dir := t.TempDir()
	data := newTestKanbanBoardData()

	if err := ExportKanbanBundle(dir, data, "", ""); err != nil {
		t.Fatalf("ExportKanbanBundle: %v", err)
	}

	forbidden := []string{
		"beads.sqlite3",
		filepath.Join("data", "meta.json"),
		filepath.Join("data", "history.json"),
	}
	for _, rel := range forbidden {
		path := filepath.Join(dir, rel)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("unexpected file %s should not exist in kanban bundle", rel)
		}
	}
}

func TestCustomizeKanbanIndexHTML_ReplacesTitle(t *testing.T) {
	content := `<html><head><title>Kanban Board</title><!-- KANBAN_FAVICON_PLACEHOLDER --></head><body><h1>Kanban Board</h1></body></html>`
	result := customizeKanbanIndexHTML(content, "My Sprint", "")

	if !strings.Contains(result, "<title>My Sprint</title>") {
		t.Error("expected <title> to be replaced")
	}
	if !strings.Contains(result, "<h1>My Sprint</h1>") {
		t.Error("expected <h1> to be replaced")
	}
}

func TestCustomizeKanbanIndexHTML_HTMLEscapesTitle(t *testing.T) {
	content := `<html><head><title>Kanban Board</title><!-- KANBAN_FAVICON_PLACEHOLDER --></head><body><h1>Kanban Board</h1></body></html>`
	result := customizeKanbanIndexHTML(content, `<script>alert("xss")</script>`, "")

	if strings.Contains(result, "<script>") {
		t.Error("XSS payload should be escaped, not injected as raw HTML")
	}
	if !strings.Contains(result, "&lt;script&gt;") {
		t.Error("expected HTML-escaped script tag in output")
	}
}

func TestCustomizeKanbanIndexHTML_EmptyNoOp(t *testing.T) {
	content := `<html><head><title>Kanban Board</title></head></html>`
	result := customizeKanbanIndexHTML(content, "", "")

	if result != content {
		t.Error("empty title should return content unchanged")
	}
}

func TestCustomizeKanbanIndexHTML_AddsFavicon(t *testing.T) {
	content := `<html><head><title>Kanban Board</title><!-- KANBAN_FAVICON_PLACEHOLDER --></head><body><h1>Kanban Board</h1></body></html>`
	result := customizeKanbanIndexHTML(content, "", "favicon.ico")

	if !strings.Contains(result, `<link rel="icon" href="favicon.ico">`) {
		t.Error("expected favicon <link rel=\"icon\"> to be injected")
	}
	if strings.Contains(result, "KANBAN_FAVICON_PLACEHOLDER") {
		t.Error("expected favicon placeholder to be removed")
	}
}

func TestCustomizeKanbanIndexHTML_HTMLEscapesFavicon(t *testing.T) {
	content := `<html><head><title>Kanban Board</title><!-- KANBAN_FAVICON_PLACEHOLDER --></head><body><h1>Kanban Board</h1></body></html>`
	result := customizeKanbanIndexHTML(content, "", `" onerror="alert('xss')`)

	if strings.Contains(result, `onerror="alert`) {
		t.Error("favicon href should be HTML-escaped to prevent attribute injection")
	}
	if !strings.Contains(result, `&#34; onerror=&#34;alert(&#39;xss&#39;)`) {
		t.Error("expected escaped favicon href in injected link")
	}
}

func TestHasKanbanEmbeddedAssets(t *testing.T) {
	if !HasKanbanEmbeddedAssets() {
		t.Error("expected HasKanbanEmbeddedAssets to return true")
	}
}

func TestCopyKanbanAssets_TitleAndFaviconReplacement(t *testing.T) {
	dir := t.TempDir()
	title := "Custom Kanban Title"
	favicon := "favicon.ico"

	if err := CopyKanbanAssets(dir, title, favicon); err != nil {
		t.Fatalf("CopyKanbanAssets: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}

	html := string(raw)
	if !strings.Contains(html, title) {
		t.Errorf("expected index.html to contain custom title %q", title)
	}
	if !strings.Contains(html, `<link rel="icon" href="`+favicon+`">`) {
		t.Errorf("expected index.html to contain favicon href %q", favicon)
	}
}

func TestPrepareExportFavicon_CopiesLocalFile(t *testing.T) {
	workDir := t.TempDir()
	outputDir := filepath.Join(workDir, "out")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("mkdir output: %v", err)
	}

	sourcePath := filepath.Join(workDir, "board-icon.png")
	sourceContent := []byte("png-bytes")
	if err := os.WriteFile(sourcePath, sourceContent, 0644); err != nil {
		t.Fatalf("write source icon: %v", err)
	}

	href, err := prepareExportFavicon(outputDir, sourcePath)
	if err != nil {
		t.Fatalf("prepareExportFavicon: %v", err)
	}
	if href != "favicon.png" {
		t.Fatalf("expected href favicon.png, got %q", href)
	}

	copiedPath := filepath.Join(outputDir, "favicon.png")
	copiedContent, err := os.ReadFile(copiedPath)
	if err != nil {
		t.Fatalf("read copied favicon: %v", err)
	}
	if string(copiedContent) != string(sourceContent) {
		t.Fatalf("copied favicon content mismatch: got %q", string(copiedContent))
	}
}

func TestPrepareExportFavicon_RejectsURLs(t *testing.T) {
	outputDir := t.TempDir()
	_, err := prepareExportFavicon(outputDir, "https://example.com/favicon.ico")
	if err == nil {
		t.Fatal("expected error for URL-based favicon")
	}
	if !strings.Contains(err.Error(), "local file path") {
		t.Fatalf("expected local-file-path error, got %v", err)
	}
}
