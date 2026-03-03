package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lawrips/tkt/internal/ticket"
)

var filterBarStyle = lipgloss.NewStyle().Foreground(colorMuted)
var filterActiveStyle = lipgloss.NewStyle().Foreground(colorPrimary)
var filterInputStyle = lipgloss.NewStyle().Foreground(colorYellow)

// Filter holds the active filter criteria for the board.
type Filter struct {
	text       string // free-text search (id + title + description)
	assignee   string
	tag        string
	ticketType string
	priority   int    // -1 = any
	status     string // "" = any
	parent     string
}

// isEmpty returns true when no filter criteria are active.
func (f Filter) isEmpty() bool {
	return f.text == "" &&
		f.assignee == "" &&
		f.tag == "" &&
		f.ticketType == "" &&
		f.priority == -1 &&
		f.status == "" &&
		f.parent == ""
}

// description returns a human-readable summary of the active filter.
func (f Filter) description() string {
	if f.isEmpty() {
		return "none"
	}
	parts := make([]string, 0, 6)
	if f.ticketType != "" {
		parts = append(parts, "type="+f.ticketType)
	}
	if f.assignee != "" {
		parts = append(parts, "assignee="+f.assignee)
	}
	if f.tag != "" {
		parts = append(parts, "tag="+f.tag)
	}
	if f.priority != -1 {
		parts = append(parts, "priority="+strconv.Itoa(f.priority))
	}
	if f.status != "" {
		parts = append(parts, "status="+f.status)
	}
	if f.parent != "" {
		parts = append(parts, "parent="+f.parent)
	}
	if f.text != "" {
		parts = append(parts, "text="+f.text)
	}
	return strings.Join(parts, " ")
}

// parseFilter parses a filter input string into a Filter.
// Format: plain text for search, or prefixed tokens like type:bug priority:1.
// Multiple tokens are ANDed together. Any non-prefixed token is treated as free text.
func parseFilter(input string) Filter {
	f := Filter{priority: -1}
	input = strings.TrimSpace(input)
	if input == "" {
		return f
	}

	tokens := strings.Fields(input)
	textParts := make([]string, 0)

	for _, tok := range tokens {
		before, after, found := strings.Cut(tok, ":")
		if !found {
			textParts = append(textParts, tok)
			continue
		}
		key := strings.ToLower(before)
		val := after
		switch key {
		case "type":
			f.ticketType = val
		case "assignee":
			f.assignee = val
		case "tag":
			f.tag = val
		case "priority":
			if n, err := strconv.Atoi(val); err == nil {
				f.priority = n
			}
		case "status":
			f.status = val
		case "parent":
			f.parent = val
		default:
			// Unknown prefix — treat whole token as free text.
			textParts = append(textParts, tok)
		}
	}

	f.text = strings.Join(textParts, " ")
	return f
}

// applyFilter returns the subset of records that match f.
// All active criteria must match (AND semantics).
func applyFilter(records []ticket.Record, f Filter) []ticket.Record {
	if f.isEmpty() {
		return records
	}
	out := make([]ticket.Record, 0, len(records))
	text := strings.ToLower(f.text)
	for _, rec := range records {
		if f.ticketType != "" && !strings.EqualFold(rec.Front.Type, f.ticketType) {
			continue
		}
		if f.assignee != "" && !strings.EqualFold(rec.Front.Assignee, f.assignee) {
			continue
		}
		if f.tag != "" && !hasTag(rec.Front.Tags, f.tag) {
			continue
		}
		if f.priority != -1 && rec.Front.Priority != f.priority {
			continue
		}
		if f.status != "" && !strings.EqualFold(rec.Front.Status, f.status) {
			continue
		}
		if f.parent != "" && !strings.EqualFold(rec.Front.Parent, f.parent) {
			continue
		}
		if text != "" {
			id := strings.ToLower(rec.Front.ID)
			title := strings.ToLower(rec.Body.Title)
			desc := strings.ToLower(rec.Body.Description)
			if !strings.Contains(id, text) && !strings.Contains(title, text) && !strings.Contains(desc, text) {
				continue
			}
		}
		out = append(out, rec)
	}
	return out
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

// FilterInput is the interactive text-input model used to enter filter criteria.
type FilterInput struct {
	input textinput.Model
}

func newFilterInput() FilterInput {
	ti := textinput.New()
	ti.Placeholder = "type:bug priority:0 auth  (Enter to apply, Esc to cancel)"
	ti.CharLimit = 200
	ti.Width = 60
	return FilterInput{input: ti}
}

// Focus activates the input field.
func (fi *FilterInput) Focus() {
	fi.input.Focus()
}

// Blur deactivates the input field.
func (fi *FilterInput) Blur() {
	fi.input.Blur()
}

// SetValue populates the input with an existing filter string.
func (fi *FilterInput) SetValue(s string) {
	fi.input.SetValue(s)
}

// Value returns the current input text.
func (fi FilterInput) Value() string {
	return fi.input.Value()
}

// Update forwards tea messages to the underlying textinput.
func (fi FilterInput) Update(msg tea.Msg) (FilterInput, tea.Cmd) {
	var cmd tea.Cmd
	fi.input, cmd = fi.input.Update(msg)
	return fi, cmd
}

// View renders the filter input bar.
func (fi FilterInput) View(width int) string {
	prompt := filterInputStyle.Render("/") + " "
	return prompt + fi.input.View()
}

// filterToInput converts an active Filter back to an editable input string.
func filterToInput(f Filter) string {
	parts := make([]string, 0, 7)
	if f.ticketType != "" {
		parts = append(parts, "type:"+f.ticketType)
	}
	if f.assignee != "" {
		parts = append(parts, "assignee:"+f.assignee)
	}
	if f.tag != "" {
		parts = append(parts, "tag:"+f.tag)
	}
	if f.priority != -1 {
		parts = append(parts, "priority:"+strconv.Itoa(f.priority))
	}
	if f.status != "" {
		parts = append(parts, "status:"+f.status)
	}
	if f.parent != "" {
		parts = append(parts, "parent:"+f.parent)
	}
	if f.text != "" {
		parts = append(parts, f.text)
	}
	return strings.Join(parts, " ")
}

// filterBarView renders the filter status line shown above the board.
func filterBarView(f Filter, width int) string {
	label := filterBarStyle.Render("Filter: ")
	var value string
	if f.isEmpty() {
		value = filterBarStyle.Render("none")
	} else {
		value = filterActiveStyle.Render(f.description())
	}
	_ = width
	return label + value
}
