//go:build kanban_release

package main

type watchOptions struct{}

func registerWatchFlags() watchOptions {
	return watchOptions{}
}

func (watchOptions) enabledRequested() bool {
	return false
}

func (watchOptions) run(exportOptions) error {
	return nil
}
