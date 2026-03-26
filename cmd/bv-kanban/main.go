package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/hooks"
	"github.com/Dicklesworthstone/beads_viewer/pkg/kanban"
	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
)

type exportOptions struct {
	outputDir     string
	title         string
	favicon       string
	includeClosed bool
	noHooks       bool
}

func main() {
	outputDir := flag.String("output", ".kanban", "Output directory for Kanban board")
	title := flag.String("title", "Kanban Board", "Board title")
	favicon := flag.String("favicon", "", "Local favicon file path (copied into export folder)")
	includeClosed := flag.Bool("include-closed", true, "Include closed/tombstone tasks")
	noHooks := flag.Bool("no-hooks", false, "Skip pre/post export hooks")
	dbPath := flag.String("db", "", "Path to beads JSONL file or .beads directory")
	watchMode := registerWatchFlags()
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

	if !watchMode.enabledRequested() {
		fmt.Printf("✓ Kanban export complete → %s\n", *outputDir)
		return
	}

	if err := watchMode.run(opts); err != nil {
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
