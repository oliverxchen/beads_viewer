package kanban

import (
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	json "github.com/goccy/go-json"
)

// ExportKanbanBundle writes the Kanban output bundle to disk.
// It creates the output directory, writes kanban_board.json, copies
// the Kanban frontend assets, and inlines the JSON data into index.html
// so the page works when opened directly from the filesystem (file:// URLs).
func ExportKanbanBundle(outputDir string, data KanbanBoardData, title, faviconPath string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := WriteKanbanBoardData(outputDir, data); err != nil {
		return fmt.Errorf("writing kanban board data: %w", err)
	}

	faviconHref, err := prepareExportFavicon(outputDir, faviconPath)
	if err != nil {
		return fmt.Errorf("preparing favicon: %w", err)
	}

	if err := CopyKanbanAssets(outputDir, title, faviconHref); err != nil {
		return fmt.Errorf("copying kanban assets: %w", err)
	}

	// Inline the JSON data into index.html so it works with file:// URLs.
	if err := inlineKanbanData(outputDir, data); err != nil {
		return fmt.Errorf("inlining kanban data: %w", err)
	}

	return nil
}

// prepareExportFavicon copies a local favicon file into the export directory
// and returns the relative href that should be used in index.html.
func prepareExportFavicon(outputDir, faviconPath string) (string, error) {
	trimmed := strings.TrimSpace(faviconPath)
	if trimmed == "" {
		return "", nil
	}
	if strings.Contains(trimmed, "://") {
		return "", fmt.Errorf("favicon must be a local file path; URLs are not supported")
	}

	info, err := os.Stat(trimmed)
	if err != nil {
		return "", fmt.Errorf("stat favicon %q: %w", trimmed, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("favicon path %q is a directory", trimmed)
	}

	ext := strings.ToLower(filepath.Ext(trimmed))
	if ext == "" {
		ext = ".ico"
	}

	destName := "favicon" + ext
	destPath := filepath.Join(outputDir, destName)

	content, err := os.ReadFile(trimmed)
	if err != nil {
		return "", fmt.Errorf("reading favicon %q: %w", trimmed, err)
	}
	if err := os.WriteFile(destPath, content, 0644); err != nil {
		return "", fmt.Errorf("writing favicon %q: %w", destPath, err)
	}

	return filepath.ToSlash(destName), nil
}

// WriteKanbanBoardData marshals KanbanBoardData as indented JSON and writes
// it to data/kanban_board.json under the given output directory.
func WriteKanbanBoardData(outputDir string, data KanbanBoardData) error {
	dataDir := filepath.Join(outputDir, "data")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling kanban board data: %w", err)
	}

	dest := filepath.Join(dataDir, "kanban_board.json")
	if err := os.WriteFile(dest, content, 0644); err != nil {
		return fmt.Errorf("writing kanban_board.json: %w", err)
	}

	return nil
}

// CopyKanbanAssets copies Kanban frontend assets to the output directory.
// It first tries embedded assets (from KanbanAssetsFS), then falls back to
// the filesystem for development builds.
func CopyKanbanAssets(outputDir, title, faviconHref string) error {
	if HasKanbanEmbeddedAssets() {
		return copyKanbanEmbeddedAssets(outputDir, title, faviconHref)
	}

	// Filesystem fallback for development
	assetsDir := findKanbanAssetsDir()
	if assetsDir == "" {
		return fmt.Errorf("kanban assets not found: no embedded assets and no kanban_assets/ directory on disk")
	}

	return copyKanbanFilesystemAssets(assetsDir, outputDir, title, faviconHref)
}

// HasKanbanEmbeddedAssets returns true if Kanban assets are embedded in the binary.
func HasKanbanEmbeddedAssets() bool {
	_, err := KanbanAssetsFS.ReadFile("kanban_assets/index.html")
	return err == nil
}

// copyKanbanEmbeddedAssets walks the embedded KanbanAssetsFS and copies all
// files to the output directory, replacing the title in index.html if provided.
func copyKanbanEmbeddedAssets(outputDir, title, faviconHref string) error {
	return fs.WalkDir(KanbanAssetsFS, "kanban_assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// embed.FS always uses forward slashes
		relPath := strings.TrimPrefix(path, "kanban_assets/")
		if relPath == path {
			// Root "kanban_assets" directory itself
			return nil
		}

		destPath := filepath.Join(outputDir, filepath.FromSlash(relPath))

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		content, err := KanbanAssetsFS.ReadFile(path)
		if err != nil {
			return err
		}

		if relPath == "index.html" {
			contentStr := customizeKanbanIndexHTML(string(content), title, faviconHref)
			content = []byte(contentStr)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		return os.WriteFile(destPath, content, 0644)
	})
}

// copyKanbanFilesystemAssets copies Kanban assets from a directory on disk.
func copyKanbanFilesystemAssets(assetsDir, outputDir, title, faviconHref string) error {
	return filepath.WalkDir(assetsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(assetsDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(outputDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if filepath.Base(relPath) == "index.html" && filepath.Dir(relPath) == "." {
			contentStr := customizeKanbanIndexHTML(string(content), title, faviconHref)
			content = []byte(contentStr)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		return os.WriteFile(destPath, content, 0644)
	})
}

// inlineKanbanData reads the exported index.html, replaces the placeholder
// comment with a <script> block that sets window.__KANBAN_DATA, and rewrites
// the file. This makes the page work when opened directly via file:// URLs
// where fetch() is blocked by browser security policy.
func inlineKanbanData(outputDir string, data KanbanBoardData) error {
	indexPath := filepath.Join(outputDir, "index.html")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("reading index.html: %w", err)
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling inline data: %w", err)
	}

	scriptTag := "<script>window.__KANBAN_DATA=" + string(jsonBytes) + ";</script>"
	replaced := strings.Replace(string(content), "<!-- KANBAN_DATA_PLACEHOLDER -->", scriptTag, 1)

	if err := os.WriteFile(indexPath, []byte(replaced), 0644); err != nil {
		return fmt.Errorf("writing index.html: %w", err)
	}
	return nil
}

// customizeKanbanIndexHTML applies title and favicon customizations to the
// exported Kanban index.html content.
func customizeKanbanIndexHTML(content, title, faviconHref string) string {
	if title == "" {
		// Keep original title when no custom title was provided.
	} else {
		safeTitle := html.EscapeString(title)
		content = strings.Replace(content, "<title>Kanban Board</title>", "<title>"+safeTitle+"</title>", 1)
		content = strings.Replace(content, "<h1>Kanban Board</h1>", "<h1>"+safeTitle+"</h1>", 1)
	}

	faviconTag := ""
	trimmedFaviconHref := strings.TrimSpace(faviconHref)
	if trimmedFaviconHref != "" {
		safeHref := html.EscapeString(trimmedFaviconHref)
		faviconTag = `<link rel="icon" href="` + safeHref + `">`
	}

	if strings.Contains(content, "<!-- KANBAN_FAVICON_PLACEHOLDER -->") {
		content = strings.Replace(content, "<!-- KANBAN_FAVICON_PLACEHOLDER -->", faviconTag, 1)
	} else if faviconTag != "" {
		content = strings.Replace(content, "</head>", "    "+faviconTag+"\n</head>", 1)
	}

	return content
}

// findKanbanAssetsDir locates the kanban_assets directory on the filesystem
// for development builds where assets are not embedded.
func findKanbanAssetsDir() string {
	// Try relative to working directory
	candidates := []string{
		filepath.Join("pkg", "kanban", "kanban_assets"),
	}

	// Try relative to the binary location
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append(candidates,
			filepath.Join(execDir, "pkg", "kanban", "kanban_assets"),
		)
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	return ""
}
