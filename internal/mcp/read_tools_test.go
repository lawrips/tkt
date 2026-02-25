package mcp

import (
	stdctx "context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func TestHandleWorkflow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	workflowPath, err := project.WorkflowPath()
	if err != nil {
		t.Fatalf("workflow path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(workflowPath), 0755); err != nil {
		t.Fatalf("mkdir workflow dir: %v", err)
	}

	want := "# MCP Workflow\n"
	if err := os.WriteFile(workflowPath, []byte(want), 0644); err != nil {
		t.Fatalf("write workflow file: %v", err)
	}

	srv := NewServer("demo", t.TempDir())
	result, err := srv.handleWorkflow(stdctx.Background(), mcplib.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleWorkflow: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(result.Content))
	}

	text, ok := result.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("unmarshal workflow payload: %v text=%q", err, text.Text)
	}
	if payload["content"] != want {
		t.Fatalf("expected workflow content %q, got %#v", want, payload["content"])
	}
}

func TestHandleListReturnsSummariesAndShowRemainsDetailed(t *testing.T) {
	ticketDir := t.TempDir()
	seedMCPTestTicket(t, ticketDir, ticket.Record{
		ID:   "demo-1",
		Path: filepath.Join(ticketDir, "demo-1.md"),
		Front: ticket.Frontmatter{
			ID:       "demo-1",
			Status:   "open",
			Type:     "feature",
			Priority: 2,
			Created:  "2026-03-01T00:00:00Z",
			Deps:     []string{},
			Links:    []string{},
			Tags:     []string{},
		},
		Body: ticket.Body{
			Title:       "Summary only",
			Description: "This should only be returned by show",
		},
	})

	srv := NewServer("demo", ticketDir)

	listResult, err := srv.handleList(stdctx.Background(), mcplib.CallToolRequest{})
	listPayload := decodeMCPPayloadFromCall(t, listResult, err)
	items := listPayload["items"].([]any)
	item := items[0].(map[string]any)
	if _, ok := item["description"]; ok {
		t.Fatalf("list item should not include description: %#v", item)
	}
	if item["title"] != "Summary only" || item["status"] != "open" || item["type"] != "feature" || int(item["priority"].(float64)) != 2 {
		t.Fatalf("unexpected list summary item: %#v", item)
	}

	showResult, err := srv.handleShow(stdctx.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Arguments: map[string]any{"ticket_id": "demo-1"},
		},
	})
	showPayload := decodeMCPPayloadFromCall(t, showResult, err)
	if showPayload["description"] != "This should only be returned by show" {
		t.Fatalf("show payload should still include full description: %#v", showPayload)
	}
}

func TestDashboardEpicViewAndProgressReturnSummaries(t *testing.T) {
	ticketDir := t.TempDir()
	now := time.Now().UTC().Format(time.RFC3339)

	seedMCPTestTicket(t, ticketDir, ticket.Record{
		ID:   "epic-1",
		Path: filepath.Join(ticketDir, "epic-1.md"),
		Front: ticket.Frontmatter{
			ID:       "epic-1",
			Status:   "in_progress",
			Type:     "epic",
			Priority: 1,
			Created:  now,
			Deps:     []string{},
			Links:    []string{},
			Tags:     []string{},
		},
		Body: ticket.Body{
			Title:       "Epic",
			Description: "Detailed epic description",
		},
	})
	seedMCPTestTicket(t, ticketDir, ticket.Record{
		ID:   "child-1",
		Path: filepath.Join(ticketDir, "child-1.md"),
		Front: ticket.Frontmatter{
			ID:       "child-1",
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Created:  now,
			Parent:   "epic-1",
			Deps:     []string{},
			Links:    []string{},
			Tags:     []string{},
		},
		Body: ticket.Body{
			Title:       "Child",
			Description: "Detailed child description",
		},
	})
	seedMCPTestTicket(t, ticketDir, ticket.Record{
		ID:   "closed-1",
		Path: filepath.Join(ticketDir, "closed-1.md"),
		Front: ticket.Frontmatter{
			ID:       "closed-1",
			Status:   "closed",
			Type:     "bug",
			Priority: 3,
			Created:  now,
			Deps:     []string{},
			Links:    []string{},
			Tags:     []string{},
		},
		Body: ticket.Body{
			Title:       "Closed",
			Description: "Detailed closed description",
		},
	})

	srv := NewServer("demo", ticketDir)

	dashboardResult, err := srv.handleDashboard(stdctx.Background(), mcplib.CallToolRequest{})
	dashboardPayload := decodeMCPPayloadFromCall(t, dashboardResult, err)
	inProgress := dashboardPayload["in_progress"].([]any)
	if _, ok := inProgress[0].(map[string]any)["description"]; ok {
		t.Fatalf("dashboard item should not include description: %#v", inProgress[0])
	}

	epicResult, err := srv.handleEpicView(stdctx.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Arguments: map[string]any{"ticket_id": "epic-1"},
		},
	})
	epicPayload := decodeMCPPayloadFromCall(t, epicResult, err)
	if _, ok := epicPayload["epic"].(map[string]any)["description"]; ok {
		t.Fatalf("epic summary should not include description: %#v", epicPayload["epic"])
	}
	children := epicPayload["children"].([]any)
	if _, ok := children[0].(map[string]any)["description"]; ok {
		t.Fatalf("epic child summary should not include description: %#v", children[0])
	}

	progressResult, err := srv.handleProgress(stdctx.Background(), mcplib.CallToolRequest{})
	progressPayload := decodeMCPPayloadFromCall(t, progressResult, err)
	closed := progressPayload["closed"].([]any)
	if len(closed) == 0 {
		t.Fatalf("expected closed tickets in progress payload")
	}
	if _, ok := closed[0].(map[string]any)["description"]; ok {
		t.Fatalf("progress closed summary should not include description: %#v", closed[0])
	}
}

func decodeMCPPayloadFromCall(t *testing.T, result *mcplib.CallToolResult, err error) map[string]any {
	t.Helper()
	if err != nil {
		t.Fatalf("mcp handler error: %v", err)
	}
	return decodeMCPPayload(t, result)
}

func decodeMCPPayload(t *testing.T, result *mcplib.CallToolResult) map[string]any {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(result.Content))
	}
	text, ok := result.Content[0].(mcplib.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("unmarshal MCP payload: %v text=%q", err, text.Text)
	}
	return payload
}

func seedMCPTestTicket(t *testing.T, dir string, record ticket.Record) {
	t.Helper()
	if err := ticket.EnsureDir(dir); err != nil {
		t.Fatalf("ensure ticket dir: %v", err)
	}
	if record.Front.ID == "" {
		record.Front.ID = record.ID
	}
	if record.Front.Deps == nil {
		record.Front.Deps = []string{}
	}
	if record.Front.Links == nil {
		record.Front.Links = []string{}
	}
	if record.Front.Tags == nil {
		record.Front.Tags = []string{}
	}
	if err := ticket.SaveRecord(record); err != nil {
		t.Fatalf("save ticket %s: %v", record.ID, err)
	}
}
