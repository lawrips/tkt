package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

// Server wraps the mcp-go server and holds shared state.
type Server struct {
	s           *server.MCPServer
	projectName string
	ticketDir   string
	projectRoot string // git root or cwd; used for project-level config lookups
}

// NewServer creates and configures the MCP server with all tools registered.
func NewServer(projectName string, ticketDir string, projectRoot string) *Server {
	s := &Server{
		projectName: projectName,
		ticketDir:   ticketDir,
		projectRoot: projectRoot,
	}
	s.s = server.NewMCPServer(
		"tkt",
		"v2",
		server.WithToolCapabilities(false),
	)
	s.registerReadTools()
	s.registerWriteTools()
	return s
}

// ServeStdio starts the stdio JSON-RPC transport (blocks until stdin closes).
func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.s)
}

func (s *Server) registerReadTools() {
	// show
	s.s.AddTool(
		mcplib.NewTool("show",
			mcplib.WithDescription("Display ticket details"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
		),
		s.handleShow,
	)

	// list
	s.s.AddTool(
		mcplib.NewTool("list",
			mcplib.WithDescription("List tickets with optional filters. Returns only open tickets when no status filter is provided."),
			mcplib.WithString("status", mcplib.Description("Filter by status: open, in_progress, needs_testing, closed")),
			mcplib.WithString("type", mcplib.Description("Filter by type: bug, feature, task, epic, chore")),
			mcplib.WithNumber("priority", mcplib.Description("Filter by priority (0-4)")),
			mcplib.WithString("assignee", mcplib.Description("Filter by assignee")),
			mcplib.WithString("tag", mcplib.Description("Filter by tag")),
			mcplib.WithString("parent", mcplib.Description("Filter by parent ticket ID")),
			mcplib.WithString("search", mcplib.Description("Text search on ticket ID and title (case-insensitive substring match)")),
			mcplib.WithString("sort", mcplib.Description("Sort field: id, created, modified, priority, title. Append :desc for descending (e.g. created:desc)")),
			mcplib.WithNumber("limit", mcplib.Description("Maximum number of results to return")),
		),
		s.handleList,
	)

	// ready
	s.s.AddTool(
		mcplib.NewTool("ready",
			mcplib.WithDescription("Tickets with all dependencies resolved"),
		),
		s.handleReady,
	)

	// blocked
	s.s.AddTool(
		mcplib.NewTool("blocked",
			mcplib.WithDescription("Tickets with unresolved dependencies"),
		),
		s.handleBlocked,
	)

	// closed
	s.s.AddTool(
		mcplib.NewTool("closed",
			mcplib.WithDescription("Recently closed tickets"),
			mcplib.WithNumber("limit", mcplib.Description("Maximum number to return (default 20)")),
			mcplib.WithString("sort", mcplib.Description("Sort field: id, created, modified, priority, title. Append :desc for descending (e.g. modified:desc)")),
		),
		s.handleClosed,
	)

	// stats
	s.s.AddTool(
		mcplib.NewTool("stats",
			mcplib.WithDescription("Project health summary counts"),
		),
		s.handleStats,
	)

	// timeline
	s.s.AddTool(
		mcplib.NewTool("timeline",
			mcplib.WithDescription("Closed tickets grouped by week"),
			mcplib.WithNumber("weeks", mcplib.Description("Number of weeks to show (default 4)")),
		),
		s.handleTimeline,
	)

	// workflow
	s.s.AddTool(
		mcplib.NewTool("workflow",
			mcplib.WithDescription("Read the user's tkt workflow guide"),
		),
		s.handleWorkflow,
	)

	// dep_tree
	s.s.AddTool(
		mcplib.NewTool("dep_tree",
			mcplib.WithDescription("Show dependency tree for a ticket"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Root ticket ID")),
			mcplib.WithBoolean("full", mcplib.Description("Include closed dependencies")),
		),
		s.handleDepTree,
	)

	// epic_view
	s.s.AddTool(
		mcplib.NewTool("epic_view",
			mcplib.WithDescription("Precomputed epic hierarchy with children and commits"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Epic ticket ID")),
		),
		s.handleEpicView,
	)

	// dashboard
	s.s.AddTool(
		mcplib.NewTool("dashboard",
			mcplib.WithDescription("Project-level summary: in progress, blocked, ready, recent commits"),
		),
		s.handleDashboard,
	)

	// progress
	s.s.AddTool(
		mcplib.NewTool("progress",
			mcplib.WithDescription("Recent progress: closed tickets and commit links in a time window"),
			mcplib.WithString("window", mcplib.Description("Time window: today or week (default week)")),
		),
		s.handleProgress,
	)

	// lifecycle
	s.s.AddTool(
		mcplib.NewTool("lifecycle",
			mcplib.WithDescription("Lifecycle data for a ticket: status history and duration"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
		),
		s.handleLifecycle,
	)

	// context
	s.s.AddTool(
		mcplib.NewTool("context",
			mcplib.WithDescription("Composite view: ticket + parent + deps status + linked tickets + children + recent commits"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
		),
		s.handleContext,
	)
}

func (s *Server) registerWriteTools() {
	// create
	s.s.AddTool(
		mcplib.NewTool("create",
			mcplib.WithDescription("Create a new ticket"),
			mcplib.WithString("title", mcplib.Required(), mcplib.Description("Ticket title")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
			mcplib.WithString("description", mcplib.Description("Ticket description")),
			mcplib.WithString("type", mcplib.Description("Ticket type: bug, feature, task, epic, chore")),
			mcplib.WithNumber("priority", mcplib.Description("Priority 0-4")),
			mcplib.WithString("assignee", mcplib.Description("Assignee")),
			mcplib.WithString("parent", mcplib.Description("Parent ticket ID")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
			mcplib.WithString("id", mcplib.Description("Custom ticket ID")),
		),
		s.handleCreate,
	)

	// edit
	s.s.AddTool(
		mcplib.NewTool("edit",
			mcplib.WithDescription("Update ticket fields"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
			mcplib.WithString("title", mcplib.Description("New title")),
			mcplib.WithString("description", mcplib.Description("New description")),
			mcplib.WithString("status", mcplib.Description("New status")),
			mcplib.WithString("type", mcplib.Description("New type")),
			mcplib.WithNumber("priority", mcplib.Description("New priority")),
			mcplib.WithString("assignee", mcplib.Description("New assignee")),
			mcplib.WithString("parent", mcplib.Description("New parent ticket ID (empty string clears parent)")),
			mcplib.WithString("tags", mcplib.Description("Comma-separated tags")),
		),
		s.handleEdit,
	)

	// add_note
	s.s.AddTool(
		mcplib.NewTool("add_note",
			mcplib.WithDescription("Append a timestamped note to a ticket"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
			mcplib.WithString("text", mcplib.Required(), mcplib.Description("Note text")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
		),
		s.handleAddNote,
	)

	// delete
	s.s.AddTool(
		mcplib.NewTool("delete",
			mcplib.WithDescription("Delete one or more tickets"),
			mcplib.WithArray("ticket_ids", mcplib.Required(), mcplib.Description("List of ticket IDs to delete")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
		),
		s.handleDelete,
	)

	// dep
	s.s.AddTool(
		mcplib.NewTool("dep",
			mcplib.WithDescription("Add a dependency edge"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
			mcplib.WithString("dep_id", mcplib.Required(), mcplib.Description("Dependency ticket ID")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
		),
		s.handleDep,
	)

	// undep
	s.s.AddTool(
		mcplib.NewTool("undep",
			mcplib.WithDescription("Remove a dependency edge"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
			mcplib.WithString("dep_id", mcplib.Required(), mcplib.Description("Dependency ticket ID to remove")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
		),
		s.handleUndep,
	)

	// link
	s.s.AddTool(
		mcplib.NewTool("link",
			mcplib.WithDescription("Create symmetric links between tickets"),
			mcplib.WithArray("ticket_ids", mcplib.Required(), mcplib.Description("List of ticket IDs to link (first is source, rest are targets)")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
		),
		s.handleLink,
	)

	// unlink
	s.s.AddTool(
		mcplib.NewTool("unlink",
			mcplib.WithDescription("Remove a symmetric link between tickets"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Source ticket ID")),
			mcplib.WithString("target_id", mcplib.Required(), mcplib.Description("Target ticket ID to unlink")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
		),
		s.handleUnlink,
	)
}

// resultJSON returns a CallToolResult with JSON-encoded data.
func resultJSON(data any) (*mcplib.CallToolResult, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return mcplib.NewToolResultText(string(raw)), nil
}

func errResult(msg string) (*mcplib.CallToolResult, error) {
	return mcplib.NewToolResultError(msg), nil
}

// loadTickets reads all tickets from disk.
func (s *Server) loadTickets() ([]ticket.Record, error) {
	return ticket.List(s.ticketDir)
}

// loadByID resolves a ticket by full or partial ID.
func (s *Server) loadByID(id string) (ticket.Record, error) {
	return ticket.LoadByID(s.ticketDir, id)
}

// loadJournal reads commit journal entries for the project.
func (s *Server) loadJournal() []engine.CommitJournalEntry {
	entries, _ := engine.ReadJournalEntries(s.projectName)
	return entries
}

// resolveProjectFromCwd resolves the project name, ticket dir, and project root from the cwd.
func resolveProjectFromCwd() (name, ticketDir, projectRoot string, err error) {
	cfg, err := project.Load()
	if err != nil {
		return "", "", "", err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", "", err
	}
	name, _ = project.ResolveName(cfg, cwd, "")
	if name == "" {
		return "", "", "", fmt.Errorf("no project resolved; run tkt init first")
	}
	entry, ok := cfg.Projects[name]
	if !ok {
		return "", "", "", fmt.Errorf("project %q not found in config", name)
	}
	dir := ""
	if entry.Store == "central" {
		d, err := engine.CentralProjectDir(name)
		if err != nil {
			return "", "", "", err
		}
		dir = d
	} else if entry.Path != "" {
		dir = filepath.Join(entry.Path, ".tickets")
	}
	root := project.DetectProjectPath(cwd)
	return name, dir, root, nil
}

// NewServerFromCwd creates an MCP server by resolving the project from the cwd.
func NewServerFromCwd() (*Server, error) {
	name, dir, root, err := resolveProjectFromCwd()
	if err != nil {
		return nil, err
	}
	return NewServer(name, dir, root), nil
}
