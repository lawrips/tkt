package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Adaptive color palette — lipgloss auto-selects based on terminal background.
var (
	colorPrimary       = lipgloss.AdaptiveColor{Light: "235", Dark: "255"} // primary text
	colorSecondary     = lipgloss.AdaptiveColor{Light: "240", Dark: "252"} // descriptions
	colorHelpKey       = lipgloss.AdaptiveColor{Light: "243", Dark: "248"} // help key labels
	colorLabel         = lipgloss.AdaptiveColor{Light: "242", Dark: "245"} // secondary labels
	colorDetailLabel   = lipgloss.AdaptiveColor{Light: "241", Dark: "244"} // detail labels
	colorMuted         = lipgloss.AdaptiveColor{Light: "240", Dark: "243"} // status bar, filter
	colorHint          = lipgloss.AdaptiveColor{Light: "248", Dark: "241"} // hints, placeholders

	colorMagenta       = lipgloss.AdaptiveColor{Light: "125", Dark: "205"} // headings
	colorMagentaBright = lipgloss.AdaptiveColor{Light: "125", Dark: "212"} // selected, cursor
	colorYellow        = lipgloss.AdaptiveColor{Light: "136", Dark: "226"} // in-progress
	colorGreen         = lipgloss.AdaptiveColor{Light: "28", Dark: "82"}   // closed
	colorCyan          = lipgloss.AdaptiveColor{Light: "30", Dark: "51"}   // needs-testing
	colorRed           = lipgloss.AdaptiveColor{Light: "160", Dark: "196"} // priority 0
	colorOrange        = lipgloss.AdaptiveColor{Light: "166", Dark: "208"} // priority 1
	colorLink          = lipgloss.AdaptiveColor{Light: "25", Dark: "39"}   // links
	colorFocusBorder   = lipgloss.AdaptiveColor{Light: "63", Dark: "63"}   // focused borders

	colorSelectedBg = lipgloss.AdaptiveColor{Light: "254", Dark: "237"} // selected card bg
	colorBorder     = lipgloss.AdaptiveColor{Light: "250", Dark: "238"} // borders
)

var (
	// Column header styles per status.
	headerOpenStyle          = lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	headerInProgressStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorYellow)
	headerNeedsTestingStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	headerClosedStyle        = lipgloss.NewStyle().Bold(true).Foreground(colorGreen)

	// Card styles.
	cardNormalStyle   = lipgloss.NewStyle().Padding(0, 1)
	cardSelectedStyle = lipgloss.NewStyle().Padding(0, 1).
				Background(colorSelectedBg).
				Bold(true)

	// Priority badge colors.
	priorityBadgeStyles = map[int]lipgloss.Style{
		0: lipgloss.NewStyle().Foreground(colorRed).Bold(true), // red
		1: lipgloss.NewStyle().Foreground(colorOrange),         // orange
		2: lipgloss.NewStyle().Foreground(colorYellow),         // yellow
		3: lipgloss.NewStyle().Foreground(colorPrimary),        // white
		4: lipgloss.NewStyle().Foreground(colorLabel),          // grey
	}

	// Column border / container.
	columnStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	columnFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorFocusBorder).
				Padding(0, 1)

	// Preview pane.
	previewStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	previewLabelStyle = lipgloss.NewStyle().Foreground(colorLabel)
	previewValueStyle = lipgloss.NewStyle().Foreground(colorPrimary)
)

func priorityBadge(p int) string {
	label := itoa(p)
	if style, ok := priorityBadgeStyles[p]; ok {
		return style.Render(label)
	}
	return priorityBadgeStyles[4].Render(label)
}

// StatusStyle returns the lipgloss style for a given ticket status.
func StatusStyle(status string) lipgloss.Style {
	switch strings.ToLower(status) {
	case "closed":
		return lipgloss.NewStyle().Foreground(colorGreen)
	case "in_progress":
		return lipgloss.NewStyle().Foreground(colorYellow)
	case "needs_testing":
		return lipgloss.NewStyle().Foreground(colorCyan)
	default:
		return lipgloss.NewStyle().Foreground(colorPrimary)
	}
}

// RenderStatus returns a colored status string.
func RenderStatus(status string) string {
	if status == "" {
		status = "open"
	}
	return StatusStyle(status).Render(status)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
