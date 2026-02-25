package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/lawrips/tkt/internal/engine"
	"github.com/lawrips/tkt/internal/project"
	"github.com/lawrips/tkt/internal/ticket"
)

type jsonEnvelope struct {
	Meta jsonMeta `json:"meta"`
	Data any      `json:"data"`
}

type jsonMeta struct {
	Command     string `json:"command"`
	Project     string `json:"project"`
	GeneratedAt string `json:"generated_at"`
	Version     string `json:"version"`
}

func emitJSON(ctx context, data any) error {
	projectName := "unknown"
	if cwd, err := os.Getwd(); err == nil {
		cfg, _ := project.Load()
		if resolved, _ := project.ResolveName(cfg, cwd, ctx.projectOverride); resolved != "" {
			projectName = resolved
		}
	}

	payload := jsonEnvelope{
		Meta: jsonMeta{
			Command:     ctx.command,
			Project:     projectName,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Version:     "v2",
		},
		Data: data,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(ctx.stdout, string(raw))
	return nil
}

func ticketToJSON(record ticket.Record) map[string]any {
	return engine.TicketToMap(record)
}

func ticketsToJSON(records []ticket.Record) []map[string]any {
	return engine.TicketsToMaps(records)
}

func ticketSummaryToJSON(record ticket.Record) map[string]any {
	return engine.TicketSummaryToMap(record)
}

func ticketSummariesToJSON(records []ticket.Record) []map[string]any {
	return engine.TicketSummariesToMaps(records)
}
