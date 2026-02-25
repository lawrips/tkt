package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/journal"
	"github.com/lawrips/tkt/internal/ticket"
)

type listFilters struct {
	Status   string
	Type     string
	Priority int
	Assignee string
	Tag      string
	Parent   string
	Limit    int
	Sort     string
	Search   string
	OnlyOpen bool
}

func runShow(ctx context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: tkt show <id>")
	}

	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}
	record, err := ticket.LoadByID(dir, args[0])
	if err != nil {
		return err
	}

	projectName, _ := resolvedProjectName(ctx)
	links, _ := engine.ReadJournalEntries(projectName)
	allCommitLinks := engine.FilterJournalByTickets(links, []string{record.ID})

	if ctx.json {
		payload := ticketToJSON(record)
		payload["commit_links"] = allCommitLinks
		return emitJSON(ctx, payload)
	}

	raw, readErr := os.ReadFile(record.Path)
	if readErr != nil {
		return readErr
	}
	_, _ = fmt.Fprint(ctx.stdout, string(raw))

	recent := engine.LastNJournalEntries(allCommitLinks, 5)
	if len(recent) > 0 {
		_, _ = fmt.Fprintln(ctx.stdout)
		_, _ = fmt.Fprintln(ctx.stdout, "## Recent Commits")
		for _, entry := range recent {
			stats := ""
			if entry.LinesAdded > 0 || entry.LinesRemoved > 0 {
				stats = fmt.Sprintf("\t+%d -%d, %d file(s)", entry.LinesAdded, entry.LinesRemoved, len(entry.FilesChanged))
			}
			_, _ = fmt.Fprintf(ctx.stdout, "- %s\t%s\t%s\t%s%s\n",
				engine.ShortSHA(entry.SHA),
				entry.TS,
				entry.Action,
				journal.FirstLine(entry.Msg),
				stats,
			)
		}
	}
	return nil
}

func runCreate(ctx context, args []string) error {
	opts, title, source, err := parseCreateArgs(args)
	if err != nil {
		return err
	}

	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}
	if err := ticket.EnsureDir(dir); err != nil {
		return err
	}

	id := opts.CustomID
	if id == "" {
		var err error
		id, err = ticket.GenerateID(dir)
		if err != nil {
			return err
		}
	} else {
		if strings.ContainsAny(id, " \t\r\n") {
			return fmt.Errorf("invalid ticket id %q", id)
		}
		if _, err := os.Stat(filepath.Join(dir, id+".md")); err == nil {
			return fmt.Errorf("ticket id %q already exists", id)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	record := ticket.Record{
		ID:   id,
		Path: filepath.Join(dir, id+".md"),
		Front: ticket.Frontmatter{
			ID:          id,
			Status:      "open",
			Deps:        []string{},
			Links:       []string{},
			Created:     time.Now().UTC().Format(time.RFC3339),
			Type:        opts.Type,
			Priority:    opts.Priority,
			Assignee:    opts.Assignee,
			Parent:      opts.Parent,
			Tags:        engine.ParseCSV(opts.Tags),
			ExternalRef: opts.ExternalRef,
			Extra:       map[string]ticket.ExtraField{},
		},
		Body: ticket.Body{
			Title:              title,
			Description:        opts.Description,
			Design:             opts.Design,
			AcceptanceCriteria: opts.Acceptance,
		},
	}

	if err := ticket.SaveRecord(record); err != nil {
		return err
	}

	projectName, _ := resolvedProjectName(ctx)
	engine.AppendMutationLog(projectName, engine.MutationEntry{
		TicketID:      id,
		Operation:     "create",
		Source:        source,
		FieldsChanged: []string{"title", "status", "type", "priority"},
	})

	if ctx.json {
		return emitJSON(ctx, ticketToJSON(record))
	}

	_, _ = fmt.Fprintf(ctx.stdout, "created %s\n", id)
	return nil
}

func runEdit(ctx context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: tkt edit <id> [options]")
	}

	id := args[0]
	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	var opts editOptions
	var source string
	fs.StringVar(&opts.Title, "title", "", "")
	fs.StringVar(&opts.Description, "d", "", "")
	fs.StringVar(&opts.Description, "description", "", "")
	fs.StringVar(&opts.Design, "design", "", "")
	fs.StringVar(&opts.Acceptance, "acceptance", "", "")
	fs.StringVar(&opts.Type, "t", "", "")
	fs.StringVar(&opts.Type, "type", "", "")
	fs.IntVar(&opts.Priority, "p", -1, "")
	fs.IntVar(&opts.Priority, "priority", -1, "")
	fs.StringVar(&opts.Status, "s", "", "")
	fs.StringVar(&opts.Status, "status", "", "")
	fs.StringVar(&opts.Assignee, "a", "", "")
	fs.StringVar(&opts.Assignee, "assignee", "", "")
	fs.StringVar(&opts.Parent, "parent", "", "")
	fs.StringVar(&opts.Tags, "tags", "", "")
	fs.StringVar(&opts.ExternalRef, "external-ref", "", "")
	fs.StringVar(&source, "source", "", "")

	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	record, err := ticket.LoadByID(dir, id)
	if err != nil {
		return err
	}

	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})

	changed := make([]string, 0)
	if flagWasSet(visited, "title") {
		record.Body.Title = opts.Title
		changed = append(changed, "title")
	}
	if flagWasSet(visited, "d", "description") {
		record.Body.Description = opts.Description
		changed = append(changed, "description")
	}
	if flagWasSet(visited, "design") {
		record.Body.Design = opts.Design
		changed = append(changed, "design")
	}
	if flagWasSet(visited, "acceptance") {
		record.Body.AcceptanceCriteria = opts.Acceptance
		changed = append(changed, "acceptance_criteria")
	}
	if opts.Type != "" {
		record.Front.Type = opts.Type
		changed = append(changed, "type")
	}
	if opts.Priority >= 0 {
		record.Front.Priority = opts.Priority
		changed = append(changed, "priority")
	}
	if opts.Status != "" {
		record.Front.Status = opts.Status
		changed = append(changed, "status")
	}
	if flagWasSet(visited, "a", "assignee") {
		record.Front.Assignee = opts.Assignee
		changed = append(changed, "assignee")
	}
	if flagWasSet(visited, "parent") {
		record.Front.Parent = opts.Parent
		changed = append(changed, "parent")
	}
	if flagWasSet(visited, "tags") {
		record.Front.Tags = engine.ParseCSV(opts.Tags)
		changed = append(changed, "tags")
	}
	if flagWasSet(visited, "external-ref") {
		record.Front.ExternalRef = opts.ExternalRef
		changed = append(changed, "external_ref")
	}

	if err := ticket.SaveRecord(record); err != nil {
		return err
	}

	projectName, _ := resolvedProjectName(ctx)
	engine.AppendMutationLog(projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "edit",
		Source:        source,
		FieldsChanged: changed,
	})

	if ctx.json {
		return emitJSON(ctx, ticketToJSON(record))
	}

	_, _ = fmt.Fprintf(ctx.stdout, "updated %s\n", record.ID)
	return nil
}

func flagWasSet(visited map[string]bool, names ...string) bool {
	for _, name := range names {
		if visited[name] {
			return true
		}
	}
	return false
}

func runDelete(ctx context, args []string) error {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	var source string
	fs.StringVar(&source, "source", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("usage: tkt delete <id> [id...]")
	}

	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}

	projectName, _ := resolvedProjectName(ctx)
	deleted := make([]string, 0, len(rest))
	for _, arg := range rest {
		path, err := ticket.ResolvePath(dir, arg)
		if err != nil {
			return err
		}
		id := strings.TrimSuffix(filepath.Base(path), ".md")
		if err := os.Remove(path); err != nil {
			return err
		}
		deleted = append(deleted, id)
		engine.AppendMutationLog(projectName, engine.MutationEntry{
			TicketID:  id,
			Operation: "delete",
			Source:    source,
		})
		if !ctx.json {
			_, _ = fmt.Fprintf(ctx.stdout, "deleted %s\n", id)
		}
	}

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"deleted":   deleted,
			"not_found": []string{},
		})
	}

	return nil
}

func runLS(ctx context, args []string) error {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	filters := listFilters{Priority: -1, OnlyOpen: true}
	fs.StringVar(&filters.Status, "status", "", "")
	fs.StringVar(&filters.Type, "t", "", "")
	fs.StringVar(&filters.Type, "type", "", "")
	fs.IntVar(&filters.Priority, "P", -1, "")
	fs.IntVar(&filters.Priority, "priority", -1, "")
	fs.StringVar(&filters.Assignee, "a", "", "")
	fs.StringVar(&filters.Assignee, "assignee", "", "")
	fs.StringVar(&filters.Tag, "T", "", "")
	fs.StringVar(&filters.Tag, "tag", "", "")
	fs.StringVar(&filters.Parent, "parent", "", "")
	fs.StringVar(&filters.Sort, "sort", "", "")
	fs.IntVar(&filters.Limit, "limit", 0, "")
	fs.StringVar(&filters.Search, "search", "", "")
	_ = fs.String("group-by", "", "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if filters.Search == "" {
		if rest := fs.Args(); len(rest) > 0 {
			filters.Search = strings.Join(rest, " ")
		}
	}

	if filters.Status != "" {
		filters.OnlyOpen = false
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}

	filtered := filterRecords(records, filters)
	if filters.Sort != "" {
		if err := engine.SortRecords(filtered, filters.Sort); err != nil {
			return err
		}
	}
	filtered = engine.LimitRecords(filtered, filters.Limit)
	return printRecords(ctx, filtered)
}

func runClosed(ctx context, args []string) error {
	fs := flag.NewFlagSet("closed", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	filters := listFilters{Status: "closed", Priority: -1, Limit: 20}
	fs.IntVar(&filters.Limit, "limit", 20, "")
	fs.StringVar(&filters.Sort, "sort", "", "")
	fs.StringVar(&filters.Assignee, "a", "", "")
	fs.StringVar(&filters.Assignee, "assignee", "", "")
	fs.StringVar(&filters.Tag, "T", "", "")
	fs.StringVar(&filters.Tag, "tag", "", "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}
	filtered := filterRecords(records, filters)
	if filters.Sort != "" {
		if err := engine.SortRecords(filtered, filters.Sort); err != nil {
			return err
		}
	}
	filtered = engine.LimitRecords(filtered, filters.Limit)
	return printRecords(ctx, filtered)
}

func runReady(ctx context, args []string) error {
	fs := flag.NewFlagSet("ready", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	filters := listFilters{Status: "open", Priority: -1}
	skipParentChecks := false
	fs.StringVar(&filters.Assignee, "a", "", "")
	fs.StringVar(&filters.Assignee, "assignee", "", "")
	fs.StringVar(&filters.Tag, "T", "", "")
	fs.StringVar(&filters.Tag, "tag", "", "")
	fs.BoolVar(&skipParentChecks, "open", false, "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}

	byID := engine.IndexByID(records)
	base := filterRecords(records, filters)

	out := make([]ticket.Record, 0)
	for _, record := range base {
		if engine.HasOpenDeps(record, byID) {
			continue
		}
		if skipParentChecks {
			out = append(out, record)
			continue
		}
		if record.Front.Parent == "" {
			out = append(out, record)
			continue
		}
		parent, ok := byID[record.Front.Parent]
		if ok && parent.Front.Status == "in_progress" {
			out = append(out, record)
		}
	}

	return printRecords(ctx, out)
}

func runBlocked(ctx context, args []string) error {
	fs := flag.NewFlagSet("blocked", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)

	filters := listFilters{Status: "open", Priority: -1}
	fs.StringVar(&filters.Assignee, "a", "", "")
	fs.StringVar(&filters.Assignee, "assignee", "", "")
	fs.StringVar(&filters.Tag, "T", "", "")
	fs.StringVar(&filters.Tag, "tag", "", "")

	if err := fs.Parse(args); err != nil {
		return err
	}

	records, err := listRecordsWithFallback(ctx)
	if err != nil {
		return err
	}

	byID := engine.IndexByID(records)
	base := filterRecords(records, filters)
	out := make([]ticket.Record, 0)
	for _, record := range base {
		if engine.HasOpenDeps(record, byID) {
			out = append(out, record)
		}
	}
	return printRecords(ctx, out)
}

type ticketOptions struct {
	Description string
	Design      string
	Acceptance  string
	Type        string
	Priority    int
	Assignee    string
	CustomID    string
	Parent      string
	Tags        string
	ExternalRef string
}

type editOptions struct {
	Title       string
	Description string
	Design      string
	Acceptance  string
	Type        string
	Priority    int
	Status      string
	Assignee    string
	Parent      string
	Tags        string
	ExternalRef string
}

func parseCreateArgs(args []string) (ticketOptions, string, string, error) {
	opts := ticketOptions{
		Type:     "task",
		Priority: 2,
	}
	titleParts := make([]string, 0)
	source := ""

	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "--" {
			titleParts = append(titleParts, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(token, "-") || token == "-" {
			titleParts = append(titleParts, token)
			continue
		}

		name := token
		value := ""
		if strings.Contains(token, "=") {
			parts := strings.SplitN(token, "=", 2)
			name, value = parts[0], parts[1]
		}

		consumeValue := func() (string, error) {
			if value != "" {
				return value, nil
			}
			i++
			if i >= len(args) {
				return "", fmt.Errorf("missing value for %s", name)
			}
			return args[i], nil
		}

		switch name {
		case "-d", "--description":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			opts.Description = v
		case "--design":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			opts.Design = v
		case "--acceptance":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			opts.Acceptance = v
		case "-t", "--type":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			opts.Type = v
		case "-p", "--priority":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return opts, "", "", fmt.Errorf("invalid priority %q", v)
			}
			opts.Priority = n
		case "-a", "--assignee":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			opts.Assignee = v
		case "--id":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			opts.CustomID = v
		case "--parent":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			opts.Parent = v
		case "--tags":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			opts.Tags = v
		case "--external-ref":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			opts.ExternalRef = v
		case "--source":
			v, err := consumeValue()
			if err != nil {
				return opts, "", "", err
			}
			source = v
		default:
			return opts, "", "", fmt.Errorf("unknown flag: %s", name)
		}
	}

	title := strings.TrimSpace(strings.Join(titleParts, " "))
	if title == "" {
		return opts, "", "", fmt.Errorf("create requires a title")
	}
	return opts, title, source, nil
}

func filterRecords(records []ticket.Record, filters listFilters) []ticket.Record {
	out := make([]ticket.Record, 0, len(records))
	for _, record := range records {
		if filters.OnlyOpen && record.Front.Status != "open" {
			continue
		}
		if filters.Status != "" && record.Front.Status != filters.Status {
			continue
		}
		if filters.Type != "" && record.Front.Type != filters.Type {
			continue
		}
		if filters.Priority >= 0 && record.Front.Priority != filters.Priority {
			continue
		}
		if filters.Assignee != "" && record.Front.Assignee != filters.Assignee {
			continue
		}
		if filters.Tag != "" && !engine.Contains(record.Front.Tags, filters.Tag) {
			continue
		}
		if filters.Parent != "" && record.Front.Parent != filters.Parent {
			continue
		}
		if filters.Search != "" {
			q := strings.ToLower(filters.Search)
			id := strings.ToLower(record.ID)
			title := strings.ToLower(record.Body.Title)
			if !strings.Contains(id, q) && !strings.Contains(title, q) {
				continue
			}
		}
		out = append(out, record)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})

	return out
}

func printRecords(ctx context, records []ticket.Record) error {
	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"items": ticketSummariesToJSON(records),
			"total": len(records),
		})
	}

	if len(records) == 0 {
		_, _ = fmt.Fprintln(ctx.stdout, "No tickets.")
		return nil
	}

	for _, record := range records {
		title := strings.TrimSpace(record.Body.Title)
		if title == "" {
			title = "(untitled)"
		}
		_, _ = fmt.Fprintf(ctx.stdout, "%s\t%s\t%s\tp%d\t%s\n",
			record.ID,
			record.Front.Status,
			record.Front.Type,
			record.Front.Priority,
			title,
		)
	}
	return nil
}

func runAddNote(ctx context, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: tkt add-note <id> [text]")
	}

	ticketID := args[0]
	fs := flag.NewFlagSet("add-note", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	var source string
	fs.StringVar(&source, "source", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	note := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(note) == "" {
		raw, err := io.ReadAll(ctx.stdin)
		if err != nil {
			return err
		}
		note = strings.TrimSpace(string(raw))
	}
	if strings.TrimSpace(note) == "" {
		return errors.New("note text is required (arg or stdin)")
	}

	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}
	record, err := ticket.LoadByID(dir, ticketID)
	if err != nil {
		return err
	}

	ts := time.Now().UTC().Format(time.RFC3339)
	existing := engine.FindSection(record.Body.OtherSections, "Notes")
	header := fmt.Sprintf("**%s**", ts)
	if strings.TrimSpace(source) != "" {
		header = fmt.Sprintf("**%s [%s]**", ts, source)
	}
	entry := fmt.Sprintf("%s\n\n%s", header, note)
	if strings.TrimSpace(existing.Content) != "" {
		existing.Content = strings.TrimSpace(existing.Content) + "\n\n" + entry
	} else {
		existing.Content = entry
	}

	record.Body.OtherSections = engine.UpsertSection(record.Body.OtherSections, existing)
	if err := ticket.SaveRecord(record); err != nil {
		return err
	}

	projectName, _ := resolvedProjectName(ctx)
	engine.AppendMutationLog(projectName, engine.MutationEntry{
		TicketID:      record.ID,
		Operation:     "add-note",
		Source:        source,
		FieldsChanged: []string{"notes"},
	})

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"ticket_id": record.ID,
			"note": map[string]string{
				"at":   ts,
				"text": note,
			},
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "note added to %s\n", record.ID)
	return nil
}
