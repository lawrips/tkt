package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// overlayKind identifies what action is in progress.
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayStatus
	overlayPriority
	overlayAssignee
	overlayType
	overlayNote
	overlayDep
	overlayCreate
	overlayDelete
	overlayFilterPicker
)

// mutationDoneMsg is sent after a tk subprocess completes.
type mutationDoneMsg struct {
	err error
}

// filterPickerDoneMsg is sent when the user selects from the filter picker.
type filterPickerDoneMsg struct {
	filter Filter
}

// overlayState holds the full state of the action overlay.
type overlayState struct {
	kind     overlayKind
	ticketID string // ID of the ticket being acted on (empty for create)

	// dropdown state
	choices      []string
	filterValues []Filter // parallel to choices; pre-built Filter for overlayFilterPicker
	cursor       int

	// text input state
	input textinput.Model
}

var (
	overlayBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFocusBorder).
			Padding(1, 2)

	overlayTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorMagenta)
	overlayChoiceStyle  = lipgloss.NewStyle().Foreground(colorSecondary)
	overlaySelectedStyle = lipgloss.NewStyle().
				Foreground(colorMagentaBright).
				Bold(true)
	overlayHintStyle = lipgloss.NewStyle().Foreground(colorHint)
)

// newDropdownOverlay creates an overlay with a fixed choices list.
func newDropdownOverlay(kind overlayKind, ticketID string, choices []string) overlayState {
	return overlayState{
		kind:     kind,
		ticketID: ticketID,
		choices:  choices,
	}
}

// newTextOverlay creates an overlay with a text input.
func newTextOverlay(kind overlayKind, ticketID string, placeholder string) overlayState {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Focus()
	ti.CharLimit = 256
	return overlayState{
		kind:     kind,
		ticketID: ticketID,
		input:    ti,
	}
}

// newDeleteOverlay creates a confirmation overlay.
func newDeleteOverlay(ticketID string) overlayState {
	return overlayState{
		kind:     overlayDelete,
		ticketID: ticketID,
		choices:  []string{"no", "yes"},
	}
}

// overlayTitle returns the title string for the overlay header.
func (o overlayState) overlayTitle() string {
	switch o.kind {
	case overlayStatus:
		return fmt.Sprintf("Set status  [%s]", o.ticketID)
	case overlayPriority:
		return fmt.Sprintf("Set priority  [%s]", o.ticketID)
	case overlayAssignee:
		return fmt.Sprintf("Set assignee  [%s]", o.ticketID)
	case overlayType:
		return fmt.Sprintf("Set type  [%s]", o.ticketID)
	case overlayNote:
		return fmt.Sprintf("Add note  [%s]", o.ticketID)
	case overlayDep:
		return fmt.Sprintf("Add/remove dep  [%s]", o.ticketID)
	case overlayCreate:
		return "Create ticket"
	case overlayDelete:
		return fmt.Sprintf("Delete ticket  [%s]", o.ticketID)
	case overlayFilterPicker:
		return fmt.Sprintf("Filter by  [%s]", o.ticketID)
	}
	return ""
}

// isDropdown returns true for choice-list overlays.
func (o overlayState) isDropdown() bool {
	return o.kind == overlayStatus ||
		o.kind == overlayPriority ||
		o.kind == overlayType ||
		o.kind == overlayDelete ||
		o.kind == overlayFilterPicker
}

// Update processes a key message for the overlay.
// Returns (updated state, cmd, consumed).
// consumed=true means the key was handled and should not propagate.
func (o overlayState) update(msg tea.Msg) (overlayState, tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Cancel
			o.kind = overlayNone
			return o, nil, true

		case "up", "k":
			if o.isDropdown() && o.cursor > 0 {
				o.cursor--
			}
			return o, nil, true

		case "down", "j":
			if o.isDropdown() && o.cursor < len(o.choices)-1 {
				o.cursor++
			}
			return o, nil, true

		case "enter":
			if o.isDropdown() {
				return o, o.executeDropdown(), true
			}
			// text input: fire mutation
			return o, o.executeText(), true
		}

		// For text inputs, forward to the textinput model
		if !o.isDropdown() {
			var cmd tea.Cmd
			o.input, cmd = o.input.Update(msg)
			return o, cmd, true
		}
	}
	return o, nil, false
}

// executeDropdown builds and runs the right tk command for dropdown overlays.
func (o overlayState) executeDropdown() tea.Cmd {
	selected := o.choices[o.cursor]
	var args []string
	switch o.kind {
	case overlayFilterPicker:
		f := o.filterValues[o.cursor]
		return func() tea.Msg { return filterPickerDoneMsg{filter: f} }
	case overlayStatus:
		args = []string{"edit", o.ticketID, "-s", selected}
	case overlayPriority:
		args = []string{"edit", o.ticketID, "-p", selected}
	case overlayType:
		args = []string{"edit", o.ticketID, "-t", selected}
	case overlayDelete:
		if selected != "yes" {
			return func() tea.Msg { return mutationDoneMsg{} }
		}
		args = []string{"delete", o.ticketID}
	}
	return runTKCmd(args)
}

// executeText builds and runs the right tk command for text overlays.
func (o overlayState) executeText() tea.Cmd {
	value := strings.TrimSpace(o.input.Value())
	if value == "" {
		return func() tea.Msg { return mutationDoneMsg{} }
	}
	var args []string
	switch o.kind {
	case overlayAssignee:
		args = []string{"edit", o.ticketID, "-a", value}
	case overlayNote:
		args = []string{"add-note", o.ticketID, value}
	case overlayDep:
		if strings.HasPrefix(value, "-") {
			args = []string{"undep", o.ticketID, strings.TrimPrefix(value, "-")}
		} else {
			args = []string{"dep", o.ticketID, value}
		}
	case overlayCreate:
		args = []string{"create", value}
	}
	return runTKCmd(args)
}

// RunFunc is a function that executes a tk command given args.
// Injected by the caller to avoid import cycles with cli package.
type RunFunc func(args []string) error

// runTKCmd runs a tk command via the injected runner.
var tkRunner RunFunc

func runTKCmd(args []string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if tkRunner != nil {
			err = tkRunner(args)
		}
		return mutationDoneMsg{err: err}
	}
}

// openEditorCmd suspends the TUI and opens $EDITOR on the given path.
// Supports editors with args like "code -w".
func openEditorCmd(path string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	args := append(parts[1:], path)
	c := exec.Command(parts[0], args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return mutationDoneMsg{err: err}
	})
}

// View renders the overlay as a string to be placed over the main view.
func (o overlayState) view() string {
	var sb strings.Builder

	sb.WriteString(overlayTitleStyle.Render(o.overlayTitle()))
	sb.WriteString("\n\n")

	if o.isDropdown() {
		for i, choice := range o.choices {
			if i == o.cursor {
				sb.WriteString(overlaySelectedStyle.Render("> " + choice))
			} else {
				sb.WriteString(overlayChoiceStyle.Render("  " + choice))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
		sb.WriteString(overlayHintStyle.Render("j/k move  enter select  esc cancel"))
	} else {
		sb.WriteString(o.input.View())
		sb.WriteString("\n\n")
		sb.WriteString(overlayHintStyle.Render("enter confirm  esc cancel"))
	}

	return overlayBoxStyle.Render(sb.String())
}
