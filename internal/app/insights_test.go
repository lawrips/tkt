package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/ticket"
)

type insightsFixture struct {
	id       string
	status   string
	typ      string
	priority int
	parent   string
	deps     []string
	created  string
	closedAt string
}

func writeInsightsTicket(t *testing.T, dir string, fx insightsFixture) {
	t.Helper()
	deps := fx.deps
	if deps == nil {
		deps = []string{}
	}
	created := fx.created
	if created == "" {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	record := ticket.Record{
		ID:   fx.id,
		Path: filepath.Join(dir, fx.id+".md"),
		Front: ticket.Frontmatter{
			ID:       fx.id,
			Status:   fx.status,
			Deps:     deps,
			Links:    []string{},
			Created:  created,
			ClosedAt: fx.closedAt,
			Type:     fx.typ,
			Priority: fx.priority,
			Parent:   fx.parent,
			Extra:    map[string]ticket.ExtraField{},
		},
		Body: ticket.Body{Title: "Ticket " + fx.id},
	}
	if err := ticket.SaveRecord(record); err != nil {
		t.Fatalf("save ticket %s: %v", fx.id, err)
	}
}

func setupInsightsProject(t *testing.T, now time.Time) *Service {
	t.Helper()
	_, repo, ticketDir := setupProject(t)

	thisWeek := now.Format(time.RFC3339)
	twoWeeksAgo := now.AddDate(0, 0, -14).Format(time.RFC3339)
	twoMonthsAgo := now.AddDate(0, -2, 0).Format(time.RFC3339)

	writeInsightsTicket(t, ticketDir, insightsFixture{id: "epic-1", status: "open", typ: "epic", priority: 1})
	writeInsightsTicket(t, ticketDir, insightsFixture{id: "child-closed", status: "closed", typ: "task", priority: 2, parent: "epic-1", created: twoMonthsAgo})
	writeInsightsTicket(t, ticketDir, insightsFixture{id: "child-ready", status: "open", typ: "task", priority: 2, parent: "epic-1", deps: []string{"child-closed"}})
	writeInsightsTicket(t, ticketDir, insightsFixture{id: "child-blocked", status: "open", typ: "task", priority: 2, parent: "epic-1", deps: []string{"task-free"}})
	writeInsightsTicket(t, ticketDir, insightsFixture{id: "task-free", status: "open", typ: "task", priority: 2})
	writeInsightsTicket(t, ticketDir, insightsFixture{id: "bug-active", status: "in_progress", typ: "bug", priority: 0})
	writeInsightsTicket(t, ticketDir, insightsFixture{id: "chore-done", status: "closed", typ: "chore", priority: 4, created: twoMonthsAgo, closedAt: twoWeeksAgo})
	writeInsightsJournal(t, []engine.CommitJournalEntry{
		{Ticket: "child-closed", TS: thisWeek, Action: "close"},
	})

	return New(Options{CWD: repo, Now: func() time.Time { return now }})
}

func writeInsightsJournal(t *testing.T, entries []engine.CommitJournalEntry) {
	t.Helper()
	path, err := engine.JournalPath("demo")
	if err != nil {
		t.Fatalf("journal path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir journal: %v", err)
	}
	raw := make([]byte, 0)
	for _, entry := range entries {
		raw = append(raw, []byte(`{"ticket":"`+entry.Ticket+`","ts":"`+entry.TS+`","action":"`+entry.Action+`"}`+"\n")...)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("write journal: %v", err)
	}
}

func TestStatsCountsStatusesTypesAndQueues(t *testing.T) {
	now := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	svc := setupInsightsProject(t, now)

	stats, err := svc.Stats("demo")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stats.Total != 7 {
		t.Fatalf("expected 7 tickets, got %d", stats.Total)
	}
	if stats.ByStatus["open"] != 4 || stats.ByStatus["in_progress"] != 1 || stats.ByStatus["closed"] != 2 {
		t.Fatalf("unexpected status counts: %#v", stats.ByStatus)
	}
	if stats.ByType["epic"] != 1 || stats.ByType["task"] != 4 || stats.ByType["bug"] != 1 || stats.ByType["chore"] != 1 {
		t.Fatalf("unexpected type counts: %#v", stats.ByType)
	}
	if stats.ByPriority[2] != 4 || stats.ByPriority[0] != 1 || stats.ByPriority[1] != 1 || stats.ByPriority[4] != 1 {
		t.Fatalf("unexpected priority counts: %#v", stats.ByPriority)
	}
	// Ready: epic-1, child-ready (dep closed), task-free. Blocked: child-blocked.
	if stats.Ready != 3 {
		t.Fatalf("expected 3 ready, got %d", stats.Ready)
	}
	if stats.Blocked != 1 {
		t.Fatalf("expected 1 blocked, got %d", stats.Blocked)
	}
}

func TestTimelineBucketsClosedTicketsByWeek(t *testing.T) {
	// 2026-01-20 is a Tuesday; week starts Monday 2026-01-19.
	now := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	svc := setupInsightsProject(t, now)

	report, err := svc.Timeline("demo", 4)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(report.Weeks) != 4 {
		t.Fatalf("expected 4 weeks, got %d", len(report.Weeks))
	}
	expected := map[string]int{
		"2025-12-29": 0,
		"2026-01-05": 1, // chore-done closed_at two weeks before now
		"2026-01-12": 0,
		"2026-01-19": 1, // child-closed has a close journal entry this week
	}
	for i, week := range report.Weeks {
		want, ok := expected[week.WeekStart]
		if !ok {
			t.Fatalf("unexpected week %q at index %d", week.WeekStart, i)
		}
		if week.ClosedCount != want {
			t.Fatalf("week %s: expected %d closed, got %d", week.WeekStart, want, week.ClosedCount)
		}
		if week.WeekStart == "2026-01-19" && (len(week.TicketIDs) != 1 || week.TicketIDs[0] != "child-closed") {
			t.Fatalf("expected child-closed ticket id in current week, got %#v", week.TicketIDs)
		}
	}
}

func TestTimelineDefaultsAndValidatesWeeks(t *testing.T) {
	now := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	svc := setupInsightsProject(t, now)

	report, err := svc.Timeline("demo", 0)
	if err != nil {
		t.Fatalf("timeline default: %v", err)
	}
	if len(report.Weeks) != 4 {
		t.Fatalf("expected default of 4 weeks, got %d", len(report.Weeks))
	}

	if _, err := svc.Timeline("demo", 500); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected validation error for oversized weeks, got %v", err)
	}
}

func TestEpicOverviewReportsChildProgress(t *testing.T) {
	now := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	svc := setupInsightsProject(t, now)

	report, err := svc.EpicOverview("demo")
	if err != nil {
		t.Fatalf("epic overview: %v", err)
	}
	if len(report.Epics) != 1 {
		t.Fatalf("expected 1 epic, got %#v", report.Epics)
	}
	epic := report.Epics[0]
	if epic.ID != "epic-1" || epic.Status != "open" || epic.Priority != 1 {
		t.Fatalf("unexpected epic summary: %#v", epic)
	}
	if epic.TotalChildren != 3 || epic.ClosedChildren != 1 {
		t.Fatalf("expected 3 children / 1 closed, got %d / %d", epic.TotalChildren, epic.ClosedChildren)
	}
	if epic.ChildrenByStatus["open"] != 2 || epic.ChildrenByStatus["closed"] != 1 {
		t.Fatalf("unexpected child status counts: %#v", epic.ChildrenByStatus)
	}
}

func TestInsightsUnknownProject(t *testing.T) {
	now := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)
	svc := setupInsightsProject(t, now)

	if _, err := svc.Stats("nope"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("stats: expected project not found, got %v", err)
	}
	if _, err := svc.Timeline("nope", 4); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("timeline: expected project not found, got %v", err)
	}
	if _, err := svc.EpicOverview("nope"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("epic overview: expected project not found, got %v", err)
	}
}
