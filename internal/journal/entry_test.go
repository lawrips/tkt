package journal

import "testing"

// --- Effort ---

func TestEffortEmptySlice(t *testing.T) {
	got := Effort(nil)
	if got.LinesAdded != 0 || got.LinesRemoved != 0 || got.FilesChanged != 0 || got.Commits != 0 {
		t.Fatalf("expected zero EffortSummary, got %+v", got)
	}
}

func TestEffortSingleEntry(t *testing.T) {
	entries := []Entry{
		{LinesAdded: 10, LinesRemoved: 3, FilesChanged: []string{"a.go", "b.go"}},
	}
	got := Effort(entries)
	if got.LinesAdded != 10 {
		t.Fatalf("LinesAdded: want 10, got %d", got.LinesAdded)
	}
	if got.LinesRemoved != 3 {
		t.Fatalf("LinesRemoved: want 3, got %d", got.LinesRemoved)
	}
	if got.FilesChanged != 2 {
		t.Fatalf("FilesChanged: want 2, got %d", got.FilesChanged)
	}
	if got.Commits != 1 {
		t.Fatalf("Commits: want 1, got %d", got.Commits)
	}
}

func TestEffortMultipleEntriesAggregates(t *testing.T) {
	entries := []Entry{
		{LinesAdded: 5, LinesRemoved: 1, FilesChanged: []string{"a.go"}},
		{LinesAdded: 7, LinesRemoved: 2, FilesChanged: []string{"b.go"}},
		{LinesAdded: 3, LinesRemoved: 0, FilesChanged: []string{"c.go"}},
	}
	got := Effort(entries)
	if got.LinesAdded != 15 {
		t.Fatalf("LinesAdded: want 15, got %d", got.LinesAdded)
	}
	if got.LinesRemoved != 3 {
		t.Fatalf("LinesRemoved: want 3, got %d", got.LinesRemoved)
	}
	if got.FilesChanged != 3 {
		t.Fatalf("FilesChanged: want 3, got %d", got.FilesChanged)
	}
	if got.Commits != 3 {
		t.Fatalf("Commits: want 3, got %d", got.Commits)
	}
}

func TestEffortDeduplicatesFilesChanged(t *testing.T) {
	entries := []Entry{
		{LinesAdded: 1, FilesChanged: []string{"a.go", "b.go"}},
		{LinesAdded: 2, FilesChanged: []string{"b.go", "c.go"}},
	}
	got := Effort(entries)
	// b.go appears in both entries but should count once
	if got.FilesChanged != 3 {
		t.Fatalf("FilesChanged: want 3, got %d", got.FilesChanged)
	}
	if got.LinesAdded != 3 {
		t.Fatalf("LinesAdded: want 3, got %d", got.LinesAdded)
	}
	if got.Commits != 2 {
		t.Fatalf("Commits: want 2, got %d", got.Commits)
	}
}

// --- EffortSummary.String ---

func TestEffortSummaryStringAllZero(t *testing.T) {
	got := EffortSummary{}.String()
	if got != "" {
		t.Fatalf("want empty string for zero summary, got %q", got)
	}
}

func TestEffortSummaryStringNonZero(t *testing.T) {
	s := EffortSummary{LinesAdded: 4, LinesRemoved: 1, FilesChanged: 2, Commits: 1}
	got := s.String()
	want := "+4 -1, 2 file(s)"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// --- GroupByTicket ---

func TestGroupByTicketEmptySlice(t *testing.T) {
	got := GroupByTicket(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %+v", got)
	}
}

func TestGroupByTicketDifferentTickets(t *testing.T) {
	entries := []Entry{
		{SHA: "aaa", Ticket: "tk-1"},
		{SHA: "bbb", Ticket: "tk-2"},
	}
	got := GroupByTicket(entries)
	if len(got) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(got))
	}
	if len(got["tk-1"]) != 1 || got["tk-1"][0].SHA != "aaa" {
		t.Fatalf("unexpected tk-1 group: %+v", got["tk-1"])
	}
	if len(got["tk-2"]) != 1 || got["tk-2"][0].SHA != "bbb" {
		t.Fatalf("unexpected tk-2 group: %+v", got["tk-2"])
	}
}

func TestGroupByTicketSameTicketMultipleEntries(t *testing.T) {
	entries := []Entry{
		{SHA: "aaa", Ticket: "tk-1"},
		{SHA: "bbb", Ticket: "tk-1"},
		{SHA: "ccc", Ticket: "tk-1"},
	}
	got := GroupByTicket(entries)
	if len(got) != 1 {
		t.Fatalf("expected 1 group, got %d", len(got))
	}
	if len(got["tk-1"]) != 3 {
		t.Fatalf("expected 3 entries in tk-1, got %d", len(got["tk-1"]))
	}
}

// --- FirstLine ---

func TestFirstLineSingleLine(t *testing.T) {
	got := FirstLine("hello world")
	if got != "hello world" {
		t.Fatalf("want %q, got %q", "hello world", got)
	}
}

func TestFirstLineMultiLine(t *testing.T) {
	got := FirstLine("first line\nsecond line\nthird line")
	if got != "first line" {
		t.Fatalf("want %q, got %q", "first line", got)
	}
}

func TestFirstLineEmpty(t *testing.T) {
	got := FirstLine("")
	if got != "" {
		t.Fatalf("want empty string, got %q", got)
	}
}

func TestFirstLineNoNewline(t *testing.T) {
	got := FirstLine("no newline here")
	if got != "no newline here" {
		t.Fatalf("want %q, got %q", "no newline here", got)
	}
}
