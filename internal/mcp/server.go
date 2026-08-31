package mcp

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	resolver    projectResolver
}

type projectTarget struct {
	name      string
	ticketDir string
}

type projectResolver func(requested string, explicit bool) (projectTarget, error)

type projectHandler func(*Server, stdctx.Context, mcplib.CallToolRequest) (*mcplib.CallToolResult, error)

// NewServer creates and configures the MCP server with all tools registered.
func NewServer(projectName string, ticketDir string) *Server {
	s := &Server{
		projectName: projectName,
		ticketDir:   ticketDir,
		resolver:    registeredProjectResolver(projectName),
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

func projectOption() mcplib.ToolOption {
	return mcplib.WithString("project", mcplib.Description("Registered tkt project name. Omit to use the server's cwd-resolved default project."))
}

func (s *Server) withProject(handler projectHandler) server.ToolHandlerFunc {
	return func(ctx stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		requested, explicit, err := projectArgument(req)
		if err != nil {
			return errResult(err.Error())
		}
		target, err := s.resolver(requested, explicit)
		if err != nil {
			return errResult(err.Error())
		}

		// Handlers operate on a request-local copy. Never mutate the shared
		// server's project or store fields when selecting another project.
		scoped := *s
		scoped.projectName = target.name
		scoped.ticketDir = target.ticketDir
		return handler(&scoped, ctx, req)
	}
}

func projectArgument(req mcplib.CallToolRequest) (string, bool, error) {
	value, ok := req.GetArguments()["project"]
	if !ok {
		return "", false, nil
	}
	name, ok := value.(string)
	if !ok {
		return "", true, fmt.Errorf("project must be a string")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", true, fmt.Errorf("project must not be empty")
	}
	return name, true, nil
}

func registeredProjectResolver(defaultProject string) projectResolver {
	return func(requested string, explicit bool) (projectTarget, error) {
		name := defaultProject
		if explicit {
			name = requested
		}
		if err := validateProjectName(name); err != nil {
			return projectTarget{}, err
		}

		// Reload the catalog for every call so removing a registration revokes
		// access without restarting a long-lived MCP process.
		cfg, err := project.Load()
		if err != nil {
			return projectTarget{}, fmt.Errorf("registered project catalog is unavailable")
		}
		entry, ok := cfg.Projects[name]
		if !ok {
			return projectTarget{}, fmt.Errorf("project %q is not registered", name)
		}
		target, err := targetForRegisteredProject(name, entry)
		if err != nil {
			return projectTarget{}, fmt.Errorf("registered project %q is unavailable", name)
		}
		return target, nil
	}
}

func validateProjectName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("no default project is available")
	}
	if filepath.IsAbs(name) || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) || strings.ContainsRune(name, '\x00') {
		return fmt.Errorf("project must be a registered project name")
	}
	return nil
}

func targetForRegisteredProject(name string, entry project.ProjectConfig) (projectTarget, error) {
	store := strings.TrimSpace(entry.Store)
	if store == "" {
		store = "local"
	}

	switch store {
	case "central":
		root, err := engine.CentralStoreRoot()
		if err != nil {
			return projectTarget{}, err
		}
		dir := filepath.Join(root, name)
		if !pathWithinRoot(root, dir) {
			return projectTarget{}, fmt.Errorf("central project escapes store root")
		}
		return projectTarget{name: name, ticketDir: dir}, nil
	case "local":
		if strings.TrimSpace(entry.Path) == "" {
			return projectTarget{}, fmt.Errorf("local project has no registered path")
		}
		return projectTarget{name: name, ticketDir: filepath.Join(entry.Path, ticket.DefaultDir)}, nil
	default:
		return projectTarget{}, fmt.Errorf("unsupported project store")
	}
}

func pathWithinRoot(root string, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	if !relativePathWithinRoot(rootAbs, candidateAbs) {
		return false
	}

	// A missing project directory is safe to create after the lexical check.
	// If it exists, resolve symlinks and ensure the registered name does not
	// alias a directory outside the configured central store.
	candidateResolved, candidateErr := filepath.EvalSymlinks(candidateAbs)
	if candidateErr != nil {
		return os.IsNotExist(candidateErr)
	}
	rootResolved, rootErr := filepath.EvalSymlinks(rootAbs)
	if rootErr != nil {
		return false
	}
	return relativePathWithinRoot(rootResolved, candidateResolved)
}

func relativePathWithinRoot(root string, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
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
			projectOption(),
		),
		s.withProject((*Server).handleShow),
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
			projectOption(),
		),
		s.withProject((*Server).handleList),
	)

	// ready
	s.s.AddTool(
		mcplib.NewTool("ready",
			mcplib.WithDescription("Tickets with all dependencies resolved"),
			projectOption(),
		),
		s.withProject((*Server).handleReady),
	)

	// blocked
	s.s.AddTool(
		mcplib.NewTool("blocked",
			mcplib.WithDescription("Tickets with unresolved dependencies"),
			projectOption(),
		),
		s.withProject((*Server).handleBlocked),
	)

	// closed
	s.s.AddTool(
		mcplib.NewTool("closed",
			mcplib.WithDescription("Recently closed tickets"),
			mcplib.WithNumber("limit", mcplib.Description("Maximum number to return (default 20)")),
			mcplib.WithString("sort", mcplib.Description("Sort field: id, created, modified, priority, title. Append :desc for descending (e.g. modified:desc)")),
			projectOption(),
		),
		s.withProject((*Server).handleClosed),
	)

	// stats
	s.s.AddTool(
		mcplib.NewTool("stats",
			mcplib.WithDescription("Project health summary counts"),
			projectOption(),
		),
		s.withProject((*Server).handleStats),
	)

	// timeline
	s.s.AddTool(
		mcplib.NewTool("timeline",
			mcplib.WithDescription("Closed tickets grouped by week"),
			mcplib.WithNumber("weeks", mcplib.Description("Number of weeks to show (default 4)")),
			projectOption(),
		),
		s.withProject((*Server).handleTimeline),
	)

	// workflow
	s.s.AddTool(
		mcplib.NewTool("workflow",
			mcplib.WithDescription("Read the user's tkt workflow guide"),
			projectOption(),
		),
		s.withProject((*Server).handleWorkflow),
	)

	// dep_tree
	s.s.AddTool(
		mcplib.NewTool("dep_tree",
			mcplib.WithDescription("Show dependency tree for a ticket"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Root ticket ID")),
			mcplib.WithBoolean("full", mcplib.Description("Include closed dependencies")),
			projectOption(),
		),
		s.withProject((*Server).handleDepTree),
	)

	// epic_view
	s.s.AddTool(
		mcplib.NewTool("epic_view",
			mcplib.WithDescription("Precomputed epic hierarchy with children and commits"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Epic ticket ID")),
			projectOption(),
		),
		s.withProject((*Server).handleEpicView),
	)

	// dashboard
	s.s.AddTool(
		mcplib.NewTool("dashboard",
			mcplib.WithDescription("Project-level summary: in progress, blocked, ready, recent commits"),
			projectOption(),
		),
		s.withProject((*Server).handleDashboard),
	)

	// progress
	s.s.AddTool(
		mcplib.NewTool("progress",
			mcplib.WithDescription("Recent progress: closed tickets and commit links in a time window"),
			mcplib.WithString("window", mcplib.Description("Time window: today or week (default week)")),
			projectOption(),
		),
		s.withProject((*Server).handleProgress),
	)

	// lifecycle
	s.s.AddTool(
		mcplib.NewTool("lifecycle",
			mcplib.WithDescription("Lifecycle data for a ticket: status history and duration"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
			projectOption(),
		),
		s.withProject((*Server).handleLifecycle),
	)

	// context
	s.s.AddTool(
		mcplib.NewTool("context",
			mcplib.WithDescription("Composite view: ticket + parent + deps status + linked tickets + children + recent commits"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
			projectOption(),
		),
		s.withProject((*Server).handleContext),
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
			mcplib.WithString("design", mcplib.Description("Design section content")),
			mcplib.WithString("acceptance_criteria", mcplib.Description("Acceptance criteria content")),
			mcplib.WithString("external_ref", mcplib.Description("External reference")),
			projectOption(),
		),
		s.withProject((*Server).handleCreate),
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
			mcplib.WithString("design", mcplib.Description("New design content (empty string clears design)")),
			mcplib.WithString("acceptance_criteria", mcplib.Description("New acceptance criteria (empty string clears acceptance_criteria)")),
			mcplib.WithString("external_ref", mcplib.Description("New external reference (empty string clears external_ref)")),
			projectOption(),
		),
		s.withProject((*Server).handleEdit),
	)

	// add_note
	s.s.AddTool(
		mcplib.NewTool("add_note",
			mcplib.WithDescription("Append a timestamped note to a ticket"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
			mcplib.WithString("text", mcplib.Required(), mcplib.Description("Note text")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
			projectOption(),
		),
		s.withProject((*Server).handleAddNote),
	)

	// delete
	s.s.AddTool(
		mcplib.NewTool("delete",
			mcplib.WithDescription("Delete one or more tickets"),
			mcplib.WithArray("ticket_ids", mcplib.Required(), mcplib.Description("List of ticket IDs to delete")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
			projectOption(),
		),
		s.withProject((*Server).handleDelete),
	)

	// dep
	s.s.AddTool(
		mcplib.NewTool("dep",
			mcplib.WithDescription("Add a dependency edge"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
			mcplib.WithString("dep_id", mcplib.Required(), mcplib.Description("Dependency ticket ID")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
			projectOption(),
		),
		s.withProject((*Server).handleDep),
	)

	// undep
	s.s.AddTool(
		mcplib.NewTool("undep",
			mcplib.WithDescription("Remove a dependency edge"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Ticket ID")),
			mcplib.WithString("dep_id", mcplib.Required(), mcplib.Description("Dependency ticket ID to remove")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
			projectOption(),
		),
		s.withProject((*Server).handleUndep),
	)

	// link
	s.s.AddTool(
		mcplib.NewTool("link",
			mcplib.WithDescription("Create symmetric links between tickets"),
			mcplib.WithArray("ticket_ids", mcplib.Required(), mcplib.Description("List of ticket IDs to link (first is source, rest are targets)")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
			projectOption(),
		),
		s.withProject((*Server).handleLink),
	)

	// unlink
	s.s.AddTool(
		mcplib.NewTool("unlink",
			mcplib.WithDescription("Remove a symmetric link between tickets"),
			mcplib.WithString("ticket_id", mcplib.Required(), mcplib.Description("Source ticket ID")),
			mcplib.WithString("target_id", mcplib.Required(), mcplib.Description("Target ticket ID to unlink")),
			mcplib.WithString("source", mcplib.Required(), mcplib.Description("Caller identity for attribution")),
			projectOption(),
		),
		s.withProject((*Server).handleUnlink),
	)
}

// resultJSON returns a CallToolResult with JSON-encoded data and the resolved
// project while preserving each tool's existing top-level result shape.
func (s *Server) resultJSON(data map[string]any) (*mcplib.CallToolResult, error) {
	payload := make(map[string]any, len(data)+1)
	for key, value := range data {
		payload[key] = value
	}
	payload["resolved_project"] = s.projectName
	raw, err := json.Marshal(payload)
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

// resolveProjectFromCwd resolves the project name from the current working directory.
func resolveProjectFromCwd() (string, string, error) {
	cfg, err := project.Load()
	if err != nil {
		return "", "", err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", err
	}
	name, _ := project.ResolveName(cfg, cwd, "")
	if name == "" {
		return "", "", fmt.Errorf("no project resolved; run tkt init first")
	}
	entry, ok := cfg.Projects[name]
	if !ok {
		return "", "", fmt.Errorf("project %q not found in config", name)
	}
	if err := validateProjectName(name); err != nil {
		return "", "", err
	}
	target, err := targetForRegisteredProject(name, entry)
	if err != nil {
		return "", "", fmt.Errorf("project %q is unavailable", name)
	}
	return target.name, target.ticketDir, nil
}

// NewServerFromCwd creates an MCP server by resolving the project from the cwd.
func NewServerFromCwd() (*Server, error) {
	name, dir, err := resolveProjectFromCwd()
	if err != nil {
		return nil, err
	}
	return NewServer(name, dir), nil
}
