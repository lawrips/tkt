package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func TestCreateEditShowDeleteWorkflow(t *testing.T) {
	withWorkspace(t, func(_ string) {
		out, _, err := runCmd(t, "", "create", "Initial title", "--id", "x-100", "-d", "desc", "--design", "design", "--acceptance", "ac")
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if !strings.Contains(out, "created x-100") {
			t.Fatalf("unexpected create output: %q", out)
		}

		if _, err := os.Stat(filepath.Join(".tickets", "x-100.md")); err != nil {
			t.Fatalf("expected ticket file to exist: %v", err)
		}

		out, _, err = runCmd(t, "", "edit", "x-100", "--title", "Updated title", "-s", "in_progress", "--tags", "ui,api")
		if err != nil {
			t.Fatalf("edit: %v", err)
		}
		if !strings.Contains(out, "updated x-100") {
			t.Fatalf("unexpected edit output: %q", out)
		}

		out, _, err = runCmd(t, "", "show", "x-1")
		if err != nil {
			t.Fatalf("show by partial id: %v", err)
		}
		if !strings.Contains(out, "id: x-100") || !strings.Contains(out, "# Updated title") {
			t.Fatalf("unexpected show output: %q", out)
		}

		out, _, err = runCmd(t, "", "delete", "x-100")
		if err != nil {
			t.Fatalf("delete: %v", err)
		}
		if !strings.Contains(out, "deleted x-100") {
			t.Fatalf("unexpected delete output: %q", out)
		}
		if _, err := os.Stat(filepath.Join(".tickets", "x-100.md")); !os.IsNotExist(err) {
			t.Fatalf("expected ticket file deleted, stat err=%v", err)
		}
	})
}

func TestEditClearParent(t *testing.T) {
	withWorkspace(t, func(_ string) {
		// Create a parent epic and a child ticket.
		if _, _, err := runCmd(t, "", "create", "Epic", "--id", "parent-1", "-t", "epic"); err != nil {
			t.Fatalf("create parent: %v", err)
		}
		if _, _, err := runCmd(t, "", "create", "Child", "--id", "child-1", "--parent", "parent-1"); err != nil {
			t.Fatalf("create child: %v", err)
		}

		// Verify parent is set.
		rec, err := ticket.LoadByID(".tickets", "child-1")
		if err != nil {
			t.Fatalf("load child: %v", err)
		}
		if rec.Front.Parent != "parent-1" {
			t.Fatalf("expected parent=parent-1, got %q", rec.Front.Parent)
		}

		// Clear parent with --parent "".
		out, _, err := runCmd(t, "", "edit", "child-1", "--parent", "")
		if err != nil {
			t.Fatalf("edit clear parent: %v", err)
		}
		if !strings.Contains(out, "updated child-1") {
			t.Fatalf("unexpected edit output: %q", out)
		}

		// Verify parent is cleared.
		rec, err = ticket.LoadByID(".tickets", "child-1")
		if err != nil {
			t.Fatalf("reload child: %v", err)
		}
		if rec.Front.Parent != "" {
			t.Fatalf("expected parent cleared, got %q", rec.Front.Parent)
		}
	})
}

func TestEditReplacesBodyAndOptionalFields(t *testing.T) {
	withWorkspace(t, func(_ string) {
		if _, _, err := runCmd(t, "", "create", "Editable", "--id", "edit-1", "-d", "desc", "--design", "design", "--acceptance", "accept", "--tags", "ui,api", "--external-ref", "ext-1", "-a", "alice"); err != nil {
			t.Fatalf("create: %v", err)
		}

		if _, _, err := runCmd(t, "", "edit", "edit-1", "-d", "new desc", "--design", "new design", "--acceptance", "new accept", "--tags", "backend", "--external-ref", "ext-2", "-a", "bob"); err != nil {
			t.Fatalf("edit replace: %v", err)
		}

		rec, err := ticket.LoadByID(".tickets", "edit-1")
		if err != nil {
			t.Fatalf("load edited ticket: %v", err)
		}
		if rec.Body.Description != "new desc" {
			t.Fatalf("expected description replaced, got %q", rec.Body.Description)
		}
		if rec.Body.Design != "new design" {
			t.Fatalf("expected design replaced, got %q", rec.Body.Design)
		}
		if rec.Body.AcceptanceCriteria != "new accept" {
			t.Fatalf("expected acceptance replaced, got %q", rec.Body.AcceptanceCriteria)
		}
		if rec.Front.Assignee != "bob" {
			t.Fatalf("expected assignee replaced, got %q", rec.Front.Assignee)
		}
		if rec.Front.ExternalRef != "ext-2" {
			t.Fatalf("expected external_ref replaced, got %q", rec.Front.ExternalRef)
		}
		if len(rec.Front.Tags) != 1 || rec.Front.Tags[0] != "backend" {
			t.Fatalf("expected tags replaced, got %#v", rec.Front.Tags)
		}
	})
}

func TestEditClearsBodyAndOptionalFields(t *testing.T) {
	withWorkspace(t, func(_ string) {
		if _, _, err := runCmd(t, "", "create", "Clearable", "--id", "clear-1", "-d", "desc", "--design", "design", "--acceptance", "accept", "--tags", "ui,api", "--external-ref", "ext-1", "-a", "alice", "--parent", "parent-1"); err != nil {
			t.Fatalf("create: %v", err)
		}

		if _, _, err := runCmd(t, "", "edit", "clear-1", "--title", "", "-d", "", "--design", "", "--acceptance", "", "--tags", "", "--external-ref", "", "-a", "", "--parent", ""); err != nil {
			t.Fatalf("edit clear: %v", err)
		}

		rec, err := ticket.LoadByID(".tickets", "clear-1")
		if err != nil {
			t.Fatalf("load cleared ticket: %v", err)
		}
		if rec.Body.Title != "Untitled ticket" {
			t.Fatalf("expected title cleared to persisted fallback, got %q", rec.Body.Title)
		}
		if rec.Body.Description != "" {
			t.Fatalf("expected description cleared, got %q", rec.Body.Description)
		}
		if rec.Body.Design != "" {
			t.Fatalf("expected design cleared, got %q", rec.Body.Design)
		}
		if rec.Body.AcceptanceCriteria != "" {
			t.Fatalf("expected acceptance cleared, got %q", rec.Body.AcceptanceCriteria)
		}
		if rec.Front.Assignee != "" {
			t.Fatalf("expected assignee cleared, got %q", rec.Front.Assignee)
		}
		if rec.Front.ExternalRef != "" {
			t.Fatalf("expected external_ref cleared, got %q", rec.Front.ExternalRef)
		}
		if len(rec.Front.Tags) != 0 {
			t.Fatalf("expected tags cleared, got %#v", rec.Front.Tags)
		}
		if rec.Front.Parent != "" {
			t.Fatalf("expected parent cleared, got %q", rec.Front.Parent)
		}
	})
}

func TestCreateAndListJSONMode(t *testing.T) {
	withWorkspace(t, func(_ string) {
		out, _, err := runCmd(t, "", "create", "JSON title", "--id", "json-1", "--json")
		if err != nil {
			t.Fatalf("create --json: %v", err)
		}

		var createEnvelope map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &createEnvelope); err != nil {
			t.Fatalf("invalid create json envelope: %v output=%q", err, out)
		}
		meta := createEnvelope["meta"].(map[string]any)
		if meta["command"] != "create" {
			t.Fatalf("unexpected command in envelope: %v", meta["command"])
		}

		out, _, err = runCmd(t, "", "ls", "--json")
		if err != nil {
			t.Fatalf("ls --json: %v", err)
		}

		var listEnvelope map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &listEnvelope); err != nil {
			t.Fatalf("invalid list json envelope: %v output=%q", err, out)
		}
		data := listEnvelope["data"].(map[string]any)
		items := data["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected one item in ls --json, got %d", len(items))
		}
		item := items[0].(map[string]any)
		if item["id"] != "json-1" {
			t.Fatalf("unexpected item id in ls --json: %v", item["id"])
		}
	})
}

func TestCreateRejectsDuplicateCustomID(t *testing.T) {
	withWorkspace(t, func(_ string) {
		if _, _, err := runCmd(t, "", "create", "First", "--id", "dup-1"); err != nil {
			t.Fatalf("first create: %v", err)
		}
		if _, _, err := runCmd(t, "", "create", "Second", "--id", "dup-1"); err == nil {
			t.Fatalf("expected duplicate id error")
		}
	})
}

func TestShowIncludesCommitLinks(t *testing.T) {
	withWorkspace(t, func(dir string) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		cfg := project.Config{
			Projects: map[string]project.ProjectConfig{
				"demo": {
					Path:         project.DetectProjectPath(dir),
					Store:        "local",
					AutoLink:     true,
					AutoClose:    true,
					RegisteredAt: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
				},
			},
		}
		if err := project.Save(cfg); err != nil {
			t.Fatalf("save config: %v", err)
		}

		seedTicket(t, "show-1", ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Created:  time.Now().UTC().Format(time.RFC3339),
		}, ticket.Body{Title: "Show target"})
		seedTicket(t, "show-2", ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Created:  time.Now().UTC().Format(time.RFC3339),
		}, ticket.Body{Title: "Other ticket"})

		now := time.Now().UTC()
		writeJournalForTest(t, "demo", []engine.CommitJournalEntry{
			{SHA: "abcdef012345", Ticket: "show-1", Repo: dir, TS: now.Add(-2 * time.Minute).Format(time.RFC3339), Msg: "[show-1] first", Author: "tkt", Action: "ref", FilesChanged: []string{"a.go", "b.go"}, LinesAdded: 50, LinesRemoved: 10},
			{SHA: "123456789abc", Ticket: "show-2", Repo: dir, TS: now.Add(-time.Minute).Format(time.RFC3339), Msg: "[show-2] unrelated", Author: "tkt", Action: "ref"},
			{SHA: "fedcba987654", Ticket: "show-1", Repo: dir, TS: now.Format(time.RFC3339), Msg: "fixes [show-1] done", Author: "tkt", Action: "close", FilesChanged: []string{"c.go"}, LinesAdded: 20, LinesRemoved: 5},
		})

		out, _, err := runCmd(t, "", "show", "show-1")
		if err != nil {
			t.Fatalf("show: %v", err)
		}
		if !strings.Contains(out, "## Recent Commits") {
			t.Fatalf("expected Recent Commits section, got %q", out)
		}
		if !strings.Contains(out, "abcdef0") || !strings.Contains(out, "fedcba9") {
			t.Fatalf("expected ticket commit shas in output, got %q", out)
		}
		if strings.Contains(out, "1234567") {
			t.Fatalf("expected unrelated ticket commit to be excluded, got %q", out)
		}
		// Diff stats should appear in the output.
		if !strings.Contains(out, "+50 -10") {
			t.Fatalf("expected diff stats for first commit, got %q", out)
		}
		if !strings.Contains(out, "+20 -5") {
			t.Fatalf("expected diff stats for second commit, got %q", out)
		}

		out, _, err = runCmd(t, "", "show", "show-1", "--json")
		if err != nil {
			t.Fatalf("show --json: %v", err)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("parse show json: %v output=%q", err, out)
		}
		data := envelope["data"].(map[string]any)
		links := data["commit_links"].([]any)
		if len(links) != 2 {
			t.Fatalf("expected 2 commit_links for show-1, got %d", len(links))
		}
	})
}

func TestListAndFilterBehavior(t *testing.T) {
	withWorkspace(t, func(_ string) {
		seedTicket(t, "o-1", ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
		}, ticket.Body{Title: "Open task"})
		seedTicket(t, "o-2", ticket.Frontmatter{
			Status:   "open",
			Type:     "bug",
			Priority: 0,
			Assignee: "lawrence",
			Tags:     []string{"backend"},
		}, ticket.Body{Title: "Open bug"})
		seedTicket(t, "c-1", ticket.Frontmatter{
			Status:   "closed",
			Type:     "task",
			Priority: 3,
			Assignee: "other",
		}, ticket.Body{Title: "Closed task"})

		out, _, err := runCmd(t, "", "ls")
		if err != nil {
			t.Fatalf("ls: %v", err)
		}
		if strings.Contains(out, "c-1") {
			t.Fatalf("ls should default to open-only, got: %q", out)
		}
		if !strings.Contains(out, "o-1") || !strings.Contains(out, "o-2") {
			t.Fatalf("ls missing expected open tickets: %q", out)
		}

		out, _, err = runCmd(t, "", "ls", "--status", "closed")
		if err != nil {
			t.Fatalf("ls --status closed: %v", err)
		}
		if !strings.Contains(out, "c-1") || strings.Contains(out, "o-1") {
			t.Fatalf("unexpected closed filter output: %q", out)
		}

		out, _, err = runCmd(t, "", "ls", "-t", "bug", "-a", "lawrence", "-T", "backend")
		if err != nil {
			t.Fatalf("ls filter combo: %v", err)
		}
		if !strings.Contains(out, "o-2") || strings.Contains(out, "o-1") {
			t.Fatalf("unexpected filtered list output: %q", out)
		}

		out, _, err = runCmd(t, "", "closed")
		if err != nil {
			t.Fatalf("closed: %v", err)
		}
		if !strings.Contains(out, "c-1") || strings.Contains(out, "o-1") {
			t.Fatalf("unexpected closed output: %q", out)
		}

		// text search by title
		out, _, err = runCmd(t, "", "ls", "bug")
		if err != nil {
			t.Fatalf("ls search: %v", err)
		}
		if !strings.Contains(out, "o-2") || strings.Contains(out, "o-1") {
			t.Fatalf("search should match 'Open bug' only: %q", out)
		}

		// text search by ID
		out, _, err = runCmd(t, "", "ls", "o-1")
		if err != nil {
			t.Fatalf("ls search by id: %v", err)
		}
		if !strings.Contains(out, "o-1") || strings.Contains(out, "o-2") {
			t.Fatalf("search by id should match o-1 only: %q", out)
		}

		// text search combined with filter (use --search flag when mixing with other flags)
		out, _, err = runCmd(t, "", "ls", "--search", "task", "--status", "closed")
		if err != nil {
			t.Fatalf("ls search+status: %v", err)
		}
		if !strings.Contains(out, "c-1") || strings.Contains(out, "o-1") {
			t.Fatalf("search+closed should match c-1 only: %q", out)
		}

		// text search positional arg combined with filter flag (flags must come before positional)
		out, _, err = runCmd(t, "", "ls", "-t", "bug", "bug")
		if err != nil {
			t.Fatalf("ls positional+filter: %v", err)
		}
		if !strings.Contains(out, "o-2") || strings.Contains(out, "o-1") {
			t.Fatalf("positional search+type filter should match o-2 only: %q", out)
		}

		// text search no match
		out, _, err = runCmd(t, "", "ls", "nonexistent")
		if err != nil {
			t.Fatalf("ls search no match: %v", err)
		}
		if strings.Contains(out, "o-1") || strings.Contains(out, "o-2") {
			t.Fatalf("search for nonexistent should return nothing: %q", out)
		}

		// text search case insensitive
		out, _, err = runCmd(t, "", "ls", "OPEN BUG")
		if err != nil {
			t.Fatalf("ls search case: %v", err)
		}
		if !strings.Contains(out, "o-2") {
			t.Fatalf("search should be case insensitive: %q", out)
		}
	})
}

func TestReadyAndBlockedBehavior(t *testing.T) {
	withWorkspace(t, func(_ string) {
		seedTicket(t, "dep-closed", ticket.Frontmatter{
			Status:   "closed",
			Type:     "task",
			Priority: 1,
		}, ticket.Body{Title: "Dependency closed"})
		seedTicket(t, "dep-open", ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 1,
		}, ticket.Body{Title: "Dependency open"})
		seedTicket(t, "parent-ip", ticket.Frontmatter{
			Status:   "in_progress",
			Type:     "epic",
			Priority: 1,
		}, ticket.Body{Title: "Parent in progress"})
		seedTicket(t, "parent-closed", ticket.Frontmatter{
			Status:   "closed",
			Type:     "epic",
			Priority: 1,
		}, ticket.Body{Title: "Parent closed"})
		seedTicket(t, "ready-child", ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Deps:     []string{"dep-closed"},
			Parent:   "parent-ip",
		}, ticket.Body{Title: "Ready child"})
		seedTicket(t, "blocked-child", ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Deps:     []string{"dep-open"},
			Parent:   "parent-ip",
		}, ticket.Body{Title: "Blocked child"})
		seedTicket(t, "parent-gated", ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Deps:     []string{"dep-closed"},
			Parent:   "parent-closed",
		}, ticket.Body{Title: "Gated by parent"})

		out, _, err := runCmd(t, "", "ready")
		if err != nil {
			t.Fatalf("ready: %v", err)
		}
		if !strings.Contains(out, "ready-child") {
			t.Fatalf("ready should include ready-child: %q", out)
		}
		if strings.Contains(out, "parent-gated") || strings.Contains(out, "blocked-child") {
			t.Fatalf("ready should exclude parent-gated and blocked-child: %q", out)
		}

		out, _, err = runCmd(t, "", "ready", "--open")
		if err != nil {
			t.Fatalf("ready --open: %v", err)
		}
		if !strings.Contains(out, "parent-gated") {
			t.Fatalf("ready --open should bypass parent checks: %q", out)
		}

		out, _, err = runCmd(t, "", "blocked")
		if err != nil {
			t.Fatalf("blocked: %v", err)
		}
		if !strings.Contains(out, "blocked-child") || strings.Contains(out, "ready-child") {
			t.Fatalf("unexpected blocked output: %q", out)
		}
	})
}
