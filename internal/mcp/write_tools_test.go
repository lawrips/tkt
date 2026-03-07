package mcp

import (
	stdctx "context"
	"path/filepath"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/lawrips/tkt/internal/ticket"
)

func TestToolSchemaIncludesAllFields(t *testing.T) {
	ticketDir := t.TempDir()
	srv := NewServer("demo", ticketDir)
	tools := srv.s.ListTools()

	tests := []struct {
		tool   string
		params []string
	}{
		{"create", []string{"title", "source", "description", "type", "priority", "assignee", "parent", "tags", "id", "design", "acceptance_criteria", "external_ref"}},
		{"edit", []string{"ticket_id", "source", "title", "description", "status", "type", "priority", "assignee", "parent", "tags", "design", "acceptance_criteria", "external_ref"}},
	}

	for _, tt := range tests {
		serverTool, exists := tools[tt.tool]
		if !exists {
			t.Fatalf("tool %q not registered", tt.tool)
		}
		props := serverTool.Tool.InputSchema.Properties
		for _, param := range tt.params {
			if _, ok := props[param]; !ok {
				t.Errorf("tool %q schema missing parameter %q", tt.tool, param)
			}
		}
	}
}

func TestCreateWithDesignAndAcceptanceCriteria(t *testing.T) {
	ticketDir := t.TempDir()
	srv := NewServer("demo", ticketDir)

	result, err := srv.handleCreate(stdctx.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Arguments: map[string]any{
				"source":              "test",
				"title":               "Test ticket",
				"design":              "The design section",
				"acceptance_criteria": "- AC one\n- AC two",
			},
		},
	})
	payload := decodeMCPPayloadFromCall(t, result, err)

	if payload["design"] != "The design section" {
		t.Fatalf("expected design %q, got %q", "The design section", payload["design"])
	}
	if payload["acceptance_criteria"] != "- AC one\n- AC two" {
		t.Fatalf("expected acceptance_criteria %q, got %q", "- AC one\n- AC two", payload["acceptance_criteria"])
	}
}

func TestCreateWithExternalRef(t *testing.T) {
	ticketDir := t.TempDir()
	srv := NewServer("demo", ticketDir)

	result, err := srv.handleCreate(stdctx.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Arguments: map[string]any{
				"source":       "test",
				"title":        "Test ticket",
				"external_ref": "GH-123",
			},
		},
	})
	payload := decodeMCPPayloadFromCall(t, result, err)

	if payload["external_ref"] != "GH-123" {
		t.Fatalf("expected external_ref %q, got %q", "GH-123", payload["external_ref"])
	}
}

func TestEditDesignAcceptanceCriteriaExternalRef(t *testing.T) {
	ticketDir := t.TempDir()
	seedMCPTestTicket(t, ticketDir, ticket.Record{
		ID:   "edit-1",
		Path: filepath.Join(ticketDir, "edit-1.md"),
		Front: ticket.Frontmatter{
			ID:       "edit-1",
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Created:  "2026-03-01T00:00:00Z",
			Deps:     []string{},
			Links:    []string{},
			Tags:     []string{},
		},
		Body: ticket.Body{
			Title:       "Original title",
			Description: "Original description",
		},
	})

	srv := NewServer("demo", ticketDir)

	result, err := srv.handleEdit(stdctx.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Arguments: map[string]any{
				"source":              "test",
				"ticket_id":          "edit-1",
				"design":              "New design",
				"acceptance_criteria": "- New AC",
				"external_ref":        "EXT-456",
			},
		},
	})
	payload := decodeMCPPayloadFromCall(t, result, err)

	if payload["design"] != "New design" {
		t.Fatalf("expected design %q, got %q", "New design", payload["design"])
	}
	if payload["acceptance_criteria"] != "- New AC" {
		t.Fatalf("expected acceptance_criteria %q, got %q", "- New AC", payload["acceptance_criteria"])
	}
	if payload["external_ref"] != "EXT-456" {
		t.Fatalf("expected external_ref %q, got %q", "EXT-456", payload["external_ref"])
	}
}

func TestEditEmptyStringClearsFields(t *testing.T) {
	ticketDir := t.TempDir()
	seedMCPTestTicket(t, ticketDir, ticket.Record{
		ID:   "clear-1",
		Path: filepath.Join(ticketDir, "clear-1.md"),
		Front: ticket.Frontmatter{
			ID:          "clear-1",
			Status:      "open",
			Type:        "task",
			Priority:    2,
			Created:     "2026-03-01T00:00:00Z",
			Assignee:    "alice",
			Parent:      "parent-1",
			ExternalRef: "REF-1",
			Deps:        []string{},
			Links:       []string{},
			Tags:        []string{"tag1"},
		},
		Body: ticket.Body{
			Title:              "Original title",
			Description:        "Original desc",
			Design:             "Original design",
			AcceptanceCriteria: "Original AC",
		},
	})

	srv := NewServer("demo", ticketDir)

	result, err := srv.handleEdit(stdctx.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Arguments: map[string]any{
				"source":              "test",
				"ticket_id":          "clear-1",
				"title":               "",
				"description":         "",
				"design":              "",
				"acceptance_criteria": "",
				"assignee":            "",
				"parent":              "",
				"tags":                "",
				"external_ref":        "",
			},
		},
	})
	payload := decodeMCPPayloadFromCall(t, result, err)

	if payload["title"] != "" {
		t.Fatalf("expected title cleared, got %q", payload["title"])
	}
	if payload["description"] != "" {
		t.Fatalf("expected description cleared, got %q", payload["description"])
	}
	if payload["design"] != "" {
		t.Fatalf("expected design cleared, got %q", payload["design"])
	}
	if payload["acceptance_criteria"] != "" {
		t.Fatalf("expected acceptance_criteria cleared, got %q", payload["acceptance_criteria"])
	}
	if payload["assignee"] != "" {
		t.Fatalf("expected assignee cleared, got %q", payload["assignee"])
	}
	if payload["parent"] != "" {
		t.Fatalf("expected parent cleared, got %q", payload["parent"])
	}
	if payload["external_ref"] != "" {
		t.Fatalf("expected external_ref cleared, got %q", payload["external_ref"])
	}
	tags, ok := payload["tags"].([]any)
	if !ok || len(tags) != 0 {
		t.Fatalf("expected tags cleared, got %v", payload["tags"])
	}
}

func TestEditOmittedFieldsUnchanged(t *testing.T) {
	ticketDir := t.TempDir()
	seedMCPTestTicket(t, ticketDir, ticket.Record{
		ID:   "keep-1",
		Path: filepath.Join(ticketDir, "keep-1.md"),
		Front: ticket.Frontmatter{
			ID:          "keep-1",
			Status:      "in_progress",
			Type:        "bug",
			Priority:    1,
			Created:     "2026-03-01T00:00:00Z",
			Assignee:    "bob",
			Parent:      "epic-1",
			ExternalRef: "GH-99",
			Deps:        []string{},
			Links:       []string{},
			Tags:        []string{"important"},
		},
		Body: ticket.Body{
			Title:              "Keep title",
			Description:        "Keep desc",
			Design:             "Keep design",
			AcceptanceCriteria: "Keep AC",
		},
	})

	srv := NewServer("demo", ticketDir)

	// Edit only priority — all other fields should remain unchanged.
	result, err := srv.handleEdit(stdctx.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Arguments: map[string]any{
				"source":    "test",
				"ticket_id": "keep-1",
				"priority":  float64(3),
			},
		},
	})
	payload := decodeMCPPayloadFromCall(t, result, err)

	if payload["title"] != "Keep title" {
		t.Fatalf("expected title unchanged, got %q", payload["title"])
	}
	if payload["description"] != "Keep desc" {
		t.Fatalf("expected description unchanged, got %q", payload["description"])
	}
	if payload["design"] != "Keep design" {
		t.Fatalf("expected design unchanged, got %q", payload["design"])
	}
	if payload["acceptance_criteria"] != "Keep AC" {
		t.Fatalf("expected acceptance_criteria unchanged, got %q", payload["acceptance_criteria"])
	}
	if payload["assignee"] != "bob" {
		t.Fatalf("expected assignee unchanged, got %q", payload["assignee"])
	}
	if payload["parent"] != "epic-1" {
		t.Fatalf("expected parent unchanged, got %q", payload["parent"])
	}
	if payload["external_ref"] != "GH-99" {
		t.Fatalf("expected external_ref unchanged, got %q", payload["external_ref"])
	}
	if payload["status"] != "in_progress" {
		t.Fatalf("expected status unchanged, got %q", payload["status"])
	}
	if payload["type"] != "bug" {
		t.Fatalf("expected type unchanged, got %q", payload["type"])
	}
	if int(payload["priority"].(float64)) != 3 {
		t.Fatalf("expected priority updated to 3, got %v", payload["priority"])
	}
}

func TestEditStatusAndTypeRequireNonEmpty(t *testing.T) {
	ticketDir := t.TempDir()
	seedMCPTestTicket(t, ticketDir, ticket.Record{
		ID:   "nonempty-1",
		Path: filepath.Join(ticketDir, "nonempty-1.md"),
		Front: ticket.Frontmatter{
			ID:       "nonempty-1",
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Created:  "2026-03-01T00:00:00Z",
			Deps:     []string{},
			Links:    []string{},
			Tags:     []string{},
		},
		Body: ticket.Body{
			Title: "Test ticket",
		},
	})

	srv := NewServer("demo", ticketDir)

	// Passing empty status and type should not clear them.
	result, err := srv.handleEdit(stdctx.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{
			Arguments: map[string]any{
				"source":    "test",
				"ticket_id": "nonempty-1",
				"status":    "",
				"type":      "",
			},
		},
	})
	payload := decodeMCPPayloadFromCall(t, result, err)

	if payload["status"] != "open" {
		t.Fatalf("expected status unchanged from 'open', got %q", payload["status"])
	}
	if payload["type"] != "task" {
		t.Fatalf("expected type unchanged from 'task', got %q", payload["type"])
	}
}
