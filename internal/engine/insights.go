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
	WeekStart   string `json:"week_start"`
	ClosedCount int    `json:"closed_count"`
}

// ClosedByWeek buckets closed tickets by the Monday of their created week and
// returns the trailing N weeks ending at the week containing now, oldest
// first. Buckets outside the window are dropped.
func ClosedByWeek(records []ticket.Record, weeks int, now time.Time) []TimelineWeek {
	closedByWeek := map[string]int{}
	for _, record := range records {
		if record.Front.Status != "closed" {
			continue
		}
		created, err := time.Parse(time.RFC3339, record.Front.Created)
		if err != nil {
			continue
		}
		closedByWeek[Monday(created).Format("2006-01-02")]++
	}

	currentWeek := Monday(now.UTC())
	rows := make([]TimelineWeek, 0, weeks)
	for i := weeks - 1; i >= 0; i-- {
		start := currentWeek.AddDate(0, 0, -7*i)
		key := start.Format("2006-01-02")
		rows = append(rows, TimelineWeek{WeekStart: key, ClosedCount: closedByWeek[key]})
	}
	return rows
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
