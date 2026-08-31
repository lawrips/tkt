package mcp

import (
	"bufio"
	stdctx "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func TestMCPProjectSelectionDefaultsAndSwitchesPerCall(t *testing.T) {
	srv, alphaDir, betaDir := setupCrossProjectServer(t)
	seedProjectTicket(t, alphaDir, "shared-ticket", "Alpha ticket")
	seedProjectTicket(t, betaDir, "shared-ticket", "Beta ticket")
	localRoot := t.TempDir()
	localDir := filepath.Join(localRoot, ticket.DefaultDir)
	seedProjectTicket(t, localDir, "shared-ticket", "Local ticket")
	cfg, err := project.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Projects["local-project"] = project.ProjectConfig{Store: "local", Path: localRoot}
	if err := project.Save(cfg); err != nil {
		t.Fatalf("register local project: %v", err)
	}

	defaultResult, err := invokeTool(srv, "show", map[string]any{"ticket_id": "shared-ticket"})
	defaultPayload := requireSuccessfulPayload(t, defaultResult, err)
	if defaultPayload["title"] != "Alpha ticket" || defaultPayload["resolved_project"] != "alpha" {
		t.Fatalf("unexpected default-project payload: %#v", defaultPayload)
	}

	betaResult, err := invokeTool(srv, "show", map[string]any{
		"project":   "beta",
		"ticket_id": "shared-ticket",
	})
	betaPayload := requireSuccessfulPayload(t, betaResult, err)
	if betaPayload["title"] != "Beta ticket" || betaPayload["resolved_project"] != "beta" {
		t.Fatalf("unexpected selected-project payload: %#v", betaPayload)
	}

	localResult, err := invokeTool(srv, "show", map[string]any{
		"project":   "local-project",
		"ticket_id": "shared-ticket",
	})
	localPayload := requireSuccessfulPayload(t, localResult, err)
	if localPayload["title"] != "Local ticket" || localPayload["resolved_project"] != "local-project" {
		t.Fatalf("unexpected local-project payload: %#v", localPayload)
	}

	defaultAgain, err := invokeTool(srv, "show", map[string]any{"ticket_id": "shared-ticket"})
	defaultAgainPayload := requireSuccessfulPayload(t, defaultAgain, err)
	if defaultAgainPayload["title"] != "Alpha ticket" || defaultAgainPayload["resolved_project"] != "alpha" {
		t.Fatalf("explicit selection mutated server default: %#v", defaultAgainPayload)
	}
}

func TestPathWithinRootAllowsMissingProjectUnderSymlinkedRoot(t *testing.T) {
	realRoot := t.TempDir()
	linkParent := t.TempDir()
	linkedRoot := filepath.Join(linkParent, "ticket-root")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Fatalf("create root symlink: %v", err)
	}
	if !pathWithinRoot(linkedRoot, filepath.Join(linkedRoot, "new-project")) {
		t.Fatal("missing project beneath a symlinked central root should be allowed")
	}
	if pathWithinRoot(linkedRoot, filepath.Join(linkParent, "outside")) {
		t.Fatal("candidate outside the symlinked central root should be rejected")
	}
}

func TestMCPProjectSelectionRejectsUnsafeUnknownAndRevokedProjects(t *testing.T) {
	srv, _, _ := setupCrossProjectServer(t)
	root := os.Getenv("TKT_ROOT")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("create escaping project symlink: %v", err)
	}

	cfg, err := project.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Projects["escape"] = project.ProjectConfig{Store: "central"}
	if err := project.Save(cfg); err != nil {
		t.Fatalf("save escaping project: %v", err)
	}

	for _, value := range []string{"", ".", "..", "../beta", "/tmp/beta", `alpha\\beta`} {
		result, callErr := invokeTool(srv, "stats", map[string]any{"project": value})
		requireSelectionError(t, result, callErr, root, outside)
	}

	unknown, callErr := invokeTool(srv, "stats", map[string]any{"project": "missing"})
	requireSelectionError(t, unknown, callErr, root, outside)

	escape, callErr := invokeTool(srv, "stats", map[string]any{"project": "escape"})
	requireSelectionError(t, escape, callErr, root, outside)

	delete(cfg.Projects, "beta")
	if err := project.Save(cfg); err != nil {
		t.Fatalf("revoke beta registration: %v", err)
	}
	revoked, callErr := invokeTool(srv, "stats", map[string]any{"project": "beta"})
	requireSelectionError(t, revoked, callErr, root, outside)
}

func TestMCPParallelCrossProjectMutationsRestoreContentAndKeepAuditAttribution(t *testing.T) {
	srv, alphaDir, betaDir := setupCrossProjectServer(t)
	seedProjectTicket(t, alphaDir, "alpha-ticket", "Alpha original")
	seedProjectTicket(t, betaDir, "beta-ticket", "Beta original")

	type mutationCase struct {
		project  string
		ticketID string
		original string
		source   string
	}
	cases := []mutationCase{
		{project: "alpha", ticketID: "alpha-ticket", original: "Alpha original", source: "parallel-alpha"},
		{project: "beta", ticketID: "beta-ticket", original: "Beta original", source: "parallel-beta"},
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(cases))
	for _, tc := range cases {
		tc := tc
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				title := fmt.Sprintf("%s mutation %d", tc.project, i)
				result, err := invokeTool(srv, "edit", map[string]any{
					"project":   tc.project,
					"ticket_id": tc.ticketID,
					"source":    tc.source,
					"title":     title,
				})
				payload, err := successfulPayload(result, err)
				if err != nil {
					errCh <- fmt.Errorf("%s edit %d: %w", tc.project, i, err)
					return
				}
				if payload["resolved_project"] != tc.project || payload["title"] != title {
					errCh <- fmt.Errorf("%s edit %d returned wrong project payload: %#v", tc.project, i, payload)
					return
				}
			}

			result, err := invokeTool(srv, "edit", map[string]any{
				"project":   tc.project,
				"ticket_id": tc.ticketID,
				"source":    tc.source,
				"title":     tc.original,
			})
			if _, err := successfulPayload(result, err); err != nil {
				errCh <- fmt.Errorf("%s restore: %w", tc.project, err)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	for _, tc := range cases {
		result, err := invokeTool(srv, "show", map[string]any{"project": tc.project, "ticket_id": tc.ticketID})
		payload := requireSuccessfulPayload(t, result, err)
		if payload["title"] != tc.original {
			t.Fatalf("%s ticket was not restored: %#v", tc.project, payload)
		}

		entries := readMutationEntries(t, tc.project)
		if len(entries) != 21 {
			t.Fatalf("%s: expected 21 audit entries, got %d", tc.project, len(entries))
		}
		for _, entry := range entries {
			if entry.TicketID != tc.ticketID || entry.Source != tc.source || entry.Operation != "edit" {
				t.Fatalf("%s: cross-project or incorrect audit entry: %#v", tc.project, entry)
			}
		}
	}
}

func setupCrossProjectServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, "tickets")
	t.Setenv("HOME", home)
	t.Setenv("TKT_ROOT", root)

	cfg := project.Config{Projects: map[string]project.ProjectConfig{
		"alpha": {Store: "central"},
		"beta":  {Store: "central"},
	}}
	if err := project.Save(cfg); err != nil {
		t.Fatalf("save project config: %v", err)
	}
	alphaDir := filepath.Join(root, "alpha")
	betaDir := filepath.Join(root, "beta")
	for _, dir := range []string{alphaDir, betaDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("create project store: %v", err)
		}
	}
	return NewServer("alpha", alphaDir), alphaDir, betaDir
}

func seedProjectTicket(t *testing.T, dir string, id string, title string) {
	t.Helper()
	seedMCPTestTicket(t, dir, ticket.Record{
		ID:   id,
		Path: filepath.Join(dir, id+".md"),
		Front: ticket.Frontmatter{
			ID:       id,
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Created:  "2026-08-31T00:00:00Z",
		},
		Body: ticket.Body{Title: title},
	})
}

func invokeTool(srv *Server, name string, arguments map[string]any) (*mcplib.CallToolResult, error) {
	tool := srv.s.GetTool(name)
	if tool == nil {
		return nil, fmt.Errorf("tool %q is not registered", name)
	}
	return tool.Handler(stdctx.Background(), mcplib.CallToolRequest{
		Params: mcplib.CallToolParams{Name: name, Arguments: arguments},
	})
}

func requireSuccessfulPayload(t *testing.T, result *mcplib.CallToolResult, err error) map[string]any {
	t.Helper()
	payload, payloadErr := successfulPayload(result, err)
	if payloadErr != nil {
		t.Fatal(payloadErr)
	}
	return payload
}

func successfulPayload(result *mcplib.CallToolResult, err error) (map[string]any, error) {
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("nil MCP result")
	}
	if result.IsError {
		return nil, fmt.Errorf("MCP error: %s", resultText(result))
	}
	var payload map[string]any
	if unmarshalErr := json.Unmarshal([]byte(resultText(result)), &payload); unmarshalErr != nil {
		return nil, fmt.Errorf("decode MCP payload: %w", unmarshalErr)
	}
	return payload, nil
}

func requireSelectionError(t *testing.T, result *mcplib.CallToolResult, err error, forbidden ...string) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected protocol error: %v", err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("expected MCP selection error, got %#v", result)
	}
	text := resultText(result)
	for _, value := range forbidden {
		if value != "" && strings.Contains(text, value) {
			t.Fatalf("selection error leaked filesystem path %q: %s", value, text)
		}
	}
}

func resultText(result *mcplib.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if text, ok := result.Content[0].(mcplib.TextContent); ok {
		return text.Text
	}
	return ""
}

func readMutationEntries(t *testing.T, projectName string) []engine.MutationEntry {
	t.Helper()
	path, err := engine.MutationLogPath(projectName)
	if err != nil {
		t.Fatalf("resolve mutation log: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open mutation log: %v", err)
	}
	defer file.Close()

	entries := make([]engine.MutationEntry, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry engine.MutationEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decode mutation entry: %v", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read mutation log: %v", err)
	}
	return entries
}
