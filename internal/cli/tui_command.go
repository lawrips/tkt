package cli

import (
	"io"
	"path/filepath"
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/journal"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
	"github.com/lawrips/tkt/internal/tui"
)

func runTUI(ctx context, args []string) error {
	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return err
	}
	projectName, _ := resolvedProjectName(ctx)

	// Load all project names for the picker.
	projectNames := loadProjectNames()

	// Check if the resolved name is actually a registered project.
	// ResolveName has fallbacks (git remote, dir name) that return non-empty
	// names even from random directories — treat unregistered names as empty.
	registered := false
	for _, n := range projectNames {
		if n == projectName {
			registered = true
			break
		}
	}
	if !registered {
		projectName = ""
		dir = ticket.DefaultDir
		// Auto-select the only registered project, or let the TUI
		// show the picker for multiple projects.
		if len(projectNames) == 1 {
			projectName = projectNames[0]
			dir = resolveProjectDirByName(projectName)
		}
	}

	for {
		currentProject := projectName
		runner := func(args []string) error {
			if currentProject != "" {
				args = append([]string{"--project", currentProject}, args...)
			} else if ctx.projectOverride != "" {
				args = append([]string{"--project", ctx.projectOverride}, args...)
			}
			return Run(args, io.Discard, io.Discard)
		}
		factory := buildCommitLoaderFactory(currentProject)
		m := tui.New(dir, projectName, runner, factory, projectNames)
		p := tea.NewProgram(m, tea.WithAltScreen())
		result, err := p.Run()
		if err != nil {
			return err
		}
		final := result.(tui.Model)
		if final.SwitchTo == "" {
			return nil
		}
		// Switch to the selected project.
		projectName = final.SwitchTo
		dir = resolveProjectDirByName(projectName)
	}
}

func loadProjectNames() []string {
	cfg, err := project.Load()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func resolveProjectDirByName(name string) string {
	cfg, err := project.Load()
	if err != nil {
		return ticket.DefaultDir
	}
	entry, ok := cfg.Projects[name]
	if !ok {
		return ticket.DefaultDir
	}
	if entry.Store == "central" {
		dir, err := centralProjectDir(name)
		if err != nil {
			return ticket.DefaultDir
		}
		return dir
	}
	if entry.Path != "" {
		return filepath.Join(entry.Path, ".tickets")
	}
	return ticket.DefaultDir
}

func buildCommitLoaderFactory(projectName string) tui.CommitLoaderFactory {
	return func() tui.CommitLoader {
		engineEntries, err := engine.ReadJournalEntries(projectName)
		if err != nil {
			return func(string) []journal.Entry { return nil }
		}
		entries := toJournalEntries(engineEntries)
		grouped := journal.GroupByTicket(entries)
		return func(ticketID string) []journal.Entry {
			return grouped[ticketID]
		}
	}
}
