package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lawrips/tkt/internal/journal"
	"github.com/lawrips/tkt/internal/ticket"
)

var actionStatuses = []string{"open", "in_progress", "needs_testing", "closed"}
var actionPriorities = []string{"0", "1", "2", "3", "4"}
var actionTypes = []string{"task", "epic", "feature", "bug", "chore"}

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorMagenta)
	helpStyle    = lipgloss.NewStyle().Foreground(colorSecondary)
	helpKeyStyle = lipgloss.NewStyle().Bold(true).Foreground(colorHelpKey)
	helpDescStyle = lipgloss.NewStyle().Foreground(colorHint)
	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2)
)

// contextualHelpBar renders a single-line help bar from the given bindings,
// truncating from the right if the terminal is too narrow.
func contextualHelpBar(bindings []key.Binding, width int) string {
	sep := "  "
	var parts []string
	for _, b := range bindings {
		h := b.Help()
		parts = append(parts, helpKeyStyle.Render(h.Key)+helpDescStyle.Render(":"+h.Desc))
	}
	line := strings.Join(parts, sep)
	// Truncate if wider than terminal.
	if lipgloss.Width(line) > width && width > 3 {
		// Rebuild, adding parts until we'd exceed width.
		line = ""
		for i, p := range parts {
			candidate := p
			if i > 0 {
				candidate = sep + p
			}
			if lipgloss.Width(line+candidate) > width-3 {
				line += helpDescStyle.Render("…")
				break
			}
			line += candidate
		}
	}
	return line
}

// boardHints returns the key bindings shown in the board view help bar.
func boardHints(km KeyMap) []key.Binding {
	return []key.Binding{km.Up, km.Down, km.Left, km.Right, km.HalfPageDown, km.HalfPageUp, km.GoTop, km.GoBottom, km.Status, km.Priority, km.Create, km.Filter, km.FocusFilter, km.ProjectPicker, km.Help}
}

// detailHints returns the key bindings shown in the detail view help bar.
func detailHints(km KeyMap, hasLinks bool) []key.Binding {
	esc := key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	scroll := key.NewBinding(key.WithKeys("j"), key.WithHelp("j/k", "scroll"))
	hints := []key.Binding{esc, scroll, km.HalfPageDown, km.HalfPageUp}
	if hasLinks {
		tab := key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "links"))
		enter := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "follow"))
		hints = append(hints, tab, enter)
	}
	hints = append(hints, km.Status, km.Priority, km.Note, km.Edit)
	return hints
}

// epicHints returns the key bindings shown in the epic view help bar.
func epicHints(km KeyMap, onLink bool) []key.Binding {
	esc := key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	if onLink {
		enter := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "details"))
		return []key.Binding{enter, km.Down, esc}
	}
	enter := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	return []key.Binding{km.Up, km.Down, enter, esc, km.Status, km.Priority}
}

// filterHints returns the key bindings shown during filter input.
func filterHints() []key.Binding {
	enter := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply"))
	esc := key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel"))
	return []key.Binding{enter, esc}
}

type viewState int

const (
	viewBoard viewState = iota
	viewDetail
	viewEpic
)

const defaultPollInterval = 2 * time.Second

type autoRefreshTickMsg struct{}

// CommitLoader returns commit entries linked to a given ticket ID.
type CommitLoader func(ticketID string) []journal.Entry

// CommitLoaderFactory creates a fresh CommitLoader that reads the journal once
// and caches the result. Called on each refresh cycle so the cache stays current.
type CommitLoaderFactory func() CommitLoader

// Model is the root TUI application model.
type Model struct {
	keys            KeyMap
	showHelp        bool
	width           int
	height          int
	view            viewState
	board           Board
	detail          DetailModel
	epic            EpicModel
	records         []ticket.Record // cached for epic view lookups
	overlay         overlayState
	filter          Filter
	filterInputMode bool
	filterInput     FilterInput
	commitLoader    CommitLoader
	commitFactory   CommitLoaderFactory
	latestByID      map[string]string
	pollInterval    time.Duration
	loading         bool
	refreshPending  bool
	editorActive    bool
	prevView        viewState // where to return on detail back-nav

	// Project picker state.
	projectName       string
	projectNames      []string
	showProjectPicker bool
	pickerCursor      int

	// SwitchTo is set to a project name when the user picks a different project.
	// The CLI layer reads this after tea.Quit and restarts with the new project.
	SwitchTo string
}

// New returns an initialized Model for the given ticket directory.
// runner is called for in-process tk mutations (e.g. cli.Run).
// commitFactory creates a fresh CommitLoader on each refresh cycle.
// projectName is the display name for the status bar; projectNames lists all
// available projects for the picker.
func New(dir string, projectName string, runner RunFunc, commitFactory CommitLoaderFactory, projectNames []string) Model {
	tkRunner = runner
	b := newBoard(dir)
	b.headerLines = 3 // status bar + filter bar + help bar
	var loader CommitLoader
	if commitFactory != nil {
		loader = commitFactory()
	}
	// Show picker immediately if no project resolved but multiple available.
	showPicker := projectName == "" && len(projectNames) > 1

	return Model{
		keys:              DefaultKeyMap,
		board:             b,
		view:              viewBoard,
		filter:            Filter{priority: -1},
		filterInput:       newFilterInput(),
		commitLoader:      loader,
		commitFactory:     commitFactory,
		latestByID:        map[string]string{},
		pollInterval:      defaultPollInterval,
		loading:           true,
		projectName:       projectName,
		projectNames:      projectNames,
		showProjectPicker: showPicker,
	}
}

// findRecord looks up a record by ID in a slice.
func findRecord(records []ticket.Record, id string) (ticket.Record, bool) {
	for _, r := range records {
		if r.ID == id {
			return r, true
		}
	}
	return ticket.Record{}, false
}

// openDetail creates a detail model for the given record, wired with current state.
func (m *Model) openDetail(rec ticket.Record) {
	m.detail = NewDetail(rec, m.loadCommits(rec.ID))
	if !m.filter.isEmpty() {
		m.detail.filterDesc = m.filter.description()
	}
	m.detail.SetSize(m.width, m.height)
	if m.view != viewDetail {
		m.prevView = m.view
	}
	m.view = viewDetail
}

func (m Model) latestCommitByID(records []ticket.Record) map[string]string {
	out := make(map[string]string, len(records))
	for _, rec := range records {
		entries := m.loadCommits(rec.ID)
		if len(entries) == 0 {
			continue
		}
		sha := entries[len(entries)-1].SHA
		if len(sha) > 7 {
			sha = sha[:7]
		}
		out[rec.ID] = sha
	}
	return out
}

func (m Model) effortByID(records []ticket.Record) map[string]string {
	out := make(map[string]string, len(records))
	for _, rec := range records {
		entries := m.loadCommits(rec.ID)
		if len(entries) == 0 {
			continue
		}
		e := journal.Effort(entries)
		if s := e.String(); s != "" {
			out[rec.ID] = s
		}
	}
	return out
}

// loadCommits returns commit entries for a ticket, or nil if no loader is set.
func (m Model) loadCommits(ticketID string) []journal.Entry {
	if m.commitLoader == nil {
		return nil
	}
	return m.commitLoader(ticketID)
}

func autoRefreshTickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(time.Time) tea.Msg {
		return autoRefreshTickMsg{}
	})
}

func (m Model) canAutoRefresh() bool {
	return !m.filterInputMode && m.overlay.kind == overlayNone && !m.editorActive && !m.showProjectPicker
}

func (m *Model) requestLoad() tea.Cmd {
	if m.loading {
		return nil
	}
	m.loading = true
	return loadTicketsCmd(m.board.dir)
}

func (m *Model) flushPendingLoad() tea.Cmd {
	if !m.refreshPending || !m.canAutoRefresh() {
		return nil
	}
	m.refreshPending = false
	return m.requestLoad()
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.board.init(),
		autoRefreshTickCmd(m.pollInterval),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case autoRefreshTickMsg:
		nextTick := autoRefreshTickCmd(m.pollInterval)
		if !m.canAutoRefresh() {
			m.refreshPending = true
			return m, nextTick
		}
		if cmd := m.requestLoad(); cmd != nil {
			return m, tea.Batch(nextTick, cmd)
		}
		return m, nextTick

	case ticketsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.board.err = msg.err
		} else {
			m.board.err = nil
			m.records = msg.records
			if m.commitFactory != nil {
				m.commitLoader = m.commitFactory()
			}
			m.latestByID = m.latestCommitByID(m.records)
			m.board.setLatestCommitsByID(m.latestByID)
			// Re-apply current filter whenever records are (re)loaded.
			filtered := applyFilter(m.records, m.filter)
			m.board.setColumns(filtered)
			m.refreshPending = false

			// Refresh active detail/epic views so they don't show stale data.
			if m.view == viewDetail {
				if rec, ok := findRecord(m.records, m.detail.record.ID); ok {
					oldOffset := m.detail.viewport.YOffset
					oldLinkCursor := m.detail.linkCursor
					m.openDetail(rec)
					m.detail.viewport.SetYOffset(oldOffset)
					m.detail.linkCursor = oldLinkCursor
					m.detail.viewport.SetContent(m.detail.renderContent())
				} else {
					// Record was deleted — fall back to board.
					m.view = viewBoard
				}
			}
			if m.view == viewEpic {
				if rec, ok := findRecord(m.records, m.epic.epic.ID); ok {
					selectedChildID := ""
					if len(m.epic.children) > 0 && m.epic.cursor >= 0 && m.epic.cursor < len(m.epic.children) {
						selectedChildID = m.epic.children[m.epic.cursor].ID
					}
					wasOnLink := m.epic.cursorOnLink
					updated := NewEpicModel(rec, m.records, m.latestByID)
					updated.effortByID = m.effortByID(m.records)
					updated.cursorOnLink = wasOnLink
					if selectedChildID != "" {
						for i, child := range updated.children {
							if child.ID == selectedChildID {
								updated.cursor = i
								break
							}
						}
					}
					m.epic = updated
					if !m.filter.isEmpty() {
						m.epic.filterDesc = m.filter.description()
					}
				} else {
					m.view = viewBoard
				}
			}
		}
		if cmd := m.flushPendingLoad(); cmd != nil {
			return m, cmd
		}

	case mutationDoneMsg:
		m.overlay = overlayState{}
		m.editorActive = false
		if cmd := m.requestLoad(); cmd != nil {
			return m, cmd
		}
		m.refreshPending = true
		return m, nil

	case filterPickerDoneMsg:
		m.overlay = overlayState{}
		m.filter = msg.filter
		m.board.setColumns(applyFilter(m.records, m.filter))
		m.view = viewBoard
		return m, nil

	case tea.KeyMsg:
		// Filter input mode takes priority over everything else.
		if m.filterInputMode {
			return m.updateFilterInput(msg)
		}

		// If an overlay is active, route all input through it.
		if m.overlay.kind != overlayNone {
			previous := m.overlay.kind
			var cmd tea.Cmd
			m.overlay, cmd, _ = m.overlay.update(msg)
			if previous != overlayNone && m.overlay.kind == overlayNone && cmd == nil {
				if pending := m.flushPendingLoad(); pending != nil {
					return m, pending
				}
			}
			return m, cmd
		}

		// Project picker takes priority when visible.
		if m.showProjectPicker {
			return m.updateProjectPicker(msg)
		}

		// Global keys handled regardless of view
		switch {
		case key.Matches(msg, m.keys.Quit):
			if m.showHelp {
				m.showHelp = false
				return m, nil
			}
			if m.view != viewBoard {
				m.view = viewBoard
				return m, nil
			}
			return m, tea.Quit
		case key.Matches(msg, m.keys.Help):
			m.showHelp = !m.showHelp
			return m, nil
		case key.Matches(msg, m.keys.Filter):
			m.filterInputMode = true
			if !m.filter.isEmpty() {
				m.filterInput.SetValue(filterToInput(m.filter))
			} else {
				m.filterInput.SetValue("")
			}
			m.filterInput.Focus()
			return m, nil
		case key.Matches(msg, m.keys.ProjectPicker):
			if len(m.projectNames) > 1 {
				m.showProjectPicker = true
				m.pickerCursor = 0
				// Position cursor on current project.
				for i, name := range m.projectNames {
					if name == m.projectName {
						m.pickerCursor = i
						break
					}
				}
			}
			return m, nil
		}
	}

	// Dismiss help on any non-key message or after key handling above.
	if m.showHelp {
		if _, ok := msg.(tea.KeyMsg); ok {
			m.showHelp = false
			return m, nil
		}
	}

	switch m.view {
	case viewBoard:
		return m.updateBoard(msg)
	case viewDetail:
		return m.updateDetail(msg)
	case viewEpic:
		return m.updateEpic(msg)
	}

	return m, nil
}

// updateFilterInput processes key events while the filter input is open.
func (m Model) updateFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filter = parseFilter(m.filterInput.Value())
		m.filterInput.Blur()
		m.filterInputMode = false
		filtered := applyFilter(m.records, m.filter)
		m.board.setColumns(filtered)
		m.view = viewBoard
		if cmd := m.flushPendingLoad(); cmd != nil {
			return m, cmd
		}
		return m, nil
	case "esc":
		m.filterInput.Blur()
		m.filterInputMode = false
		if cmd := m.flushPendingLoad(); cmd != nil {
			return m, cmd
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	return m, cmd
}

// updateProjectPicker handles key events for the project picker overlay.
func (m Model) updateProjectPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "o":
		m.showProjectPicker = false
		return m, nil
	case "up", "k":
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
		return m, nil
	case "down", "j":
		if m.pickerCursor < len(m.projectNames)-1 {
			m.pickerCursor++
		}
		return m, nil
	case "enter":
		selected := m.projectNames[m.pickerCursor]
		m.showProjectPicker = false
		if selected != m.projectName {
			m.SwitchTo = selected
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

func (m Model) updateBoard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyMsg); ok {
		if key.Matches(kmsg, key.NewBinding(key.WithKeys("enter"))) {
			rec, ok := m.board.selectedRecord()
			if ok {
				if rec.Front.Type == "epic" {
					m.epic = NewEpicModel(rec, m.records, m.latestByID)
					m.epic.effortByID = m.effortByID(m.records)
					if !m.filter.isEmpty() {
						m.epic.filterDesc = m.filter.description()
					}
					m.view = viewEpic
					return m, nil
				}
				m.openDetail(rec)
				return m, m.detail.Init()
			}
		}

		// Esc clears active filter on the board.
		if key.Matches(kmsg, key.NewBinding(key.WithKeys("esc"))) {
			if !m.filter.isEmpty() {
				m.filter = Filter{priority: -1}
				m.board.setColumns(applyFilter(m.records, m.filter))
				return m, nil
			}
			return m, nil
		}

		// Contextual filter picker.
		if key.Matches(kmsg, m.keys.FocusFilter) {
			rec, ok := m.board.selectedRecord()
			if !ok {
				return m, nil
			}
			choices, values := buildFilterPickerChoices(rec)
			if len(choices) == 0 {
				return m, nil
			}
			ov := newDropdownOverlay(overlayFilterPicker, rec.ID, choices)
			ov.filterValues = values
			m.overlay = ov
			return m, nil
		}

		// Action keys — require a selected ticket (except create).
		if cmd, consumed := m.handleActionKey(kmsg); consumed {
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.board, cmd = m.board.update(msg, m.keys)
	return m, cmd
}

// buildFilterPickerChoices builds the display choices and pre-built Filter values
// for the contextual filter picker based on the selected ticket's fields.
func buildFilterPickerChoices(rec ticket.Record) (choices []string, values []Filter) {
	base := Filter{priority: -1} // empty filter base

	// parent: — epic uses own ID (children), non-epic with parent uses parent ID (siblings)
	if strings.EqualFold(rec.Front.Type, "epic") {
		choices = append(choices, "parent:"+rec.ID+" (children)")
		f := base
		f.parent = rec.ID
		values = append(values, f)
	} else if rec.Front.Parent != "" {
		choices = append(choices, "parent:"+rec.Front.Parent+" (siblings)")
		f := base
		f.parent = rec.Front.Parent
		values = append(values, f)
	}

	// assignee:
	if rec.Front.Assignee != "" {
		choices = append(choices, "assignee:"+rec.Front.Assignee)
		f := base
		f.assignee = rec.Front.Assignee
		values = append(values, f)
	}

	// tag: — one entry per tag
	for _, tag := range rec.Front.Tags {
		choices = append(choices, "tag:"+tag)
		f := base
		f.tag = tag
		values = append(values, f)
	}

	// type: — always present
	{
		choices = append(choices, "type:"+rec.Front.Type)
		f := base
		f.ticketType = rec.Front.Type
		values = append(values, f)
	}

	// priority: — always present
	{
		choices = append(choices, "priority:"+fmt.Sprintf("%d", rec.Front.Priority))
		f := base
		f.priority = rec.Front.Priority
		values = append(values, f)
	}

	return choices, values
}

// handleActionKey checks if the key message matches an action binding.
// It sets m.overlay or returns a tea.Cmd (for editor launch).
// Returns (cmd, consumed): consumed=true means the key was handled.
func (m *Model) handleActionKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.keys.Create):
		m.overlay = newTextOverlay(overlayCreate, "", "ticket title")
		return nil, true

	case key.Matches(msg, m.keys.Status):
		rec, ok := m.board.selectedRecord()
		if !ok {
			return nil, true
		}
		m.overlay = newDropdownOverlay(overlayStatus, rec.ID, actionStatuses)
		return nil, true

	case key.Matches(msg, m.keys.Priority):
		rec, ok := m.board.selectedRecord()
		if !ok {
			return nil, true
		}
		m.overlay = newDropdownOverlay(overlayPriority, rec.ID, actionPriorities)
		return nil, true

	case key.Matches(msg, m.keys.Assignee):
		rec, ok := m.board.selectedRecord()
		if !ok {
			return nil, true
		}
		m.overlay = newTextOverlay(overlayAssignee, rec.ID, "assignee name")
		return nil, true

	case key.Matches(msg, m.keys.Type):
		rec, ok := m.board.selectedRecord()
		if !ok {
			return nil, true
		}
		m.overlay = newDropdownOverlay(overlayType, rec.ID, actionTypes)
		return nil, true

	case key.Matches(msg, m.keys.Note):
		rec, ok := m.board.selectedRecord()
		if !ok {
			return nil, true
		}
		m.overlay = newTextOverlay(overlayNote, rec.ID, "note text")
		return nil, true

	case key.Matches(msg, m.keys.Dep):
		rec, ok := m.board.selectedRecord()
		if !ok {
			return nil, true
		}
		m.overlay = newTextOverlay(overlayDep, rec.ID, "dep ID (prefix - to remove)")
		return nil, true

	case key.Matches(msg, m.keys.Delete):
		rec, ok := m.board.selectedRecord()
		if !ok {
			return nil, true
		}
		m.overlay = newDeleteOverlay(rec.ID)
		return nil, true

	case key.Matches(msg, m.keys.Edit):
		rec, ok := m.board.selectedRecord()
		if !ok {
			return nil, true
		}
		m.editorActive = true
		return openEditorCmd(rec.Path), true
	}
	return nil, false
}

func (m Model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case GoBackMsg:
		m.view = m.prevView
		return m, nil
	case NavigateToTicketMsg:
		if rec, ok := findRecord(m.records, msg.TicketID); ok {
			m.openDetail(rec)
			return m, m.detail.Init()
		}
		return m, nil
	}

	if kmsg, ok := msg.(tea.KeyMsg); ok {
		if cmd, consumed := m.handleDetailActionKey(kmsg); consumed {
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

// handleDetailActionKey handles action keys when in the detail view.
func (m *Model) handleDetailActionKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	rec := m.detail.record
	switch {
	case key.Matches(msg, m.keys.Status):
		m.overlay = newDropdownOverlay(overlayStatus, rec.ID, actionStatuses)
		return nil, true
	case key.Matches(msg, m.keys.Priority):
		m.overlay = newDropdownOverlay(overlayPriority, rec.ID, actionPriorities)
		return nil, true
	case key.Matches(msg, m.keys.Assignee):
		m.overlay = newTextOverlay(overlayAssignee, rec.ID, "assignee name")
		return nil, true
	case key.Matches(msg, m.keys.Type):
		m.overlay = newDropdownOverlay(overlayType, rec.ID, actionTypes)
		return nil, true
	case key.Matches(msg, m.keys.Note):
		m.overlay = newTextOverlay(overlayNote, rec.ID, "note text")
		return nil, true
	case key.Matches(msg, m.keys.Dep):
		m.overlay = newTextOverlay(overlayDep, rec.ID, "dep ID (prefix - to remove)")
		return nil, true
	case key.Matches(msg, m.keys.Delete):
		m.overlay = newDeleteOverlay(rec.ID)
		return nil, true
	case key.Matches(msg, m.keys.Edit):
		m.editorActive = true
		return openEditorCmd(rec.Path), true
	case key.Matches(msg, m.keys.Create):
		m.overlay = newTextOverlay(overlayCreate, "", "ticket title")
		return nil, true
	}
	return nil, false
}

func (m Model) updateEpic(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case EpicGoBackMsg:
		m.view = viewBoard
		return m, nil
	case EpicDetailMsg:
		m.openDetail(msg.Record)
		return m, m.detail.Init()
	case DrillIntoTicketMsg:
		m.openDetail(msg.Record)
		return m, m.detail.Init()
	}

	if kmsg, ok := msg.(tea.KeyMsg); ok {
		if cmd, consumed := m.handleEpicActionKey(kmsg); consumed {
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.epic, cmd = m.epic.Update(msg)
	return m, cmd
}

// handleEpicActionKey handles action keys when in the epic view.
func (m *Model) handleEpicActionKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	// Create is always valid regardless of cursor position.
	if key.Matches(msg, m.keys.Create) {
		m.overlay = newTextOverlay(overlayCreate, "", "ticket title")
		return nil, true
	}
	// No child actions when cursor is on the link or there are no children.
	if m.epic.cursorOnLink || len(m.epic.children) == 0 {
		return nil, false
	}
	rec := m.epic.children[m.epic.cursor]
	switch {
	case key.Matches(msg, m.keys.Status):
		m.overlay = newDropdownOverlay(overlayStatus, rec.ID, actionStatuses)
		return nil, true
	case key.Matches(msg, m.keys.Priority):
		m.overlay = newDropdownOverlay(overlayPriority, rec.ID, actionPriorities)
		return nil, true
	case key.Matches(msg, m.keys.Assignee):
		m.overlay = newTextOverlay(overlayAssignee, rec.ID, "assignee name")
		return nil, true
	case key.Matches(msg, m.keys.Type):
		m.overlay = newDropdownOverlay(overlayType, rec.ID, actionTypes)
		return nil, true
	case key.Matches(msg, m.keys.Dep):
		m.overlay = newTextOverlay(overlayDep, rec.ID, "dep ID (prefix - to remove)")
		return nil, true
	case key.Matches(msg, m.keys.Note):
		m.overlay = newTextOverlay(overlayNote, rec.ID, "note text")
		return nil, true
	case key.Matches(msg, m.keys.Delete):
		m.overlay = newDeleteOverlay(rec.ID)
		return nil, true
	case key.Matches(msg, m.keys.Edit):
		m.editorActive = true
		return openEditorCmd(rec.Path), true
	case key.Matches(msg, m.keys.Create):
		m.overlay = newTextOverlay(overlayCreate, "", "ticket title")
		return nil, true
	}
	return nil, false
}

// View implements tea.Model.
func (m Model) View() string {
	if m.showHelp {
		return m.helpView()
	}

	if m.showProjectPicker {
		return m.projectPickerView()
	}

	statusBar := m.statusBarView()

	if m.filterInputMode {
		filterBar := m.filterInput.View(m.width)
		helpBar := contextualHelpBar(filterHints(), m.width)
		switch m.view {
		case viewDetail:
			return lipgloss.JoinVertical(lipgloss.Left, statusBar, filterBar, m.detail.View(), helpBar)
		case viewEpic:
			return lipgloss.JoinVertical(lipgloss.Left, statusBar, filterBar, m.epic.View(), helpBar)
		default:
			return lipgloss.JoinVertical(lipgloss.Left, statusBar, filterBar, m.board.view(), helpBar)
		}
	}

	if m.overlay.kind != overlayNone {
		return m.overlay.view()
	}

	switch m.view {
	case viewDetail:
		helpBar := contextualHelpBar(detailHints(m.keys, len(m.detail.ticketLinks) > 0), m.width)
		return lipgloss.JoinVertical(lipgloss.Left, statusBar, m.detail.View(), helpBar)
	case viewEpic:
		helpBar := contextualHelpBar(epicHints(m.keys, m.epic.cursorOnLink), m.width)
		return lipgloss.JoinVertical(lipgloss.Left, statusBar, m.epic.View(), helpBar)
	default:
		return m.boardView()
	}
}

// boardView renders the status bar, filter bar, board, and help bar.
func (m Model) boardView() string {
	statusBar := m.statusBarView()
	filterLine := filterBarView(m.filter, m.width)
	helpBar := contextualHelpBar(boardHints(m.keys), m.width)
	return lipgloss.JoinVertical(lipgloss.Left, statusBar, filterLine, m.board.view(), helpBar)
}

// statusBarView renders the top status bar with project name and ticket counts.
func (m Model) statusBarView() string {
	label := lipgloss.NewStyle().Bold(true).Render("tkt")
	projName := lipgloss.NewStyle().Foreground(colorMagentaBright).Render("  " + m.projectName)

	counts := lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf(
		"  %d open  %d in progress  %d testing  %d closed",
		len(m.board.columns[colOpen]),
		len(m.board.columns[colInProgress]),
		len(m.board.columns[colNeedsTesting]),
		len(m.board.columns[colClosed]),
	))

	return label + projName + counts
}

// projectPickerView renders a centered overlay listing all projects.
func (m Model) projectPickerView() string {
	var sb strings.Builder
	sb.WriteString(overlayTitleStyle.Render("Switch Project"))
	sb.WriteString("\n\n")

	for i, name := range m.projectNames {
		label := name
		if name == m.projectName {
			label = name + " (current)"
		}
		if i == m.pickerCursor {
			sb.WriteString(overlaySelectedStyle.Render("> " + label))
		} else {
			sb.WriteString(overlayChoiceStyle.Render("  " + label))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(overlayHintStyle.Render("j/k move  enter select  esc cancel"))

	return overlayBoxStyle.Render(sb.String())
}

func (m Model) helpView() string {
	content := fmt.Sprintf(
		"%s\n\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n%s\n\n%s\n",
		titleStyle.Render("Help"),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "h/l/←/→", "switch columns")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "j/k/↑/↓", "move up/down")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "ctrl+d/ctrl+u", "half page down/up")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "g/G", "jump to top/bottom")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "enter", "open ticket / drill in")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "esc", "clear filter / go back")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "?", "toggle help")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "/", "filter tickets")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "f", "filter from ticket")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "q", "quit (from board) / back")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "s", "set status")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "p", "set priority")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "a", "set assignee")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "t", "set type")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "d", "add/remove dep")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "n", "add note")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "c", "create ticket")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "x", "delete ticket")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "e", "open in $EDITOR")),
		helpStyle.Render(fmt.Sprintf("  %-14s %s", "o", "switch project")),
		helpStyle.Render("  press any key to close help"),
	)
	return overlayStyle.Render(content)
}
