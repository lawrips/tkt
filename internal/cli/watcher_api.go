package cli

import (
	"os"
	"path/filepath"

	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

func resolveTicketDir(ctx context) (string, error) {
	cfg, err := project.Load()
	if err != nil {
		return ticket.DefaultDir, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ticket.DefaultDir, nil
	}

	projectName, _ := project.ResolveName(cfg, cwd, ctx.projectOverride)
	if projectName == "" {
		return ticket.DefaultDir, nil
	}

	entry, ok := cfg.Projects[projectName]
	if !ok {
		return ticket.DefaultDir, nil
	}
	if entry.Store == "central" {
		return centralProjectDir(projectName)
	}
	if entry.Path != "" {
		return filepath.Join(entry.Path, ".tickets"), nil
	}
	return ticket.DefaultDir, nil
}

func listRecordsWithFallback(ctx context) ([]ticket.Record, error) {
	dir, err := resolveTicketDir(ctx)
	if err != nil {
		return nil, err
	}
	return ticket.List(dir)
}
