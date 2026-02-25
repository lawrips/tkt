package cli

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/journal"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

type depEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type statusChange struct {
	TicketID string `json:"ticket_id"`
	From     string `json:"from"`
	To       string `json:"to"`
	At       string `json:"at"`
}

type epicViewChild struct {
	DirectCommits   int `json:"direct_commits"`
	RolledUpCommits int `json:"rolled_up_commits"`
	TotalCommits    int `json:"total_commits"`
}

func runEpicView(ctx context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: tkt epic-view <id>")
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}
	epic, ok := engine.ResolveRecordFromList(records, args[0])
	if !ok {
		return fmt.Errorf("ticket not found: %s", args[0])
	}

	children := make([]ticket.Record, 0)
	for _, record := range records {
		if record.Front.Parent == epic.ID {
			children = append(children, record)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })

	projectName, _ := resolvedProjectName(ctx)
	links, _ := engine.ReadJournalEntries(projectName)
	related := engine.FilterJournalByTickets(links, append([]string{epic.ID}, engine.IDsFromRecords(children)...))
	epicCommitCount := engine.CountJournalForTicket(related, epic.ID)

	deps := make([]depEdge, 0)
	for _, record := range children {
		for _, dep := range record.Front.Deps {
			deps = append(deps, depEdge{From: record.ID, To: dep})
		}
	}

	childrenJSON := make([]map[string]any, 0, len(children))
	childCounts := make(map[string]epicViewChild, len(children))
	for _, child := range children {
		direct := engine.CountJournalForTicket(related, child.ID)
		rolledUp := epicCommitCount
		total := direct + rolledUp

		childJSON := ticketSummaryToJSON(child)
		childJSON["direct_commits"] = direct
		childJSON["rolled_up_commits"] = rolledUp
		childJSON["total_commits"] = total
		childrenJSON = append(childrenJSON, childJSON)

		childCounts[child.ID] = epicViewChild{
			DirectCommits:   direct,
			RolledUpCommits: rolledUp,
			TotalCommits:    total,
		}
	}

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"epic":     ticketSummaryToJSON(epic),
			"children": childrenJSON,
			"deps":     deps,
			"commits":  related,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "Epic: %s [%s] %s\n", epic.ID, epic.Front.Status, epic.Body.Title)
	_, _ = fmt.Fprintln(ctx.stdout, "Children:")
	if len(children) == 0 {
		_, _ = fmt.Fprintln(ctx.stdout, "  (none)")
	} else {
		for _, child := range children {
			commitCount := childCounts[child.ID].TotalCommits
			_, _ = fmt.Fprintf(ctx.stdout, "  %s\t%s\tp%d\tcommits=%d\t%s\n",
				child.ID, child.Front.Status, child.Front.Priority, commitCount, child.Body.Title)
		}
	}
	_, _ = fmt.Fprintf(ctx.stdout, "Dependencies: %d edge(s)\n", len(deps))
	return nil
}

func runProgress(ctx context, args []string) error {
	fs := flag.NewFlagSet("progress", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	today := false
	week := false
	fs.BoolVar(&today, "today", false, "")
	fs.BoolVar(&week, "week", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: tkt progress [--today|--week]")
	}
	if today && week {
		return fmt.Errorf("--today and --week are mutually exclusive")
	}

	window := "week"
	if today {
		window = "today"
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}
	byID := engine.IndexByID(records)

	projectName, _ := resolvedProjectName(ctx)
	links, _ := engine.ReadJournalEntries(projectName)
	start := engine.WindowStart(window, time.Now().UTC())

	windowEntries := make([]engine.CommitJournalEntry, 0)
	closedIDs := map[string]struct{}{}
	changes := make([]statusChange, 0)
	for _, entry := range links {
		ts, err := time.Parse(time.RFC3339, entry.TS)
		if err != nil {
			continue
		}
		if ts.Before(start) {
			continue
		}
		windowEntries = append(windowEntries, entry)
		if entry.Action == "close" {
			closedIDs[entry.Ticket] = struct{}{}
			changes = append(changes, statusChange{
				TicketID: entry.Ticket,
				From:     "open",
				To:       "closed",
				At:       entry.TS,
			})
		}
	}

	for _, record := range records {
		if record.Front.Status == "closed" && !record.ModTime.IsZero() && record.ModTime.After(start) {
			closedIDs[record.ID] = struct{}{}
		}
	}

	closedTickets := make([]ticket.Record, 0, len(closedIDs))
	for id := range closedIDs {
		if record, ok := byID[id]; ok {
			closedTickets = append(closedTickets, record)
		}
	}
	sort.Slice(closedTickets, func(i, j int) bool { return closedTickets[i].ID < closedTickets[j].ID })

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"window":         window,
			"closed":         ticketSummariesToJSON(closedTickets),
			"commit_links":   windowEntries,
			"status_changes": changes,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "Progress window: %s (since %s)\n", window, start.Format(time.RFC3339))
	_, _ = fmt.Fprintf(ctx.stdout, "Closed tickets: %d\n", len(closedTickets))
	for _, record := range closedTickets {
		_, _ = fmt.Fprintf(ctx.stdout, "  %s\t%s\n", record.ID, record.Body.Title)
	}
	_, _ = fmt.Fprintf(ctx.stdout, "Commit links: %d\n", len(windowEntries))
	return nil
}

func runDashboard(ctx context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: tkt dashboard")
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}
	byID := engine.IndexByID(records)

	inProgress := make([]ticket.Record, 0)
	blocked := make([]ticket.Record, 0)
	ready := make([]ticket.Record, 0)

	for _, record := range records {
		if record.Front.Status == "in_progress" {
			inProgress = append(inProgress, record)
		}
		if record.Front.Status == "open" && engine.HasOpenDeps(record, byID) {
			blocked = append(blocked, record)
		}
		if record.Front.Status == "open" && !engine.HasOpenDeps(record, byID) {
			if record.Front.Parent == "" {
				ready = append(ready, record)
			} else if parent, ok := byID[record.Front.Parent]; ok && parent.Front.Status == "in_progress" {
				ready = append(ready, record)
			}
		}
	}

	projectName, _ := resolvedProjectName(ctx)
	links, _ := engine.ReadJournalEntries(projectName)
	recent := engine.LastNJournalEntries(links, 5)

	summary := map[string]int{
		"total":         len(records),
		"open":          0,
		"in_progress":   0,
		"needs_testing": 0,
		"closed":        0,
		"ready":         len(ready),
		"blocked":       len(blocked),
	}
	for _, record := range records {
		summary[record.Front.Status]++
	}

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"summary":        summary,
			"in_progress":    ticketSummariesToJSON(inProgress),
			"blocked":        ticketSummariesToJSON(blocked),
			"ready":          ticketSummariesToJSON(ready),
			"recent_commits": recent,
		})
	}

	_, _ = fmt.Fprintln(ctx.stdout, "Dashboard")
	_, _ = fmt.Fprintf(ctx.stdout, "  total=%d open=%d in_progress=%d needs_testing=%d closed=%d\n",
		summary["total"], summary["open"], summary["in_progress"], summary["needs_testing"], summary["closed"])
	_, _ = fmt.Fprintf(ctx.stdout, "  ready=%d blocked=%d\n", summary["ready"], summary["blocked"])
	_, _ = fmt.Fprintf(ctx.stdout, "In progress: %d\n", len(inProgress))
	for _, record := range inProgress {
		_, _ = fmt.Fprintf(ctx.stdout, "  %s\t%s\n", record.ID, record.Body.Title)
	}
	_, _ = fmt.Fprintf(ctx.stdout, "Recent commits: %d\n", len(recent))
	for _, entry := range recent {
		_, _ = fmt.Fprintf(ctx.stdout, "  %s\t%s\t%s\n", engine.ShortSHA(entry.SHA), entry.Ticket, entry.Action)
	}
	return nil
}

func runLifecycle(ctx context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: tkt lifecycle <id>")
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}
	rec, ok := engine.ResolveRecordFromList(records, args[0])
	if !ok {
		return fmt.Errorf("ticket not found: %s", args[0])
	}

	projectName, _ := resolvedProjectName(ctx)
	engineLinks, _ := engine.ReadJournalEntries(projectName)
	engineTicketLinks := engine.FilterJournalByTickets(engineLinks, []string{rec.ID})
	ticketLinks := toJournalEntries(engineTicketLinks)
	lc := journal.Lifecycle(rec.Front.Created, rec.Front.Status, ticketLinks, time.Now().UTC())
	effort := journal.Effort(ticketLinks)

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"id":               rec.ID,
			"title":            rec.Body.Title,
			"status":           rec.Front.Status,
			"created":          rec.Front.Created,
			"opened":           lc.Opened,
			"first_commit":     lc.FirstCommit,
			"last_commit":      lc.LastCommit,
			"work_started":     lc.WorkStarted,
			"work_ended":       lc.WorkEnded,
			"closed_at":        lc.ClosedAt,
			"calendar_seconds": lc.CalendarSeconds,
			"work_seconds":     lc.WorkSeconds,
			"idle_seconds":     lc.IdleSeconds,
			"total_commits":    effort.Commits,
			"lines_added":      effort.LinesAdded,
			"lines_removed":    effort.LinesRemoved,
			"files_touched":    effort.FilesChanged,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "Lifecycle: %s\n", rec.ID)
	_, _ = fmt.Fprintf(ctx.stdout, "  Title:        %s\n", rec.Body.Title)
	_, _ = fmt.Fprintf(ctx.stdout, "  Status:       %s\n", rec.Front.Status)
	if lc.Opened != "" {
		_, _ = fmt.Fprintf(ctx.stdout, "  Opened:       %s\n", lc.Opened)
	}
	if lc.FirstCommit != "" {
		_, _ = fmt.Fprintf(ctx.stdout, "  First commit: %s\n", lc.FirstCommit)
	}
	if lc.LastCommit != "" {
		_, _ = fmt.Fprintf(ctx.stdout, "  Last commit:  %s\n", lc.LastCommit)
	}
	if lc.WorkStarted != "" {
		_, _ = fmt.Fprintf(ctx.stdout, "  Work started: %s\n", lc.WorkStarted)
	}
	if lc.WorkEnded != "" {
		_, _ = fmt.Fprintf(ctx.stdout, "  Work ended:   %s\n", lc.WorkEnded)
	}
	if lc.ClosedAt != "" {
		_, _ = fmt.Fprintf(ctx.stdout, "  Closed:       %s\n", lc.ClosedAt)
	}
	_, _ = fmt.Fprintf(ctx.stdout, "  Commits:      %d\n", effort.Commits)
	_, _ = fmt.Fprintf(ctx.stdout, "  Calendar:     %s\n", journal.FormatSeconds(lc.CalendarSeconds))
	_, _ = fmt.Fprintf(ctx.stdout, "  Work:         %s\n", journal.FormatSeconds(lc.WorkSeconds))
	_, _ = fmt.Fprintf(ctx.stdout, "  Idle:         %s\n", journal.FormatSeconds(lc.IdleSeconds))
	if effort.LinesAdded > 0 || effort.LinesRemoved > 0 {
		_, _ = fmt.Fprintf(ctx.stdout, "  Lines:        %s\n", effort.String())
	}
	return nil
}

func toJournalEntries(entries []engine.CommitJournalEntry) []journal.Entry {
	out := make([]journal.Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, journal.Entry{
			SHA:          e.SHA,
			Ticket:       e.Ticket,
			Repo:         e.Repo,
			TS:           e.TS,
			Msg:          e.Msg,
			Author:       e.Author,
			Action:       e.Action,
			FilesChanged: e.FilesChanged,
			LinesAdded:   e.LinesAdded,
			LinesRemoved: e.LinesRemoved,
			Branch:       e.Branch,
			WorkStarted:  e.WorkStarted,
			WorkEnded:    e.WorkEnded,
			DurationSecs: e.DurationSecs,
		})
	}
	return out
}

func resolvedProjectName(ctx context) (string, bool) {
	cfg, err := project.Load()
	if err != nil {
		return "", false
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	name, _ := project.ResolveName(cfg, cwd, ctx.projectOverride)
	if name == "" {
		return "", false
	}
	return name, true
}
