package kanban

import "embed"

// KanbanAssetsFS embeds the kanban_assets directory for static Kanban export.
//
//go:embed kanban_assets
var KanbanAssetsFS embed.FS
