package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lawrips/tkt/internal/ticket"
)

// column indices
const (
	colOpen         = 0
	colInProgress   = 1
	colNeedsTesting = 2
	colClosed       = 3
	numCols         = 4
)

var columnStatuses = [numCols]string{
	"open",
	"in_progress",
	"needs_testing",
	"closed",
}

var columnLabels = [numCols]string{
	"Open",
	"In Progress",
	"Needs Testing",
	"Closed",
}

// Board is the kanban board view model.
type Board struct {
	dir            string
	columns        [numCols][]ticket.Record
	columnIndents  [numCols][]bool // parallel to columns: true if row is an epic child
	col            int // focused column index
	row            int // focused row within column
	width          int
	height         int
	err            error
	headerLines    int // extra header lines reserved by parent (e.g. filter bar)
	latestByID     map[string]string
}

// ticketsLoadedMsg carries tickets loaded from disk.
type ticketsLoadedMsg struct {
	records []ticket.Record
	err     error
}

func newBoard(dir string) Board {
	return Board{dir: dir}
}

func loadTicketsCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		records, err := ticket.List(dir)
		return ticketsLoadedMsg{records: records, err: err}
	}
}

func (b Board) init() tea.Cmd {
	return loadTicketsCmd(b.dir)
}

func (b Board) update(msg tea.Msg, km KeyMap) (Board, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		b.width = msg.Width
		b.height = msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, km.Left):
			if b.col > 0 {
				b.col--
				b.clampRow()
			}
		case key.Matches(msg, km.Right):
			if b.col < numCols-1 {
				b.col++
				b.clampRow()
			}
		case key.Matches(msg, km.Up):
			if b.row > 0 {
				b.row--
			}
		case key.Matches(msg, km.Down):
			col := b.columns[b.col]
			if b.row < len(col)-1 {
				b.row++
			}
		case key.Matches(msg, km.HalfPageDown):
			col := b.columns[b.col]
			half := b.visibleRows() / 2
			if half < 1 {
				half = 1
			}
			b.row += half
			if max := len(col) - 1; max >= 0 && b.row > max {
				b.row = max
			}
		case key.Matches(msg, km.HalfPageUp):
			half := b.visibleRows() / 2
			if half < 1 {
				half = 1
			}
			b.row -= half
			if b.row < 0 {
				b.row = 0
			}
		case key.Matches(msg, km.GoTop):
			b.row = 0
		case key.Matches(msg, km.GoBottom):
			col := b.columns[b.col]
			if len(col) > 0 {
				b.row = len(col) - 1
			}
		}
	}
	return b, nil
}

// visibleRows returns the number of ticket rows visible in a column.
func (b Board) visibleRows() int {
	previewHeight := 5
	h := b.height - previewHeight - 4 - b.headerLines
	if h < 3 {
		h = 3
	}
	return h
}

func (b *Board) clampRow() {
	col := b.columns[b.col]
	if b.row >= len(col) {
		if len(col) == 0 {
			b.row = 0
		} else {
			b.row = len(col) - 1
		}
	}
}

func (b Board) view() string {
	if b.err != nil {
		return fmt.Sprintf("error loading tickets: %v\n", b.err)
	}

	if b.width == 0 {
		return ""
	}

	previewHeight := 5
	columnsHeight := b.height - previewHeight - 4 - b.headerLines // borders + padding + header
	if columnsHeight < 3 {
		columnsHeight = 3
	}

	// Build each column.
	colWidth := (b.width - numCols*4) / numCols // subtract border+padding per col
	if colWidth < 12 {
		colWidth = 12
	}

	cols := make([]string, numCols)
	for i := 0; i < numCols; i++ {
		cols[i] = b.renderColumn(i, colWidth, columnsHeight)
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)

	preview := b.renderPreview(b.width - 4)

	return lipgloss.JoinVertical(lipgloss.Left, board, preview)
}

func (b Board) renderColumn(idx, width, height int) string {
	records := b.columns[idx]
	isFocused := idx == b.col

	headerStyle := columnHeaderStyle(idx)
	header := headerStyle.Render(fmt.Sprintf("%s (%d)", columnLabels[idx], len(records)))

	indents := b.columnIndents[idx]
	lines := make([]string, 0, len(records))
	for i, rec := range records {
		child := i < len(indents) && indents[i]
		lines = append(lines, b.renderCard(rec, idx == b.col && i == b.row, width, child))
	}

	if len(lines) == 0 {
		lines = append(lines, helpStyle.Render("  (empty)"))
	}

	// Truncate visible rows to fit height (leave 1 row for header).
	maxCards := height - 2 // header + padding
	if maxCards < 1 {
		maxCards = 1
	}

	// Scroll so selected item is visible.
	start := 0
	if isFocused && b.row >= maxCards {
		start = b.row - maxCards + 1
	}
	end := start + maxCards
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[start:end]

	body := strings.Join(visible, "\n")
	content := header + "\n" + body

	style := columnStyle
	if isFocused {
		style = columnFocusedStyle
	}

	return style.Width(width).Height(height).Render(content)
}

func (b Board) renderCard(rec ticket.Record, selected bool, width int, child bool) string {
	badge := priorityBadge(rec.Front.Priority)
	// badge is now a single digit + space = 2 chars rendered, padded to 2
	badgeLen := 2 // "0" + space
	indent := 0
	if child {
		indent = 1
	}
	titleWidth := width - badgeLen - 4 - indent
	if titleWidth < 4 {
		titleWidth = 4
	}

	rawTitle := rec.Body.Title
	if rec.Front.Type == "epic" && rawTitle != "" {
		rawTitle = "EPIC: " + rawTitle
	}
	title := truncate(rawTitle, titleWidth)
	if title == "" {
		title = rec.ID
	}

	var line string
	if child {
		line = fmt.Sprintf(" %s %s", badge, title)
	} else {
		line = fmt.Sprintf("%s %s", badge, title)
	}

	if selected {
		return cardSelectedStyle.Render(line)
	}
	return cardNormalStyle.Render(line)
}

func (b Board) renderPreview(width int) string {
	rec, ok := b.selectedRecord()
	if !ok {
		return previewStyle.Width(width).Render(helpStyle.Render("no ticket selected"))
	}

	f := rec.Front

	// Build label:value pairs.
	pairs := []struct{ label, value string }{
		{"id", rec.ID},
		{"status", f.Status},
		{"type", f.Type},
		{"priority", fmt.Sprintf("p%d", f.Priority)},
		{"assignee", orDash(f.Assignee)},
		{"parent", orDash(f.Parent)},
		{"deps", orDash(strings.Join(f.Deps, ", "))},
		{"title", orDash(rec.Body.Title)},
		{"commit", b.latestCommitShort(rec.ID)},
	}

	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		label := previewLabelStyle.Render(fmt.Sprintf("%-10s", p.label))
		value := previewValueStyle.Render(p.value)
		parts = append(parts, label+" "+value)
	}

	// Lay out in two rows of four.
	row1 := lipgloss.JoinHorizontal(lipgloss.Top,
		padRight(parts[0], 28),
		padRight(parts[1], 24),
		padRight(parts[2], 20),
		parts[3],
	)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top,
		padRight(parts[4], 28),
		padRight(parts[5], 24),
		padRight(parts[6], 20),
		parts[7],
	)
	row3 := parts[8]

	content := row1 + "\n" + row2 + "\n" + row3
	return previewStyle.Width(width).Render(content)
}

func (b Board) latestCommitShort(ticketID string) string {
	if b.latestByID == nil {
		return "-"
	}
	sha := b.latestByID[ticketID]
	if sha == "" {
		return "-"
	}
	return sha
}

func (b Board) selectedRecord() (ticket.Record, bool) {
	col := b.columns[b.col]
	if len(col) == 0 || b.row >= len(col) {
		return ticket.Record{}, false
	}
	return col[b.row], true
}

// setColumns updates the board columns from the given (possibly filtered) records
// and resets the selection so it stays in bounds.
func (b *Board) setColumns(records []ticket.Record) {
	selectedID := ""
	if rec, ok := b.selectedRecord(); ok {
		selectedID = rec.ID
	}

	b.columns, b.columnIndents = groupByStatus(records)

	if selectedID != "" {
		for c := 0; c < numCols; c++ {
			for r, rec := range b.columns[c] {
				if rec.ID == selectedID {
					b.col = c
					b.row = r
					return
				}
			}
		}
	}
	b.clampRow()
}

func (b *Board) setLatestCommitsByID(latest map[string]string) {
	b.latestByID = latest
}

func groupByStatus(records []ticket.Record) ([numCols][]ticket.Record, [numCols][]bool) {
	var cols [numCols][]ticket.Record
	for i := range cols {
		cols[i] = []ticket.Record{}
	}
	for _, rec := range records {
		idx := statusToCol(rec.Front.Status)
		cols[idx] = append(cols[idx], rec)
	}
	var indents [numCols][]bool
	for i := range cols {
		sortColumnRecords(cols[i], i)
		cols[i], indents[i] = applyEpicGrouping(cols[i])
	}
	return cols, indents
}

// applyEpicGrouping reorders a sorted column slice so each epic is immediately
// followed by its children (tickets whose Front.Parent matches the epic's ID).
// Children that appear later in the walk are skipped at their original position.
// Returns the reordered slice and a parallel bool slice marking child rows.
func applyEpicGrouping(records []ticket.Record) ([]ticket.Record, []bool) {
	// Build a set of epic IDs present in this column for O(1) lookup.
	epicIDs := make(map[string]bool, len(records))
	for _, rec := range records {
		if rec.Front.Type == "epic" {
			epicIDs[rec.ID] = true
		}
	}

	// Build map from epic ID → children in this column (in original sort order).
	childrenOf := make(map[string][]ticket.Record, len(records))
	childSet := make(map[string]bool, len(records))
	for _, rec := range records {
		if rec.Front.Parent != "" && epicIDs[rec.Front.Parent] {
			childrenOf[rec.Front.Parent] = append(childrenOf[rec.Front.Parent], rec)
			childSet[rec.ID] = true
		}
	}

	out := make([]ticket.Record, 0, len(records))
	indents := make([]bool, 0, len(records))

	for _, rec := range records {
		// Skip children — they'll be emitted under their epic.
		if childSet[rec.ID] {
			continue
		}
		out = append(out, rec)
		indents = append(indents, false)

		// If this is an epic with children in this column, emit them now.
		if rec.Front.Type == "epic" {
			for _, child := range childrenOf[rec.ID] {
				out = append(out, child)
				indents = append(indents, true)
			}
		}
	}

	return out, indents
}

// sortColumnRecords applies the default board ordering for a status column:
// - active columns: priority asc (p0 first), modtime desc, id asc
// - closed column: modtime desc, id asc
func sortColumnRecords(records []ticket.Record, col int) {
	sort.SliceStable(records, func(i, j int) bool {
		a := records[i]
		b := records[j]

		if col != colClosed {
			if a.Front.Priority != b.Front.Priority {
				return a.Front.Priority < b.Front.Priority
			}
		}

		if !a.ModTime.Equal(b.ModTime) {
			return a.ModTime.After(b.ModTime)
		}

		return a.ID < b.ID
	})
}

func statusToCol(status string) int {
	for i, s := range columnStatuses {
		if s == status {
			return i
		}
	}
	// unknown / empty status goes to Open column
	return colOpen
}

func columnHeaderStyle(idx int) lipgloss.Style {
	switch idx {
	case colOpen:
		return headerOpenStyle
	case colInProgress:
		return headerInProgressStyle
	case colNeedsTesting:
		return headerNeedsTestingStyle
	case colClosed:
		return headerClosedStyle
	}
	return headerOpenStyle
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func padRight(s string, width int) string {
	// pad with spaces to reach a fixed layout width (approximate — ANSI safe)
	return fmt.Sprintf("%-*s", width, s)
}
