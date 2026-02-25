package engine

import "github.com/lawrips/tkt/internal/ticket"

// CommitJournalEntry represents one line in commits.jsonl.
type CommitJournalEntry struct {
	SHA          string   `json:"sha"`
	Ticket       string   `json:"ticket"`
	Repo         string   `json:"repo"`
	TS           string   `json:"ts"`
	Msg          string   `json:"msg"`
	Author       string   `json:"author"`
	Action       string   `json:"action"`
	FilesChanged []string `json:"files_changed,omitempty"`
	LinesAdded   int      `json:"lines_added,omitempty"`
	LinesRemoved int      `json:"lines_removed,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	WorkStarted  string   `json:"work_started,omitempty"`
	WorkEnded    string   `json:"work_ended,omitempty"`
	DurationSecs int      `json:"duration_seconds,omitempty"`
}

// MutationEntry represents one line in mutations.jsonl.
type MutationEntry struct {
	Timestamp     string   `json:"timestamp"`
	TicketID      string   `json:"ticket_id"`
	Operation     string   `json:"operation"`
	Source        string   `json:"source,omitempty"`
	FieldsChanged []string `json:"fields_changed,omitempty"`
}

// Section re-exports ticket.Section for convenience.
type Section = ticket.Section
