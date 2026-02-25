package engine

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ReadJournalEntries loads all commit journal entries for the given project.
func ReadJournalEntries(projectName string) ([]CommitJournalEntry, error) {
	if strings.TrimSpace(projectName) == "" {
		return []CommitJournalEntry{}, nil
	}
	path, err := JournalPath(projectName)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []CommitJournalEntry{}, nil
		}
		return nil, err
	}
	defer f.Close()

	out := make([]CommitJournalEntry, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry CommitJournalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		out = append(out, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// FilterJournalByTickets returns entries whose Ticket is in the given set of IDs.
func FilterJournalByTickets(entries []CommitJournalEntry, ids []string) []CommitJournalEntry {
	set := map[string]struct{}{}
	for _, id := range ids {
		set[id] = struct{}{}
	}
	out := make([]CommitJournalEntry, 0)
	for _, entry := range entries {
		if _, ok := set[entry.Ticket]; ok {
			out = append(out, entry)
		}
	}
	return out
}

// CountJournalForTicket counts journal entries for a specific ticket ID.
func CountJournalForTicket(entries []CommitJournalEntry, ticketID string) int {
	count := 0
	for _, entry := range entries {
		if entry.Ticket == ticketID {
			count++
		}
	}
	return count
}

// LastNJournalEntries returns the last n entries (or all if fewer).
func LastNJournalEntries(entries []CommitJournalEntry, n int) []CommitJournalEntry {
	if len(entries) <= n {
		return entries
	}
	return entries[len(entries)-n:]
}

// AppendMutationLog appends a mutation entry to the project's mutations.jsonl.
func AppendMutationLog(projectName string, entry MutationEntry) {
	if strings.TrimSpace(projectName) == "" {
		return
	}
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	path, err := MutationLogPath(projectName)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	_ = enc.Encode(entry)
}
