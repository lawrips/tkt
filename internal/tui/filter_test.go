package tui

import (
	"testing"

	"github.com/lawrips/tkt/internal/ticket"
)

func makeRecord(id, title, desc, typ string, priority int, status string, tags []string, assignee string) ticket.Record {
	return ticket.Record{
		Front: ticket.Frontmatter{
			ID:       id,
			Type:     typ,
			Priority: priority,
			Status:   status,
			Tags:     tags,
			Assignee: assignee,
		},
		Body: ticket.Body{
			Title:       title,
			Description: desc,
		},
	}
}

func TestApplyFilterMatchesTicketID(t *testing.T) {
	records := []ticket.Record{
		makeRecord("tkv2-mcp-protocol", "MCP stdio transport", "", "feature", 1, "open", nil, ""),
		makeRecord("tkv2-watcher", "Watcher daemon", "", "feature", 1, "open", nil, ""),
		makeRecord("fix-123", "Fix login bug", "", "bug", 0, "open", nil, ""),
	}

	tests := []struct {
		name    string
		input   string
		wantIDs []string
	}{
		{
			name:    "search by full ticket ID",
			input:   "tkv2-mcp-protocol",
			wantIDs: []string{"tkv2-mcp-protocol"},
		},
		{
			name:    "search by partial ticket ID",
			input:   "tkv2-mcp",
			wantIDs: []string{"tkv2-mcp-protocol"},
		},
		{
			name:    "search by ID prefix matches multiple",
			input:   "tkv2",
			wantIDs: []string{"tkv2-mcp-protocol", "tkv2-watcher"},
		},
		{
			name:    "search by title still works",
			input:   "login",
			wantIDs: []string{"fix-123"},
		},
		{
			name:    "search by description still works",
			input:   "stdio",
			wantIDs: []string{"tkv2-mcp-protocol"},
		},
		{
			name:    "case insensitive ID search",
			input:   "TKV2-MCP",
			wantIDs: []string{"tkv2-mcp-protocol"},
		},
		{
			name:    "no match returns empty",
			input:   "nonexistent",
			wantIDs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseFilter(tt.input)
			got := applyFilter(records, f)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("expected %d results, got %d", len(tt.wantIDs), len(got))
			}
			for i, want := range tt.wantIDs {
				if got[i].Front.ID != want {
					t.Errorf("result[%d] = %s, want %s", i, got[i].Front.ID, want)
				}
			}
		})
	}
}

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Filter
	}{
		{
			name:  "plain text",
			input: "auth",
			want:  Filter{text: "auth", priority: -1},
		},
		{
			name:  "type prefix",
			input: "type:bug",
			want:  Filter{ticketType: "bug", priority: -1},
		},
		{
			name:  "priority prefix",
			input: "priority:1",
			want:  Filter{priority: 1},
		},
		{
			name:  "combined",
			input: "type:feature auth flow",
			want:  Filter{ticketType: "feature", text: "auth flow", priority: -1},
		},
		{
			name:  "empty",
			input: "",
			want:  Filter{priority: -1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFilter(tt.input)
			if got.text != tt.want.text {
				t.Errorf("text = %q, want %q", got.text, tt.want.text)
			}
			if got.ticketType != tt.want.ticketType {
				t.Errorf("ticketType = %q, want %q", got.ticketType, tt.want.ticketType)
			}
			if got.priority != tt.want.priority {
				t.Errorf("priority = %d, want %d", got.priority, tt.want.priority)
			}
			if got.assignee != tt.want.assignee {
				t.Errorf("assignee = %q, want %q", got.assignee, tt.want.assignee)
			}
			if got.tag != tt.want.tag {
				t.Errorf("tag = %q, want %q", got.tag, tt.want.tag)
			}
			if got.status != tt.want.status {
				t.Errorf("status = %q, want %q", got.status, tt.want.status)
			}
		})
	}
}

func TestApplyFilterStructuredFields(t *testing.T) {
	records := []ticket.Record{
		makeRecord("t-1", "Auth service", "handles login", "feature", 1, "open", []string{"backend"}, "alice"),
		makeRecord("t-2", "Fix crash", "null pointer", "bug", 0, "closed", []string{"urgent"}, "bob"),
		makeRecord("t-3", "Refactor DB", "clean up queries", "task", 2, "open", []string{"backend"}, "alice"),
	}

	tests := []struct {
		name    string
		input   string
		wantIDs []string
	}{
		{
			name:    "filter by type",
			input:   "type:bug",
			wantIDs: []string{"t-2"},
		},
		{
			name:    "filter by assignee",
			input:   "assignee:alice",
			wantIDs: []string{"t-1", "t-3"},
		},
		{
			name:    "filter by tag",
			input:   "tag:backend",
			wantIDs: []string{"t-1", "t-3"},
		},
		{
			name:    "filter by priority",
			input:   "priority:0",
			wantIDs: []string{"t-2"},
		},
		{
			name:    "filter by status",
			input:   "status:closed",
			wantIDs: []string{"t-2"},
		},
		{
			name:    "combined type and text",
			input:   "type:feature login",
			wantIDs: []string{"t-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := parseFilter(tt.input)
			got := applyFilter(records, f)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("expected %d results, got %d", len(tt.wantIDs), len(got))
			}
			for i, want := range tt.wantIDs {
				if got[i].Front.ID != want {
					t.Errorf("result[%d] = %s, want %s", i, got[i].Front.ID, want)
				}
			}
		})
	}
}
