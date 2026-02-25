package engine

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/lawrips/tkt/internal/ticket"
)

// IndexByID builds a map from ticket ID to Record.
func IndexByID(records []ticket.Record) map[string]ticket.Record {
	out := make(map[string]ticket.Record, len(records))
	for _, record := range records {
		out[record.ID] = record
	}
	return out
}

// HasOpenDeps returns true if any dep of the record is not closed.
func HasOpenDeps(record ticket.Record, byID map[string]ticket.Record) bool {
	for _, depID := range record.Front.Deps {
		dep, ok := byID[depID]
		if !ok || dep.Front.Status != "closed" {
			return true
		}
	}
	return false
}

// AppendUnique appends value to the slice if not already present.
func AppendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// RemoveValue removes all occurrences of target from the slice.
func RemoveValue(values []string, target string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

// Contains checks if target exists in items.
func Contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// ParseCSV splits a comma-separated string, trims whitespace, and drops empties.
func ParseCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		item := strings.TrimSpace(p)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// ResolveRecordFromList finds a record by exact or partial ID match.
func ResolveRecordFromList(records []ticket.Record, query string) (ticket.Record, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return ticket.Record{}, false
	}

	for _, record := range records {
		if record.ID == query {
			return record, true
		}
	}

	matches := make([]ticket.Record, 0)
	for _, record := range records {
		if strings.HasPrefix(record.ID, query) || strings.Contains(record.ID, query) {
			matches = append(matches, record)
		}
	}
	if len(matches) != 1 {
		return ticket.Record{}, false
	}
	return matches[0], true
}

// IDsFromRecords extracts IDs from a slice of records.
func IDsFromRecords(records []ticket.Record) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.ID)
	}
	return out
}

// TicketToMap converts a record to a map with snake_case keys for JSON output.
func TicketToMap(record ticket.Record) map[string]any {
	deps := record.Front.Deps
	if deps == nil {
		deps = []string{}
	}
	links := record.Front.Links
	if links == nil {
		links = []string{}
	}
	tags := record.Front.Tags
	if tags == nil {
		tags = []string{}
	}

	payload := map[string]any{
		"id":                  record.ID,
		"title":               record.Body.Title,
		"status":              record.Front.Status,
		"type":                record.Front.Type,
		"priority":            record.Front.Priority,
		"assignee":            record.Front.Assignee,
		"parent":              record.Front.Parent,
		"deps":                deps,
		"links":               links,
		"tags":                tags,
		"created":             record.Front.Created,
		"external_ref":        record.Front.ExternalRef,
		"description":         record.Body.Description,
		"design":              record.Body.Design,
		"acceptance_criteria": record.Body.AcceptanceCriteria,
	}

	for _, section := range record.Body.OtherSections {
		key := ToSnakeCase(section.Heading)
		if key == "" {
			continue
		}
		payload[key] = section.Content
	}

	return payload
}

// TicketSummaryToMap converts a record to a lightweight summary map for list views.
func TicketSummaryToMap(record ticket.Record) map[string]any {
	return map[string]any{
		"id":       record.ID,
		"title":    record.Body.Title,
		"status":   record.Front.Status,
		"type":     record.Front.Type,
		"priority": record.Front.Priority,
	}
}

// TicketsToMaps converts a slice of records to maps.
func TicketsToMaps(records []ticket.Record) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, TicketToMap(record))
	}
	return out
}

// TicketSummariesToMaps converts a slice of records to lightweight summary maps.
func TicketSummariesToMaps(records []ticket.Record) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, TicketSummaryToMap(record))
	}
	return out
}

// RenderDepTree produces indented lines showing the dependency tree.
func RenderDepTree(rootID string, byID map[string]ticket.Record, full bool) []string {
	lines := make([]string, 0)
	path := map[string]bool{}

	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		record, ok := byID[id]
		prefix := strings.Repeat("  ", depth)
		if !ok {
			lines = append(lines, fmt.Sprintf("%s%s [missing]", prefix, id))
			return
		}

		title := strings.TrimSpace(record.Body.Title)
		if title == "" {
			title = "(untitled)"
		}
		line := fmt.Sprintf("%s%s [%s] %s", prefix, record.ID, record.Front.Status, title)
		if depth > 0 && path[id] {
			lines = append(lines, line+" (cycle)")
			return
		}
		lines = append(lines, line)

		path[id] = true
		for _, depID := range record.Front.Deps {
			depRecord, ok := byID[depID]
			if ok && depRecord.Front.Status == "closed" && !full {
				continue
			}
			walk(depID, depth+1)
		}
		delete(path, id)
	}

	walk(rootID, 0)
	return lines
}

// WindowStart returns the start time for a "today" or "week" window.
func WindowStart(window string, now time.Time) time.Time {
	now = now.UTC()
	if window == "today" {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
	return now.Add(-7 * 24 * time.Hour)
}

// Monday returns the Monday of the week containing t.
func Monday(t time.Time) time.Time {
	t = t.UTC()
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()-(weekday-1), 0, 0, 0, 0, time.UTC)
}

// ShortSHA truncates a SHA to 7 characters.
func ShortSHA(sha string) string {
	if len(sha) <= 7 {
		return sha
	}
	return sha[:7]
}

// FindSection returns the section with the given heading, or a new empty one.
func FindSection(sections []ticket.Section, heading string) ticket.Section {
	for _, section := range sections {
		if strings.EqualFold(section.Heading, heading) {
			return section
		}
	}
	return ticket.Section{Heading: heading}
}

// UpsertSection replaces a section by heading or appends it.
func UpsertSection(sections []ticket.Section, replacement ticket.Section) []ticket.Section {
	for i, section := range sections {
		if strings.EqualFold(section.Heading, replacement.Heading) {
			sections[i] = replacement
			return sections
		}
	}
	return append(sections, replacement)
}

// ToSnakeCase converts a heading string to snake_case.
func ToSnakeCase(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}

	return strings.Trim(b.String(), "_")
}

// SortRecords sorts records in place by the given field.
// Append ":desc" for descending order (e.g. "created:desc").
// Default is ascending. Supported fields: id, created, modified, priority, title.
// Returns an error for unknown fields. No-op if sortBy is empty.
func SortRecords(records []ticket.Record, sortBy string) error {
	if sortBy == "" {
		return nil
	}

	desc := false
	if strings.HasSuffix(sortBy, ":desc") {
		desc = true
		sortBy = strings.TrimSuffix(sortBy, ":desc")
	} else {
		sortBy = strings.TrimSuffix(sortBy, ":asc")
	}

	var less func(i, j int) bool
	switch sortBy {
	case "id":
		less = func(i, j int) bool { return records[i].ID < records[j].ID }
	case "created":
		less = func(i, j int) bool { return records[i].Front.Created < records[j].Front.Created }
	case "modified":
		less = func(i, j int) bool { return records[i].ModTime.Before(records[j].ModTime) }
	case "priority":
		less = func(i, j int) bool { return records[i].Front.Priority < records[j].Front.Priority }
	case "title":
		less = func(i, j int) bool {
			return strings.ToLower(records[i].Body.Title) < strings.ToLower(records[j].Body.Title)
		}
	default:
		return fmt.Errorf("unknown sort field: %s (valid: id, created, modified, priority, title)", sortBy)
	}

	if desc {
		sort.Slice(records, func(i, j int) bool { return less(j, i) })
	} else {
		sort.Slice(records, func(i, j int) bool { return less(i, j) })
	}
	return nil
}

// LimitRecords truncates records to at most n entries. No-op if n <= 0.
func LimitRecords(records []ticket.Record, n int) []ticket.Record {
	if n <= 0 || len(records) <= n {
		return records
	}
	return records[:n]
}
