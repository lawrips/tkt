package engine

import (
	"sort"
	"time"

	"github.com/lawrips/tkt/internal/ticket"
)

// StatsCounts aggregates ticket counts for a project. Ready and Blocked use
// the same semantics as list filtering: an open ticket is ready when all its
// deps are closed, blocked when at least one dep is open or missing.
type StatsCounts struct {
	Total      int            `json:"total"`
	ByStatus   map[string]int `json:"by_status"`
	ByType     map[string]int `json:"by_type"`
	ByPriority map[int]int    `json:"by_priority"`
	Ready      int            `json:"ready"`
	Blocked    int            `json:"blocked"`
}

// ComputeStats tallies status/type/priority distributions and ready/blocked
// counts across all records.
func ComputeStats(records []ticket.Record) StatsCounts {
	stats := StatsCounts{
		Total:      len(records),
		ByStatus:   map[string]int{},
		ByType:     map[string]int{},
		ByPriority: map[int]int{},
	}
	byID := IndexByID(records)
	for _, record := range records {
		stats.ByStatus[record.Front.Status]++
		stats.ByType[record.Front.Type]++
		stats.ByPriority[record.Front.Priority]++
		if record.Front.Status != "open" {
			continue
		}
		if HasOpenDeps(record, byID) {
			stats.Blocked++
		} else {
			stats.Ready++
		}
	}
	return stats
}

// TimelineWeek is one weekly bucket of closed tickets.
type TimelineWeek struct {
	WeekStart   string   `json:"week_start"`
	ClosedCount int      `json:"closed_count"`
	TicketIDs   []string `json:"ticket_ids"`
}

// ClosedByWeek buckets closed tickets by close time and returns the trailing N
// weeks ending at the week containing now, oldest first. Close time uses
// frontmatter closed_at, then commit journal close entries, then file modtime,
// then created as a historical fallback.
func ClosedByWeek(records []ticket.Record, journalEntries []CommitJournalEntry, weeks int, now time.Time) []TimelineWeek {
	closedByWeek := map[string][]string{}
	closeTimes := journalCloseTimes(journalEntries)
	for _, record := range records {
		if record.Front.Status != "closed" {
			continue
		}
		closedAt, ok := closeTimeForRecord(record, closeTimes)
		if !ok {
			continue
		}
		key := Monday(closedAt).Format("2006-01-02")
		closedByWeek[key] = append(closedByWeek[key], record.ID)
	}

	currentWeek := Monday(now.UTC())
	rows := make([]TimelineWeek, 0, weeks)
	for i := weeks - 1; i >= 0; i-- {
		start := currentWeek.AddDate(0, 0, -7*i)
		key := start.Format("2006-01-02")
		ids := closedByWeek[key]
		if ids == nil {
			ids = []string{}
		}
		sort.Strings(ids)
		rows = append(rows, TimelineWeek{WeekStart: key, ClosedCount: len(ids), TicketIDs: ids})
	}
	return rows
}

func journalCloseTimes(entries []CommitJournalEntry) map[string]time.Time {
	out := map[string]time.Time{}
	for _, entry := range entries {
		if entry.Action != "close" {
			continue
		}
		t, err := time.Parse(time.RFC3339, entry.TS)
		if err != nil {
			continue
		}
		if existing, ok := out[entry.Ticket]; !ok || t.After(existing) {
			out[entry.Ticket] = t
		}
	}
	return out
}

func closeTimeForRecord(record ticket.Record, journalCloseTimes map[string]time.Time) (time.Time, bool) {
	if record.Front.ClosedAt != "" {
		if t, err := time.Parse(time.RFC3339, record.Front.ClosedAt); err == nil {
			return t.UTC(), true
		}
	}
	if t, ok := journalCloseTimes[record.ID]; ok {
		return t.UTC(), true
	}
	if !record.ModTime.IsZero() {
		return record.ModTime.UTC(), true
	}
	if record.Front.Created != "" {
		if t, err := time.Parse(time.RFC3339, record.Front.Created); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// EpicProgress summarizes completion for one epic and its direct children.
type EpicProgress struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Status           string         `json:"status"`
	Priority         int            `json:"priority"`
	TotalChildren    int            `json:"total_children"`
	ClosedChildren   int            `json:"closed_children"`
	ChildrenByStatus map[string]int `json:"children_by_status"`
}

// ComputeEpicProgress returns progress for every epic in the record set,
// sorted by epic ID. Children are matched by direct parent reference.
func ComputeEpicProgress(records []ticket.Record) []EpicProgress {
	epics := make([]EpicProgress, 0)
	index := map[string]int{}
	for _, record := range records {
		if record.Front.Type != "epic" {
			continue
		}
		index[record.ID] = len(epics)
		epics = append(epics, EpicProgress{
			ID:               record.ID,
			Title:            record.Body.Title,
			Status:           record.Front.Status,
			Priority:         record.Front.Priority,
			ChildrenByStatus: map[string]int{},
		})
	}
	for _, record := range records {
		parent := record.Front.Parent
		if parent == "" {
			continue
		}
		at, ok := index[parent]
		if !ok {
			continue
		}
		epics[at].TotalChildren++
		epics[at].ChildrenByStatus[record.Front.Status]++
		if record.Front.Status == "closed" {
			epics[at].ClosedChildren++
		}
	}
	sort.Slice(epics, func(i, j int) bool { return epics[i].ID < epics[j].ID })
	return epics
}
