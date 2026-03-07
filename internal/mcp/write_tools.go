package mcp

import (
	stdctx "context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/ticket"
)

func (s *Server) handleCreate(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	src, err := req.RequireString("source")
	if err != nil || strings.TrimSpace(src) == "" {
		return errResult("source is required")
	}
	title, err := req.RequireString("title")
	if err != nil || strings.TrimSpace(title) == "" {
		return errResult("title is required")
	}

	if err := ticket.EnsureDir(s.ticketDir); err != nil {
		return errResult(fmt.Sprintf("ensure ticket dir: %v", err))
	}

	customID := req.GetString("id", "")
	id := customID
	if id == "" {
		var genErr error
		id, genErr = ticket.GenerateID(s.ticketDir)
		if genErr != nil {
			return errResult(fmt.Sprintf("generate id: %v", genErr))
		}
	} else {
		if strings.ContainsAny(id, " \t\r\n") {
			return errResult(fmt.Sprintf("invalid ticket id %q", id))
		}
		if _, statErr := os.Stat(filepath.Join(s.ticketDir, id+".md")); statErr == nil {
			return errResult(fmt.Sprintf("ticket id %q already exists", id))
		}
	}

	ticketType := req.GetString("type", "task")
	if ticketType == "" {
		ticketType = "task"
	}
	priority := req.GetInt("priority", 2)
	if priority < 0 {
		priority = 2
	}

	record := ticket.Record{
		ID:   id,
		Path: filepath.Join(s.ticketDir, id+".md"),
		Front: ticket.Frontmatter{
			ID:          id,
			Status:      "open",
			Deps:        []string{},
			Links:       []string{},
			Created:     time.Now().UTC().Format(time.RFC3339),
			Type:        ticketType,
			Priority:    priority,
			Assignee:    req.GetString("assignee", ""),
			Parent:      req.GetString("parent", ""),
			Tags:        engine.ParseCSV(req.GetString("tags", "")),
			ExternalRef: req.GetString("external_ref", ""),
			Extra:       map[string]ticket.ExtraField{},
		},
		Body: ticket.Body{
			Title:              title,
			Description:        req.GetString("description", ""),
			Design:             req.GetString("design", ""),
			AcceptanceCriteria: req.GetString("acceptance_criteria", ""),
		},
	}

	if err := ticket.SaveRecord(record); err != nil {
		return errResult(fmt.Sprintf("save ticket: %v", err))
	}

	engine.AppendMutationLog(s.projectName, engine.MutationEntry{
		TicketID:      id,
		Operation:     "create",
		Source:        src,
		FieldsChanged: []string{"title", "status", "type", "priority"},
	})

	return resultJSON(engine.TicketToMap(record))
}

func (s *Server) handleEdit(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	src, err := req.RequireString("source")
	if err != nil || strings.TrimSpace(src) == "" {
		return errResult("source is required")
	}
	id := req.GetString("ticket_id", "")
	if id == "" {
		return errResult("ticket_id is required")
	}

	record, loadErr := s.loadByID(id)
	if loadErr != nil {
		return errResult(fmt.Sprintf("ticket not found: %s", id))
	}

	changed := make([]string, 0)
	args := req.GetArguments()
	if _, ok := args["title"]; ok {
		record.Body.Title = req.GetString("title", "")
		changed = append(changed, "title")
	}
	if _, ok := args["description"]; ok {
		record.Body.Description = req.GetString("description", "")
		changed = append(changed, "description")
	}
	if v := req.GetString("status", ""); v != "" {
		record.Front.Status = v
		changed = append(changed, "status")
	}
	if v := req.GetString("type", ""); v != "" {
		record.Front.Type = v
		changed = append(changed, "type")
	}
	if p := req.GetInt("priority", -1); p >= 0 {
		record.Front.Priority = p
		changed = append(changed, "priority")
	}
	if _, ok := args["assignee"]; ok {
		record.Front.Assignee = req.GetString("assignee", "")
		changed = append(changed, "assignee")
	}
	if _, ok := args["parent"]; ok {
		record.Front.Parent = req.GetString("parent", "")
		changed = append(changed, "parent")
	}
	if _, ok := args["tags"]; ok {
		record.Front.Tags = engine.ParseCSV(req.GetString("tags", ""))
		changed = append(changed, "tags")
	}
	if _, ok := args["design"]; ok {
		record.Body.Design = req.GetString("design", "")
		changed = append(changed, "design")
	}
	if _, ok := args["acceptance_criteria"]; ok {
		record.Body.AcceptanceCriteria = req.GetString("acceptance_criteria", "")
		changed = append(changed, "acceptance_criteria")
	}
	if _, ok := args["external_ref"]; ok {
		record.Front.ExternalRef = req.GetString("external_ref", "")
		changed = append(changed, "external_ref")
	}

	if err := ticket.SaveRecord(record); err != nil {
		return errResult(fmt.Sprintf("save ticket: %v", err))
	}

	engine.AppendMutationLog(s.projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "edit",
		Source:        src,
		FieldsChanged: changed,
	})

	return resultJSON(engine.TicketToMap(record))
}

func (s *Server) handleAddNote(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	src, err := req.RequireString("source")
	if err != nil || strings.TrimSpace(src) == "" {
		return errResult("source is required")
	}
	id := req.GetString("ticket_id", "")
	if id == "" {
		return errResult("ticket_id is required")
	}
	text, err := req.RequireString("text")
	if err != nil || strings.TrimSpace(text) == "" {
		return errResult("text is required")
	}

	record, loadErr := s.loadByID(id)
	if loadErr != nil {
		return errResult(fmt.Sprintf("ticket not found: %s", id))
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	header := fmt.Sprintf("**%s [%s]**", ts, src)
	entry := fmt.Sprintf("%s\n\n%s", header, text)

	existing := engine.FindSection(record.Body.OtherSections, "Notes")
	if strings.TrimSpace(existing.Content) != "" {
		existing.Content = strings.TrimSpace(existing.Content) + "\n\n" + entry
	} else {
		existing.Content = entry
	}
	record.Body.OtherSections = engine.UpsertSection(record.Body.OtherSections, existing)

	if err := ticket.SaveRecord(record); err != nil {
		return errResult(fmt.Sprintf("save ticket: %v", err))
	}

	engine.AppendMutationLog(s.projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "add-note",
		Source:        src,
		FieldsChanged: []string{"notes"},
	})

	return resultJSON(map[string]any{
		"ticket_id": record.ID,
		"note":      map[string]string{"at": ts, "text": text},
	})
}

func (s *Server) handleDelete(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	src, err := req.RequireString("source")
	if err != nil || strings.TrimSpace(src) == "" {
		return errResult("source is required")
	}
	ids, err := req.RequireStringSlice("ticket_ids")
	if err != nil || len(ids) == 0 {
		return errResult("ticket_ids is required")
	}

	deleted := make([]string, 0, len(ids))
	notFound := make([]string, 0)
	for _, id := range ids {
		path, pathErr := ticket.ResolvePath(s.ticketDir, id)
		if pathErr != nil {
			notFound = append(notFound, id)
			continue
		}
		resolvedID := strings.TrimSuffix(filepath.Base(path), ".md")
		if removeErr := os.Remove(path); removeErr != nil {
			notFound = append(notFound, id)
			continue
		}
		deleted = append(deleted, resolvedID)
		engine.AppendMutationLog(s.projectName, engine.MutationEntry{
			TicketID:  resolvedID,
			Operation: "delete",
			Source:    src,
		})
	}

	return resultJSON(map[string]any{
		"deleted":   deleted,
		"not_found": notFound,
	})
}

func (s *Server) handleDep(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	src, err := req.RequireString("source")
	if err != nil || strings.TrimSpace(src) == "" {
		return errResult("source is required")
	}
	id := req.GetString("ticket_id", "")
	depID := req.GetString("dep_id", "")
	if id == "" || depID == "" {
		return errResult("ticket_id and dep_id are required")
	}

	record, loadErr := s.loadByID(id)
	if loadErr != nil {
		return errResult(fmt.Sprintf("ticket not found: %s", id))
	}
	depRecord, loadErr := s.loadByID(depID)
	if loadErr != nil {
		return errResult(fmt.Sprintf("dep ticket not found: %s", depID))
	}
	if record.ID == depRecord.ID {
		return errResult("ticket cannot depend on itself")
	}

	record.Front.Deps = engine.AppendUnique(record.Front.Deps, depRecord.ID)
	if err := ticket.SaveRecord(record); err != nil {
		return errResult(fmt.Sprintf("save ticket: %v", err))
	}

	engine.AppendMutationLog(s.projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "dep",
		Source:        src,
		FieldsChanged: []string{"deps"},
	})

	return resultJSON(map[string]any{
		"ticket_id":     record.ID,
		"updated_field": "deps",
		"values":        record.Front.Deps,
	})
}

func (s *Server) handleUndep(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	src, err := req.RequireString("source")
	if err != nil || strings.TrimSpace(src) == "" {
		return errResult("source is required")
	}
	id := req.GetString("ticket_id", "")
	depID := req.GetString("dep_id", "")
	if id == "" || depID == "" {
		return errResult("ticket_id and dep_id are required")
	}

	record, loadErr := s.loadByID(id)
	if loadErr != nil {
		return errResult(fmt.Sprintf("ticket not found: %s", id))
	}

	// Resolve dep ID if the ticket still exists, otherwise use the raw ID
	// so stale references can still be cleaned up.
	resolvedDepID := depID
	if depRecord, err := s.loadByID(depID); err == nil {
		resolvedDepID = depRecord.ID
	}

	record.Front.Deps = engine.RemoveValue(record.Front.Deps, resolvedDepID)
	if err := ticket.SaveRecord(record); err != nil {
		return errResult(fmt.Sprintf("save ticket: %v", err))
	}

	engine.AppendMutationLog(s.projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "undep",
		Source:        src,
		FieldsChanged: []string{"deps"},
	})

	return resultJSON(map[string]any{
		"ticket_id":     record.ID,
		"updated_field": "deps",
		"values":        record.Front.Deps,
	})
}

func (s *Server) handleLink(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	src, err := req.RequireString("source")
	if err != nil || strings.TrimSpace(src) == "" {
		return errResult("source is required")
	}
	ids, err := req.RequireStringSlice("ticket_ids")
	if err != nil || len(ids) < 2 {
		return errResult("ticket_ids requires at least 2 IDs")
	}

	sourceRecord, loadErr := s.loadByID(ids[0])
	if loadErr != nil {
		return errResult(fmt.Sprintf("ticket not found: %s", ids[0]))
	}

	targets := map[string]ticket.Record{}
	for _, targetID := range ids[1:] {
		t, loadErr := s.loadByID(targetID)
		if loadErr != nil {
			return errResult(fmt.Sprintf("ticket not found: %s", targetID))
		}
		if t.ID == sourceRecord.ID {
			return errResult("ticket cannot link to itself")
		}
		targets[t.ID] = t
	}

	sortedIDs := make([]string, 0, len(targets))
	for id := range targets {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, id := range sortedIDs {
		sourceRecord.Front.Links = engine.AppendUnique(sourceRecord.Front.Links, id)
	}
	if err := ticket.SaveRecord(sourceRecord); err != nil {
		return errResult(fmt.Sprintf("save ticket: %v", err))
	}
	for _, id := range sortedIDs {
		t := targets[id]
		t.Front.Links = engine.AppendUnique(t.Front.Links, sourceRecord.ID)
		if err := ticket.SaveRecord(t); err != nil {
			return errResult(fmt.Sprintf("save target ticket: %v", err))
		}
	}

	engine.AppendMutationLog(s.projectName, engine.MutationEntry{
		TicketID:      sourceRecord.ID,
		Operation:     "link",
		Source:        src,
		FieldsChanged: []string{"links"},
	})

	return resultJSON(map[string]any{
		"ticket_id":     sourceRecord.ID,
		"updated_field": "links",
		"values":        sourceRecord.Front.Links,
	})
}

func (s *Server) handleUnlink(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	src, err := req.RequireString("source")
	if err != nil || strings.TrimSpace(src) == "" {
		return errResult("source is required")
	}
	id := req.GetString("ticket_id", "")
	targetID := req.GetString("target_id", "")
	if id == "" || targetID == "" {
		return errResult("ticket_id and target_id are required")
	}

	sourceRecord, loadErr := s.loadByID(id)
	if loadErr != nil {
		return errResult(fmt.Sprintf("ticket not found: %s", id))
	}

	// Resolve target; if the target still exists, update both sides.
	// If deleted, just clean the stale reference from the source.
	resolvedTargetID := targetID
	targetRecord, targetErr := s.loadByID(targetID)
	if targetErr == nil {
		resolvedTargetID = targetRecord.ID
	}

	sourceRecord.Front.Links = engine.RemoveValue(sourceRecord.Front.Links, resolvedTargetID)
	if err := ticket.SaveRecord(sourceRecord); err != nil {
		return errResult(fmt.Sprintf("save ticket: %v", err))
	}

	if targetErr == nil {
		targetRecord.Front.Links = engine.RemoveValue(targetRecord.Front.Links, sourceRecord.ID)
		if err := ticket.SaveRecord(targetRecord); err != nil {
			return errResult(fmt.Sprintf("save target ticket: %v", err))
		}
	}

	engine.AppendMutationLog(s.projectName, engine.MutationEntry{
		TicketID:      sourceRecord.ID,
		Operation:     "unlink",
		Source:        src,
		FieldsChanged: []string{"links"},
	})

	return resultJSON(map[string]any{
		"ticket_id":     sourceRecord.ID,
		"updated_field": "links",
		"values":        sourceRecord.Front.Links,
	})
}
