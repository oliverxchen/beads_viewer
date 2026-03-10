package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/hooks"
	"github.com/Dicklesworthstone/beads_viewer/pkg/kanban"
	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
	"github.com/Dicklesworthstone/beads_viewer/pkg/watcher"
)

const (
	watchPortRangeStart = 9000
	watchPortRangeEnd   = 9100
	pollInterval        = 1000 * time.Millisecond
)

type exportOptions struct {
	outputDir     string
	title         string
	favicon       string
	includeClosed bool
	noHooks       bool
}

type kanbanPreviewServer struct {
	server *http.Server
	errCh  chan error
	url    string
}

func main() {
	outputDir := flag.String("output", ".kanban", "Output directory for Kanban board")
	title := flag.String("title", "Kanban Board", "Board title")
	favicon := flag.String("favicon", "", "Local favicon file path (copied into export folder)")
	includeClosed := flag.Bool("include-closed", true, "Include closed/tombstone tasks")
	noHooks := flag.Bool("no-hooks", false, "Skip pre/post export hooks")
	dbPath := flag.String("db", "", "Path to beads JSONL file or .beads directory")
	watchMode := flag.Bool("watch", false, "Watch beads data, auto-rebuild, and serve with live refresh")
	watchPort := flag.Int("watch-port", 0, "Port for --watch preview server (0 = auto-select 9000-9100)")
	noBrowser := flag.Bool("no-browser", false, "Do not auto-open browser in --watch mode")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: bv-kanban -output <dir> [options]")
		fmt.Fprintln(os.Stderr, "\nExport a standalone Kanban board from Beads issues.")
		fmt.Fprintln(os.Stderr)
		flag.PrintDefaults()
	}
	flag.Parse()

	// Set BEADS_DB if --db provided
	if *dbPath != "" {
		os.Setenv(loader.BeadsDBEnvVar, *dbPath)
	}

	opts := exportOptions{
		outputDir:     *outputDir,
		title:         *title,
		favicon:       *favicon,
		includeClosed: *includeClosed,
		noHooks:       *noHooks,
	}

	if err := runKanbanExport(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if !*watchMode {
		fmt.Printf("✓ Kanban export complete → %s\n", *outputDir)
		return
	}

	if err := runWatchMode(opts, *watchPort, !*noBrowser); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runKanbanExport(opts exportOptions) error {
	// Load issues
	fmt.Println("Loading issues...")
	issues, err := loader.LoadIssues("")
	if err != nil {
		return fmt.Errorf("loading issues: %w", err)
	}
	fmt.Printf("  → Loaded %d issues\n", len(issues))

	// Run pre-export hooks
	executor, err := runPreExportHooks(opts, len(issues))
	if err != nil {
		return err
	}

	// Build and export
	fmt.Println("  → Building Kanban board data...")
	boardData := kanban.BuildKanbanBoardData(issues, opts.title, opts.includeClosed)
	fmt.Printf("  → %d task cards across %d columns\n", boardData.Meta.TaskCardCount, len(boardData.Columns))

	for _, w := range boardData.Meta.Warnings {
		fmt.Printf("  → Warning: %s\n", w)
	}

	fmt.Println("  → Writing Kanban bundle...")
	if err := kanban.ExportKanbanBundle(opts.outputDir, boardData, opts.title, opts.favicon); err != nil {
		return fmt.Errorf("writing kanban bundle: %w", err)
	}

	// Run post-export hooks
	runPostExportHooks(executor)

	return nil
}

func runPreExportHooks(opts exportOptions, issueCount int) (*hooks.Executor, error) {
	if opts.noHooks {
		return nil, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	hookLoader := hooks.NewLoader(hooks.WithProjectDir(cwd))
	if err := hookLoader.Load(); err != nil {
		fmt.Printf("  → Warning: failed to load hooks: %v\n", err)
		return nil, nil
	}

	if !hookLoader.HasHooks() {
		return nil, nil
	}

	fmt.Println("  → Running pre-export hooks...")
	ctx := hooks.ExportContext{
		ExportPath:   opts.outputDir,
		ExportFormat: "html",
		IssueCount:   issueCount,
		Timestamp:    time.Now(),
	}

	executor := hooks.NewExecutor(hookLoader.Config(), ctx)
	executor.SetLogger(func(msg string) {
		fmt.Printf("  → %s\n", msg)
	})

	if err := executor.RunPreExport(); err != nil {
		return nil, fmt.Errorf("pre-export hook failed: %w", err)
	}

	return executor, nil
}

func runPostExportHooks(executor *hooks.Executor) {
	if executor == nil {
		return
	}

	fmt.Println("  → Running post-export hooks...")
	if err := executor.RunPostExport(); err != nil {
		fmt.Printf("  → Warning: post-export hook failed: %v\n", err)
	}

	if len(executor.Results()) > 0 {
		fmt.Println("")
		fmt.Println(executor.Summary())
	}
}

func runWatchMode(opts exportOptions, requestedPort int, openBrowser bool) error {
	preview, err := startPreviewServer(opts.outputDir, requestedPort, openBrowser)
	if err != nil {
		return err
	}
	defer stopPreviewServer(preview)

	watchFiles, err := resolveWatchFiles("")
	if err != nil {
		return err
	}

	watchers, mergedChanges, err := startIssueWatchers(watchFiles)
	if err != nil {
		return err
	}
	defer stopIssueWatchers(watchers)

	fmt.Println("")
	fmt.Println("Watch mode enabled.")
	fmt.Printf("  → Preview URL: %s\n", preview.url)
	fmt.Println("  → Live refresh: enabled")
	fmt.Println("  → Watching data files:")
	for _, path := range watchFiles {
		fmt.Printf("    - %s\n", path)
	}
	fmt.Println("  → Press Ctrl+C to stop")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	for {
		select {
		case err, ok := <-preview.errCh:
			if !ok {
				return nil
			}
			return fmt.Errorf("preview server failed: %w", err)

		case <-mergedChanges:
			fmt.Printf("\n  → Change detected. Rebuilding [%s]...\n", time.Now().Format("15:04:05"))
			if err := runKanbanExport(opts); err != nil {
				fmt.Printf("  → Rebuild error: %v\n", err)
				continue
			}
			fmt.Printf("✓ Rebuild complete [%s]\n", time.Now().Format("15:04:05"))

		case <-sigCh:
			fmt.Println("\nStopping watch mode...")
			return nil
		}
	}
}

func resolveWatchFiles(repoPath string) ([]string, error) {
	beadsDir, err := loader.GetBeadsDir(repoPath)
	if err != nil {
		return nil, fmt.Errorf("resolving beads directory: %w", err)
	}

	var files []string
	seen := make(map[string]struct{})

	addFile := func(path string) {
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if _, exists := seen[abs]; exists {
			return
		}
		seen[abs] = struct{}{}
		files = append(files, abs)
	}

	for _, name := range loader.PreferredJSONLNames {
		addFile(filepath.Join(beadsDir, name))
	}

	// Explicitly include issues.jsonl because many beads workflows still write to it.
	addFile(filepath.Join(beadsDir, "issues.jsonl"))

	activeFile, err := loader.FindJSONLPath(beadsDir)
	if err == nil {
		addFile(activeFile)
	}

	envDB := strings.TrimSpace(os.Getenv(loader.BeadsDBEnvVar))
	if envDB != "" {
		if info, err := os.Stat(envDB); err == nil {
			if !info.IsDir() {
				addFile(envDB)
			}
		} else if strings.HasSuffix(strings.ToLower(envDB), ".jsonl") {
			// If the explicit DB file does not exist yet, watch it anyway.
			addFile(envDB)
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no candidate beads files found to watch in %s", beadsDir)
	}

	sort.Strings(files)
	return files, nil
}

func startIssueWatchers(files []string) ([]*watcher.Watcher, <-chan struct{}, error) {
	merged := make(chan struct{}, 1)
	watchers := make([]*watcher.Watcher, 0, len(files))

	for _, path := range files {
		watchPath := path
		w, err := watcher.NewWatcher(
			watchPath,
			watcher.WithDebounceDuration(500*time.Millisecond),
			watcher.WithOnError(func(err error) {
				fmt.Printf("  → Watch error (%s): %v\n", watchPath, err)
			}),
		)
		if err != nil {
			stopIssueWatchers(watchers)
			return nil, nil, fmt.Errorf("creating watcher for %s: %w", watchPath, err)
		}

		if err := w.Start(); err != nil {
			stopIssueWatchers(watchers)
			return nil, nil, fmt.Errorf("starting watcher for %s: %w", watchPath, err)
		}

		watchers = append(watchers, w)

		go func(ch <-chan struct{}) {
			for range ch {
				select {
				case merged <- struct{}{}:
				default:
					// A rebuild is already queued.
				}
			}
		}(w.Changed())
	}

	return watchers, merged, nil
}

func stopIssueWatchers(watchers []*watcher.Watcher) {
	for _, w := range watchers {
		w.Stop()
	}
}

func startPreviewServer(outputDir string, requestedPort int, openBrowser bool) (*kanbanPreviewServer, error) {
	indexPath := filepath.Join(outputDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return nil, fmt.Errorf("preview requires an existing index.html at %s: %w", indexPath, err)
	}

	port := requestedPort
	if port == 0 {
		var err error
		port, err = findAvailablePort(watchPortRangeStart, watchPortRangeEnd)
		if err != nil {
			return nil, err
		}
	}

	fileServer := http.FileServer(http.Dir(outputDir))
	script := liveReloadPollingScript(pollInterval)
	mux := http.NewServeMux()
	mux.HandleFunc("/__kanban__/stamp", func(w http.ResponseWriter, _ *http.Request) {
		setNoCacheHeaders(w)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(readIndexStamp(indexPath)))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			serveInjectedIndex(w, indexPath, script)
			return
		}

		setNoCacheHeaders(w)
		fileServer.ServeHTTP(w, r)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	if openBrowser {
		go func() {
			time.Sleep(500 * time.Millisecond)
			if err := openInBrowser(url); err != nil {
				fmt.Printf("  → Warning: could not open browser automatically: %v\n", err)
				fmt.Printf("  → Open %s in your browser\n", url)
			}
		}()
	}

	return &kanbanPreviewServer{
		server: server,
		errCh:  errCh,
		url:    url,
	}, nil
}

func stopPreviewServer(preview *kanbanPreviewServer) {
	if preview == nil || preview.server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := preview.server.Shutdown(ctx); err != nil {
		fmt.Printf("  → Warning: preview server shutdown error: %v\n", err)
	}
}

func serveInjectedIndex(w http.ResponseWriter, indexPath, script string) {
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("reading index.html: %v", err), http.StatusInternalServerError)
		return
	}

	setNoCacheHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(injectScriptBeforeBody(raw, []byte(script)))
}

func injectScriptBeforeBody(htmlContent, script []byte) []byte {
	if len(htmlContent) == 0 {
		return append([]byte{}, script...)
	}

	bodyCloseTag := []byte("</body>")
	lower := bytes.ToLower(htmlContent)
	idx := bytes.LastIndex(lower, bodyCloseTag)
	if idx == -1 {
		combined := make([]byte, 0, len(htmlContent)+len(script))
		combined = append(combined, htmlContent...)
		combined = append(combined, script...)
		return combined
	}

	combined := make([]byte, 0, len(htmlContent)+len(script))
	combined = append(combined, htmlContent[:idx]...)
	combined = append(combined, script...)
	combined = append(combined, htmlContent[idx:]...)
	return combined
}

func readIndexStamp(indexPath string) string {
	info, err := os.Stat(indexPath)
	if err != nil {
		return "0"
	}
	return strconv.FormatInt(info.ModTime().UnixNano(), 10)
}

func liveReloadPollingScript(interval time.Duration) string {
	ms := interval.Milliseconds()
	if ms < 250 {
		ms = 250
	}

	return fmt.Sprintf(`<script>
(function () {
  var lastStamp = null;

  async function checkForChanges() {
    try {
      var response = await fetch('/__kanban__/stamp?ts=' + Date.now(), { cache: 'no-store' });
      if (!response.ok) {
        return;
      }
      var stamp = (await response.text()).trim();
      if (!stamp) {
        return;
      }
      if (lastStamp === null) {
        lastStamp = stamp;
        return;
      }
      if (stamp !== lastStamp) {
        window.location.reload();
      }
    } catch (_) {
      // Keep polling silently while the server restarts or network blips.
    }
  }

  checkForChanges();
  setInterval(checkForChanges, %d);
})();
</script>`, ms)
}

func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func findAvailablePort(start, end int) (int, error) {
	for port := start; port <= end; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			continue
		}
		_ = listener.Close()
		return port, nil
	}

	return 0, fmt.Errorf("no available port in range %d-%d", start, end)
}

func openInBrowser(url string) error {
	if os.Getenv("BV_NO_BROWSER") != "" || os.Getenv("BV_TEST_MODE") != "" {
		return nil
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
