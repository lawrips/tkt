package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func TestProjectsReportsUninitializedDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := t.TempDir()

	svc := New(Options{CWD: cwd})
	overview, err := svc.Projects()
	if err != nil {
		t.Fatalf("projects: %v", err)
	}
	if overview.Initialized {
		t.Fatalf("expected uninitialized overview")
	}
	if overview.Message == "" {
		t.Fatalf("expected setup guidance message")
	}
}

func TestDirWritableUsesDirectoryMode(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if !dirWritable(dir) {
		t.Fatalf("expected writable mode to be reported writable")
	}
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })
	if dirWritable(dir) {
		t.Fatalf("expected read-only mode to be reported not writable")
	}
}

func TestListTicketsReturnsParseDiagnostics(t *testing.T) {
	home, repo, ticketDir := setupProject(t)
	_ = home
	writeTicket(t, ticketDir, "c-good", "Good ticket")
	if err := os.WriteFile(filepath.Join(ticketDir, "bad.md"), []byte("# Missing frontmatter\n"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := New(Options{CWD: repo})
	list, err := svc.ListTickets("demo", ListOptions{})
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if list.Total != 1 || list.Items[0].ID != "c-good" {
		t.Fatalf("expected one valid ticket, got %#v", list.Items)
	}
	if len(list.Diagnostics) != 1 {
		t.Fatalf("expected one diagnostic, got %#v", list.Diagnostics)
	}
	if list.Diagnostics[0].Code != "missing_frontmatter" {
		t.Fatalf("expected missing_frontmatter diagnostic, got %#v", list.Diagnostics[0])
	}
}

func TestUpdateTicketRejectsStaleRevision(t *testing.T) {
	_, repo, ticketDir := setupProject(t)
	writeTicket(t, ticketDir, "c-one", "Original")

	svc := New(Options{CWD: repo})
	detail, err := svc.TicketDetail("demo", "c-one")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}

	record, err := ticket.LoadByID(ticketDir, "c-one")
	if err != nil {
		t.Fatal(err)
	}
	record.Body.Title = "Changed elsewhere"
	if err := ticket.SaveRecord(record); err != nil {
		t.Fatal(err)
	}

	next := "Web edit"
	_, err = svc.UpdateTicket("demo", "c-one", UpdateTicketInput{
		Source:           "test",
		ExpectedRevision: &detail.Revision,
		Title:            &next,
	})
	if !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("expected stale revision, got %v", err)
	}
}

func TestUpdateTicketWritesMutationLog(t *testing.T) {
	home, repo, ticketDir := setupProject(t)
	writeTicket(t, ticketDir, "c-one", "Original")

	svc := New(Options{CWD: repo})
	detail, err := svc.TicketDetail("demo", "c-one")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	next := "Updated"
	updated, err := svc.UpdateTicket("demo", "c-one", UpdateTicketInput{
		Source:           "test",
		ExpectedRevision: &detail.Revision,
		Title:            &next,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != next {
		t.Fatalf("expected title %q, got %q", next, updated.Title)
	}

	raw, err := os.ReadFile(filepath.Join(home, ".tkt", "state", "demo", "mutations.jsonl"))
	if err != nil {
		t.Fatalf("read mutation log: %v", err)
	}
	if !strings.Contains(string(raw), `"operation":"edit"`) || !strings.Contains(string(raw), `"source":"test"`) {
		t.Fatalf("mutation log missing edit/source: %s", string(raw))
	}
}

func TestAddNoteAndLinkDependencyMutations(t *testing.T) {
	_, repo, ticketDir := setupProject(t)
	writeTicket(t, ticketDir, "c-one", "One")
	writeTicket(t, ticketDir, "c-two", "Two")

	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	svc := New(Options{CWD: repo, Now: func() time.Time { return now }})
	detail, err := svc.TicketDetail("demo", "c-one")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	withNote, err := svc.AddNote("demo", "c-one", NoteInput{
		Source:           "test",
		Text:             "Remember this.",
		ExpectedRevision: &detail.Revision,
	})
	if err != nil {
		t.Fatalf("add note: %v", err)
	}
	if len(withNote.OtherSections) == 0 || !strings.Contains(withNote.OtherSections[0].Content, "Remember this.") {
		t.Fatalf("note not added: %#v", withNote.OtherSections)
	}

	withDep, err := svc.AddDependency("demo", "c-one", EdgeInput{
		Source:           "test",
		TargetID:         "c-two",
		ExpectedRevision: &withNote.Revision,
	})
	if err != nil {
		t.Fatalf("add dependency: %v", err)
	}
	if len(withDep.Deps) != 1 || withDep.Deps[0].ID != "c-two" {
		t.Fatalf("dependency not added: %#v", withDep.Deps)
	}

	withLink, err := svc.LinkTickets("demo", "c-one", EdgeInput{
		Source:           "test",
		TargetID:         "c-two",
		ExpectedRevision: &withDep.Revision,
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if len(withLink.Links) != 1 || withLink.Links[0].ID != "c-two" {
		t.Fatalf("link not added: %#v", withLink.Links)
	}
}

func setupProject(t *testing.T) (home, repo, ticketDir string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	repo = t.TempDir()
	ticketDir = filepath.Join(repo, ".tickets")
	if err := os.MkdirAll(ticketDir, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := project.Config{Projects: map[string]project.ProjectConfig{
		"demo": {
			Path:         repo,
			Store:        "local",
			AutoLink:     true,
			AutoClose:    true,
			RegisteredAt: time.Now().UTC().Format(time.RFC3339),
		},
	}}
	if err := project.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return home, repo, ticketDir
}

func writeTicket(t *testing.T, dir, id, title string) {
	t.Helper()
	record := ticket.Record{
		ID:   id,
		Path: filepath.Join(dir, id+".md"),
		Front: ticket.Frontmatter{
			ID:       id,
			Status:   "open",
			Deps:     []string{},
			Links:    []string{},
			Created:  time.Now().UTC().Format(time.RFC3339),
			Type:     "task",
			Priority: 2,
			Extra:    map[string]ticket.ExtraField{},
		},
		Body: ticket.Body{Title: title},
	}
	if err := ticket.SaveRecord(record); err != nil {
		t.Fatalf("save ticket %s: %v", id, err)
	}
}
