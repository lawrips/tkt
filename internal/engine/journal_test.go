package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadJournalEntries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Empty project name returns empty
	entries, err := ReadJournalEntries("")
	if err != nil || len(entries) != 0 {
		t.Fatalf("expected empty for blank project, got %v %v", entries, err)
	}

	// Missing file returns empty
	entries, err = ReadJournalEntries("nonexistent")
	if err != nil || len(entries) != 0 {
		t.Fatalf("expected empty for missing file, got %v %v", entries, err)
	}

	// Write some entries
	dir := filepath.Join(home, ".tkt", "state", "testproj")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "commits.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	_ = enc.Encode(CommitJournalEntry{SHA: "aaa", Ticket: "t-1", Action: "ref"})
	_ = enc.Encode(CommitJournalEntry{SHA: "bbb", Ticket: "t-2", Action: "close"})
	f.Close()

	entries, err = ReadJournalEntries("testproj")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].SHA != "aaa" || entries[1].Action != "close" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestFilterJournalByTickets(t *testing.T) {
	entries := []CommitJournalEntry{
		{SHA: "1", Ticket: "a"},
		{SHA: "2", Ticket: "b"},
		{SHA: "3", Ticket: "a"},
		{SHA: "4", Ticket: "c"},
	}

	filtered := FilterJournalByTickets(entries, []string{"a", "c"})
	if len(filtered) != 3 {
		t.Fatalf("expected 3, got %d", len(filtered))
	}

	filtered = FilterJournalByTickets(entries, []string{"z"})
	if len(filtered) != 0 {
		t.Fatalf("expected 0, got %d", len(filtered))
	}
}

func TestCountJournalForTicket(t *testing.T) {
	entries := []CommitJournalEntry{
		{Ticket: "a"}, {Ticket: "b"}, {Ticket: "a"},
	}
	if got := CountJournalForTicket(entries, "a"); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
	if got := CountJournalForTicket(entries, "z"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestLastNJournalEntries(t *testing.T) {
	entries := []CommitJournalEntry{
		{SHA: "1"}, {SHA: "2"}, {SHA: "3"}, {SHA: "4"}, {SHA: "5"},
	}
	got := LastNJournalEntries(entries, 3)
	if len(got) != 3 || got[0].SHA != "3" {
		t.Fatalf("expected last 3 starting with SHA 3, got %+v", got)
	}

	got = LastNJournalEntries(entries, 10)
	if len(got) != 5 {
		t.Fatalf("expected all 5, got %d", len(got))
	}
}

func TestAppendMutationLog(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Empty project is a no-op
	AppendMutationLog("", MutationEntry{TicketID: "x"})

	// Write a mutation
	AppendMutationLog("testproj", MutationEntry{
		TicketID:      "t-1",
		Operation:     "create",
		Source:        "test",
		FieldsChanged: []string{"title"},
	})

	path := filepath.Join(home, ".tkt", "state", "testproj", "mutations.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mutation log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var entry MutationEntry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.TicketID != "t-1" || entry.Operation != "create" || entry.Source != "test" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.Timestamp == "" {
		t.Fatal("expected auto-filled timestamp")
	}

	// Append another
	AppendMutationLog("testproj", MutationEntry{
		TicketID:  "t-1",
		Operation: "edit",
		Source:    "test",
	})
	raw, _ = os.ReadFile(path)
	lines = strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}
