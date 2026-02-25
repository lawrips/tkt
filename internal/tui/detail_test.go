package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lawrips/tkt/internal/ticket"
)

func TestNewDetailCollectsLinks(t *testing.T) {
	tests := []struct {
		name      string
		front     ticket.Frontmatter
		wantLinks []string
	}{
		{
			name:      "no links",
			front:     ticket.Frontmatter{ID: "t-1"},
			wantLinks: nil,
		},
		{
			name:      "parent only",
			front:     ticket.Frontmatter{ID: "t-1", Parent: "epic-1"},
			wantLinks: []string{"epic-1"},
		},
		{
			name:      "deps only",
			front:     ticket.Frontmatter{ID: "t-1", Deps: []string{"dep-1", "dep-2"}},
			wantLinks: []string{"dep-1", "dep-2"},
		},
		{
			name:      "links only",
			front:     ticket.Frontmatter{ID: "t-1", Links: []string{"link-1"}},
			wantLinks: []string{"link-1"},
		},
		{
			name: "parent + deps + links ordered",
			front: ticket.Frontmatter{
				ID:     "t-1",
				Parent: "epic-1",
				Deps:   []string{"dep-1"},
				Links:  []string{"link-1", "link-2"},
			},
			wantLinks: []string{"epic-1", "dep-1", "link-1", "link-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := ticket.Record{Front: tt.front}
			m := NewDetail(r, nil)
			if len(m.ticketLinks) != len(tt.wantLinks) {
				t.Fatalf("got %d links, want %d", len(m.ticketLinks), len(tt.wantLinks))
			}
			for i, want := range tt.wantLinks {
				if m.ticketLinks[i] != want {
					t.Errorf("ticketLinks[%d] = %q, want %q", i, m.ticketLinks[i], want)
				}
			}
			if m.linkCursor != -1 {
				t.Errorf("linkCursor = %d, want -1", m.linkCursor)
			}
		})
	}
}

func sendKey(m DetailModel, k string) DetailModel {
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	return m
}

func sendSpecialKey(m DetailModel, kt tea.KeyType) DetailModel {
	m, _ = m.Update(tea.KeyMsg{Type: kt})
	return m
}

func TestDetailTabCyclesLinks(t *testing.T) {
	r := ticket.Record{
		Front: ticket.Frontmatter{
			ID:   "t-1",
			Deps: []string{"dep-1", "dep-2", "dep-3"},
		},
	}
	m := NewDetail(r, nil)
	m.SetSize(80, 40)

	// Initially no link selected.
	if m.linkCursor != -1 {
		t.Fatalf("initial linkCursor = %d, want -1", m.linkCursor)
	}

	// Tab selects first link.
	m = sendSpecialKey(m, tea.KeyTab)
	if m.linkCursor != 0 {
		t.Fatalf("after first tab linkCursor = %d, want 0", m.linkCursor)
	}

	// Tab again selects second.
	m = sendSpecialKey(m, tea.KeyTab)
	if m.linkCursor != 1 {
		t.Fatalf("after second tab linkCursor = %d, want 1", m.linkCursor)
	}

	// Tab wraps around.
	m = sendSpecialKey(m, tea.KeyTab)
	if m.linkCursor != 2 {
		t.Fatalf("after third tab linkCursor = %d, want 2", m.linkCursor)
	}
	m = sendSpecialKey(m, tea.KeyTab)
	if m.linkCursor != 0 {
		t.Fatalf("after wrap tab linkCursor = %d, want 0", m.linkCursor)
	}
}

func TestDetailShiftTabCyclesBackward(t *testing.T) {
	r := ticket.Record{
		Front: ticket.Frontmatter{
			ID:   "t-1",
			Deps: []string{"dep-1", "dep-2"},
		},
	}
	m := NewDetail(r, nil)
	m.SetSize(80, 40)

	// Shift+Tab from -1 goes to last.
	m = sendSpecialKey(m, tea.KeyShiftTab)
	if m.linkCursor != 1 {
		t.Fatalf("after shift+tab from -1, linkCursor = %d, want 1", m.linkCursor)
	}

	// Shift+Tab goes backward.
	m = sendSpecialKey(m, tea.KeyShiftTab)
	if m.linkCursor != 0 {
		t.Fatalf("after second shift+tab, linkCursor = %d, want 0", m.linkCursor)
	}

	// Wraps to end.
	m = sendSpecialKey(m, tea.KeyShiftTab)
	if m.linkCursor != 1 {
		t.Fatalf("after wrap shift+tab, linkCursor = %d, want 1", m.linkCursor)
	}
}

func TestDetailEscDeselectsLinkFirst(t *testing.T) {
	r := ticket.Record{
		Front: ticket.Frontmatter{
			ID:     "t-1",
			Parent: "epic-1",
		},
	}
	m := NewDetail(r, nil)
	m.SetSize(80, 40)

	// Select a link.
	m = sendSpecialKey(m, tea.KeyTab)
	if m.linkCursor != 0 {
		t.Fatalf("expected linkCursor=0, got %d", m.linkCursor)
	}

	// Esc should deselect, not go back.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.linkCursor != -1 {
		t.Fatalf("expected linkCursor=-1 after esc, got %d", m.linkCursor)
	}
	if cmd != nil {
		// Should not have sent GoBackMsg.
		t.Fatalf("expected nil cmd after esc deselect, got non-nil")
	}

	// Second Esc should send GoBackMsg.
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if cmd == nil {
		t.Fatalf("expected GoBackMsg cmd after second esc")
	}
	msg := cmd()
	if _, ok := msg.(GoBackMsg); !ok {
		t.Fatalf("expected GoBackMsg, got %T", msg)
	}
}

func TestDetailEnterOnSelectedLink(t *testing.T) {
	r := ticket.Record{
		Front: ticket.Frontmatter{
			ID:     "t-1",
			Parent: "epic-1",
			Deps:   []string{"dep-1"},
		},
	}
	m := NewDetail(r, nil)
	m.SetSize(80, 40)

	// Enter without selection does nothing.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(NavigateToTicketMsg); ok {
			t.Fatalf("enter without selection should not navigate")
		}
	}

	// Select second link (dep-1) and press Enter.
	m = sendSpecialKey(m, tea.KeyTab) // epic-1
	m = sendSpecialKey(m, tea.KeyTab) // dep-1
	m, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected NavigateToTicketMsg cmd")
	}
	msg := cmd()
	nav, ok := msg.(NavigateToTicketMsg)
	if !ok {
		t.Fatalf("expected NavigateToTicketMsg, got %T", msg)
	}
	if nav.TicketID != "dep-1" {
		t.Fatalf("expected TicketID=dep-1, got %q", nav.TicketID)
	}
}

func TestDetailNoLinksTabIgnored(t *testing.T) {
	r := ticket.Record{
		Front: ticket.Frontmatter{ID: "t-1"},
	}
	m := NewDetail(r, nil)
	m.SetSize(80, 40)

	// Tab on a ticket with no links should not crash or change cursor.
	m = sendSpecialKey(m, tea.KeyTab)
	if m.linkCursor != -1 {
		t.Fatalf("tab on no-links ticket changed linkCursor to %d", m.linkCursor)
	}
}

func TestContextualHelpBarTruncation(t *testing.T) {
	bindings := boardHints(DefaultKeyMap)

	// Wide terminal — should not end with ellipsis.
	wide := contextualHelpBar(bindings, 200)
	if wide == "" {
		t.Fatalf("expected non-empty help bar")
	}

	// Narrow terminal — should truncate with ellipsis.
	narrow := contextualHelpBar(bindings, 40)
	if narrow == "" {
		t.Fatalf("expected non-empty help bar for narrow")
	}
	if len(narrow) >= len(wide) {
		t.Errorf("narrow bar should be shorter than wide bar")
	}
}

func TestDetailHintsIncludeLinksWhenPresent(t *testing.T) {
	km := DefaultKeyMap

	withLinks := detailHints(km, true)
	withoutLinks := detailHints(km, false)

	if len(withLinks) <= len(withoutLinks) {
		t.Fatalf("detailHints with links (%d) should have more bindings than without (%d)",
			len(withLinks), len(withoutLinks))
	}

	// Check that tab and enter hints are present when hasLinks=true.
	found := map[string]bool{}
	for _, b := range withLinks {
		h := b.Help()
		found[h.Key] = true
	}
	if !found["tab"] {
		t.Errorf("expected tab hint in detailHints with links")
	}
	if !found["enter"] {
		t.Errorf("expected enter hint in detailHints with links")
	}
}

func TestWriteFieldWithLinksHighlight(t *testing.T) {
	var b1, b2 strings.Builder
	allLinks := []string{"epic-1", "dep-1", "dep-2"}

	// No selection — all links rendered without marker.
	writeFieldWithLinks(&b1, "Deps", []string{"dep-1", "dep-2"}, allLinks, -1)
	out1 := b1.String()
	if strings.Contains(out1, "▸") {
		t.Errorf("expected no marker when cursor=-1, got %q", out1)
	}

	// Select dep-2 (index 2 in allLinks).
	writeFieldWithLinks(&b2, "Deps", []string{"dep-1", "dep-2"}, allLinks, 2)
	out2 := b2.String()
	if !strings.Contains(out2, "▸") {
		t.Errorf("expected marker when cursor=2, got %q", out2)
	}
}
