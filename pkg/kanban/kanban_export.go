package kanban

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

// kanbanColumns is the fixed ordered list of Kanban column names.
var kanbanColumns = []string{"draft", "blocked", "open", "in_progress", "review", "closed"}

// blockingStatuses are raw statuses that count as "still blocking" when
// evaluating whether a dependency actually blocks a task.
var kanbanBlockingStatuses = []string{
	string(model.StatusDraft),
	string(model.StatusOpen),
	string(model.StatusInProgress),
	string(model.StatusBlocked),
	string(model.StatusDeferred),
	string(model.StatusPinned),
	string(model.StatusHooked),
}

// nonBlockingStatuses are raw statuses where a dependency is considered resolved.
var kanbanNonBlockingStatuses = []string{
	string(model.StatusReview),
	string(model.StatusClosed),
	string(model.StatusTombstone),
}

// KanbanBoardData is the top-level structure emitted by the Kanban board builder.
type KanbanBoardData struct {
	Meta         KanbanBoardMeta     `json:"meta"`
	Columns      []string            `json:"columns"`
	Tasks        []KanbanTask        `json:"tasks"`
	BlockerGraph map[string][]string `json:"blocker_graph,omitempty"`
}

// KanbanBoardMeta holds metadata about the generated board.
type KanbanBoardMeta struct {
	GeneratedAt               time.Time `json:"generated_at"`
	Template                  string    `json:"template"`
	Title                     string    `json:"title"`
	IncludeClosed             bool      `json:"include_closed"`
	SourceIssueCount          int       `json:"source_issue_count"`
	TaskCardCount             int       `json:"task_card_count"`
	KanbanBlockingStatuses    []string  `json:"kanban_blocking_statuses"`
	KanbanNonBlockingStatuses []string  `json:"kanban_non_blocking_statuses"`
	Warnings                  []string  `json:"warnings,omitempty"`
}

// KanbanTask represents a single card on the Kanban board.
type KanbanTask struct {
	ID                      string                 `json:"id"`
	Title                   string                 `json:"title"`
	Status                  string                 `json:"status"`
	Column                  string                 `json:"column"`
	UpdatedAt               time.Time              `json:"updated_at"`
	CreatedAt               time.Time              `json:"created_at"`
	Priority                int                    `json:"priority"`
	Assignee                string                 `json:"assignee,omitempty"`
	ParentEpics             []KanbanEpicRef        `json:"parent_epics,omitempty"`
	MatchTaskTitle          string                 `json:"match_task_title"`
	MatchParentEpicTitles   []string               `json:"match_parent_epic_titles,omitempty"`
	MatchParentEpicIDs      []string               `json:"match_parent_epic_ids,omitempty"`
	BlockedByTaskIDs        []string               `json:"blocked_by_task_ids,omitempty"`
	BlockedByTasks          []KanbanBlockingTask   `json:"blocked_by_tasks,omitempty"`
	BlockedBySubtaskDetails []SubtaskBlockerDetail `json:"blocked_by_subtask_details,omitempty"`
	Subtasks                []KanbanSubtask        `json:"subtasks,omitempty"`
	Description             string                 `json:"description,omitempty"`
	Design                  string                 `json:"design,omitempty"`
	AcceptanceCriteria      string                 `json:"acceptance_criteria,omitempty"`
	Notes                   string                 `json:"notes,omitempty"`
	Labels                  []string               `json:"labels,omitempty"`
	Comments                []KanbanComment        `json:"comments,omitempty"`
}

// KanbanSubtask is a child task rendered inside a parent card.
type KanbanSubtask struct {
	ID                 string          `json:"id"`
	Title              string          `json:"title"`
	Status             string          `json:"status"`
	UpdatedAt          time.Time       `json:"updated_at"`
	CreatedAt          time.Time       `json:"created_at"`
	Priority           int             `json:"priority"`
	Assignee           string          `json:"assignee,omitempty"`
	SubtaskOrder       int             `json:"subtask_order"`
	Description        string          `json:"description,omitempty"`
	Design             string          `json:"design,omitempty"`
	AcceptanceCriteria string          `json:"acceptance_criteria,omitempty"`
	Notes              string          `json:"notes,omitempty"`
	Labels             []string        `json:"labels,omitempty"`
	Comments           []KanbanComment `json:"comments,omitempty"`
}

// KanbanEpicRef is a lightweight reference to an ancestor epic.
type KanbanEpicRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// KanbanComment represents a comment on a task.
type KanbanComment struct {
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// KanbanBlockingTask describes a task that blocks another.
type KanbanBlockingTask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// SubtaskBlockerDetail captures a subtask-level blocking relationship for
// the detail panel. It records which specific subtask (or task) is blocked
// and by what, along with the rolled-up parent task card ID of the blocker.
type SubtaskBlockerDetail struct {
	BlockedID        string `json:"blocked_id"`
	BlockedTitle     string `json:"blocked_title"`
	BlockerID        string `json:"blocker_id"`
	BlockerTitle     string `json:"blocker_title"`
	BlockerTaskID    string `json:"blocker_task_id"`
	BlockerTaskTitle string `json:"blocker_task_title"`
	BlockerStatus    string `json:"blocker_status"`
}

// BuildKanbanBoardData transforms a flat list of issues into a structured
// Kanban board with columns, subtask nesting, epic parentage, and blocking
// analysis.
func BuildKanbanBoardData(issues []model.Issue, title string, includeClosed bool) KanbanBoardData {
	sourceCount := len(issues)
	var warnings []string

	// Build issue lookup map (by ID).
	issueMap := buildIssueMap(issues)

	// Build parent-child graph: child → parent, parent → []children.
	childToParents, parentToChildren := buildParentChildGraph(issues, issueMap)

	// Determine which issues are top-level task cards vs subtasks.
	topLevelIDs, subtaskParent := classifyTaskCards(issues, issueMap, childToParents)

	// Optionally exclude closed/tombstone cards.
	if !includeClosed {
		topLevelIDs = filterOutClosed(topLevelIDs, issueMap)
	}

	// Build the KanbanTask list.
	var tasks []KanbanTask
	for _, id := range topLevelIDs {
		issue := issueMap[id]

		// Resolve timestamps.
		updatedAt := resolveTimestamp(issue.UpdatedAt, issue.CreatedAt)
		createdAt := resolveTimestamp(issue.CreatedAt, time.Time{})

		// Resolve parent epics.
		epics := resolveParentEpics(id, childToParents, issueMap)

		// Determine column and blocked-by info.
		column, blockedByTasks, colWarnings := resolveColumn(issue, issueMap, &warnings)
		warnings = append(warnings, colWarnings...)

		// Build subtask list.
		subtasks := buildSubtasks(id, parentToChildren, subtaskParent, issueMap, &warnings)

		// Precompute match fields.
		matchTitle := normalizeForMatch(issue.Title)
		var matchEpicTitles []string
		var matchEpicIDs []string
		for _, ep := range epics {
			matchEpicTitles = append(matchEpicTitles, normalizeForMatch(ep.Title))
			matchEpicIDs = append(matchEpicIDs, strings.ToLower(strings.TrimSpace(ep.ID)))
		}

		var blockedByIDs []string
		for _, bt := range blockedByTasks {
			blockedByIDs = append(blockedByIDs, bt.ID)
		}

		var comments []KanbanComment
		for _, c := range issue.Comments {
			if c == nil {
				continue
			}
			comments = append(comments, KanbanComment{
				Author:    c.Author,
				Text:      c.Text,
				CreatedAt: c.CreatedAt,
			})
		}

		tasks = append(tasks, KanbanTask{
			ID:                    issue.ID,
			Title:                 issue.Title,
			Status:                string(issue.Status),
			Column:                column,
			UpdatedAt:             updatedAt,
			CreatedAt:             createdAt,
			Priority:              issue.Priority,
			Assignee:              issue.Assignee,
			ParentEpics:           epics,
			MatchTaskTitle:        matchTitle,
			MatchParentEpicTitles: matchEpicTitles,
			MatchParentEpicIDs:    matchEpicIDs,
			BlockedByTaskIDs:      blockedByIDs,
			BlockedByTasks:        blockedByTasks,
			Subtasks:              subtasks,
			Description:           issue.Description,
			Design:                issue.Design,
			AcceptanceCriteria:    issue.AcceptanceCriteria,
			Notes:                 issue.Notes,
			Labels:                issue.Labels,
			Comments:              comments,
		})
	}

	// Roll up subtask-level blockers into each task card.
	blockingSet := buildStatusSet(kanbanBlockingStatuses)
	nonBlockingSet := buildStatusSet(kanbanNonBlockingStatuses)

	// Build a quick index from task ID → index in tasks slice.
	taskIndex := make(map[string]int, len(tasks))
	for i := range tasks {
		taskIndex[tasks[i].ID] = i
	}

	for i := range tasks {
		task := &tasks[i]

		// Collect subtask IDs belonging to this task card.
		var mySubtaskIDs []string
		for stID, parentID := range subtaskParent {
			if parentID == task.ID {
				mySubtaskIDs = append(mySubtaskIDs, stID)
			}
		}
		if len(mySubtaskIDs) == 0 {
			continue
		}

		existingBlockers := make(map[string]bool)
		for _, bt := range task.BlockedByTasks {
			existingBlockers[bt.ID] = true
		}

		var subtaskDetails []SubtaskBlockerDetail

		for _, stID := range mySubtaskIDs {
			stIssue, ok := issueMap[stID]
			if !ok {
				continue
			}
			for _, dep := range stIssue.Dependencies {
				if dep == nil || !dep.Type.IsBlocking() {
					continue
				}
				blockerID := dep.DependsOnID
				if blockerID == "" {
					continue
				}
				blockerIssue, ok := issueMap[blockerID]
				if !ok {
					continue
				}

				// Determine the blocker's task-card-level ID.
				blockerTaskCardID := blockerID
				if pID, isSub := subtaskParent[blockerID]; isSub {
					blockerTaskCardID = pID
				} else if _, isTopLevel := taskIndex[blockerID]; !isTopLevel {
					// Blocker is neither a top-level task nor a subtask — skip.
					continue
				}

				// Skip internal dependencies (blocker rolls up to same card).
				if blockerTaskCardID == task.ID {
					continue
				}

				// Check if blocker is actively blocking.
				blockerStatus := string(blockerIssue.Status)
				if nonBlockingSet[blockerStatus] {
					continue
				}
				if !blockingSet[blockerStatus] {
					warnings = append(warnings, fmt.Sprintf(
						"subtask %s of task %s has blocker %s with unknown status %q; treating as blocking",
						stID, task.ID, blockerID, blockerStatus,
					))
				}

				// Resolve the blocker task card issue for title/status.
				blockerTaskIssue, ok := issueMap[blockerTaskCardID]
				if !ok {
					continue
				}

				subtaskDetails = append(subtaskDetails, SubtaskBlockerDetail{
					BlockedID:        stID,
					BlockedTitle:     stIssue.Title,
					BlockerID:        blockerID,
					BlockerTitle:     blockerIssue.Title,
					BlockerTaskID:    blockerTaskCardID,
					BlockerTaskTitle: blockerTaskIssue.Title,
					BlockerStatus:    blockerStatus,
				})

				// Add to BlockedByTasks if not already present.
				if !existingBlockers[blockerTaskCardID] {
					existingBlockers[blockerTaskCardID] = true
					task.BlockedByTasks = append(task.BlockedByTasks, KanbanBlockingTask{
						ID:     blockerTaskCardID,
						Title:  blockerTaskIssue.Title,
						Status: string(blockerTaskIssue.Status),
					})
				}
			}
		}

		if len(subtaskDetails) > 0 {
			// Sort details by BlockedID then BlockerID for determinism.
			sort.Slice(subtaskDetails, func(a, b int) bool {
				if subtaskDetails[a].BlockedID != subtaskDetails[b].BlockedID {
					return subtaskDetails[a].BlockedID < subtaskDetails[b].BlockedID
				}
				return subtaskDetails[a].BlockerID < subtaskDetails[b].BlockerID
			})
			task.BlockedBySubtaskDetails = subtaskDetails

			// Re-sort BlockedByTasks for determinism.
			sort.Slice(task.BlockedByTasks, func(a, b int) bool {
				return task.BlockedByTasks[a].ID < task.BlockedByTasks[b].ID
			})

			// Rebuild BlockedByTaskIDs from the updated BlockedByTasks.
			var ids []string
			for _, bt := range task.BlockedByTasks {
				ids = append(ids, bt.ID)
			}
			task.BlockedByTaskIDs = dedupStrings(ids)

			// Promote to blocked column unless the task is in draft or
			// already past the blocking stage (in_progress, review, closed).
			if task.Column == "open" {
				task.Column = "blocked"
			}
		}
	}

	// Build the BlockerGraph: task card ID → sorted list of blocking task card IDs.
	blockerGraph := make(map[string][]string)
	for i := range tasks {
		if len(tasks[i].BlockedByTasks) == 0 {
			continue
		}
		var blockerIDs []string
		for _, bt := range tasks[i].BlockedByTasks {
			blockerIDs = append(blockerIDs, bt.ID)
		}
		blockerIDs = dedupStrings(blockerIDs)
		sort.Strings(blockerIDs)
		blockerGraph[tasks[i].ID] = blockerIDs
	}

	// Sort cards within each column using workflow-prioritized ordering.
	sortTasksByColumn(tasks, blockerGraph)

	var blockerGraphResult map[string][]string
	if len(blockerGraph) > 0 {
		blockerGraphResult = blockerGraph
	}

	return KanbanBoardData{
		Meta: KanbanBoardMeta{
			GeneratedAt:               time.Now().UTC(),
			Template:                  "kanban",
			Title:                     title,
			IncludeClosed:             includeClosed,
			SourceIssueCount:          sourceCount,
			TaskCardCount:             len(tasks),
			KanbanBlockingStatuses:    kanbanBlockingStatuses,
			KanbanNonBlockingStatuses: kanbanNonBlockingStatuses,
			Warnings:                  warnings,
		},
		Columns:      kanbanColumns,
		Tasks:        tasks,
		BlockerGraph: blockerGraphResult,
	}
}

// buildIssueMap indexes issues by ID. Later entries overwrite earlier ones.
func buildIssueMap(issues []model.Issue) map[string]*model.Issue {
	m := make(map[string]*model.Issue, len(issues))
	for i := range issues {
		m[issues[i].ID] = &issues[i]
	}
	return m
}

// buildParentChildGraph extracts parent-child edges from dependency lists.
// In the Beads model a DepParentChild dependency on issue X with DependsOnID=Y
// means Y is a parent of X.
func buildParentChildGraph(issues []model.Issue, issueMap map[string]*model.Issue) (
	childToParents map[string][]string, parentToChildren map[string][]string,
) {
	childToParents = make(map[string][]string)
	parentToChildren = make(map[string][]string)

	for i := range issues {
		issue := &issues[i]
		for _, dep := range issue.Dependencies {
			if dep == nil || dep.Type != model.DepParentChild {
				continue
			}
			childID := dep.IssueID
			parentID := dep.DependsOnID
			if childID == "" || parentID == "" {
				continue
			}
			// Only record edges where both endpoints exist in the dataset.
			if _, ok := issueMap[childID]; !ok {
				continue
			}
			if _, ok := issueMap[parentID]; !ok {
				continue
			}
			childToParents[childID] = append(childToParents[childID], parentID)
			parentToChildren[parentID] = append(parentToChildren[parentID], childID)
		}
	}

	// De-duplicate edges.
	for k, v := range childToParents {
		childToParents[k] = dedupStrings(v)
	}
	for k, v := range parentToChildren {
		parentToChildren[k] = dedupStrings(v)
	}

	return childToParents, parentToChildren
}

// classifyTaskCards determines which issues become top-level Kanban cards.
// An issue is a top-level card if:
//   - issue_type == "task"
//   - it does NOT have a parent that is also a task (via parent-child dep)
//
// Issues whose type is "subtask" (case-insensitive) are tracked separately.
// Returns the ordered list of top-level IDs and a map of subtask→parentTaskID.
func classifyTaskCards(issues []model.Issue, issueMap map[string]*model.Issue, childToParents map[string][]string) (
	topLevel []string, subtaskParent map[string]string,
) {
	subtaskParent = make(map[string]string)

	for i := range issues {
		issue := &issues[i]

		// Subtask detection (case-insensitive).
		if strings.EqualFold(string(issue.IssueType), "subtask") {
			// Find direct parent that is a task.
			for _, parentID := range childToParents[issue.ID] {
				if p, ok := issueMap[parentID]; ok && strings.EqualFold(string(p.IssueType), "task") {
					subtaskParent[issue.ID] = parentID
					break
				}
			}
			continue
		}

		if !strings.EqualFold(string(issue.IssueType), "task") {
			continue
		}

		// Check if any parent is also a task → not top-level.
		hasTaskParent := false
		for _, parentID := range childToParents[issue.ID] {
			if p, ok := issueMap[parentID]; ok && strings.EqualFold(string(p.IssueType), "task") {
				hasTaskParent = true
				break
			}
		}
		if hasTaskParent {
			// This is a sub-task of another task — register as subtask.
			for _, parentID := range childToParents[issue.ID] {
				if p, ok := issueMap[parentID]; ok && strings.EqualFold(string(p.IssueType), "task") {
					subtaskParent[issue.ID] = parentID
					break
				}
			}
			continue
		}

		topLevel = append(topLevel, issue.ID)
	}

	return topLevel, subtaskParent
}

// filterOutClosed removes top-level IDs whose status is closed or tombstone.
func filterOutClosed(ids []string, issueMap map[string]*model.Issue) []string {
	var filtered []string
	for _, id := range ids {
		issue, ok := issueMap[id]
		if !ok {
			continue
		}
		if issue.Status == model.StatusClosed || issue.Status == model.StatusTombstone {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}

// resolveTimestamp returns ts if non-zero, otherwise fallback. If both are
// zero it returns the Unix epoch.
func resolveTimestamp(ts, fallback time.Time) time.Time {
	if !ts.IsZero() {
		return ts
	}
	if !fallback.IsZero() {
		return fallback
	}
	return time.Unix(0, 0).UTC()
}

// resolveParentEpics walks up the parent-child graph collecting ancestors
// with issue_type == epic. De-duplicated and sorted by ID.
func resolveParentEpics(issueID string, childToParents map[string][]string, issueMap map[string]*model.Issue) []KanbanEpicRef {
	visited := make(map[string]bool)
	var epics []KanbanEpicRef

	var walk func(id string)
	walk = func(id string) {
		for _, parentID := range childToParents[id] {
			if visited[parentID] {
				continue
			}
			visited[parentID] = true
			if p, ok := issueMap[parentID]; ok {
				if strings.EqualFold(string(p.IssueType), "epic") {
					epics = append(epics, KanbanEpicRef{ID: p.ID, Title: p.Title})
				}
				walk(parentID)
			}
		}
	}
	walk(issueID)

	// Sort by ID for determinism.
	sort.Slice(epics, func(i, j int) bool { return epics[i].ID < epics[j].ID })
	return epics
}

// resolveColumn determines the Kanban column for an issue and populates the
// blocked-by task list when the column is "blocked".
func resolveColumn(issue *model.Issue, issueMap map[string]*model.Issue, warnings *[]string) (
	column string, blockedByTasks []KanbanBlockingTask, colWarnings []string,
) {
	rawStatus := string(issue.Status)

	// Check for explicit blocked status first.
	isExplicitlyBlocked := issue.Status == model.StatusBlocked

	// Collect blocking deps (for blocked column resolution).
	blockedByTasks, colWarnings = collectBlockingDeps(issue, issueMap)

	// Determine column.
	switch issue.Status {
	case model.StatusDraft:
		column = "draft"
	case model.StatusBlocked:
		column = "blocked"
	case model.StatusOpen:
		// Promote to blocked if there are active blocking deps.
		if len(blockedByTasks) > 0 {
			column = "blocked"
		} else {
			column = "open"
		}
	case model.StatusPinned, model.StatusHooked, model.StatusDeferred:
		column = "open"
	case model.StatusInProgress:
		column = "in_progress"
	case model.StatusReview:
		column = "review"
	case model.StatusClosed, model.StatusTombstone:
		column = "closed"
	default:
		// Unknown status falls back to open.
		column = "open"
	}

	// If not in blocked column, clear blocked-by info.
	if column != "blocked" {
		blockedByTasks = nil
	}

	// Explicitly blocked tasks stay blocked even with empty blocked_by_tasks.
	_ = isExplicitlyBlocked
	_ = rawStatus

	return column, blockedByTasks, colWarnings
}

// collectBlockingDeps finds tasks that actively block the given issue via
// blocking dependency edges. Returns de-duplicated, sorted blockers and any
// warnings about unknown statuses.
func collectBlockingDeps(issue *model.Issue, issueMap map[string]*model.Issue) ([]KanbanBlockingTask, []string) {
	var blockers []KanbanBlockingTask
	var warnings []string
	seen := make(map[string]bool)

	blockingSet := buildStatusSet(kanbanBlockingStatuses)
	nonBlockingSet := buildStatusSet(kanbanNonBlockingStatuses)

	for _, dep := range issue.Dependencies {
		if dep == nil {
			continue
		}
		if !dep.Type.IsBlocking() {
			continue
		}
		// The blocking dep means issue.ID depends on dep.DependsOnID.
		blockerID := dep.DependsOnID
		if blockerID == "" || seen[blockerID] {
			continue
		}

		blocker, ok := issueMap[blockerID]
		if !ok {
			// Missing blocker issue — ignore.
			continue
		}

		// Only consider task-type blockers.
		if !strings.EqualFold(string(blocker.IssueType), "task") {
			continue
		}

		blockerStatus := string(blocker.Status)
		if nonBlockingSet[blockerStatus] {
			// Resolved — not actively blocking.
			continue
		}

		if !blockingSet[blockerStatus] {
			// Unknown/custom status — treat as blocking + warn.
			warnings = append(warnings, fmt.Sprintf(
				"task %s has blocker %s with unknown status %q; treating as blocking",
				issue.ID, blockerID, blockerStatus,
			))
		}

		seen[blockerID] = true
		blockers = append(blockers, KanbanBlockingTask{
			ID:     blocker.ID,
			Title:  blocker.Title,
			Status: blockerStatus,
		})
	}

	// Sort by ID ascending for determinism.
	sort.Slice(blockers, func(i, j int) bool { return blockers[i].ID < blockers[j].ID })
	return blockers, warnings
}

// buildSubtasks gathers child subtasks for a given parent task, orders them
// by blocking chain, and assigns SubtaskOrder.
func buildSubtasks(parentID string, parentToChildren map[string][]string, subtaskParent map[string]string, issueMap map[string]*model.Issue, warnings *[]string) []KanbanSubtask {
	// Collect children that are registered as subtasks of this parent.
	var childIDs []string
	for childID, pID := range subtaskParent {
		if pID == parentID {
			childIDs = append(childIDs, childID)
		}
	}
	if len(childIDs) == 0 {
		return nil
	}

	// Try to order by linear blocking chain among siblings.
	ordered := orderByBlockingChain(childIDs, issueMap, warnings, parentID)

	var subtasks []KanbanSubtask
	for i, id := range ordered {
		issue, ok := issueMap[id]
		if !ok {
			continue
		}
		updatedAt := resolveTimestamp(issue.UpdatedAt, issue.CreatedAt)
		createdAt := resolveTimestamp(issue.CreatedAt, time.Time{})
		var comments []KanbanComment
		for _, c := range issue.Comments {
			if c != nil {
				comments = append(comments, KanbanComment{
					Author:    c.Author,
					Text:      c.Text,
					CreatedAt: c.CreatedAt,
				})
			}
		}
		subtasks = append(subtasks, KanbanSubtask{
			ID:                 issue.ID,
			Title:              issue.Title,
			Status:             string(issue.Status),
			UpdatedAt:          updatedAt,
			CreatedAt:          createdAt,
			Priority:           issue.Priority,
			Assignee:           issue.Assignee,
			SubtaskOrder:       i,
			Description:        issue.Description,
			Design:             issue.Design,
			AcceptanceCriteria: issue.AcceptanceCriteria,
			Notes:              issue.Notes,
			Labels:             issue.Labels,
			Comments:           comments,
		})
	}
	return subtasks
}

// orderByBlockingChain attempts to sort sibling subtask IDs by their linear
// blocking dependency chain. If the chain invariant is violated (multiple roots,
// branching, or incomplete coverage), it warns and falls back to ID ascending.
func orderByBlockingChain(ids []string, issueMap map[string]*model.Issue, warnings *[]string, parentID string) []string {
	if len(ids) <= 1 {
		return ids
	}

	siblingSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		siblingSet[id] = true
	}

	// Build predecessor map: for each sibling, find which other sibling blocks it.
	// predecessor[A] = B means B blocks A (A depends on B).
	predecessor := make(map[string]string)
	successor := make(map[string]string)

	for _, id := range ids {
		issue, ok := issueMap[id]
		if !ok {
			continue
		}
		for _, dep := range issue.Dependencies {
			if dep == nil || !dep.Type.IsBlocking() {
				continue
			}
			blockerID := dep.DependsOnID
			if !siblingSet[blockerID] || blockerID == id {
				continue
			}
			if _, exists := predecessor[id]; exists {
				// Multiple predecessors → not a linear chain.
				*warnings = append(*warnings, fmt.Sprintf(
					"subtasks of task %s do not form a linear blocking chain; falling back to ID sort",
					parentID,
				))
				return sortByID(ids)
			}
			predecessor[id] = blockerID
			if _, exists := successor[blockerID]; exists {
				// Multiple successors → not a linear chain.
				*warnings = append(*warnings, fmt.Sprintf(
					"subtasks of task %s do not form a linear blocking chain; falling back to ID sort",
					parentID,
				))
				return sortByID(ids)
			}
			successor[blockerID] = id
		}
	}

	// Find roots (subtasks with no predecessor among siblings).
	var roots []string
	for _, id := range ids {
		if _, hasPred := predecessor[id]; !hasPred {
			roots = append(roots, id)
		}
	}

	if len(roots) != 1 {
		*warnings = append(*warnings, fmt.Sprintf(
			"subtasks of task %s do not form a linear blocking chain; falling back to ID sort",
			parentID,
		))
		return sortByID(ids)
	}

	// Walk the chain from the single root.
	var chain []string
	current := roots[0]
	visited := make(map[string]bool)
	for {
		if visited[current] {
			break // cycle
		}
		visited[current] = true
		chain = append(chain, current)
		next, ok := successor[current]
		if !ok {
			break
		}
		current = next
	}

	if len(chain) != len(ids) {
		*warnings = append(*warnings, fmt.Sprintf(
			"subtasks of task %s do not form a linear blocking chain; falling back to ID sort",
			parentID,
		))
		return sortByID(ids)
	}

	return chain
}

// sortByID returns a copy of ids sorted ascending.
func sortByID(ids []string) []string {
	out := make([]string, len(ids))
	copy(out, ids)
	sort.Strings(out)
	return out
}

// sortTasksByColumn sorts tasks within each column.
//
// For non-closed columns (draft/blocked/open/in_progress/review):
//  1. priority ASC (lower is higher priority)
//  2. blocking_tasks_count DESC (how many task cards this task directly unblocks)
//  3. blocked_by_tasks_count ASC
//  4. created_at ASC
//  5. id ASC
//
// For closed column:
//  1. updated_at DESC
//  2. created_at DESC
//  3. id ASC
func sortTasksByColumn(tasks []KanbanTask, blockerGraph map[string][]string) {
	blockingCounts := buildBlockingTaskCounts(blockerGraph)

	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Column != tasks[j].Column {
			return columnOrder(tasks[i].Column) < columnOrder(tasks[j].Column)
		}

		// Closed keeps recency-first ordering.
		if tasks[i].Column == "closed" {
			if !tasks[i].UpdatedAt.Equal(tasks[j].UpdatedAt) {
				return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
			}
			if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
				return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
			}
			return tasks[i].ID < tasks[j].ID
		}

		// Non-closed columns: priority ASC.
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority < tasks[j].Priority
		}

		// Then by blocking_tasks_count DESC (higher unblocking impact first).
		bi := blockingCounts[tasks[i].ID]
		bj := blockingCounts[tasks[j].ID]
		if bi != bj {
			return bi > bj
		}

		// Then by blocked_by_tasks_count ASC (easier to unblock first).
		blockedByI := len(tasks[i].BlockedByTaskIDs)
		blockedByJ := len(tasks[j].BlockedByTaskIDs)
		if blockedByI != blockedByJ {
			return blockedByI < blockedByJ
		}

		// Then by created_at ASC.
		if !tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
		}

		// Final tie-break: id ASC.
		return tasks[i].ID < tasks[j].ID
	})

	// Enforce direct dependency precedence within non-closed columns so a blocker
	// card always appears before the blocked card when both are in the same
	// column. This avoids priority/impact sorting from inverting execution order.
	enforceDependencyPrecedence(tasks, blockerGraph)
}

// buildBlockingTaskCounts computes direct downstream impact per task card.
// Input graph is blocked_task_id -> blocker_task_ids.
func buildBlockingTaskCounts(blockerGraph map[string][]string) map[string]int {
	counts := make(map[string]int)
	for blockedID, blockerIDs := range blockerGraph {
		seen := make(map[string]bool)
		for _, blockerID := range blockerIDs {
			if blockerID == "" || blockerID == blockedID || seen[blockerID] {
				continue
			}
			seen[blockerID] = true
			counts[blockerID]++
		}
	}
	return counts
}

// enforceDependencyPrecedence reorders each non-closed column to satisfy direct
// blocker-before-blocked constraints for cards within that same column.
func enforceDependencyPrecedence(tasks []KanbanTask, blockerGraph map[string][]string) {
	if len(tasks) < 2 || len(blockerGraph) == 0 {
		return
	}

	for _, col := range kanbanColumns {
		if col == "closed" {
			continue
		}

		var colIndices []int
		for i := range tasks {
			if tasks[i].Column == col {
				colIndices = append(colIndices, i)
			}
		}
		if len(colIndices) < 2 {
			continue
		}

		// Current order ranking within this column (lower index = higher rank).
		rank := make(map[string]int, len(colIndices))
		colSet := make(map[string]bool, len(colIndices))
		for pos, idx := range colIndices {
			id := tasks[idx].ID
			rank[id] = pos
			colSet[id] = true
		}

		indegree := make(map[string]int, len(colIndices))
		adj := make(map[string][]string, len(colIndices))
		for _, idx := range colIndices {
			indegree[tasks[idx].ID] = 0
		}

		edgeSeen := make(map[string]bool)
		for blockedID, blockerIDs := range blockerGraph {
			if !colSet[blockedID] {
				continue
			}
			for _, blockerID := range blockerIDs {
				if !colSet[blockerID] || blockerID == blockedID {
					continue
				}
				edgeKey := blockerID + "\x00" + blockedID
				if edgeSeen[edgeKey] {
					continue
				}
				edgeSeen[edgeKey] = true
				adj[blockerID] = append(adj[blockerID], blockedID)
				indegree[blockedID]++
			}
		}

		var queue []string
		for _, idx := range colIndices {
			id := tasks[idx].ID
			if indegree[id] == 0 {
				queue = append(queue, id)
			}
		}
		sort.Slice(queue, func(i, j int) bool { return rank[queue[i]] < rank[queue[j]] })

		var orderedIDs []string
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			orderedIDs = append(orderedIDs, id)
			for _, nextID := range adj[id] {
				indegree[nextID]--
				if indegree[nextID] == 0 {
					queue = append(queue, nextID)
				}
			}
			if len(queue) > 1 {
				sort.Slice(queue, func(i, j int) bool { return rank[queue[i]] < rank[queue[j]] })
			}
		}

		// Cycle fallback: append unplaced cards in their existing order.
		if len(orderedIDs) < len(colIndices) {
			placed := make(map[string]bool, len(orderedIDs))
			for _, id := range orderedIDs {
				placed[id] = true
			}
			for _, idx := range colIndices {
				id := tasks[idx].ID
				if !placed[id] {
					orderedIDs = append(orderedIDs, id)
				}
			}
		}

		cardByID := make(map[string]KanbanTask, len(colIndices))
		for _, idx := range colIndices {
			cardByID[tasks[idx].ID] = tasks[idx]
		}
		for pos, idx := range colIndices {
			tasks[idx] = cardByID[orderedIDs[pos]]
		}
	}
}

// columnOrder returns the sort position for a column name.
func columnOrder(col string) int {
	for i, c := range kanbanColumns {
		if c == col {
			return i
		}
	}
	return len(kanbanColumns)
}

// normalizeForMatch produces a normalized string for client-side matching:
// trimmed, collapsed whitespace, lowercased.
func normalizeForMatch(s string) string {
	s = strings.TrimSpace(s)
	s = collapseSpaces(s)
	return strings.ToLower(s)
}

// collapseSpaces replaces runs of whitespace with a single space.
func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(r)
			inSpace = false
		}
	}
	return b.String()
}

// dedupStrings removes duplicate entries from a string slice, preserving order.
func dedupStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// buildStatusSet creates a quick-lookup set from a slice of status strings.
func buildStatusSet(statuses []string) map[string]bool {
	m := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		m[s] = true
	}
	return m
}
