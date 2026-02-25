package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/ticket"
)

func runContext(ctx context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: tkt context <id>")
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}

	record, ok := engine.ResolveRecordFromList(records, args[0])
	if !ok {
		return fmt.Errorf("ticket not found: %s", args[0])
	}

	byID := engine.IndexByID(records)

	// Parent summary
	var parent *ticket.Record
	if record.Front.Parent != "" {
		if p, ok := byID[record.Front.Parent]; ok {
			parent = &p
		}
	}

	// Dependencies with status
	deps := make([]map[string]any, 0, len(record.Front.Deps))
	for _, depID := range record.Front.Deps {
		if dep, ok := byID[depID]; ok {
			deps = append(deps, map[string]any{
				"id":     dep.ID,
				"title":  dep.Body.Title,
				"status": dep.Front.Status,
			})
		} else {
			deps = append(deps, map[string]any{
				"id":     depID,
				"title":  "",
				"status": "missing",
			})
		}
	}

	// Dependents (reverse-dep: who depends on this ticket)
	dependents := make([]map[string]any, 0)
	for _, r := range records {
		for _, depID := range r.Front.Deps {
			if depID == record.ID {
				dependents = append(dependents, map[string]any{
					"id":     r.ID,
					"title":  r.Body.Title,
					"status": r.Front.Status,
				})
				break
			}
		}
	}

	// Linked tickets
	linked := make([]map[string]any, 0, len(record.Front.Links))
	for _, linkID := range record.Front.Links {
		if link, ok := byID[linkID]; ok {
			linked = append(linked, map[string]any{
				"id":     link.ID,
				"title":  link.Body.Title,
				"status": link.Front.Status,
			})
		} else {
			linked = append(linked, map[string]any{
				"id":     linkID,
				"title":  "",
				"status": "missing",
			})
		}
	}

	// Children (if this is an epic)
	children := make([]ticket.Record, 0)
	for _, r := range records {
		if r.Front.Parent == record.ID {
			children = append(children, r)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })

	// Recent commits
	projectName, _ := resolvedProjectName(ctx)
	journalEntries, _ := engine.ReadJournalEntries(projectName)
	recentCommits := engine.FilterJournalByTickets(journalEntries, []string{record.ID})
	n := 10
	if len(recentCommits) > n {
		recentCommits = recentCommits[len(recentCommits)-n:]
	}

	if ctx.json {
		parentJSON := any(nil)
		if parent != nil {
			parentJSON = map[string]any{
				"id":     parent.ID,
				"title":  parent.Body.Title,
				"status": parent.Front.Status,
			}
		}
		return emitJSON(ctx, map[string]any{
			"ticket":         ticketToJSON(record),
			"parent":         parentJSON,
			"deps":           deps,
			"dependents":     dependents,
			"linked":         linked,
			"children":       ticketsToJSON(children),
			"recent_commits": recentCommits,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "%s [%s] %s\n", record.ID, record.Front.Status, record.Body.Title)
	_, _ = fmt.Fprintf(ctx.stdout, "Type: %s  Priority: p%d  Assignee: %s\n",
		record.Front.Type, record.Front.Priority, record.Front.Assignee)

	if parent != nil {
		_, _ = fmt.Fprintf(ctx.stdout, "Parent: %s [%s] %s\n", parent.ID, parent.Front.Status, parent.Body.Title)
	}

	if len(deps) > 0 {
		_, _ = fmt.Fprintln(ctx.stdout, "Dependencies:")
		for _, d := range deps {
			_, _ = fmt.Fprintf(ctx.stdout, "  %s [%s] %s\n", d["id"], d["status"], d["title"])
		}
	}

	if len(dependents) > 0 {
		_, _ = fmt.Fprintln(ctx.stdout, "Dependents:")
		for _, d := range dependents {
			_, _ = fmt.Fprintf(ctx.stdout, "  %s [%s] %s\n", d["id"], d["status"], d["title"])
		}
	}

	if len(linked) > 0 {
		_, _ = fmt.Fprintln(ctx.stdout, "Linked:")
		for _, l := range linked {
			_, _ = fmt.Fprintf(ctx.stdout, "  %s [%s] %s\n", l["id"], l["status"], l["title"])
		}
	}

	if len(children) > 0 {
		_, _ = fmt.Fprintln(ctx.stdout, "Children:")
		for _, child := range children {
			_, _ = fmt.Fprintf(ctx.stdout, "  %s [%s] p%d  %s\n",
				child.ID, child.Front.Status, child.Front.Priority, child.Body.Title)
		}
	}

	if len(recentCommits) > 0 {
		_, _ = fmt.Fprintln(ctx.stdout, "Recent Commits:")
		for _, e := range recentCommits {
			_, _ = fmt.Fprintf(ctx.stdout, "  %s  %s  %s  %s\n",
				engine.ShortSHA(e.SHA), e.TS, e.Action, e.Msg)
		}
	}

	if len(record.Front.Tags) > 0 {
		_, _ = fmt.Fprintf(ctx.stdout, "Tags: %s\n", strings.Join(record.Front.Tags, ", "))
	}

	return nil
}
