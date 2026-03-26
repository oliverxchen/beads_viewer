package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
)

func TestInjectScriptBeforeBody_InsertsBeforeBodyTag(t *testing.T) {
	html := []byte("<html><body><h1>Kanban</h1></body></html>")
	script := []byte("<script>refresh()</script>")

	got := string(injectScriptBeforeBody(html, script))
	want := "<html><body><h1>Kanban</h1><script>refresh()</script></body></html>"
	if got != want {
		t.Fatalf("injectScriptBeforeBody() = %q, want %q", got, want)
	}
}

func TestInjectScriptBeforeBody_AppendsWhenBodyTagMissing(t *testing.T) {
	html := []byte("<html><h1>Kanban</h1></html>")
	script := []byte("<script>refresh()</script>")

	got := string(injectScriptBeforeBody(html, script))
	want := "<html><h1>Kanban</h1></html><script>refresh()</script>"
	if got != want {
		t.Fatalf("injectScriptBeforeBody() = %q, want %q", got, want)
	}
}

func TestResolveWatchFiles_IncludesPreferredAndIssuesPaths(t *testing.T) {
	tmp := t.TempDir()
	beadsDir := filepath.Join(tmp, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	if err := os.WriteFile(filepath.Join(beadsDir, "beads.jsonl"), []byte(`{"id":"A"}`), 0644); err != nil {
		t.Fatalf("WriteFile beads.jsonl error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte(`{"id":"B"}`), 0644); err != nil {
		t.Fatalf("WriteFile issues.jsonl error = %v", err)
	}

	unsetBeadsDBEnv(t)

	files, err := resolveWatchFiles(tmp)
	if err != nil {
		t.Fatalf("resolveWatchFiles() error = %v", err)
	}

	for _, name := range []string{"beads.jsonl", "issues.jsonl", "beads.base.jsonl"} {
		abs, err := filepath.Abs(filepath.Join(beadsDir, name))
		if err != nil {
			t.Fatalf("Abs(%q) error = %v", name, err)
		}
		if !containsString(files, abs) {
			t.Fatalf("resolveWatchFiles() missing %q in %v", abs, files)
		}
	}
}

func TestResolveWatchFiles_IncludesExplicitBeadsDBJSONLPath(t *testing.T) {
	tmp := t.TempDir()
	explicitPath := filepath.Join(tmp, "custom", "issues.jsonl")

	setBeadsDBEnv(t, explicitPath)

	files, err := resolveWatchFiles(tmp)
	if err != nil {
		t.Fatalf("resolveWatchFiles() error = %v", err)
	}

	absExplicitPath, err := filepath.Abs(explicitPath)
	if err != nil {
		t.Fatalf("Abs(explicitPath) error = %v", err)
	}
	if !containsString(files, absExplicitPath) {
		t.Fatalf("resolveWatchFiles() missing explicit BEADS_DB path %q in %v", absExplicitPath, files)
	}
}

func TestReadIndexStamp_ReturnsZeroForMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "index.html")
	if got := readIndexStamp(missing); got != "0" {
		t.Fatalf("readIndexStamp() = %q, want %q", got, "0")
	}
}

func TestLiveReloadPollingScript_EnforcesMinimumInterval(t *testing.T) {
	script := liveReloadPollingScript(10 * time.Millisecond)
	if !strings.Contains(script, "setInterval(checkForChanges, 250)") {
		t.Fatalf("liveReloadPollingScript() should clamp interval to 250ms, got %q", script)
	}
}

func setBeadsDBEnv(t *testing.T, value string) {
	t.Helper()
	original, hadOriginal := os.LookupEnv(loader.BeadsDBEnvVar)
	if err := os.Setenv(loader.BeadsDBEnvVar, value); err != nil {
		t.Fatalf("Setenv(%q) error = %v", loader.BeadsDBEnvVar, err)
	}
	t.Cleanup(func() {
		if hadOriginal {
			_ = os.Setenv(loader.BeadsDBEnvVar, original)
			return
		}
		_ = os.Unsetenv(loader.BeadsDBEnvVar)
	})
}

func unsetBeadsDBEnv(t *testing.T) {
	t.Helper()
	original, hadOriginal := os.LookupEnv(loader.BeadsDBEnvVar)
	if err := os.Unsetenv(loader.BeadsDBEnvVar); err != nil {
		t.Fatalf("Unsetenv(%q) error = %v", loader.BeadsDBEnvVar, err)
	}
	t.Cleanup(func() {
		if hadOriginal {
			_ = os.Setenv(loader.BeadsDBEnvVar, original)
			return
		}
		_ = os.Unsetenv(loader.BeadsDBEnvVar)
	})
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
