package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lawrips/tkt/internal/journal"
	"github.com/lawrips/tkt/internal/ticket"
)

// GoBackMsg is sent when the user presses Esc in the detail view.
type GoBackMsg struct{}

// NavigateToTicketMsg is sent when the user selects a ticket link in the detail view.
type NavigateToTicketMsg struct {
	TicketID string
}

var (
	detailLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(colorDetailLabel)
	detailValueStyle = lipgloss.NewStyle()

	detailHeadingStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorMagenta)
	detailSectionStyle   = lipgloss.NewStyle().Foreground(colorSecondary)
	detailPlaceholder    = lipgloss.NewStyle().Italic(true).Foreground(colorHint)
	detailStatusBarStyle = lipgloss.NewStyle().Foreground(colorHint)
	detailLinkStyle      = lipgloss.NewStyle().Foreground(colorLink).Underline(true)
	detailLinkActiveStyle = lipgloss.NewStyle().Bold(true).Foreground(colorLink).Underline(true)
)

// DetailModel is the sub-model for the full-screen ticket detail view.
type DetailModel struct {
	record      ticket.Record
	commits     []journal.Entry
	filterDesc  string // human-readable active filter summary, empty if none
	viewport    viewport.Model
	width       int
	height      int
	escKey      key.Binding
	ticketLinks []string // navigable ticket IDs (parent, deps, links)
	linkCursor  int      // -1 = no link selected, 0+ = index into ticketLinks
}

// NewDetail returns an initialized DetailModel for the given record.
func NewDetail(r ticket.Record, commits []journal.Entry) DetailModel {
	vp := viewport.New(0, 0)

	// Collect navigable ticket IDs from parent, deps, and links.
	var links []string
	if r.Front.Parent != "" {
		links = append(links, r.Front.Parent)
	}
	links = append(links, r.Front.Deps...)
	links = append(links, r.Front.Links...)

	return DetailModel{
		record:      r,
		commits:     commits,
		viewport:    vp,
		ticketLinks: links,
		linkCursor:  -1,
		escKey: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
	}
}

// SetSize updates the dimensions of the detail view and refreshes the viewport.
func (m *DetailModel) SetSize(w, h int) {
	m.width = w
	// Reserve two lines at the bottom: status bar + help bar.
	m.height = h
	m.viewport.Width = w
	m.viewport.Height = h - 2
	m.viewport.SetContent(m.renderContent())
}

// Init implements tea.Model.
func (m DetailModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m DetailModel) Update(msg tea.Msg) (DetailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 2
		m.viewport.SetContent(m.renderContent())
		return m, nil

	case tea.KeyMsg:
		if key.Matches(msg, m.escKey) {
			if m.linkCursor >= 0 {
				// Deselect link first.
				m.linkCursor = -1
				m.viewport.SetContent(m.renderContent())
				return m, nil
			}
			return m, func() tea.Msg { return GoBackMsg{} }
		}

		if len(m.ticketLinks) > 0 {
			switch msg.String() {
			case "tab":
				m.linkCursor++
				if m.linkCursor >= len(m.ticketLinks) {
					m.linkCursor = 0
				}
				m.viewport.SetContent(m.renderContent())
				return m, nil
			case "shift+tab":
				if m.linkCursor <= 0 {
					m.linkCursor = len(m.ticketLinks) - 1
				} else {
					m.linkCursor--
				}
				m.viewport.SetContent(m.renderContent())
				return m, nil
			case "enter":
				if m.linkCursor >= 0 && m.linkCursor < len(m.ticketLinks) {
					id := m.ticketLinks[m.linkCursor]
					return m, func() tea.Msg { return NavigateToTicketMsg{TicketID: id} }
				}
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the detail view.
func (m DetailModel) View() string {
	filterInfo := ""
	if m.filterDesc != "" {
		filterInfo = fmt.Sprintf("  •  filter: %s", m.filterDesc)
	}
	statusBar := detailStatusBarStyle.Render(
		fmt.Sprintf(" %s  •  %d%%%s",
			m.record.ID,
			int(m.viewport.ScrollPercent()*100),
			filterInfo,
		),
	)
	return m.viewport.View() + "\n" + statusBar
}

// renderContent builds the full scrollable content string.
func (m DetailModel) renderContent() string {
	var b strings.Builder
	width := m.width
	if width <= 0 {
		width = 80
	}

	// ── Frontmatter ──────────────────────────────────────────────────────────
	b.WriteString(detailHeadingStyle.Render("Ticket Details"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", min(width, 60)))
	b.WriteString("\n\n")

	f := m.record.Front
	writeField(&b, "ID", f.ID)
	writeField(&b, "Status", RenderStatus(f.Status))
	if f.Type != "" {
		writeField(&b, "Type", f.Type)
	}
	writeField(&b, "Priority", fmt.Sprintf("p%d", f.Priority))
	if f.Assignee != "" {
		writeField(&b, "Assignee", f.Assignee)
	}
	if f.Parent != "" {
		writeFieldWithLinks(&b, "Parent", []string{f.Parent}, m.ticketLinks, m.linkCursor)
	}
	if len(f.Tags) > 0 {
		writeField(&b, "Tags", strings.Join(f.Tags, ", "))
	}
	if len(f.Deps) > 0 {
		writeFieldWithLinks(&b, "Deps", f.Deps, m.ticketLinks, m.linkCursor)
	}
	if len(f.Links) > 0 {
		writeFieldWithLinks(&b, "Links", f.Links, m.ticketLinks, m.linkCursor)
	}
	if f.Created != "" {
		writeField(&b, "Created", f.Created)
	}
	if f.ExternalRef != "" {
		writeField(&b, "External Ref", f.ExternalRef)
	}

	lc := journal.Lifecycle(f.Created, f.Status, m.commits, time.Now().UTC())
	if lc.Opened != "" || lc.FirstCommit != "" || lc.WorkStarted != "" || lc.ClosedAt != "" {
		b.WriteString("\n")
		b.WriteString(detailHeadingStyle.Render("Lifecycle"))
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", min(width, 60)))
		b.WriteString("\n")
		if lc.Opened != "" {
			writeField(&b, "Opened", lc.Opened)
		}
		if lc.FirstCommit != "" {
			writeField(&b, "First Commit", lc.FirstCommit)
		}
		if lc.LastCommit != "" {
			writeField(&b, "Last Commit", lc.LastCommit)
		}
		if lc.WorkStarted != "" {
			writeField(&b, "Work Start", lc.WorkStarted)
		}
		if lc.WorkEnded != "" {
			writeField(&b, "Work End", lc.WorkEnded)
		}
		if lc.ClosedAt != "" {
			writeField(&b, "Closed", lc.ClosedAt)
		}
		writeField(&b, "Calendar", journal.FormatSeconds(lc.CalendarSeconds))
		writeField(&b, "Work", journal.FormatSeconds(lc.WorkSeconds))
		writeField(&b, "Idle", journal.FormatSeconds(lc.IdleSeconds))
	}

	b.WriteString("\n")

	// ── Body ─────────────────────────────────────────────────────────────────
	body := m.record.Body

	if body.Title != "" {
		displayTitle := body.Title
		if m.record.Front.Type == "epic" {
			displayTitle = "EPIC: " + displayTitle
		}
		b.WriteString(detailHeadingStyle.Width(width).Render(displayTitle))
		b.WriteString("\n\n")
	}

	if body.Description != "" {
		b.WriteString(detailSectionStyle.Width(width).Render(body.Description))
		b.WriteString("\n\n")
	}

	if body.Design != "" {
		b.WriteString(detailHeadingStyle.Render("Design"))
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", min(width, 60)))
		b.WriteString("\n")
		b.WriteString(detailSectionStyle.Width(width).Render(body.Design))
		b.WriteString("\n\n")
	}

	if body.AcceptanceCriteria != "" {
		b.WriteString(detailHeadingStyle.Render("Acceptance Criteria"))
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", min(width, 60)))
		b.WriteString("\n")
		b.WriteString(detailSectionStyle.Width(width).Render(body.AcceptanceCriteria))
		b.WriteString("\n\n")
	}

	for _, section := range body.OtherSections {
		b.WriteString(detailHeadingStyle.Render(section.Heading))
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", min(width, 60)))
		b.WriteString("\n")
		b.WriteString(detailSectionStyle.Width(width).Render(section.Content))
		b.WriteString("\n\n")
	}

	// ── Linked Commits ───────────────────────────────────────────────────────
	b.WriteString(detailHeadingStyle.Render("Linked Commits"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", min(width, 60)))
	b.WriteString("\n")
	if len(m.commits) == 0 {
		b.WriteString(detailPlaceholder.Render("(no linked commits)"))
		b.WriteString("\n")
	} else {
		effort := journal.Effort(m.commits)
		if s := effort.String(); s != "" {
			b.WriteString(detailSectionStyle.Render(
				fmt.Sprintf("  Effort: %s across %d commit(s)", s, effort.Commits),
			))
			b.WriteString("\n")
		}
		for _, c := range m.commits {
			short := c.SHA
			if len(short) > 7 {
				short = short[:7]
			}
			action := ""
			if c.Action == "close" {
				action = " [closes]"
			}
			stats := ""
			if c.LinesAdded > 0 || c.LinesRemoved > 0 {
				stats = fmt.Sprintf(" (+%d -%d)", c.LinesAdded, c.LinesRemoved)
			}
			b.WriteString(fmt.Sprintf("  %s  %s  %s%s%s\n", short, c.TS, journal.FirstLine(c.Msg), stats, action))
		}
	}

	return b.String()
}

func writeField(b *strings.Builder, label, value string) {
	b.WriteString(detailLabelStyle.Render(fmt.Sprintf("%-14s", label)))
	b.WriteString("  ")
	b.WriteString(detailValueStyle.Render(value))
	b.WriteString("\n")
}

// writeFieldWithLinks renders a field where each ID is a navigable link.
// The active link (matching linkCursor into allLinks) is highlighted.
func writeFieldWithLinks(b *strings.Builder, label string, ids []string, allLinks []string, cursor int) {
	b.WriteString(detailLabelStyle.Render(fmt.Sprintf("%-14s", label)))
	b.WriteString("  ")
	for i, id := range ids {
		if i > 0 {
			b.WriteString(detailValueStyle.Render(", "))
		}
		// Find this ID's position in the global links list.
		active := false
		if cursor >= 0 {
			for li, linkID := range allLinks {
				if linkID == id && li == cursor {
					active = true
					break
				}
			}
		}
		if active {
			b.WriteString(detailLinkActiveStyle.Render("▸ " + id))
		} else {
			b.WriteString(detailLinkStyle.Render(id))
		}
	}
	b.WriteString("\n")
}
