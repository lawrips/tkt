package ticket

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestListSkipsMarkdownWithoutFrontmatter(t *testing.T) {
	dir := t.TempDir()

	valid := []byte("---\nid: a-1\nstatus: open\ndeps: []\nlinks: []\ncreated: 2026-02-25T00:00:00Z\ntype: task\npriority: 2\n---\n# A\n")
	if err := os.WriteFile(filepath.Join(dir, "a-1.md"), valid, 0644); err != nil {
		t.Fatalf("write valid ticket: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# scratch notes\n"), 0644); err != nil {
		t.Fatalf("write non-ticket markdown: %v", err)
	}

	records, err := List(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 valid record, got %d", len(records))
	}
	if records[0].ID != "a-1" {
		t.Fatalf("unexpected record id: %s", records[0].ID)
	}
}

func TestListSkipsMarkdownWithMalformedFrontmatterDelimiters(t *testing.T) {
	dir := t.TempDir()

	valid := []byte("---\nid: b-1\nstatus: open\ndeps: []\nlinks: []\ncreated: 2026-02-25T00:00:00Z\ntype: task\npriority: 2\n---\n# B\n")
	if err := os.WriteFile(filepath.Join(dir, "b-1.md"), valid, 0644); err != nil {
		t.Fatalf("write valid ticket: %v", err)
	}
	// Missing closing delimiter.
	malformed := []byte("---\nid: bad-1\nstatus: open\n# body without close delimiter\n")
	if err := os.WriteFile(filepath.Join(dir, "bad-1.md"), malformed, 0644); err != nil {
		t.Fatalf("write malformed ticket: %v", err)
	}

	records, err := List(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 valid record, got %d", len(records))
	}
	if records[0].ID != "b-1" {
		t.Fatalf("unexpected record id: %s", records[0].ID)
	}
}

func TestListSkipsMarkdownWithInvalidFrontmatterValue(t *testing.T) {
	dir := t.TempDir()

	valid := []byte("---\nid: c-1\nstatus: open\ndeps: []\nlinks: []\ncreated: 2026-02-25T00:00:00Z\ntype: task\npriority: 2\n---\n# C\n")
	if err := os.WriteFile(filepath.Join(dir, "c-1.md"), valid, 0644); err != nil {
		t.Fatalf("write valid ticket: %v", err)
	}
	invalid := []byte("---\nid: bad-2\nstatus: open\ndeps: []\nlinks: []\ncreated: 2026-02-25T00:00:00Z\ntype: task\npriority: high\n---\n# Bad\n")
	if err := os.WriteFile(filepath.Join(dir, "bad-2.md"), invalid, 0644); err != nil {
		t.Fatalf("write invalid ticket: %v", err)
	}

	records, err := List(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 valid record, got %d", len(records))
	}
	if records[0].ID != "c-1" {
		t.Fatalf("unexpected record id: %s", records[0].ID)
	}
}

// --- GenerateID ---

func TestGenerateIDReturnsUniqueID(t *testing.T) {
	dir := t.TempDir()
	id, err := GenerateID(dir)
	if err != nil {
		t.Fatalf("GenerateID: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	if len(id) < 3 {
		t.Fatalf("expected ID with prefix, got %q", id)
	}
}

func TestGenerateIDAvoidsCollisions(t *testing.T) {
	dir := t.TempDir()

	// Generate multiple IDs and check uniqueness.
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		id, err := GenerateID(dir)
		if err != nil {
			t.Fatalf("GenerateID iteration %d: %v", i, err)
		}
		if seen[id] {
			t.Fatalf("duplicate ID on iteration %d: %s", i, id)
		}
		seen[id] = true
		// Create the file so subsequent calls must avoid it.
		if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(""), 0644); err != nil {
			t.Fatalf("write placeholder: %v", err)
		}
	}
}

// --- ResolvePath ---

func TestResolvePathExactMatch(t *testing.T) {
	dir := t.TempDir()
	writeTicket(t, dir, "abc-123")

	path, err := ResolvePath(dir, "abc-123")
	if err != nil {
		t.Fatalf("ResolvePath exact: %v", err)
	}
	if filepath.Base(path) != "abc-123.md" {
		t.Fatalf("expected abc-123.md, got %s", filepath.Base(path))
	}
}

func TestResolvePathPartialMatch(t *testing.T) {
	dir := t.TempDir()
	writeTicket(t, dir, "nw-5c46")

	path, err := ResolvePath(dir, "5c4")
	if err != nil {
		t.Fatalf("ResolvePath partial: %v", err)
	}
	if filepath.Base(path) != "nw-5c46.md" {
		t.Fatalf("expected nw-5c46.md, got %s", filepath.Base(path))
	}
}

func TestResolvePathAmbiguous(t *testing.T) {
	dir := t.TempDir()
	writeTicket(t, dir, "feat-abc")
	writeTicket(t, dir, "feat-abd")

	_, err := ResolvePath(dir, "feat-ab")
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
	if !errors.Is(err, ErrAmbiguousID) {
		t.Fatalf("expected ErrAmbiguousID, got %v", err)
	}
}

func TestResolvePathNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := ResolvePath(dir, "nonexistent")
	if !errors.Is(err, ErrTicketNotFound) {
		t.Fatalf("expected ErrTicketNotFound, got %v", err)
	}
}

func TestResolvePathEmptyQuery(t *testing.T) {
	dir := t.TempDir()

	_, err := ResolvePath(dir, "")
	if !errors.Is(err, ErrTicketNotFound) {
		t.Fatalf("expected ErrTicketNotFound for empty query, got %v", err)
	}
}

func TestResolvePathStripsMdSuffix(t *testing.T) {
	dir := t.TempDir()
	writeTicket(t, dir, "strip-test")

	path, err := ResolvePath(dir, "strip-test.md")
	if err != nil {
		t.Fatalf("ResolvePath with .md suffix: %v", err)
	}
	if filepath.Base(path) != "strip-test.md" {
		t.Fatalf("expected strip-test.md, got %s", filepath.Base(path))
	}
}

func writeTicket(t *testing.T, dir, id string) {
	t.Helper()
	content := "---\nid: " + id + "\nstatus: open\ndeps: []\nlinks: []\ncreated: 2026-02-25T00:00:00Z\ntype: task\npriority: 2\n---\n# " + id + "\n"
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(content), 0644); err != nil {
		t.Fatalf("write ticket %s: %v", id, err)
	}
}
