package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func TestDepUndepLinkUnlink(t *testing.T) {
	withWorkspace(t, func(_ string) {
		seedTicket(t, "a-1", ticket.Frontmatter{Status: "open", Type: "task", Priority: 1}, ticket.Body{Title: "A"})
		seedTicket(t, "b-1", ticket.Frontmatter{Status: "open", Type: "task", Priority: 1}, ticket.Body{Title: "B"})
		seedTicket(t, "c-1", ticket.Frontmatter{Status: "open", Type: "task", Priority: 1}, ticket.Body{Title: "C"})

		if _, _, err := runCmd(t, "", "dep", "a-1", "b-1"); err != nil {
			t.Fatalf("dep: %v", err)
		}
		a, err := ticket.LoadByID(ticket.DefaultDir, "a-1")
		if err != nil {
			t.Fatalf("load a-1: %v", err)
		}
		if !engine.Contains(a.Front.Deps, "b-1") {
			t.Fatalf("expected dep b-1 on a-1, deps=%v", a.Front.Deps)
		}

		if _, _, err := runCmd(t, "", "undep", "a-1", "b-1"); err != nil {
			t.Fatalf("undep: %v", err)
		}
		a, err = ticket.LoadByID(ticket.DefaultDir, "a-1")
		if err != nil {
			t.Fatalf("reload a-1: %v", err)
		}
		if engine.Contains(a.Front.Deps, "b-1") {
			t.Fatalf("expected b-1 removed from deps, deps=%v", a.Front.Deps)
		}

		if _, _, err := runCmd(t, "", "link", "a-1", "b-1", "c-1"); err != nil {
			t.Fatalf("link: %v", err)
		}
		a, _ = ticket.LoadByID(ticket.DefaultDir, "a-1")
		b, _ := ticket.LoadByID(ticket.DefaultDir, "b-1")
		if !engine.Contains(a.Front.Links, "b-1") || !engine.Contains(a.Front.Links, "c-1") || !engine.Contains(b.Front.Links, "a-1") {
			t.Fatalf("unexpected links after link: a=%v b=%v", a.Front.Links, b.Front.Links)
		}

		if _, _, err := runCmd(t, "", "unlink", "a-1", "b-1"); err != nil {
			t.Fatalf("unlink: %v", err)
		}
		a, _ = ticket.LoadByID(ticket.DefaultDir, "a-1")
		b, _ = ticket.LoadByID(ticket.DefaultDir, "b-1")
		if engine.Contains(a.Front.Links, "b-1") || engine.Contains(b.Front.Links, "a-1") {
			t.Fatalf("unexpected links after unlink: a=%v b=%v", a.Front.Links, b.Front.Links)
		}
	})
}

func TestDepTreeAndCycle(t *testing.T) {
	withWorkspace(t, func(_ string) {
		seedTicket(t, "a-1", ticket.Frontmatter{Status: "open", Type: "task", Priority: 1, Deps: []string{"b-1"}}, ticket.Body{Title: "A"})
		seedTicket(t, "b-1", ticket.Frontmatter{Status: "open", Type: "task", Priority: 1, Deps: []string{"c-1"}}, ticket.Body{Title: "B"})
		seedTicket(t, "c-1", ticket.Frontmatter{Status: "open", Type: "task", Priority: 1, Deps: []string{"a-1"}}, ticket.Body{Title: "C"})

		out, _, err := runCmd(t, "", "dep", "tree", "a-1")
		if err != nil {
			t.Fatalf("dep tree: %v", err)
		}
		if !strings.Contains(out, "a-1") || !strings.Contains(out, "b-1") || !strings.Contains(out, "(cycle)") {
			t.Fatalf("unexpected dep tree output: %q", out)
		}

		out, _, err = runCmd(t, "", "dep", "cycle")
		if err != nil {
			t.Fatalf("dep cycle: %v", err)
		}
		if !strings.Contains(out, "a-1 -> b-1 -> c-1 -> a-1") {
			t.Fatalf("unexpected cycle output: %q", out)
		}
	})
}

func TestQueryStatsTimelineWorkflow(t *testing.T) {
	withWorkspace(t, func(_ string) {
		seedTicket(t, "q-1", ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Created:  "2026-02-20T12:00:00Z",
			Assignee: "lawrence",
		}, ticket.Body{
			Title:       "Query ticket",
			Description: "desc",
		})
		seedTicket(t, "q-2", ticket.Frontmatter{
			Status:   "closed",
			Type:     "bug",
			Priority: 0,
			Created:  "2026-02-21T12:00:00Z",
		}, ticket.Body{
			Title: "Closed ticket",
		})

		out, _, err := runCmd(t, "", "query")
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 JSONL rows, got %d output=%q", len(lines), out)
		}
		for _, line := range lines {
			var payload map[string]any
			if err := json.Unmarshal([]byte(line), &payload); err != nil {
				t.Fatalf("invalid json line %q: %v", line, err)
			}
			if payload["id"] == nil || payload["title"] == nil || payload["status"] == nil {
				t.Fatalf("missing query fields in payload: %v", payload)
			}
		}

		out, _, err = runCmd(t, "", "query", "--json")
		if err != nil {
			t.Fatalf("query --json: %v", err)
		}
		var queryEnvelope map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &queryEnvelope); err != nil {
			t.Fatalf("invalid query --json envelope: %v output=%q", err, out)
		}
		if queryEnvelope["meta"].(map[string]any)["command"] != "query" {
			t.Fatalf("unexpected query --json command: %v", queryEnvelope["meta"])
		}

		if _, err := exec.LookPath("jq"); err == nil {
			out, _, err = runCmd(t, "", "query", ".status == \"closed\"")
			if err != nil {
				t.Fatalf("query filter: %v", err)
			}
			if !strings.Contains(out, "\"id\":\"q-2\"") || strings.Contains(out, "\"id\":\"q-1\"") {
				t.Fatalf("unexpected filtered query output: %q", out)
			}
		}

		out, _, err = runCmd(t, "", "stats")
		if err != nil {
			t.Fatalf("stats: %v", err)
		}
		if !strings.Contains(out, "Total tickets: 2") || !strings.Contains(out, "open: 1") || !strings.Contains(out, "closed: 1") {
			t.Fatalf("unexpected stats output: %q", out)
		}

		out, _, err = runCmd(t, "", "stats", "--json")
		if err != nil {
			t.Fatalf("stats --json: %v", err)
		}
		var statsEnvelope map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &statsEnvelope); err != nil {
			t.Fatalf("invalid stats --json envelope: %v output=%q", err, out)
		}
		if statsEnvelope["meta"].(map[string]any)["command"] != "stats" {
			t.Fatalf("unexpected stats --json command: %v", statsEnvelope["meta"])
		}

		out, _, err = runCmd(t, "", "timeline", "--weeks=2")
		if err != nil {
			t.Fatalf("timeline: %v", err)
		}
		if len(strings.Split(strings.TrimSpace(out), "\n")) != 2 {
			t.Fatalf("unexpected timeline rows: %q", out)
		}

		out, _, err = runCmd(t, "", "workflow")
		if err != nil {
			t.Fatalf("workflow: %v", err)
		}
		if !strings.Contains(out, "# tkt Workflow") || !strings.Contains(out, "embedded default") {
			t.Fatalf("unexpected workflow output: %q", out)
		}

		out, _, err = runCmd(t, "", "workflow", "--json")
		if err != nil {
			t.Fatalf("workflow --json: %v", err)
		}
		var workflowEnvelope map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &workflowEnvelope); err != nil {
			t.Fatalf("invalid workflow --json envelope: %v output=%q", err, out)
		}
		if workflowEnvelope["meta"].(map[string]any)["command"] != "workflow" {
			t.Fatalf("unexpected workflow --json command: %v", workflowEnvelope["meta"])
		}
		content := workflowEnvelope["data"].(map[string]any)["content"].(string)
		if !strings.Contains(content, "Closes: [my-ticket-id]") {
			t.Fatalf("unexpected workflow --json content: %q", content)
		}
	})
}

func TestWorkflowUsesUserFile(t *testing.T) {
	withWorkspace(t, func(_ string) {
		workflowPath, err := project.WorkflowPath()
		if err != nil {
			t.Fatalf("workflow path: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(workflowPath), 0755); err != nil {
			t.Fatalf("mkdir workflow dir: %v", err)
		}

		custom := "# Team Workflow\n\nUse review gates.\n"
		if err := os.WriteFile(workflowPath, []byte(custom), 0644); err != nil {
			t.Fatalf("write workflow file: %v", err)
		}

		out, _, err := runCmd(t, "", "workflow")
		if err != nil {
			t.Fatalf("workflow: %v", err)
		}
		if !strings.Contains(out, custom) {
			t.Fatalf("expected custom workflow content, got %q", out)
		}
		if !strings.Contains(out, "(Source: ~/.tkt/workflow.md - edit to customize)") {
			t.Fatalf("expected workflow source hint, got %q", out)
		}

		out, _, err = runCmd(t, "", "workflow", "--json")
		if err != nil {
			t.Fatalf("workflow --json: %v", err)
		}
		var workflowEnvelope map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &workflowEnvelope); err != nil {
			t.Fatalf("invalid workflow --json envelope: %v output=%q", err, out)
		}
		content := workflowEnvelope["data"].(map[string]any)["content"].(string)
		if content != custom {
			t.Fatalf("expected exact custom workflow content, got %q", content)
		}
	})
}

func TestWorkflowFallsBackWhenFileMissing(t *testing.T) {
	withWorkspace(t, func(_ string) {
		workflowPath, err := project.WorkflowPath()
		if err != nil {
			t.Fatalf("workflow path: %v", err)
		}
		if err := os.Remove(workflowPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove workflow file: %v", err)
		}

		out, _, err := runCmd(t, "", "workflow")
		if err != nil {
			t.Fatalf("workflow: %v", err)
		}
		if !strings.Contains(out, "# tkt Workflow") {
			t.Fatalf("expected embedded default workflow content, got %q", out)
		}
		if !strings.Contains(out, "Source: embedded default") {
			t.Fatalf("expected embedded fallback source hint, got %q", out)
		}
	})
}

func TestWorkflowProjectOverridesGlobal(t *testing.T) {
	withWorkspace(t, func(dir string) {
		// Write a global workflow.
		globalPath, err := project.WorkflowPath()
		if err != nil {
			t.Fatalf("workflow path: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(globalPath), 0755); err != nil {
			t.Fatalf("mkdir global workflow dir: %v", err)
		}
		if err := os.WriteFile(globalPath, []byte("# Global Workflow\n"), 0644); err != nil {
			t.Fatalf("write global workflow: %v", err)
		}

		// Write a project-level workflow that should take precedence.
		projDir := filepath.Join(dir, ".tkt")
		if err := os.MkdirAll(projDir, 0755); err != nil {
			t.Fatalf("mkdir project .tkt dir: %v", err)
		}
		projWorkflow := "# Project Workflow\n\nProject-specific rules.\n"
		if err := os.WriteFile(filepath.Join(projDir, "workflow.md"), []byte(projWorkflow), 0644); err != nil {
			t.Fatalf("write project workflow: %v", err)
		}

		out, _, err := runCmd(t, "", "workflow")
		if err != nil {
			t.Fatalf("workflow: %v", err)
		}
		if !strings.Contains(out, "# Project Workflow") {
			t.Fatalf("expected project workflow content, got %q", out)
		}
		if strings.Contains(out, "# Global Workflow") {
			t.Fatalf("global workflow should not appear when project override exists, got %q", out)
		}
		if !strings.Contains(out, ".tkt/workflow.md") {
			t.Fatalf("expected source to reference .tkt/workflow.md, got %q", out)
		}

		// JSON output should also use project workflow.
		out, _, err = runCmd(t, "", "workflow", "--json")
		if err != nil {
			t.Fatalf("workflow --json: %v", err)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &envelope); err != nil {
			t.Fatalf("invalid workflow --json: %v output=%q", err, out)
		}
		content := envelope["data"].(map[string]any)["content"].(string)
		if content != projWorkflow {
			t.Fatalf("expected project workflow in JSON, got %q", content)
		}
	})
}

func TestReadyJSONReturnsSummaries(t *testing.T) {
	withWorkspace(t, func(_ string) {
		seedTicket(t, "r-1", ticket.Frontmatter{
			Status:   "open",
			Type:     "feature",
			Priority: 2,
			Created:  "2026-02-20T12:00:00Z",
		}, ticket.Body{
			Title:       "Ready ticket",
			Description: "Should not be in ready JSON",
		})

		out, _, err := runCmd(t, "", "ready", "--json")
		if err != nil {
			t.Fatalf("ready --json: %v", err)
		}
		var envelope map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &envelope); err != nil {
			t.Fatalf("invalid ready --json envelope: %v output=%q", err, out)
		}
		items := envelope["data"].(map[string]any)["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("expected 1 ready item, got %d", len(items))
		}
		item := items[0].(map[string]any)
		if _, ok := item["description"]; ok {
			t.Fatalf("ready summary item should not include description: %#v", item)
		}
		if item["id"] != "r-1" || item["title"] != "Ready ticket" || item["status"] != "open" || item["type"] != "feature" {
			t.Fatalf("unexpected ready summary item: %#v", item)
		}
	})
}

func TestAddNoteFromStdin(t *testing.T) {
	withWorkspace(t, func(_ string) {
		seedTicket(t, "n-1", ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
		}, ticket.Body{Title: "Note target"})

		if _, _, err := runCmd(t, "hello from stdin\n", "add-note", "n-1"); err != nil {
			t.Fatalf("add-note stdin: %v", err)
		}

		out, _, err := runCmd(t, "", "show", "n-1")
		if err != nil {
			t.Fatalf("show n-1: %v", err)
		}
		if !strings.Contains(out, "## Notes") || !strings.Contains(out, "hello from stdin") {
			t.Fatalf("expected notes section after add-note stdin: %q", out)
		}
	})
}
