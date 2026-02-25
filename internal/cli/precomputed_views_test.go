package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func TestEpicViewJSON(t *testing.T) {
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

		seedTicket(t, "epic-1", ticket.Frontmatter{Status: "open", Type: "epic", Priority: 1, Created: time.Now().UTC().Format(time.RFC3339)}, ticket.Body{Title: "Epic"})
		seedTicket(t, "child-1", ticket.Frontmatter{Status: "open", Type: "task", Priority: 2, Parent: "epic-1", Created: time.Now().UTC().Format(time.RFC3339), Deps: []string{}}, ticket.Body{Title: "Child 1"})
		seedTicket(t, "child-2", ticket.Frontmatter{Status: "in_progress", Type: "task", Priority: 2, Parent: "epic-1", Created: time.Now().UTC().Format(time.RFC3339), Deps: []string{"child-1"}}, ticket.Body{Title: "Child 2"})

		writeJournalForTest(t, "demo", []engine.CommitJournalEntry{
			{SHA: "a0", Ticket: "epic-1", Repo: dir, TS: time.Now().UTC().Format(time.RFC3339), Msg: "[epic-1] coordination", Author: "tkt", Action: "ref"},
			{SHA: "a1", Ticket: "child-1", Repo: dir, TS: time.Now().UTC().Format(time.RFC3339), Msg: "[child-1] work", Author: "tkt", Action: "ref"},
			{SHA: "a2", Ticket: "child-2", Repo: dir, TS: time.Now().UTC().Format(time.RFC3339), Msg: "fixes [child-2] done", Author: "tkt", Action: "close"},
		})

		out, _, err := runCmd(t, "", "epic-view", "epic-1", "--json")
		if err != nil {
			t.Fatalf("epic-view --json: %v", err)
		}

		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("parse epic-view json: %v output=%q", err, out)
		}
		data := envelope["data"].(map[string]any)
		epic := data["epic"].(map[string]any)
		if _, ok := epic["description"]; ok {
			t.Fatalf("epic summary should not include description: %#v", epic)
		}
		children := data["children"].([]any)
		commits := data["commits"].([]any)
		if len(children) != 2 {
			t.Fatalf("expected 2 children, got %d", len(children))
		}
		if len(commits) != 3 {
			t.Fatalf("expected 3 commits, got %d", len(commits))
		}

		counts := map[string]map[string]any{}
		for _, item := range children {
			child := item.(map[string]any)
			id := child["id"].(string)
			if _, ok := child["description"]; ok {
				t.Fatalf("epic child summary should not include description: %#v", child)
			}
			counts[id] = child
		}
		if int(counts["child-1"]["direct_commits"].(float64)) != 1 || int(counts["child-1"]["rolled_up_commits"].(float64)) != 1 || int(counts["child-1"]["total_commits"].(float64)) != 2 {
			t.Fatalf("unexpected commit counts for child-1: %+v", counts["child-1"])
		}
		if int(counts["child-2"]["direct_commits"].(float64)) != 1 || int(counts["child-2"]["rolled_up_commits"].(float64)) != 1 || int(counts["child-2"]["total_commits"].(float64)) != 2 {
			t.Fatalf("unexpected commit counts for child-2: %+v", counts["child-2"])
		}
	})
}

func TestProgressAndDashboardJSON(t *testing.T) {
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

		seedTicket(t, "p-open", ticket.Frontmatter{Status: "open", Type: "task", Priority: 2, Created: time.Now().UTC().Format(time.RFC3339)}, ticket.Body{Title: "Open"})
		seedTicket(t, "p-blocked", ticket.Frontmatter{Status: "open", Type: "task", Priority: 2, Deps: []string{"dep-open"}, Created: time.Now().UTC().Format(time.RFC3339)}, ticket.Body{Title: "Blocked"})
		seedTicket(t, "dep-open", ticket.Frontmatter{Status: "open", Type: "task", Priority: 2, Created: time.Now().UTC().Format(time.RFC3339)}, ticket.Body{Title: "Dep open"})
		seedTicket(t, "p-ip", ticket.Frontmatter{Status: "in_progress", Type: "task", Priority: 1, Created: time.Now().UTC().Format(time.RFC3339)}, ticket.Body{Title: "IP"})

		now := time.Now().UTC()
		writeJournalForTest(t, "demo", []engine.CommitJournalEntry{
			{SHA: "x1", Ticket: "p-open", Repo: dir, TS: now.Add(-2 * time.Hour).Format(time.RFC3339), Msg: "[p-open] work", Author: "tkt", Action: "ref"},
			{SHA: "x2", Ticket: "p-open", Repo: dir, TS: now.Add(-time.Hour).Format(time.RFC3339), Msg: "fixes [p-open] done", Author: "tkt", Action: "close"},
			{SHA: "x3", Ticket: "p-ip", Repo: dir, TS: now.Add(-10 * 24 * time.Hour).Format(time.RFC3339), Msg: "[p-ip] old", Author: "tkt", Action: "ref"},
		})

		out, _, err := runCmd(t, "", "progress", "--today", "--json")
		if err != nil {
			t.Fatalf("progress --json: %v", err)
		}
		var progressEnvelope map[string]any
		if err := json.Unmarshal([]byte(out), &progressEnvelope); err != nil {
			t.Fatalf("parse progress json: %v", err)
		}
		progressData := progressEnvelope["data"].(map[string]any)
		if progressData["window"] != "today" {
			t.Fatalf("expected today window, got %v", progressData["window"])
		}
		closed := progressData["closed"].([]any)
		if len(closed) != 1 {
			t.Fatalf("expected 1 closed ticket in progress, got %d", len(closed))
		}
		if _, ok := closed[0].(map[string]any)["description"]; ok {
			t.Fatalf("progress closed summary should not include description: %#v", closed[0])
		}

		out, _, err = runCmd(t, "", "dashboard", "--json")
		if err != nil {
			t.Fatalf("dashboard --json: %v", err)
		}
		var dashboardEnvelope map[string]any
		if err := json.Unmarshal([]byte(out), &dashboardEnvelope); err != nil {
			t.Fatalf("parse dashboard json: %v", err)
		}
		summary := dashboardEnvelope["data"].(map[string]any)["summary"].(map[string]any)
		if int(summary["blocked"].(float64)) < 1 {
			t.Fatalf("expected blocked >= 1, got %v", summary["blocked"])
		}
		if int(summary["in_progress"].(float64)) != 1 {
			t.Fatalf("expected in_progress=1, got %v", summary["in_progress"])
		}
		inProgress := dashboardEnvelope["data"].(map[string]any)["in_progress"].([]any)
		if len(inProgress) != 1 {
			t.Fatalf("expected 1 in_progress item, got %d", len(inProgress))
		}
		if _, ok := inProgress[0].(map[string]any)["description"]; ok {
			t.Fatalf("dashboard summary item should not include description: %#v", inProgress[0])
		}
	})
}

func TestProgressClosedViaEditIncluded(t *testing.T) {
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

		// Ticket closed via tk edit -s closed — no journal entry.
		seedTicket(t, "edit-closed", ticket.Frontmatter{Status: "closed", Type: "task", Priority: 2, Created: time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)}, ticket.Body{Title: "Closed via edit"})
		// Ticket still open — should not appear.
		seedTicket(t, "still-open", ticket.Frontmatter{Status: "open", Type: "task", Priority: 2, Created: time.Now().UTC().Format(time.RFC3339)}, ticket.Body{Title: "Still open"})

		// Empty journal — no commit events.
		writeJournalForTest(t, "demo", []engine.CommitJournalEntry{})

		out, _, err := runCmd(t, "", "progress", "--today", "--json")
		if err != nil {
			t.Fatalf("progress --json: %v", err)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("parse progress json: %v output=%q", err, out)
		}
		data := envelope["data"].(map[string]any)
		closed := data["closed"].([]any)
		if len(closed) != 1 {
			t.Fatalf("expected 1 closed ticket, got %d: %v", len(closed), closed)
		}
		got := closed[0].(map[string]any)["id"].(string)
		if got != "edit-closed" {
			t.Fatalf("expected closed ticket id=edit-closed, got %s", got)
		}
	})
}

func TestLifecycleWithMixedTimezones(t *testing.T) {
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

		seedTicket(t, "lc-1", ticket.Frontmatter{
			Status:   "in_progress",
			Type:     "task",
			Priority: 2,
			Created:  "2026-02-20T10:00:00Z",
		}, ticket.Body{Title: "Lifecycle target"})

		// The chronologically earliest commit uses a +05:30 offset.
		// As a string, "2026-02-21T08:00:00+05:30" > "2026-02-21T09:00:00Z"
		// but in absolute time 08:00+05:30 = 02:30Z, which is earlier than 09:00Z.
		writeJournalForTest(t, "demo", []engine.CommitJournalEntry{
			{SHA: "aaa", Ticket: "lc-1", Repo: dir, TS: "2026-02-21T09:00:00Z", Msg: "[lc-1] second", Author: "tkt", Action: "ref", LinesAdded: 10, LinesRemoved: 2, FilesChanged: []string{"a.go"}, WorkStarted: "2026-02-21T08:00:00Z", WorkEnded: "2026-02-21T09:00:00Z", DurationSecs: 3600},
			{SHA: "bbb", Ticket: "lc-1", Repo: dir, TS: "2026-02-21T08:00:00+05:30", Msg: "[lc-1] first", Author: "tkt", Action: "ref", LinesAdded: 5, LinesRemoved: 1, FilesChanged: []string{"b.go"}, WorkStarted: "2026-02-21T01:30:00Z", WorkEnded: "2026-02-21T02:30:00Z", DurationSecs: 3600},
			{SHA: "ccc", Ticket: "lc-1", Repo: dir, TS: "2026-02-22T12:00:00Z", Msg: "fixes [lc-1] done", Author: "tkt", Action: "close", LinesAdded: 3, LinesRemoved: 0, FilesChanged: []string{"a.go"}, WorkStarted: "2026-02-22T11:30:00Z", WorkEnded: "2026-02-22T12:00:00Z", DurationSecs: 1800},
		})

		out, _, err := runCmd(t, "", "lifecycle", "lc-1", "--json")
		if err != nil {
			t.Fatalf("lifecycle --json: %v", err)
		}

		var envelope map[string]any
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatalf("parse lifecycle json: %v output=%q", err, out)
		}
		data := envelope["data"].(map[string]any)

		// first_commit should be the +05:30 entry (02:30 UTC), not the 09:00Z entry.
		firstCommit := data["first_commit"].(string)
		if firstCommit != "2026-02-21T02:30:00Z" {
			t.Fatalf("expected first_commit=2026-02-21T02:30:00Z (the +05:30 entry), got %s", firstCommit)
		}

		lastCommit := data["last_commit"].(string)
		if lastCommit != "2026-02-22T12:00:00Z" {
			t.Fatalf("expected last_commit=2026-02-22T12:00:00Z, got %s", lastCommit)
		}
		closedAt := data["closed_at"].(string)
		if closedAt != "2026-02-22T12:00:00Z" {
			t.Fatalf("expected closed_at=2026-02-22T12:00:00Z, got %s", closedAt)
		}
		if int(data["work_seconds"].(float64)) != 9000 {
			t.Fatalf("expected work_seconds=9000, got %v", data["work_seconds"])
		}
		if int(data["calendar_seconds"].(float64)) != 180000 {
			t.Fatalf("expected calendar_seconds=180000, got %v", data["calendar_seconds"])
		}
		if int(data["idle_seconds"].(float64)) != 115200 {
			t.Fatalf("expected idle_seconds=115200, got %v", data["idle_seconds"])
		}

		// Effort aggregation across all 3 entries.
		if int(data["total_commits"].(float64)) != 3 {
			t.Fatalf("expected total_commits=3, got %v", data["total_commits"])
		}
		if int(data["lines_added"].(float64)) != 18 {
			t.Fatalf("expected lines_added=18, got %v", data["lines_added"])
		}
		if int(data["lines_removed"].(float64)) != 3 {
			t.Fatalf("expected lines_removed=3, got %v", data["lines_removed"])
		}
		// files_touched is deduplicated: a.go + b.go = 2.
		if int(data["files_touched"].(float64)) != 2 {
			t.Fatalf("expected files_touched=2, got %v", data["files_touched"])
		}
	})
}

func writeJournalForTest(t *testing.T, projectName string, entries []engine.CommitJournalEntry) {
	t.Helper()

	path, err := engine.JournalPath(projectName)
	if err != nil {
		t.Fatalf("journal path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir journal dir: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create journal: %v", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			t.Fatalf("encode journal entry: %v", err)
		}
	}
}
