package mcp

import (
	stdctx "context"
	"fmt"
	"sort"
	"strings"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func (s *Server) handleShow(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	id := req.GetString("ticket_id", "")
	if id == "" {
		return errResult("ticket_id is required")
	}
	record, err := s.loadByID(id)
	if err != nil {
		return errResult(fmt.Sprintf("ticket not found: %s", id))
	}
	links := s.loadJournal()
	filtered := engine.FilterJournalByTickets(links, []string{record.ID})
	payload := engine.TicketToMap(record)
	payload["commit_links"] = filtered
	return resultJSON(payload)
}

func (s *Server) handleList(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}

	status := req.GetString("status", "")
	typeFilt := req.GetString("type", "")
	priority := req.GetInt("priority", -1)
	assignee := req.GetString("assignee", "")
	tag := req.GetString("tag", "")
	parent := req.GetString("parent", "")

	search := req.GetString("search", "")
	sortBy := req.GetString("sort", "")
	limit := req.GetInt("limit", 0)

	onlyOpen := status == ""
	out := make([]ticket.Record, 0)
	for _, r := range records {
		if onlyOpen && r.Front.Status != "open" {
			continue
		}
		if status != "" && r.Front.Status != status {
			continue
		}
		if typeFilt != "" && r.Front.Type != typeFilt {
			continue
		}
		if priority >= 0 && r.Front.Priority != priority {
			continue
		}
		if assignee != "" && r.Front.Assignee != assignee {
			continue
		}
		if tag != "" && !engine.Contains(r.Front.Tags, tag) {
			continue
		}
		if parent != "" && r.Front.Parent != parent {
			continue
		}
		if search != "" {
			q := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(r.ID), q) && !strings.Contains(strings.ToLower(r.Body.Title), q) {
				continue
			}
		}
		out = append(out, r)
	}
	if sortBy != "" {
		if err := engine.SortRecords(out, sortBy); err != nil {
			return errResult(err.Error())
		}
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}
	out = engine.LimitRecords(out, limit)
	return resultJSON(map[string]any{
		"items": engine.TicketSummariesToMaps(out),
		"total": len(out),
	})
}

func (s *Server) handleReady(_ stdctx.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}
	byID := engine.IndexByID(records)
	out := make([]ticket.Record, 0)
	for _, r := range records {
		if r.Front.Status != "open" {
			continue
		}
		if engine.HasOpenDeps(r, byID) {
			continue
		}
		if r.Front.Parent == "" {
			out = append(out, r)
			continue
		}
		parent, ok := byID[r.Front.Parent]
		if ok && parent.Front.Status == "in_progress" {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return resultJSON(map[string]any{"items": engine.TicketSummariesToMaps(out), "total": len(out)})
}

func (s *Server) handleBlocked(_ stdctx.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}
	byID := engine.IndexByID(records)
	out := make([]ticket.Record, 0)
	for _, r := range records {
		if r.Front.Status != "open" {
			continue
		}
		if engine.HasOpenDeps(r, byID) {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return resultJSON(map[string]any{"items": engine.TicketSummariesToMaps(out), "total": len(out)})
}

func (s *Server) handleClosed(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	limit := req.GetInt("limit", 20)
	sortBy := req.GetString("sort", "")
	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}
	out := make([]ticket.Record, 0)
	for _, r := range records {
		if r.Front.Status == "closed" {
			out = append(out, r)
		}
	}
	if sortBy != "" {
		if err := engine.SortRecords(out, sortBy); err != nil {
			return errResult(err.Error())
		}
	} else {
		sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	}
	out = engine.LimitRecords(out, limit)
	return resultJSON(map[string]any{"items": engine.TicketSummariesToMaps(out), "total": len(out)})
}

func (s *Server) handleStats(_ stdctx.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}
	byStatus := map[string]int{}
	byType := map[string]int{}
	byPriority := map[int]int{}
	for _, r := range records {
		byStatus[r.Front.Status]++
		byType[r.Front.Type]++
		byPriority[r.Front.Priority]++
	}
	return resultJSON(map[string]any{
		"counts":      byStatus,
		"by_type":     byType,
		"by_priority": byPriority,
		"total":       len(records),
	})
}

func (s *Server) handleTimeline(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	weeks := req.GetInt("weeks", 4)
	if weeks <= 0 {
		weeks = 4
	}
	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}
	closedByWeek := map[string]int{}
	for _, r := range records {
		if r.Front.Status != "closed" {
			continue
		}
		created, err := time.Parse(time.RFC3339, r.Front.Created)
		if err != nil {
			continue
		}
		weekStart := engine.Monday(created).Format("2006-01-02")
		closedByWeek[weekStart]++
	}
	currentWeek := engine.Monday(time.Now().UTC())
	rows := make([]map[string]any, 0, weeks)
	for i := weeks - 1; i >= 0; i-- {
		start := currentWeek.AddDate(0, 0, -7*i)
		key := start.Format("2006-01-02")
		rows = append(rows, map[string]any{
			"week_start":   key,
			"closed_count": closedByWeek[key],
		})
	}
	return resultJSON(map[string]any{"weeks": rows})
}

func (s *Server) handleWorkflow(_ stdctx.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	workflow, err := project.LoadWorkflow(s.projectRoot)
	if err != nil {
		return errResult(fmt.Sprintf("load workflow: %v", err))
	}
	return resultJSON(map[string]any{"content": workflow.Content})
}

func (s *Server) handleDepTree(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	id := req.GetString("ticket_id", "")
	if id == "" {
		return errResult("ticket_id is required")
	}
	full := req.GetBool("full", false)

	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}
	byID := engine.IndexByID(records)
	if _, ok := byID[id]; !ok {
		// try partial match
		r, found := engine.ResolveRecordFromList(records, id)
		if !found {
			return errResult(fmt.Sprintf("ticket not found: %s", id))
		}
		id = r.ID
	}
	lines := engine.RenderDepTree(id, byID, full)
	nodes := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		nodes = append(nodes, map[string]any{"line": line})
	}
	return resultJSON(map[string]any{"root": id, "nodes": nodes})
}

func (s *Server) handleEpicView(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	id := req.GetString("ticket_id", "")
	if id == "" {
		return errResult("ticket_id is required")
	}

	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}
	epic, ok := engine.ResolveRecordFromList(records, id)
	if !ok {
		return errResult(fmt.Sprintf("ticket not found: %s", id))
	}

	children := make([]ticket.Record, 0)
	for _, r := range records {
		if r.Front.Parent == epic.ID {
			children = append(children, r)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })

	links := s.loadJournal()
	childIDs := make([]string, 0, len(children))
	for _, c := range children {
		childIDs = append(childIDs, c.ID)
	}
	related := engine.FilterJournalByTickets(links, append([]string{epic.ID}, childIDs...))

	deps := make([]map[string]any, 0)
	for _, r := range children {
		for _, dep := range r.Front.Deps {
			deps = append(deps, map[string]any{"from": r.ID, "to": dep})
		}
	}

	return resultJSON(map[string]any{
		"epic":     engine.TicketSummaryToMap(epic),
		"children": engine.TicketSummariesToMaps(children),
		"deps":     deps,
		"commits":  related,
	})
}

func (s *Server) handleDashboard(_ stdctx.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}
	byID := engine.IndexByID(records)

	inProgress := make([]ticket.Record, 0)
	blocked := make([]ticket.Record, 0)
	ready := make([]ticket.Record, 0)

	for _, r := range records {
		switch {
		case r.Front.Status == "in_progress":
			inProgress = append(inProgress, r)
		case r.Front.Status == "open" && engine.HasOpenDeps(r, byID):
			blocked = append(blocked, r)
		case r.Front.Status == "open" && !engine.HasOpenDeps(r, byID):
			if r.Front.Parent == "" {
				ready = append(ready, r)
			} else if parent, ok := byID[r.Front.Parent]; ok && parent.Front.Status == "in_progress" {
				ready = append(ready, r)
			}
		}
	}

	links := s.loadJournal()
	n := 5
	recent := links
	if len(recent) > n {
		recent = recent[len(recent)-n:]
	}

	summary := map[string]int{
		"total": len(records), "open": 0, "in_progress": 0,
		"needs_testing": 0, "closed": 0,
		"ready": len(ready), "blocked": len(blocked),
	}
	for _, r := range records {
		summary[r.Front.Status]++
	}

	return resultJSON(map[string]any{
		"summary":        summary,
		"in_progress":    engine.TicketSummariesToMaps(inProgress),
		"blocked":        engine.TicketSummariesToMaps(blocked),
		"ready":          engine.TicketSummariesToMaps(ready),
		"recent_commits": recent,
	})
}

func (s *Server) handleProgress(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	window := req.GetString("window", "week")
	if window != "today" && window != "week" {
		window = "week"
	}

	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}
	byID := engine.IndexByID(records)
	links := s.loadJournal()
	start := engine.WindowStart(window, time.Now().UTC())

	windowEntries := make([]engine.CommitJournalEntry, 0)
	closedIDs := map[string]struct{}{}
	changes := make([]map[string]any, 0)
	for _, e := range links {
		ts, err := time.Parse(time.RFC3339, e.TS)
		if err != nil || ts.Before(start) {
			continue
		}
		windowEntries = append(windowEntries, e)
		if e.Action == "close" {
			closedIDs[e.Ticket] = struct{}{}
			changes = append(changes, map[string]any{
				"ticket_id": e.Ticket,
				"from":      "open",
				"to":        "closed",
				"at":        e.TS,
			})
		}
	}

	for _, r := range records {
		if r.Front.Status == "closed" && !r.ModTime.IsZero() && r.ModTime.After(start) {
			closedIDs[r.ID] = struct{}{}
		}
	}

	closedTickets := make([]ticket.Record, 0, len(closedIDs))
	for id := range closedIDs {
		if r, ok := byID[id]; ok {
			closedTickets = append(closedTickets, r)
		}
	}
	sort.Slice(closedTickets, func(i, j int) bool { return closedTickets[i].ID < closedTickets[j].ID })

	return resultJSON(map[string]any{
		"window":         window,
		"closed":         engine.TicketSummariesToMaps(closedTickets),
		"commit_links":   windowEntries,
		"status_changes": changes,
	})
}

func (s *Server) handleLifecycle(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	id := req.GetString("ticket_id", "")
	if id == "" {
		return errResult("ticket_id is required")
	}
	record, err := s.loadByID(id)
	if err != nil {
		return errResult(fmt.Sprintf("ticket not found: %s", id))
	}

	links := s.loadJournal()
	filtered := engine.FilterJournalByTickets(links, []string{record.ID})

	created := record.Front.Created
	var workDuration string
	if createdTime, err := time.Parse(time.RFC3339, created); err == nil {
		if record.Front.Status == "closed" {
			workDuration = record.ModTime.UTC().Sub(createdTime).String()
		} else {
			workDuration = time.Since(createdTime).String()
		}
	}

	return resultJSON(map[string]any{
		"ticket":        engine.TicketToMap(record),
		"commit_links":  filtered,
		"work_duration": workDuration,
	})
}

func (s *Server) handleContext(_ stdctx.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	id := req.GetString("ticket_id", "")
	if id == "" {
		return errResult("ticket_id is required")
	}

	records, err := s.loadTickets()
	if err != nil {
		return errResult(fmt.Sprintf("load tickets: %v", err))
	}

	record, ok := engine.ResolveRecordFromList(records, id)
	if !ok {
		return errResult(fmt.Sprintf("ticket not found: %s", id))
	}

	byID := engine.IndexByID(records)

	var parentJSON any
	if record.Front.Parent != "" {
		if p, ok := byID[record.Front.Parent]; ok {
			parentJSON = map[string]any{
				"id":     p.ID,
				"title":  p.Body.Title,
				"status": p.Front.Status,
			}
		}
	}

	deps := make([]map[string]any, 0, len(record.Front.Deps))
	for _, depID := range record.Front.Deps {
		if dep, ok := byID[depID]; ok {
			deps = append(deps, map[string]any{
				"id": dep.ID, "title": dep.Body.Title, "status": dep.Front.Status,
			})
		} else {
			deps = append(deps, map[string]any{
				"id": depID, "title": "", "status": "missing",
			})
		}
	}

	dependents := make([]map[string]any, 0)
	for _, r := range records {
		for _, depID := range r.Front.Deps {
			if depID == record.ID {
				dependents = append(dependents, map[string]any{
					"id": r.ID, "title": r.Body.Title, "status": r.Front.Status,
				})
				break
			}
		}
	}

	linked := make([]map[string]any, 0, len(record.Front.Links))
	for _, linkID := range record.Front.Links {
		if link, ok := byID[linkID]; ok {
			linked = append(linked, map[string]any{
				"id": link.ID, "title": link.Body.Title, "status": link.Front.Status,
			})
		} else {
			linked = append(linked, map[string]any{
				"id": linkID, "title": "", "status": "missing",
			})
		}
	}

	children := make([]ticket.Record, 0)
	for _, r := range records {
		if r.Front.Parent == record.ID {
			children = append(children, r)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })

	links := s.loadJournal()
	recentCommits := engine.FilterJournalByTickets(links, []string{record.ID})
	n := 10
	if len(recentCommits) > n {
		recentCommits = recentCommits[len(recentCommits)-n:]
	}

	return resultJSON(map[string]any{
		"ticket":         engine.TicketToMap(record),
		"parent":         parentJSON,
		"deps":           deps,
		"dependents":     dependents,
		"linked":         linked,
		"children":       engine.TicketsToMaps(children),
		"recent_commits": recentCommits,
	})
}
