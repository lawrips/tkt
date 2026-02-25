package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds all key bindings for the TUI.
type KeyMap struct {
	Quit          key.Binding
	Help          key.Binding
	Left          key.Binding
	Right         key.Binding
	Up            key.Binding
	Down          key.Binding
	HalfPageDown  key.Binding
	HalfPageUp    key.Binding
	GoTop         key.Binding
	GoBottom      key.Binding
	Status        key.Binding
	Priority      key.Binding
	Assignee      key.Binding
	Type          key.Binding
	Note          key.Binding
	Create        key.Binding
	Dep           key.Binding
	Delete        key.Binding
	Edit          key.Binding
	Filter        key.Binding
	ProjectPicker key.Binding
}

// DefaultKeyMap is the default set of key bindings.
var DefaultKeyMap = KeyMap{
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "prev column"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "next column"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	HalfPageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "half page down"),
	),
	HalfPageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "half page up"),
	),
	GoTop: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "go to top"),
	),
	GoBottom: key.NewBinding(
		key.WithKeys("G"),
		key.WithHelp("G", "go to bottom"),
	),
	Status: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "set status"),
	),
	Priority: key.NewBinding(
		key.WithKeys("p"),
		key.WithHelp("p", "set priority"),
	),
	Assignee: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "set assignee"),
	),
	Type: key.NewBinding(
		key.WithKeys("t"),
		key.WithHelp("t", "set type"),
	),
	Note: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "add note"),
	),
	Create: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "create ticket"),
	),
	Dep: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "add/remove dep"),
	),
	Delete: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "delete ticket"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "open in $EDITOR"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	),
	ProjectPicker: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "switch project"),
	),
}
