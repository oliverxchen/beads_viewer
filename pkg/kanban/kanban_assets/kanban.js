(function () {
    "use strict";

    var COLUMNS = ["draft", "blocked", "open", "in_progress", "review", "closed"];
    var COLUMN_LABELS = {
        draft: "Draft",
        blocked: "Blocked",
        open: "Open",
        in_progress: "In Progress",
        review: "In Review",
        closed: "Closed",
    };

    var board = document.getElementById("kanban-board");
    var filterInput = document.getElementById("filter-input");
    var filterDateStart = document.getElementById("filter-date-start");
    var filterDateEnd = document.getElementById("filter-date-end");
    var dateFilterClear = document.getElementById("date-filter-clear");
    var filterStatus = document.getElementById("filter-status");
    var detailOverlay = document.getElementById("detail-overlay");
    var detailPanel = document.getElementById("detail-panel");
    var detailContent = document.getElementById("detail-content");
    var detailClose = document.getElementById("detail-close");
    var blockerFilterClear = document.getElementById("blocker-filter-clear");
    var allCards = [];
    var cardsByTaskId = {};
    var totalCount = 0;
    var blockerGraph = {};
    var activeBlockerFilter = null;

    function normalize(s) {
        return s.trim().replace(/\s+/g, " ").toLowerCase();
    }

    function escapeHTML(s) {
        var d = document.createElement("div");
        d.textContent = s;
        return d.innerHTML;
    }

    function groupByColumn(tasks) {
        var grouped = {};
        for (var i = 0; i < COLUMNS.length; i++) {
            grouped[COLUMNS[i]] = [];
        }
        for (var j = 0; j < tasks.length; j++) {
            var col = tasks[j].column;
            if (!grouped[col]) grouped[col] = [];
            grouped[col].push(tasks[j]);
        }
        return grouped;
    }

    function createCard(task) {
        var card = document.createElement("div");
        card.className = "kanban-card";
        card.dataset.column = task.column;

        var matchTitle = task.match_task_title || normalize(task.title);
        var matchTaskId = normalize(task.id || "");
        var matchEpicTitles = (task.match_parent_epic_titles || []);
        var matchEpicIds = (task.match_parent_epic_ids || []);

        card._match = {
            title: matchTitle,
            taskId: matchTaskId,
            epicTitles: matchEpicTitles,
            epicIds: matchEpicIds,
        };

        var html = "";
        html += '<h3 class="card-title">' + escapeHTML(task.title) + "</h3>";
        html += '<span class="card-id">' + escapeHTML(task.id) + "</span>";
        html +=
            '<span class="priority-badge priority-' +
            task.priority +
            '">P' +
            task.priority +
            "</span>";

        if (task.assignee) {
            html +=
                '<div class="card-assignee">👤 ' +
                escapeHTML(task.assignee) +
                "</div>";
        }

        if (task.parent_epics && task.parent_epics.length > 0) {
            html += '<div class="card-epics">';
            for (var i = 0; i < task.parent_epics.length; i++) {
                var ep = task.parent_epics[i];
                html +=
                    '<span class="epic-tag">' +
                    escapeHTML(ep.title || ep.id) +
                    "</span>";
            }
            html += "</div>";
        }

        if (task.blocked_by_tasks && task.blocked_by_tasks.length > 0) {
            html += '<div class="blocker-chips">';
            for (var b = 0; b < task.blocked_by_tasks.length; b++) {
                var blocker = task.blocked_by_tasks[b];
                html +=
                    '<span class="blocker-chip" data-blocker-id="' + escapeHTML(blocker.id) + '">' +
                    '<span class="blocker-chip-dot" style="background:' + statusDotColor(blocker.status) + '"></span>' +
                    '<span class="blocker-chip-id">' + escapeHTML(blocker.id) + '</span>' +
                    '<span class="blocker-chip-title">' + escapeHTML(truncate(blocker.title, 30)) + '</span>' +
                    '</span>';
            }
            html += "</div>";
        }

        var hasBlockers = task.blocked_by_tasks && task.blocked_by_tasks.length > 0;
        var hasSubtasks = task.subtasks && task.subtasks.length > 0;

        if (hasSubtasks || hasBlockers) {
            html += '<div class="card-footer-row">';
            if (hasSubtasks) {
                html += "<details class=\"card-subtasks\">";
                html += "<summary>Subtasks (" + task.subtasks.length + ")</summary>";
            html += '<ul class="subtask-list">';
            for (var s = 0; s < task.subtasks.length; s++) {
                var st = task.subtasks[s];
                html += '<li class="subtask-item" data-subtask-index="' + s + '">';
                html +=
                    '<span class="subtask-status status-' +
                    escapeHTML(st.status) +
                    '">●</span> ';
                html += escapeHTML(st.title);
                html +=
                    ' <span class="subtask-id">' +
                    escapeHTML(st.id) +
                    "</span>";
                html += "</li>";
            }
            html += "</ul></details>";
            }
            if (hasBlockers) {
                html += '<button class="show-blockers-btn" data-task-id="' + escapeHTML(task.id) + '" title="Show all blockers">🚫</button>';
            }
            html += "</div>";
        }

        card.innerHTML = html;
        card._task = task;
        card.style.cursor = "pointer";
        card.addEventListener("click", function (e) {
            var chipEl = e.target.closest(".blocker-chip");
            if (chipEl) {
                e.stopPropagation();
                var blockerId = chipEl.dataset.blockerId;
                if (cardsByTaskId[blockerId]) {
                    scrollToCard(blockerId);
                }
                return;
            }
            var showBtn = e.target.closest(".show-blockers-btn");
            if (showBtn) {
                e.stopPropagation();
                applyBlockerFilter(showBtn.dataset.taskId);
                return;
            }
            var subtaskItem = e.target.closest(".subtask-item");
            if (subtaskItem) {
                e.stopPropagation();
                var idx = parseInt(subtaskItem.dataset.subtaskIndex, 10);
                openDetail(task.subtasks[idx], card);
                return;
            }
            if (e.target.closest("summary")) return;
            openDetail(task, card);
        });
        return card;
    }

    function renderMarkdown(text) {
        if (!text) return "";
        try {
            return marked.parse(text);
        } catch (e) {
            return escapeHTML(text);
        }
    }

    function formatDate(s) {
        if (!s) return "";
        var d = new Date(s);
        if (isNaN(d.getTime())) return escapeHTML(s);
        return d.toLocaleDateString(undefined, { year: "numeric", month: "short", day: "numeric" });
    }

    function buildDetailHTML(task) {
        var html = "";

        // Header
        html += '<div class="detail-header">';
        html += '<span class="detail-id">' + escapeHTML(task.id) + "</span>";
        html +=
            '<span class="priority-badge priority-' +
            task.priority + '">P' + task.priority + "</span>";
        html +=
            '<span class="detail-status status-' +
            escapeHTML(task.status) + '">' + escapeHTML(task.status) + "</span>";
        html += "</div>";
        html += '<h2 class="detail-title">' + escapeHTML(task.title) + "</h2>";

        // Meta row
        var metaParts = [];
        if (task.assignee) metaParts.push("👤 " + escapeHTML(task.assignee));
        if (task.updated_at) metaParts.push("Updated " + formatDate(task.updated_at));
        if (task.created_at) metaParts.push("Created " + formatDate(task.created_at));
        if (metaParts.length > 0) {
            html += '<div class="detail-meta">' + metaParts.join(" · ") + "</div>";
        }

        // Labels
        if (task.labels && task.labels.length > 0) {
            html += '<div class="detail-labels">';
            for (var i = 0; i < task.labels.length; i++) {
                html += '<span class="label-tag">' + escapeHTML(task.labels[i]) + "</span>";
            }
            html += "</div>";
        }

        // Parent epics
        if (task.parent_epics && task.parent_epics.length > 0) {
            html += '<div class="detail-epics">';
            for (var e = 0; e < task.parent_epics.length; e++) {
                var ep = task.parent_epics[e];
                html += '<span class="epic-tag">' + escapeHTML(ep.title || ep.id) + "</span>";
            }
            html += "</div>";
        }

        // Blocked by — subtask-level detail if available, otherwise task-level
        var subtaskDetails = task.blocked_by_subtask_details;
        if (subtaskDetails && subtaskDetails.length > 0) {
            html += '<div class="detail-section">';
            html += '<h3 class="detail-section-title">Blocked By (detailed)</h3>';
            for (var sd = 0; sd < subtaskDetails.length; sd++) {
                var det = subtaskDetails[sd];
                html += '<div class="detail-blocker-entry" data-blocker-task-id="' + escapeHTML(det.blocker_task_id) + '">';
                html += '<span class="blocker-chip-dot" style="background:' + statusDotColor(det.blocker_status) + '"></span>';
                html += '<span>' + escapeHTML(truncate(det.blocked_title, 40)) + '</span>';
                html += '<span class="detail-blocker-arrow">→ blocked by →</span>';
                html += '<span>' + escapeHTML(truncate(det.blocker_title, 40)) + '</span>';
                html += ' <span class="detail-blocker-id">' + escapeHTML(det.blocker_id) + '</span>';
                if (det.blocker_task_id !== det.blocker_id) {
                    html += ' <span class="detail-blocker-id">(on ' + escapeHTML(det.blocker_task_title) + ')</span>';
                }
                html += ' <span class="blocker-status status-' + escapeHTML(det.blocker_status) + '">' + escapeHTML(det.blocker_status) + '</span>';
                html += '</div>';
            }
            html += "</div>";
        } else if (task.blocked_by_tasks && task.blocked_by_tasks.length > 0) {
            html += '<div class="detail-section">';
            html += '<h3 class="detail-section-title">Blocked By</h3>';
            for (var b = 0; b < task.blocked_by_tasks.length; b++) {
                var blocker = task.blocked_by_tasks[b];
                html += '<div class="detail-blocker-entry" data-blocker-task-id="' + escapeHTML(blocker.id) + '">';
                html += '<span class="blocker-chip-dot" style="background:' + statusDotColor(blocker.status) + '"></span>';
                html += '<span class="detail-blocker-id">' + escapeHTML(blocker.id) + "</span> ";
                html += '<span>' + escapeHTML(blocker.title) + '</span>';
                html += ' <span class="blocker-status status-' + escapeHTML(blocker.status) + '">' + escapeHTML(blocker.status) + "</span>";
                html += "</div>";
            }
            html += "</div>";
        }

        // Content sections
        var sections = [
            { key: "description", label: "Description" },
            { key: "design", label: "Design" },
            { key: "acceptance_criteria", label: "Acceptance Criteria" },
            { key: "notes", label: "Notes" },
        ];
        for (var s = 0; s < sections.length; s++) {
            var val = task[sections[s].key];
            if (val) {
                html += '<div class="detail-section">';
                html += '<h3 class="detail-section-title">' + sections[s].label + "</h3>";
                html += '<div class="prose">' + renderMarkdown(val) + "</div>";
                html += "</div>";
            }
        }

        // Comments
        if (task.comments && task.comments.length > 0) {
            html += '<div class="detail-section">';
            html += '<h3 class="detail-section-title">Comments (' + task.comments.length + ")</h3>";
            for (var c = 0; c < task.comments.length; c++) {
                var comment = task.comments[c];
                html += '<div class="detail-comment">';
                html += '<div class="comment-header">';
                if (comment.author) html += '<span class="comment-author">' + escapeHTML(comment.author) + "</span>";
                if (comment.created_at) html += '<span class="comment-date">' + formatDate(comment.created_at) + "</span>";
                html += "</div>";
                html += '<div class="prose prose-sm">' + renderMarkdown(comment.text) + "</div>";
                html += "</div>";
            }
            html += "</div>";
        }

        return html;
    }

    var savedScrollLeft = null;
    var boardSpacer = null;

    function openDetail(task, cardEl) {
        detailContent.innerHTML = buildDetailHTML(task);

        // Make detail blocker entries clickable
        var entries = detailContent.querySelectorAll(".detail-blocker-entry");
        for (var i = 0; i < entries.length; i++) {
            (function (entry) {
                entry.addEventListener("click", function () {
                    var targetId = entry.dataset.blockerTaskId;
                    var targetCard = cardsByTaskId[targetId];
                    if (targetCard && targetCard._task) {
                        openDetail(targetCard._task, targetCard);
                    }
                });
            })(entries[i]);
        }

        detailOverlay.classList.remove("hidden");
        detailPanel.focus();
        ensureCardVisible(cardEl);
    }

    function closeDetail() {
        detailOverlay.classList.add("hidden");
        detailContent.innerHTML = "";
        removeBoardSpacer();
        if (savedScrollLeft !== null) {
            board.scrollLeft = savedScrollLeft;
            savedScrollLeft = null;
        }
    }

    function addBoardSpacer(width) {
        if (!boardSpacer) {
            boardSpacer = document.createElement("div");
            boardSpacer.className = "board-spacer";
            boardSpacer.style.flex = "0 0 " + width + "px";
            boardSpacer.style.minWidth = width + "px";
            board.appendChild(boardSpacer);
        } else {
            boardSpacer.style.flex = "0 0 " + width + "px";
            boardSpacer.style.minWidth = width + "px";
        }
    }

    function removeBoardSpacer() {
        if (boardSpacer && boardSpacer.parentNode) {
            boardSpacer.parentNode.removeChild(boardSpacer);
            boardSpacer = null;
        }
    }

    function ensureCardVisible(cardEl) {
        if (!cardEl) return;
        var panelWidth = detailPanel.offsetWidth;
        var visibleRight = window.innerWidth - panelWidth;
        var cardRect = cardEl.getBoundingClientRect();
        // Card is already fully visible to the left of the panel
        if (cardRect.left >= 0 && cardRect.right <= visibleRight) return;
        // Need to scroll — save current position for restore on close
        savedScrollLeft = board.scrollLeft;
        var gap = 16;
        var scrollNeeded = cardRect.right - visibleRight + gap;
        // Check if the board can scroll far enough
        var maxScroll = board.scrollWidth - board.clientWidth;
        var canScroll = maxScroll - board.scrollLeft;
        if (canScroll < scrollNeeded) {
            // Add spacer to extend scrollable area
            addBoardSpacer(panelWidth + gap);
        }
        // Scroll so the card's right edge sits just left of the panel
        board.scrollLeft += scrollNeeded;
    }

    function updateColumnCounts() {
        for (var i = 0; i < COLUMNS.length; i++) {
            var col = COLUMNS[i];
            var container = document.querySelector(
                '.kanban-column[data-column="' + col + '"] .column-cards'
            );
            var countEl = document.querySelector(
                '.kanban-column[data-column="' + col + '"] .column-count'
            );
            var emptyEl = document.querySelector(
                '.kanban-column[data-column="' + col + '"] .column-empty'
            );
            var visible = container.querySelectorAll(
                ".kanban-card:not([style*='display: none'])"
            );
            countEl.textContent = visible.length;
            emptyEl.style.display = visible.length === 0 ? "block" : "none";
        }
    }

    function statusDotColor(status) {
        var colors = {
            open: "#6ab7e8", in_progress: "#f5c56a", review: "#c39bd3",
            closed: "#6ad4a0", blocked: "#f08080", draft: "#9a9aaa"
        };
        return colors[status] || "#6a6a8a";
    }

    function truncate(s, max) {
        if (!s) return "";
        return s.length > max ? s.substring(0, max) + "…" : s;
    }

    function scrollToCard(taskId) {
        var card = cardsByTaskId[taskId];
        if (!card) return;
        card.scrollIntoView({ behavior: "smooth", block: "nearest", inline: "center" });
        card.classList.remove("highlight-pulse");
        // Force reflow to restart animation
        void card.offsetWidth;
        card.classList.add("highlight-pulse");
        setTimeout(function () {
            card.classList.remove("highlight-pulse");
        }, 1000);
    }

    function collectTransitiveBlockers(taskId) {
        var result = {};
        var queue = [taskId];
        var depth = 0;
        var hasCycle = false;
        while (queue.length > 0 && depth < 10) {
            var next = [];
            for (var i = 0; i < queue.length; i++) {
                var deps = blockerGraph[queue[i]];
                if (!deps) continue;
                for (var j = 0; j < deps.length; j++) {
                    if (deps[j] === taskId) {
                        hasCycle = true;
                        continue;
                    }
                    if (!result[deps[j]]) {
                        result[deps[j]] = true;
                        next.push(deps[j]);
                    }
                }
            }
            queue = next;
            depth++;
        }
        return { ids: result, hasCycle: hasCycle };
    }

    function applyBlockerFilter(taskId) {
        activeBlockerFilter = taskId;
        var transitive = collectTransitiveBlockers(taskId);
        var allowSet = transitive.ids;
        allowSet[taskId] = true;

        var shown = 0;
        var offBoard = 0;
        for (var i = 0; i < allCards.length; i++) {
            var card = allCards[i];
            if (allowSet[card._task.id]) {
                card.style.display = "";
                shown++;
            } else {
                card.style.display = "none";
            }
        }

        // Count blockers not on board
        for (var id in allowSet) {
            if (id !== taskId && !cardsByTaskId[id]) {
                offBoard++;
            }
        }

        var statusMsg = "Showing blockers for " + taskId + " (" + shown + " of " + totalCount + " tasks)";
        if (transitive.hasCycle) {
            statusMsg = "⚠️ Circular dependency detected — showing reachable blockers for " + taskId;
        }
        if (offBoard > 0) {
            statusMsg += " · " + offBoard + " blocker" + (offBoard > 1 ? "s" : "") + " not on board";
        }
        filterStatus.textContent = statusMsg;
        blockerFilterClear.style.display = "inline-block";
        updateColumnCounts();
    }

    function clearBlockerFilter() {
        activeBlockerFilter = null;
        blockerFilterClear.style.display = "none";
        applyFilter();
    }

    function updateDateFilterClearVisibility() {
        var hasDate = filterDateStart.value !== "" || filterDateEnd.value !== "";
        dateFilterClear.style.display = hasDate ? "inline-block" : "none";
    }

    function parseDateBound(value, isEnd) {
        if (!value) return null;
        var d = new Date(value + "T00:00:00");
        if (isNaN(d.getTime())) return null;
        if (isEnd) {
            d.setDate(d.getDate() + 1);
        }
        return d;
    }

    function applyFilter() {
        var raw = filterInput.value;
        var query = normalize(raw);
        var startDate = parseDateBound(filterDateStart.value, false);
        var endDate = parseDateBound(filterDateEnd.value, true);
        var hasTextFilter = query !== "";
        var hasDateFilter = startDate !== null || endDate !== null;

        if (!hasTextFilter && !hasDateFilter) {
            for (var i = 0; i < allCards.length; i++) {
                allCards[i].style.display = "";
            }
            filterStatus.textContent = "";
            updateColumnCounts();
            return;
        }

        var shown = 0;
        for (var j = 0; j < allCards.length; j++) {
            var card = allCards[j];
            var m = card._match;
            var task = card._task;
            var matched = true;

            if (hasTextFilter) {
                var textMatch = false;
                if (m.title.indexOf(query) !== -1) {
                    textMatch = true;
                }
                if (!textMatch && m.taskId.indexOf(query) !== -1) {
                    textMatch = true;
                }
                if (!textMatch) {
                    for (var k = 0; k < m.epicTitles.length; k++) {
                        if (m.epicTitles[k].indexOf(query) !== -1) {
                            textMatch = true;
                            break;
                        }
                    }
                }
                if (!textMatch) {
                    for (var k2 = 0; k2 < m.epicIds.length; k2++) {
                        if (m.epicIds[k2].indexOf(query) !== -1) {
                            textMatch = true;
                            break;
                        }
                    }
                }
                if (!textMatch) matched = false;
            }

            if (matched && hasDateFilter && task.updated_at) {
                var updated = new Date(task.updated_at);
                if (!isNaN(updated.getTime())) {
                    if (startDate && updated < startDate) matched = false;
                    if (endDate && updated >= endDate) matched = false;
                }
            }

            card.style.display = matched ? "" : "none";
            if (matched) shown++;
        }

        if (shown === 0) {
            filterStatus.textContent = "No tasks match filter";
        } else {
            filterStatus.textContent =
                "Showing " + shown + " of " + totalCount + " tasks";
        }
        updateColumnCounts();
    }

    function renderBoard(data) {
        var grouped = groupByColumn(data.tasks || []);
        board.innerHTML = "";
        blockerGraph = data.blocker_graph || {};

        for (var i = 0; i < COLUMNS.length; i++) {
            var colKey = COLUMNS[i];
            var tasks = grouped[colKey] || [];

            var colEl = document.createElement("div");
            colEl.className = "kanban-column column-" + colKey;
            colEl.dataset.column = colKey;

            var header = document.createElement("div");
            header.className = "column-header";
            header.innerHTML =
                '<span class="column-name">' +
                escapeHTML(COLUMN_LABELS[colKey]) +
                '</span><span class="column-count">' +
                tasks.length +
                "</span>";
            colEl.appendChild(header);

            var cardsContainer = document.createElement("div");
            cardsContainer.className = "column-cards";

            for (var t = 0; t < tasks.length; t++) {
                var card = createCard(tasks[t]);
                cardsContainer.appendChild(card);
                allCards.push(card);
                cardsByTaskId[tasks[t].id] = card;
                totalCount++;
            }

            var emptyEl = document.createElement("div");
            emptyEl.className = "column-empty";
            emptyEl.textContent = "No tasks";
            emptyEl.style.display = tasks.length === 0 ? "block" : "none";
            cardsContainer.appendChild(emptyEl);

            colEl.appendChild(cardsContainer);
            board.appendChild(colEl);
        }

        // Mark off-board blocker chips
        for (var ci = 0; ci < allCards.length; ci++) {
            var chips = allCards[ci].querySelectorAll(".blocker-chip");
            for (var ch = 0; ch < chips.length; ch++) {
                var bid = chips[ch].dataset.blockerId;
                if (!cardsByTaskId[bid]) {
                    chips[ch].classList.add("off-board");
                    chips[ch].title = "Not visible on board";
                }
            }
        }
    }

    function showError(msg) {
        board.innerHTML =
            '<div class="board-error">' + escapeHTML(msg) + "</div>";
    }

    function init(data) {
        renderBoard(data);
        filterInput.addEventListener("input", applyFilter);
        filterDateStart.addEventListener("change", function () {
            updateDateFilterClearVisibility();
            applyFilter();
        });
        filterDateEnd.addEventListener("change", function () {
            updateDateFilterClearVisibility();
            applyFilter();
        });
        dateFilterClear.addEventListener("click", function () {
            filterDateStart.value = "";
            filterDateEnd.value = "";
            updateDateFilterClearVisibility();
            applyFilter();
        });
        blockerFilterClear.addEventListener("click", clearBlockerFilter);

        detailClose.addEventListener("click", closeDetail);
        detailOverlay.addEventListener("click", function (e) {
            if (e.target === detailOverlay) closeDetail();
        });
        document.addEventListener("keydown", function (e) {
            if (e.key === "Escape") {
                if (activeBlockerFilter) {
                    clearBlockerFilter();
                } else if (!detailOverlay.classList.contains("hidden")) {
                    closeDetail();
                }
            }
        });
    }

    // Prefer inline data (works with file:// URLs), fall back to fetch.
    if (typeof window.__KANBAN_DATA !== "undefined") {
        init(window.__KANBAN_DATA);
    } else {
        fetch("data/kanban_board.json")
            .then(function (res) {
                if (!res.ok)
                    throw new Error("Failed to load data: " + res.statusText);
                return res.json();
            })
            .then(init)
            .catch(function (err) {
                showError(err.message || "Failed to load Kanban data.");
            });
    }
})();
