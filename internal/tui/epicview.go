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

// DrillIntoTicketMsg is sent when the user presses Enter on a child ticket.
type DrillIntoTicketMsg struct {
	Record ticket.Record
}

// EpicDetailMsg is sent when the user selects the "see all details" link.
type EpicDetailMsg struct {
	Record ticket.Record
}

// EpicGoBackMsg is sent when the user presses Esc in the epic view.
type EpicGoBackMsg struct{}

// epicViewKeys holds key bindings used by the epic view.
type epicViewKeys struct {
	Up    key.Binding
	Down  key.Binding
	Enter key.Binding
	Back  key.Binding
}

var defaultEpicViewKeys = epicViewKeys{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("k/↑", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("j/↓", "move down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "open ticket"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "go back"),
	),
}

var (
	epicTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorMagenta)
	epicSubtitleStyle = lipgloss.NewStyle().Foreground(colorSecondary)
	epicCursorStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorMagentaBright)
	epicLinkStyle     = lipgloss.NewStyle().Foreground(colorLink).Underline(true)
	epicLinkActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(colorLink).Underline(true)
)

// EpicModel is the sub-model for the epic tree view.
type EpicModel struct {
	epic         ticket.Record
	children     []ticket.Record
	cursor       int  // 0-based index into children
	cursorOnLink bool // true when cursor is on the "see all details" link
	keys         epicViewKeys
	width        int
	filterDesc   string
	latestByID   map[string]string
	effortByID   map[string]string // ticketID -> "+N -N, K files"
}

// NewEpicModel builds an EpicModel from an epic record and the full records list.
func NewEpicModel(epic ticket.Record, allRecords []ticket.Record, latestByID map[string]string) EpicModel {
	children := make([]ticket.Record, 0)
	for _, r := range allRecords {
		if r.Front.Parent == epic.ID {
			children = append(children, r)
		}
	}
	sort.Slice(children, func(i, j int) bool {
		return children[i].ID < children[j].ID
	})
	return EpicModel{
		epic:     epic,
		children: children,
		cursor:   0,
		keys:     defaultEpicViewKeys,
		latestByID: latestByID,
	}
}

// SetWidth stores the terminal width for use during rendering.
func (m *EpicModel) SetWidth(w int) {
	m.width = w
}

// Update handles key messages and returns updated model plus any command.
func (m EpicModel) Update(msg tea.Msg) (EpicModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursorOnLink {
				// Already at top, do nothing.
			} else if m.cursor > 0 {
				m.cursor--
			} else {
				// At first child, move up to link.
				m.cursorOnLink = true
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursorOnLink {
				if len(m.children) > 0 {
					m.cursorOnLink = false
					m.cursor = 0
				}
			} else if m.cursor < len(m.children)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Enter):
			if m.cursorOnLink {
				rec := m.epic
				return m, func() tea.Msg { return EpicDetailMsg{Record: rec} }
			}
			if len(m.children) > 0 {
				selected := m.children[m.cursor]
				return m, func() tea.Msg { return DrillIntoTicketMsg{Record: selected} }
			}
		case key.Matches(msg, m.keys.Back):
			return m, func() tea.Msg { return EpicGoBackMsg{} }
		}
	}
	return m, nil
}

// View renders the epic view as a string.
func (m EpicModel) View() string {
	var sb strings.Builder

	// Header: epic title and status
	epicStatus := RenderStatus(m.epic.Front.Status)
	sb.WriteString(epicTitleStyle.Render(fmt.Sprintf("Epic: %s", m.epic.Body.Title)))
	sb.WriteString("  ")
	sb.WriteString(epicStatus)
	sb.WriteString("\n")
	sb.WriteString(epicSubtitleStyle.Render(fmt.Sprintf("ID: %s", m.epic.ID)))
	sb.WriteString("\n")

	// Completion percentage
	total := len(m.children)
	closed := 0
	for _, c := range m.children {
		if c.Front.Status == "closed" {
			closed++
		}
	}
	var pct int
	if total > 0 {
		pct = (closed * 100) / total
	}
	sb.WriteString(epicSubtitleStyle.Render(
		fmt.Sprintf("Progress: %d/%d children closed (%d%%)", closed, total, pct),
	))
	sb.WriteString("\n")
	if m.filterDesc != "" {
		sb.WriteString(epicSubtitleStyle.Render(fmt.Sprintf("Filter: %s", m.filterDesc)))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Description preview (first 5 lines).
	if desc := strings.TrimSpace(m.epic.Body.Description); desc != "" {
		lines := strings.Split(desc, "\n")
		if len(lines) > 5 {
			lines = append(lines[:5], "...")
		}
		sb.WriteString(epicSubtitleStyle.Render(strings.Join(lines, "\n")))
		sb.WriteString("\n\n")
	}

	// "See all details" link.
	if m.cursorOnLink {
		sb.WriteString(epicCursorStyle.Render("> "))
		sb.WriteString(epicLinkActiveStyle.Render("[see epic details]"))
	} else {
		sb.WriteString("  ")
		sb.WriteString(epicLinkStyle.Render("[see epic details]"))
	}
	sb.WriteString("\n\n")

	// Children tree
	if total == 0 {
		sb.WriteString(epicSubtitleStyle.Render("  (no children)"))
		sb.WriteString("\n")
	} else {
		// Build dep index: for each child, which deps does it have?
		depIndex := buildDepIndex(m.children)
		for i, child := range m.children {
			sb.WriteString(m.renderChild(i, child, depIndex))
			sb.WriteString("\n")
		}
	}

	// Dep edge count summary
	totalDeps := 0
	for _, child := range m.children {
		totalDeps += len(child.Front.Deps)
	}
	sb.WriteString("\n")
	sb.WriteString(epicSubtitleStyle.Render(
		fmt.Sprintf("Dependencies: %d edge(s)", totalDeps),
	))
	sb.WriteString("\n")

	return sb.String()
}

// renderChild renders one child row in the tree.
func (m EpicModel) renderChild(i int, child ticket.Record, depIndex map[string][]string) string {
	cursor := "  "
	if i == m.cursor && !m.cursorOnLink {
		cursor = epicCursorStyle.Render("> ")
	}

	status := RenderStatus(child.Front.Status)

	priority := epicSubtitleStyle.Render(fmt.Sprintf("p%d", child.Front.Priority)) + " "

	title := child.Body.Title
	if title == "" {
		title = child.ID
	}

	var deps string
	if inbound, ok := depIndex[child.ID]; ok && len(inbound) > 0 {
		deps = epicSubtitleStyle.Render(fmt.Sprintf(" ← [%s]", strings.Join(inbound, ", ")))
	}
	latest := ""
	if sha, ok := m.latestByID[child.ID]; ok && sha != "" {
		latest = epicSubtitleStyle.Render(" @" + sha)
	}
	effort := ""
	if e, ok := m.effortByID[child.ID]; ok && e != "" {
		effort = epicSubtitleStyle.Render(" [" + e + "]")
	}

	return fmt.Sprintf("%s%s %s%s%s%s%s%s",
		cursor,
		status,
		priority,
		child.ID,
		" "+title,
		deps,
		latest,
		effort,
	)
}


// buildDepIndex returns a map of childID -> list of other children that depend on it.
// A dep edge "A deps on B" means B has an inbound arrow from A: shown on B as "← [A]".
func buildDepIndex(children []ticket.Record) map[string][]string {
	// set of child IDs for fast lookup
	childSet := map[string]struct{}{}
	for _, c := range children {
		childSet[c.ID] = struct{}{}
	}

	// inbound: target -> list of sources within children
	inbound := map[string][]string{}
	for _, c := range children {
		for _, dep := range c.Front.Deps {
			if _, ok := childSet[dep]; ok {
				inbound[dep] = append(inbound[dep], c.ID)
			}
		}
	}
	return inbound
}
