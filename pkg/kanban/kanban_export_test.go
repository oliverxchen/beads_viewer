package kanban

import (
	"testing"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
)

var (
	t1 = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	t2 = time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	t3 = time.Date(2026, 3, 3, 9, 0, 0, 0, time.UTC)
	t4 = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
)

func makeIssue(id, title string, status model.Status, issueType model.IssueType, deps ...*model.Dependency) model.Issue {
	return model.Issue{
		ID:           id,
		Title:        title,
		Status:       status,
		IssueType:    issueType,
		Priority:     2,
		CreatedAt:    t1,
		UpdatedAt:    t2,
		Dependencies: deps,
	}
}

func parentChildDep(childID, parentID string) *model.Dependency {
	return &model.Dependency{IssueID: childID, DependsOnID: parentID, Type: model.DepParentChild}
}

func blocksDep(dependentID, blockerID string) *model.Dependency {
	return &model.Dependency{IssueID: dependentID, DependsOnID: blockerID, Type: model.DepBlocks}
}

func TestBuildKanbanBoardData_OnlyTasksBecomeCards(t *testing.T) {
	issues := []model.Issue{
		makeIssue("task-1", "Task One", model.StatusOpen, model.TypeTask),
		makeIssue("bug-1", "Bug One", model.StatusOpen, model.TypeBug),
		makeIssue("feat-1", "Feature One", model.StatusOpen, model.TypeFeature),
		makeIssue("epic-1", "Epic One", model.StatusOpen, model.TypeEpic),
		makeIssue("task-2", "Task Two", model.StatusInProgress, model.TypeTask),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	if len(data.Tasks) != 2 {
		t.Fatalf("expected 2 task cards, got %d", len(data.Tasks))
	}
	ids := map[string]bool{}
	for _, tk := range data.Tasks {
		ids[tk.ID] = true
	}
	if !ids["task-1"] || !ids["task-2"] {
		t.Fatalf("expected task-1 and task-2, got %v", ids)
	}
}

func TestBuildKanbanBoardData_SubtasksInExpansionOnly(t *testing.T) {
	issues := []model.Issue{
		makeIssue("task-1", "Parent Task", model.StatusOpen, model.TypeTask),
		{
			ID: "sub-1", Title: "Subtask One", Status: model.StatusOpen,
			IssueType: "subtask", Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{parentChildDep("sub-1", "task-1")},
		},
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	if len(data.Tasks) != 1 {
		t.Fatalf("expected 1 top-level card, got %d", len(data.Tasks))
	}
	if data.Tasks[0].ID != "task-1" {
		t.Fatalf("expected task-1 as card, got %s", data.Tasks[0].ID)
	}
	if len(data.Tasks[0].Subtasks) != 1 {
		t.Fatalf("expected 1 subtask, got %d", len(data.Tasks[0].Subtasks))
	}
	if data.Tasks[0].Subtasks[0].ID != "sub-1" {
		t.Fatalf("expected sub-1 as subtask, got %s", data.Tasks[0].Subtasks[0].ID)
	}
}

func TestBuildKanbanBoardData_EpicParentResolution(t *testing.T) {
	issues := []model.Issue{
		makeIssue("epic-1", "Auth Revamp", model.StatusOpen, model.TypeEpic),
		makeIssue("task-1", "Implement Login", model.StatusOpen, model.TypeTask,
			parentChildDep("task-1", "epic-1")),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	if len(data.Tasks) != 1 {
		t.Fatalf("expected 1 card, got %d", len(data.Tasks))
	}
	card := data.Tasks[0]
	if len(card.ParentEpics) != 1 || card.ParentEpics[0].ID != "epic-1" {
		t.Fatalf("expected epic-1 parent, got %v", card.ParentEpics)
	}
	if len(card.MatchParentEpicTitles) != 1 || card.MatchParentEpicTitles[0] != "auth revamp" {
		t.Fatalf("expected normalized epic title, got %v", card.MatchParentEpicTitles)
	}
	if len(card.MatchParentEpicIDs) != 1 || card.MatchParentEpicIDs[0] != "epic-1" {
		t.Fatalf("expected epic-1 in match IDs, got %v", card.MatchParentEpicIDs)
	}
}

func TestBuildKanbanBoardData_StatusMapping(t *testing.T) {
	tests := []struct {
		status model.Status
		column string
	}{
		{model.StatusDraft, "draft"},
		{model.StatusBlocked, "blocked"},
		{model.StatusOpen, "open"},
		{model.StatusInProgress, "in_progress"},
		{model.StatusReview, "review"},
		{model.StatusClosed, "closed"},
		{model.StatusTombstone, "closed"},
		{model.StatusDeferred, "open"},
		{model.StatusPinned, "open"},
		{model.StatusHooked, "open"},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			issue := makeIssue("t-1", "Test", tc.status, model.TypeTask)
			data := BuildKanbanBoardData([]model.Issue{issue}, "Test", true)
			if len(data.Tasks) != 1 {
				t.Fatalf("expected 1 card, got %d", len(data.Tasks))
			}
			if data.Tasks[0].Column != tc.column {
				t.Errorf("status %q: expected column %q, got %q", tc.status, tc.column, data.Tasks[0].Column)
			}
		})
	}
}

func TestBuildKanbanBoardData_ComputedBlocked(t *testing.T) {
	issues := []model.Issue{
		makeIssue("blocker-1", "Blocker", model.StatusInProgress, model.TypeTask),
		makeIssue("task-1", "Dependent", model.StatusOpen, model.TypeTask,
			blocksDep("task-1", "blocker-1")),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	var task *KanbanTask
	for i := range data.Tasks {
		if data.Tasks[i].ID == "task-1" {
			task = &data.Tasks[i]
			break
		}
	}
	if task == nil {
		t.Fatal("task-1 not found")
	}
	if task.Column != "blocked" {
		t.Errorf("expected blocked column, got %q", task.Column)
	}
	if len(task.BlockedByTasks) != 1 || task.BlockedByTasks[0].ID != "blocker-1" {
		t.Errorf("expected blocked_by_tasks=[blocker-1], got %v", task.BlockedByTasks)
	}
}

func TestBuildKanbanBoardData_ReviewBlockerDoesNotPromote(t *testing.T) {
	issues := []model.Issue{
		makeIssue("blocker-1", "Blocker In Review", model.StatusReview, model.TypeTask),
		makeIssue("task-1", "Dependent", model.StatusOpen, model.TypeTask,
			blocksDep("task-1", "blocker-1")),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	var task *KanbanTask
	for i := range data.Tasks {
		if data.Tasks[i].ID == "task-1" {
			task = &data.Tasks[i]
			break
		}
	}
	if task == nil {
		t.Fatal("task-1 not found")
	}
	if task.Column != "open" {
		t.Errorf("expected open column (review blocker is non-blocking), got %q", task.Column)
	}
}

func TestBuildKanbanBoardData_RawBlockedStaysWithEmptyBlockers(t *testing.T) {
	issue := makeIssue("task-1", "Blocked Task", model.StatusBlocked, model.TypeTask)
	data := BuildKanbanBoardData([]model.Issue{issue}, "Test", true)
	if len(data.Tasks) != 1 {
		t.Fatalf("expected 1 card, got %d", len(data.Tasks))
	}
	if data.Tasks[0].Column != "blocked" {
		t.Errorf("expected blocked column, got %q", data.Tasks[0].Column)
	}
	// blocked_by_tasks may be nil/empty for raw blocked status
}

func TestBuildKanbanBoardData_UnknownStatusWarning(t *testing.T) {
	issues := []model.Issue{
		{
			ID: "blocker-1", Title: "Custom Status Blocker",
			Status: model.Status("custom_status"), IssueType: model.TypeTask,
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
		},
		makeIssue("task-1", "Dependent", model.StatusOpen, model.TypeTask,
			blocksDep("task-1", "blocker-1")),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	foundWarning := false
	for _, w := range data.Meta.Warnings {
		if contains(w, "unknown status") && contains(w, "custom_status") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("expected warning about unknown status, got warnings: %v", data.Meta.Warnings)
	}
}

func TestBuildKanbanBoardData_ExcludeClosedAndTombstone(t *testing.T) {
	issues := []model.Issue{
		makeIssue("task-1", "Open Task", model.StatusOpen, model.TypeTask),
		makeIssue("task-2", "Closed Task", model.StatusClosed, model.TypeTask),
		makeIssue("task-3", "Tombstone Task", model.StatusTombstone, model.TypeTask),
	}
	data := BuildKanbanBoardData(issues, "Test", false)
	if len(data.Tasks) != 1 {
		t.Fatalf("expected 1 card with includeClosed=false, got %d", len(data.Tasks))
	}
	if data.Tasks[0].ID != "task-1" {
		t.Errorf("expected task-1, got %s", data.Tasks[0].ID)
	}
}

func TestBuildKanbanBoardData_IncludeClosedTrue(t *testing.T) {
	issues := []model.Issue{
		makeIssue("task-1", "Open Task", model.StatusOpen, model.TypeTask),
		makeIssue("task-2", "Closed Task", model.StatusClosed, model.TypeTask),
		makeIssue("task-3", "Tombstone Task", model.StatusTombstone, model.TypeTask),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	if len(data.Tasks) != 3 {
		t.Fatalf("expected 3 cards with includeClosed=true, got %d", len(data.Tasks))
	}
}

func TestBuildKanbanBoardData_NonClosedSortingPriorityThenImpact(t *testing.T) {
	issues := []model.Issue{
		// Blockers to create downstream impact counts.
		makeIssue("task-upstream-1", "Upstream 1", model.StatusOpen, model.TypeTask),
		makeIssue("task-upstream-2", "Upstream 2", model.StatusOpen, model.TypeTask),

		// P1 should outrank P2 regardless of impact.
		{ID: "task-p1", Title: "Priority 1", Status: model.StatusOpen, IssueType: model.TypeTask,
			Priority: 1, CreatedAt: t3, UpdatedAt: t1},

		// Two P2 items: impact should break tie before created_at.
		{ID: "task-p2-high-impact", Title: "P2 High Impact", Status: model.StatusOpen, IssueType: model.TypeTask,
			Priority: 2, CreatedAt: t4, UpdatedAt: t1},
		{ID: "task-p2-low-impact", Title: "P2 Low Impact", Status: model.StatusOpen, IssueType: model.TypeTask,
			Priority: 2, CreatedAt: t1, UpdatedAt: t4},

		// Dependents: two tasks blocked by high-impact item, one by low-impact item.
		makeIssue("task-dependent-1", "Dependent 1", model.StatusBlocked, model.TypeTask,
			blocksDep("task-dependent-1", "task-p2-high-impact")),
		makeIssue("task-dependent-2", "Dependent 2", model.StatusBlocked, model.TypeTask,
			blocksDep("task-dependent-2", "task-p2-high-impact")),
		makeIssue("task-dependent-3", "Dependent 3", model.StatusBlocked, model.TypeTask,
			blocksDep("task-dependent-3", "task-p2-low-impact")),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	openTasks := filterByColumn(data.Tasks, "open")
	if len(openTasks) != 5 {
		t.Fatalf("expected 5 open tasks, got %d", len(openTasks))
	}
	if openTasks[0].ID != "task-p1" {
		t.Errorf("expected task-p1 first by priority, got %s", openTasks[0].ID)
	}
	if openTasks[1].ID != "task-p2-high-impact" || openTasks[2].ID != "task-p2-low-impact" {
		t.Errorf("expected impact tie-break among P2 tasks, got [%s, %s]", openTasks[1].ID, openTasks[2].ID)
	}
}

func TestBuildKanbanBoardData_NonClosedSortingTieBreakers(t *testing.T) {
	issues := []model.Issue{
		makeIssue("task-blocker-1", "Blocker 1", model.StatusOpen, model.TypeTask),
		makeIssue("task-blocker-2", "Blocker 2", model.StatusOpen, model.TypeTask),

		// Same priority and same downstream impact (0), blocked_by_count breaks tie
		// in the blocked column.
		{ID: "task-less-blocked", Title: "Less Blocked", Status: model.StatusBlocked, IssueType: model.TypeTask,
			Priority: 2, CreatedAt: t2, UpdatedAt: t4,
			Dependencies: []*model.Dependency{blocksDep("task-less-blocked", "task-blocker-1")}},
		{ID: "task-more-blocked", Title: "More Blocked", Status: model.StatusBlocked, IssueType: model.TypeTask,
			Priority: 2, CreatedAt: t1, UpdatedAt: t1,
			Dependencies: []*model.Dependency{
				blocksDep("task-more-blocked", "task-blocker-1"),
				blocksDep("task-more-blocked", "task-blocker-2"),
			}},

		// Same priority, impact, and blocked_by_count; created_at ASC then id ASC.
		{ID: "task-b", Title: "B", Status: model.StatusOpen, IssueType: model.TypeTask,
			Priority: 2, CreatedAt: t3, UpdatedAt: t2},
		{ID: "task-a", Title: "A", Status: model.StatusOpen, IssueType: model.TypeTask,
			Priority: 2, CreatedAt: t3, UpdatedAt: t2},
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	blockedTasks := filterByColumn(data.Tasks, "blocked")
	if len(blockedTasks) != 2 {
		t.Fatalf("expected 2 blocked tasks, got %d", len(blockedTasks))
	}
	if blockedTasks[0].ID != "task-less-blocked" || blockedTasks[1].ID != "task-more-blocked" {
		t.Errorf("expected blocked order [task-less-blocked, task-more-blocked], got [%s, %s]",
			blockedTasks[0].ID, blockedTasks[1].ID)
	}

	openTasks := filterByColumn(data.Tasks, "open")
	if len(openTasks) != 4 {
		t.Fatalf("expected 4 open tasks, got %d", len(openTasks))
	}

	// task-a should come before task-b due to id ASC when created_at is tied.
	var aIdx, bIdx int = -1, -1
	for i := range openTasks {
		if openTasks[i].ID == "task-a" {
			aIdx = i
		}
		if openTasks[i].ID == "task-b" {
			bIdx = i
		}
	}
	if aIdx == -1 || bIdx == -1 {
		t.Fatalf("expected both task-a and task-b in open column, got %+v", openTasks)
	}
	if !(aIdx < bIdx) {
		t.Errorf("expected task-a before task-b, got indexes %d and %d", aIdx, bIdx)
	}
}

func TestBuildKanbanBoardData_ClosedSortingRecencyFirst(t *testing.T) {
	issues := []model.Issue{
		{ID: "closed-1", Title: "Old", Status: model.StatusClosed, IssueType: model.TypeTask,
			Priority: 1, CreatedAt: t1, UpdatedAt: t1},
		{ID: "closed-2", Title: "New", Status: model.StatusClosed, IssueType: model.TypeTask,
			Priority: 4, CreatedAt: t1, UpdatedAt: t3},
		{ID: "closed-3", Title: "Mid", Status: model.StatusClosed, IssueType: model.TypeTask,
			Priority: 0, CreatedAt: t1, UpdatedAt: t2},
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	closedTasks := filterByColumn(data.Tasks, "closed")
	if len(closedTasks) != 3 {
		t.Fatalf("expected 3 closed tasks, got %d", len(closedTasks))
	}
	if closedTasks[0].ID != "closed-2" || closedTasks[1].ID != "closed-3" || closedTasks[2].ID != "closed-1" {
		t.Errorf("expected closed recency order [closed-2, closed-3, closed-1], got [%s, %s, %s]",
			closedTasks[0].ID, closedTasks[1].ID, closedTasks[2].ID)
	}
}

func TestBuildKanbanBoardData_DependencyPrecedenceWithinColumn(t *testing.T) {
	issues := []model.Issue{
		// Task 6 blocks task 10.
		{ID: "task-6", Title: "Task 6", Status: model.StatusBlocked, IssueType: model.TypeTask,
			Priority: 2, CreatedAt: t1, UpdatedAt: t1},
		{ID: "task-10", Title: "Task 10", Status: model.StatusBlocked, IssueType: model.TypeTask,
			Priority: 2, CreatedAt: t1, UpdatedAt: t1,
			Dependencies: []*model.Dependency{blocksDep("task-10", "task-6")}},

		// Dependents of task-10 increase its downstream impact count.
		makeIssue("task-11", "Task 11", model.StatusBlocked, model.TypeTask,
			blocksDep("task-11", "task-10")),
		makeIssue("task-12", "Task 12", model.StatusBlocked, model.TypeTask,
			blocksDep("task-12", "task-10")),
	}

	data := BuildKanbanBoardData(issues, "Test", true)
	blockedTasks := filterByColumn(data.Tasks, "blocked")

	// Even though task-10 has higher blocking_tasks_count, task-6 must appear
	// first because task-10 directly depends on task-6.
	idx6, idx10 := -1, -1
	for i := range blockedTasks {
		if blockedTasks[i].ID == "task-6" {
			idx6 = i
		}
		if blockedTasks[i].ID == "task-10" {
			idx10 = i
		}
	}
	if idx6 == -1 || idx10 == -1 {
		t.Fatalf("expected both task-6 and task-10 in blocked column, got %+v", blockedTasks)
	}
	if !(idx6 < idx10) {
		t.Errorf("expected task-6 before task-10 due to dependency precedence, got indexes %d and %d", idx6, idx10)
	}
}

func TestBuildKanbanBoardData_BlockedByTasksSortedByID(t *testing.T) {
	issues := []model.Issue{
		makeIssue("blocker-c", "C Blocker", model.StatusOpen, model.TypeTask),
		makeIssue("blocker-a", "A Blocker", model.StatusInProgress, model.TypeTask),
		makeIssue("blocker-b", "B Blocker", model.StatusDraft, model.TypeTask),
		makeIssue("task-1", "Dependent", model.StatusOpen, model.TypeTask,
			blocksDep("task-1", "blocker-c"),
			blocksDep("task-1", "blocker-a"),
			blocksDep("task-1", "blocker-b")),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	var task *KanbanTask
	for i := range data.Tasks {
		if data.Tasks[i].ID == "task-1" {
			task = &data.Tasks[i]
			break
		}
	}
	if task == nil {
		t.Fatal("task-1 not found")
	}
	if len(task.BlockedByTasks) != 3 {
		t.Fatalf("expected 3 blockers, got %d", len(task.BlockedByTasks))
	}
	if task.BlockedByTasks[0].ID != "blocker-a" || task.BlockedByTasks[1].ID != "blocker-b" || task.BlockedByTasks[2].ID != "blocker-c" {
		t.Errorf("expected sorted [blocker-a, blocker-b, blocker-c], got [%s, %s, %s]",
			task.BlockedByTasks[0].ID, task.BlockedByTasks[1].ID, task.BlockedByTasks[2].ID)
	}
}

func TestBuildKanbanBoardData_OnlyDirectBlockers(t *testing.T) {
	issues := []model.Issue{
		makeIssue("root", "Root", model.StatusOpen, model.TypeTask),
		makeIssue("mid", "Middle", model.StatusOpen, model.TypeTask,
			blocksDep("mid", "root")),
		makeIssue("leaf", "Leaf", model.StatusOpen, model.TypeTask,
			blocksDep("leaf", "mid")),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	var leaf *KanbanTask
	for i := range data.Tasks {
		if data.Tasks[i].ID == "leaf" {
			leaf = &data.Tasks[i]
			break
		}
	}
	if leaf == nil {
		t.Fatal("leaf not found")
	}
	if len(leaf.BlockedByTasks) != 1 {
		t.Fatalf("expected 1 direct blocker, got %d", len(leaf.BlockedByTasks))
	}
	if leaf.BlockedByTasks[0].ID != "mid" {
		t.Errorf("expected mid as blocker, got %s", leaf.BlockedByTasks[0].ID)
	}
}

func TestBuildKanbanBoardData_ColumnOrderFixed(t *testing.T) {
	data := BuildKanbanBoardData(nil, "Test", true)
	expected := []string{"draft", "blocked", "open", "in_progress", "review", "closed"}
	if len(data.Columns) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(data.Columns))
	}
	for i, col := range expected {
		if data.Columns[i] != col {
			t.Errorf("column[%d]: expected %q, got %q", i, col, data.Columns[i])
		}
	}
}

func TestBuildKanbanBoardData_MetaFields(t *testing.T) {
	issues := []model.Issue{
		makeIssue("task-1", "T1", model.StatusOpen, model.TypeTask),
	}
	data := BuildKanbanBoardData(issues, "My Board", true)
	if data.Meta.Template != "kanban" {
		t.Errorf("expected template=kanban, got %q", data.Meta.Template)
	}
	if data.Meta.Title != "My Board" {
		t.Errorf("expected title='My Board', got %q", data.Meta.Title)
	}
	if data.Meta.SourceIssueCount != 1 {
		t.Errorf("expected source_issue_count=1, got %d", data.Meta.SourceIssueCount)
	}
	if len(data.Meta.KanbanBlockingStatuses) == 0 {
		t.Error("expected non-empty blocking statuses")
	}
	if len(data.Meta.KanbanNonBlockingStatuses) == 0 {
		t.Error("expected non-empty non-blocking statuses")
	}
}

func TestBuildKanbanBoardData_SubtaskLinearChainOrder(t *testing.T) {
	issues := []model.Issue{
		makeIssue("task-1", "Parent", model.StatusOpen, model.TypeTask),
		{ID: "sub-a", Title: "Step A", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{parentChildDep("sub-a", "task-1")}},
		{ID: "sub-b", Title: "Step B", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{
				parentChildDep("sub-b", "task-1"),
				blocksDep("sub-b", "sub-a"),
			}},
		{ID: "sub-c", Title: "Step C", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{
				parentChildDep("sub-c", "task-1"),
				blocksDep("sub-c", "sub-b"),
			}},
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	if len(data.Tasks) != 1 {
		t.Fatalf("expected 1 card, got %d", len(data.Tasks))
	}
	subs := data.Tasks[0].Subtasks
	if len(subs) != 3 {
		t.Fatalf("expected 3 subtasks, got %d", len(subs))
	}
	// Chain: sub-a → sub-b → sub-c
	if subs[0].ID != "sub-a" || subs[1].ID != "sub-b" || subs[2].ID != "sub-c" {
		t.Errorf("expected chain [sub-a, sub-b, sub-c], got [%s, %s, %s]",
			subs[0].ID, subs[1].ID, subs[2].ID)
	}
}

func TestBuildKanbanBoardData_SubtaskChainViolationFallback(t *testing.T) {
	issues := []model.Issue{
		makeIssue("task-1", "Parent", model.StatusOpen, model.TypeTask),
		{ID: "sub-b", Title: "B", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{parentChildDep("sub-b", "task-1")}},
		{ID: "sub-a", Title: "A", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{parentChildDep("sub-a", "task-1")}},
	}
	// No blocking deps between subtasks → two roots → falls back to ID sort
	data := BuildKanbanBoardData(issues, "Test", true)
	subs := data.Tasks[0].Subtasks
	if len(subs) != 2 {
		t.Fatalf("expected 2 subtasks, got %d", len(subs))
	}
	// ID ascending: sub-a before sub-b
	if subs[0].ID != "sub-a" || subs[1].ID != "sub-b" {
		t.Errorf("expected fallback [sub-a, sub-b], got [%s, %s]", subs[0].ID, subs[1].ID)
	}
	// Should have a warning
	foundWarning := false
	for _, w := range data.Meta.Warnings {
		if contains(w, "linear blocking chain") {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Errorf("expected chain violation warning, got: %v", data.Meta.Warnings)
	}
}

func TestBuildKanbanBoardData_MatchFieldNormalization(t *testing.T) {
	issue := model.Issue{
		ID: "task-1", Title: "  Hello   World  ", Status: model.StatusOpen,
		IssueType: model.TypeTask, Priority: 2, CreatedAt: t1, UpdatedAt: t2,
	}
	data := BuildKanbanBoardData([]model.Issue{issue}, "Test", true)
	if data.Tasks[0].MatchTaskTitle != "hello world" {
		t.Errorf("expected normalized 'hello world', got %q", data.Tasks[0].MatchTaskTitle)
	}
}

func TestBuildKanbanBoardData_EpicMatchExpansion(t *testing.T) {
	issues := []model.Issue{
		makeIssue("epic-1", "Shared Epic", model.StatusOpen, model.TypeEpic),
		makeIssue("task-1", "T1", model.StatusOpen, model.TypeTask,
			parentChildDep("task-1", "epic-1")),
		makeIssue("task-2", "T2", model.StatusOpen, model.TypeTask,
			parentChildDep("task-2", "epic-1")),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	if len(data.Tasks) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(data.Tasks))
	}
	for _, tk := range data.Tasks {
		if len(tk.MatchParentEpicIDs) != 1 || tk.MatchParentEpicIDs[0] != "epic-1" {
			t.Errorf("task %s: expected epic-1 in match IDs, got %v", tk.ID, tk.MatchParentEpicIDs)
		}
		if len(tk.MatchParentEpicTitles) != 1 || tk.MatchParentEpicTitles[0] != "shared epic" {
			t.Errorf("task %s: expected 'shared epic' in match titles, got %v", tk.ID, tk.MatchParentEpicTitles)
		}
	}
}

func TestBuildKanbanBoardData_SubtaskBlockerRollup(t *testing.T) {
	// Task X has subtask sub-x, Task Y has subtask sub-y.
	// sub-x is blocked by sub-y → Task X should show Task Y as a blocker.
	issues := []model.Issue{
		makeIssue("task-x", "Task X", model.StatusOpen, model.TypeTask),
		makeIssue("task-y", "Task Y", model.StatusInProgress, model.TypeTask),
		{ID: "sub-x", Title: "Subtask X", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{
				parentChildDep("sub-x", "task-x"),
				blocksDep("sub-x", "sub-y"),
			}},
		{ID: "sub-y", Title: "Subtask Y", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{parentChildDep("sub-y", "task-y")}},
	}
	data := BuildKanbanBoardData(issues, "Test", true)

	var taskX *KanbanTask
	for i := range data.Tasks {
		if data.Tasks[i].ID == "task-x" {
			taskX = &data.Tasks[i]
			break
		}
	}
	if taskX == nil {
		t.Fatal("task-x not found")
	}
	if taskX.Column != "blocked" {
		t.Errorf("expected task-x in blocked column, got %q", taskX.Column)
	}
	if len(taskX.BlockedByTasks) != 1 || taskX.BlockedByTasks[0].ID != "task-y" {
		t.Errorf("expected blocked_by_tasks=[task-y], got %v", taskX.BlockedByTasks)
	}
	if len(taskX.BlockedBySubtaskDetails) != 1 {
		t.Fatalf("expected 1 subtask blocker detail, got %d", len(taskX.BlockedBySubtaskDetails))
	}
	det := taskX.BlockedBySubtaskDetails[0]
	if det.BlockedID != "sub-x" || det.BlockerID != "sub-y" || det.BlockerTaskID != "task-y" {
		t.Errorf("unexpected detail: %+v", det)
	}

	// BlockerGraph should have task-x → [task-y]
	if data.BlockerGraph == nil {
		t.Fatal("expected non-nil blocker_graph")
	}
	deps, ok := data.BlockerGraph["task-x"]
	if !ok || len(deps) != 1 || deps[0] != "task-y" {
		t.Errorf("expected blocker_graph[task-x]=[task-y], got %v", deps)
	}
}

func TestBuildKanbanBoardData_DraftWithBlockersStaysInDraft(t *testing.T) {
	// A draft task with blocking deps should remain in "draft", not be promoted to "blocked".
	issues := []model.Issue{
		makeIssue("blocker-1", "Blocker", model.StatusInProgress, model.TypeTask),
		makeIssue("task-1", "Draft Task", model.StatusDraft, model.TypeTask,
			blocksDep("task-1", "blocker-1")),
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	var task *KanbanTask
	for i := range data.Tasks {
		if data.Tasks[i].ID == "task-1" {
			task = &data.Tasks[i]
			break
		}
	}
	if task == nil {
		t.Fatal("task-1 not found")
	}
	if task.Column != "draft" {
		t.Errorf("expected draft column, got %q", task.Column)
	}
}

func TestBuildKanbanBoardData_DraftWithSubtaskBlockersStaysInDraft(t *testing.T) {
	// A draft task whose subtask is blocked by another task's subtask should stay in "draft".
	issues := []model.Issue{
		makeIssue("task-x", "Draft Task", model.StatusDraft, model.TypeTask),
		makeIssue("task-y", "Other Task", model.StatusInProgress, model.TypeTask),
		{ID: "sub-x", Title: "Subtask X", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{
				parentChildDep("sub-x", "task-x"),
				blocksDep("sub-x", "sub-y"),
			}},
		{ID: "sub-y", Title: "Subtask Y", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{parentChildDep("sub-y", "task-y")}},
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	var taskX *KanbanTask
	for i := range data.Tasks {
		if data.Tasks[i].ID == "task-x" {
			taskX = &data.Tasks[i]
			break
		}
	}
	if taskX == nil {
		t.Fatal("task-x not found")
	}
	if taskX.Column != "draft" {
		t.Errorf("expected draft column, got %q", taskX.Column)
	}
	// Should still have blocker info even though it stays in draft
	if len(taskX.BlockedByTasks) != 1 || taskX.BlockedByTasks[0].ID != "task-y" {
		t.Errorf("expected blocked_by_tasks=[task-y], got %v", taskX.BlockedByTasks)
	}
}

func TestBuildKanbanBoardData_InternalSubtaskBlockerSkipped(t *testing.T) {
	// sub-a blocks sub-b, both under task-1 → no external blocker chip.
	issues := []model.Issue{
		makeIssue("task-1", "Parent", model.StatusOpen, model.TypeTask),
		{ID: "sub-a", Title: "A", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{parentChildDep("sub-a", "task-1")}},
		{ID: "sub-b", Title: "B", Status: model.StatusOpen, IssueType: "subtask",
			Priority: 2, CreatedAt: t1, UpdatedAt: t2,
			Dependencies: []*model.Dependency{
				parentChildDep("sub-b", "task-1"),
				blocksDep("sub-b", "sub-a"),
			}},
	}
	data := BuildKanbanBoardData(issues, "Test", true)
	task := data.Tasks[0]
	if len(task.BlockedBySubtaskDetails) != 0 {
		t.Errorf("expected no subtask blocker details for internal deps, got %d", len(task.BlockedBySubtaskDetails))
	}
	if task.Column != "open" {
		t.Errorf("expected open column (internal blockers only), got %q", task.Column)
	}
}

// helpers

func filterByColumn(tasks []KanbanTask, col string) []KanbanTask {
	var out []KanbanTask
	for _, tk := range tasks {
		if tk.Column == col {
			out = append(out, tk)
		}
	}
	return out
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstr(s, substr)
}

func searchSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
