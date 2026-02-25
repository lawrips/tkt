package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func runDep(ctx context, args []string) error {
	fs := flag.NewFlagSet("dep", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	var source string
	fs.StringVar(&source, "source", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("usage: tkt dep <id> <dep-id>")
	}

	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}

	record, err := ticket.LoadByID(dir, rest[0])
	if err != nil {
		return err
	}
	depRecord, err := ticket.LoadByID(dir, rest[1])
	if err != nil {
		return err
	}
	if record.ID == depRecord.ID {
		return fmt.Errorf("ticket cannot depend on itself")
	}

	record.Front.Deps = engine.AppendUnique(record.Front.Deps, depRecord.ID)
	if err := ticket.SaveRecord(record); err != nil {
		return err
	}

	projectName, _ := resolvedProjectName(ctx)
	engine.AppendMutationLog(projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "dep",
		Source:        source,
		FieldsChanged: []string{"deps"},
	})

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"ticket_id":     record.ID,
			"updated_field": "deps",
			"values":        record.Front.Deps,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "dependency added: %s -> %s\n", record.ID, depRecord.ID)
	return nil
}

func runUndep(ctx context, args []string) error {
	fs := flag.NewFlagSet("undep", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	var source string
	fs.StringVar(&source, "source", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("usage: tkt undep <id> <dep-id>")
	}

	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}

	record, err := ticket.LoadByID(dir, rest[0])
	if err != nil {
		return err
	}
	depRecord, err := ticket.LoadByID(dir, rest[1])
	if err != nil {
		return err
	}

	record.Front.Deps = engine.RemoveValue(record.Front.Deps, depRecord.ID)
	if err := ticket.SaveRecord(record); err != nil {
		return err
	}

	projectName, _ := resolvedProjectName(ctx)
	engine.AppendMutationLog(projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "undep",
		Source:        source,
		FieldsChanged: []string{"deps"},
	})

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"ticket_id":     record.ID,
			"updated_field": "deps",
			"values":        record.Front.Deps,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "dependency removed: %s -/-> %s\n", record.ID, depRecord.ID)
	return nil
}

func runLink(ctx context, args []string) error {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	var sourceName string
	fs.StringVar(&sourceName, "source", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 2 {
		return fmt.Errorf("usage: tkt link <id> <id> [id...]")
	}

	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}

	source, err := ticket.LoadByID(dir, rest[0])
	if err != nil {
		return err
	}

	targets := map[string]ticket.Record{}
	for _, arg := range rest[1:] {
		target, err := ticket.LoadByID(dir, arg)
		if err != nil {
			return err
		}
		if target.ID == source.ID {
			return fmt.Errorf("ticket cannot link to itself")
		}
		targets[target.ID] = target
	}

	ids := sortedRecordIDs(targets)
	for _, id := range ids {
		source.Front.Links = engine.AppendUnique(source.Front.Links, id)
	}
	if err := ticket.SaveRecord(source); err != nil {
		return err
	}

	for _, id := range ids {
		target := targets[id]
		target.Front.Links = engine.AppendUnique(target.Front.Links, source.ID)
		if err := ticket.SaveRecord(target); err != nil {
			return err
		}
	}

	projectName, _ := resolvedProjectName(ctx)
	engine.AppendMutationLog(projectName, engine.MutationEntry{
		TicketID:      source.ID,
		Operation:     "link",
		Source:        sourceName,
		FieldsChanged: []string{"links"},
	})

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"ticket_id":     source.ID,
			"updated_field": "links",
			"values":        source.Front.Links,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "linked %s <-> %s\n", source.ID, strings.Join(ids, ", "))
	return nil
}

func runUnlink(ctx context, args []string) error {
	fs := flag.NewFlagSet("unlink", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	var sourceName string
	fs.StringVar(&sourceName, "source", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return fmt.Errorf("usage: tkt unlink <id> <target-id>")
	}

	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}

	source, err := ticket.LoadByID(dir, rest[0])
	if err != nil {
		return err
	}
	target, err := ticket.LoadByID(dir, rest[1])
	if err != nil {
		return err
	}

	source.Front.Links = engine.RemoveValue(source.Front.Links, target.ID)
	target.Front.Links = engine.RemoveValue(target.Front.Links, source.ID)

	if err := ticket.SaveRecord(source); err != nil {
		return err
	}
	if err := ticket.SaveRecord(target); err != nil {
		return err
	}

	projectName, _ := resolvedProjectName(ctx)
	engine.AppendMutationLog(projectName, engine.MutationEntry{
		TicketID:      source.ID,
		Operation:     "unlink",
		Source:        sourceName,
		FieldsChanged: []string{"links"},
	})

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"ticket_id":     source.ID,
			"updated_field": "links",
			"values":        source.Front.Links,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "unlinked %s x %s\n", source.ID, target.ID)
	return nil
}

func runDepTree(ctx context, args []string) error {
	fs := flag.NewFlagSet("dep tree", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	full := false
	fs.BoolVar(&full, "full", false, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("usage: tkt dep tree [--full] <id>")
	}

	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}
	root, err := ticket.LoadByID(dir, rest[0])
	if err != nil {
		return err
	}
	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}
	byID := engine.IndexByID(records)

	lines := engine.RenderDepTree(root.ID, byID, full)
	if ctx.json {
		nodes := make([]map[string]any, 0, len(lines))
		for _, line := range lines {
			nodes = append(nodes, map[string]any{"line": line})
		}
		return emitJSON(ctx, map[string]any{
			"root":  root.ID,
			"nodes": nodes,
		})
	}
	for _, line := range lines {
		_, _ = fmt.Fprintln(ctx.stdout, line)
	}
	return nil
}

func runDepCycle(ctx context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: tkt dep cycle")
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}
	byID := engine.IndexByID(records)

	openIDs := make([]string, 0)
	for _, record := range records {
		if record.Front.Status != "closed" {
			openIDs = append(openIDs, record.ID)
		}
	}
	sort.Strings(openIDs)

	color := map[string]int{}
	stack := make([]string, 0)
	stackIndex := map[string]int{}
	seen := map[string]struct{}{}
	cycles := make([]string, 0)

	var dfs func(id string)
	dfs = func(id string) {
		color[id] = 1
		stackIndex[id] = len(stack)
		stack = append(stack, id)

		record := byID[id]
		for _, dep := range record.Front.Deps {
			depRecord, ok := byID[dep]
			if !ok || depRecord.Front.Status == "closed" {
				continue
			}

			if color[dep] == 0 {
				dfs(dep)
				continue
			}
			if color[dep] == 1 {
				start := stackIndex[dep]
				cycle := append([]string{}, stack[start:]...)
				cycle = append(cycle, dep)
				key := canonicalCycleKey(cycle)
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					cycles = append(cycles, key)
				}
			}
		}

		stack = stack[:len(stack)-1]
		delete(stackIndex, id)
		color[id] = 2
	}

	for _, id := range openIDs {
		if color[id] == 0 {
			dfs(id)
		}
	}

	if len(cycles) == 0 {
		if ctx.json {
			return emitJSON(ctx, map[string]any{"cycles": [][]string{}})
		}
		_, _ = fmt.Fprintln(ctx.stdout, "No cycles found.")
		return nil
	}

	sort.Strings(cycles)
	if ctx.json {
		structured := make([][]string, 0, len(cycles))
		for _, cycle := range cycles {
			structured = append(structured, strings.Split(cycle, " -> "))
		}
		return emitJSON(ctx, map[string]any{"cycles": structured})
	}
	for _, cycle := range cycles {
		_, _ = fmt.Fprintln(ctx.stdout, cycle)
	}
	return nil
}

func runQuery(ctx context, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: tkt query [jq-filter]")
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })

	lines := make([]string, 0, len(records))
	for _, record := range records {
		raw, err := json.Marshal(engine.TicketToMap(record))
		if err != nil {
			return err
		}
		lines = append(lines, string(raw))
	}

	payload := strings.Join(lines, "\n")
	if payload != "" {
		payload += "\n"
	}

	filter := ""
	if len(args) > 0 {
		filter = strings.TrimSpace(args[0])
	}

	if filter == "" {
		if ctx.json {
			return emitJSON(ctx, map[string]any{
				"items":  ticketsToJSON(records),
				"filter": nil,
			})
		}
		_, _ = fmt.Fprint(ctx.stdout, payload)
		return nil
	}

	filtered, err := runJQFilter(payload, filter, ctx.stderr)
	if err != nil {
		return err
	}

	if ctx.json {
		items := make([]map[string]any, 0, len(filtered))
		for _, line := range filtered {
			var item map[string]any
			if err := json.Unmarshal([]byte(line), &item); err != nil {
				return err
			}
			items = append(items, item)
		}
		return emitJSON(ctx, map[string]any{
			"items":  items,
			"filter": filter,
		})
	}

	for _, line := range filtered {
		_, _ = fmt.Fprintln(ctx.stdout, line)
	}
	return nil
}

func runStats(ctx context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: tkt stats")
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}

	statuses := []string{"open", "in_progress", "needs_testing", "closed"}
	types := []string{"bug", "feature", "task", "epic", "chore"}
	byStatus := map[string]int{}
	byType := map[string]int{}
	byPriority := map[int]int{}

	for _, record := range records {
		byStatus[record.Front.Status]++
		byType[record.Front.Type]++
		byPriority[record.Front.Priority]++
	}

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"counts":      byStatus,
			"by_type":     byType,
			"by_priority": byPriority,
			"total":       len(records),
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "Total tickets: %d\n", len(records))
	_, _ = fmt.Fprintln(ctx.stdout, "By status:")
	for _, status := range statuses {
		_, _ = fmt.Fprintf(ctx.stdout, "  %s: %d\n", status, byStatus[status])
	}
	_, _ = fmt.Fprintln(ctx.stdout, "By type:")
	for _, typ := range types {
		_, _ = fmt.Fprintf(ctx.stdout, "  %s: %d\n", typ, byType[typ])
	}
	_, _ = fmt.Fprintln(ctx.stdout, "By priority:")
	for p := 0; p <= 4; p++ {
		_, _ = fmt.Fprintf(ctx.stdout, "  p%d: %d\n", p, byPriority[p])
	}
	return nil
}

func runTimeline(ctx context, args []string) error {
	fs := flag.NewFlagSet("timeline", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	weeks := 4
	fs.IntVar(&weeks, "weeks", 4, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if weeks <= 0 {
		return fmt.Errorf("--weeks must be > 0")
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}

	closedByWeek := map[string]int{}
	for _, record := range records {
		if record.Front.Status != "closed" {
			continue
		}
		created, err := time.Parse(time.RFC3339, record.Front.Created)
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
		count := closedByWeek[key]
		rows = append(rows, map[string]any{
			"week_start":   key,
			"closed_count": count,
		})
		if ctx.json {
			continue
		}
		_, _ = fmt.Fprintf(ctx.stdout, "%s\t%d\n", key, closedByWeek[key])
	}
	if ctx.json {
		return emitJSON(ctx, map[string]any{"weeks": rows})
	}
	return nil
}

func runWorkflow(ctx context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: tkt workflow")
	}

	workflow, err := project.LoadWorkflow()
	if err != nil {
		return err
	}

	if ctx.json {
		return emitJSON(ctx, map[string]any{"content": workflow.Content})
	}

	_, _ = fmt.Fprint(ctx.stdout, strings.TrimRight(workflow.Content, "\n"))
	_, _ = fmt.Fprintln(ctx.stdout)
	_, _ = fmt.Fprintln(ctx.stdout)
	if workflow.UsingDefault {
		_, _ = fmt.Fprintf(ctx.stdout, "(Source: embedded default; create %s or run `tkt init` to create it, then edit to customize)\n", workflow.PathDisplay)
		return nil
	}
	_, _ = fmt.Fprintf(ctx.stdout, "(Source: %s - edit to customize)\n", workflow.PathDisplay)
	return nil
}

func sortedRecordIDs(records map[string]ticket.Record) []string {
	ids := make([]string, 0, len(records))
	for id := range records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func canonicalCycleKey(cycle []string) string {
	if len(cycle) < 2 {
		return ""
	}

	loop := append([]string{}, cycle[:len(cycle)-1]...)
	best := append([]string{}, loop...)

	for i := 1; i < len(loop); i++ {
		rotated := append(append([]string{}, loop[i:]...), loop[:i]...)
		if strings.Join(rotated, "\x00") < strings.Join(best, "\x00") {
			best = rotated
		}
	}

	best = append(best, best[0])
	return strings.Join(best, " -> ")
}

func runJQFilter(payload string, filter string, stderr io.Writer) ([]string, error) {
	jqPath, err := exec.LookPath("jq")
	if err != nil {
		return nil, fmt.Errorf("jq is required for query filtering")
	}

	cmd := exec.Command(jqPath, "-c", fmt.Sprintf("select(%s)", filter))
	cmd.Stdin = bytes.NewBufferString(payload)
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	text := strings.TrimSpace(string(out))
	if text == "" {
		return []string{}, nil
	}
	return strings.Split(text, "\n"), nil
}
