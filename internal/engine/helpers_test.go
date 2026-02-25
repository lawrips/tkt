package engine

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lawrips/tkt/internal/ticket"
)

func TestIndexByID(t *testing.T) {
	records := []ticket.Record{
		{ID: "a"}, {ID: "b"}, {ID: "c"},
	}
	byID := IndexByID(records)
	if len(byID) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(byID))
	}
	if byID["b"].ID != "b" {
		t.Fatalf("expected b, got %s", byID["b"].ID)
	}
}

func TestHasOpenDeps(t *testing.T) {
	byID := map[string]ticket.Record{
		"dep-open":   {ID: "dep-open", Front: ticket.Frontmatter{Status: "open"}},
		"dep-closed": {ID: "dep-closed", Front: ticket.Frontmatter{Status: "closed"}},
	}

	cases := []struct {
		name string
		deps []string
		want bool
	}{
		{"no deps", nil, false},
		{"all closed", []string{"dep-closed"}, false},
		{"one open", []string{"dep-open"}, true},
		{"mixed", []string{"dep-closed", "dep-open"}, true},
		{"missing dep", []string{"nonexistent"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ticket.Record{Front: ticket.Frontmatter{Deps: tc.deps}}
			if got := HasOpenDeps(r, byID); got != tc.want {
				t.Fatalf("HasOpenDeps(%v) = %v, want %v", tc.deps, got, tc.want)
			}
		})
	}
}

func TestAppendUnique(t *testing.T) {
	s := []string{"a", "b"}
	s = AppendUnique(s, "c")
	if !reflect.DeepEqual(s, []string{"a", "b", "c"}) {
		t.Fatalf("expected [a b c], got %v", s)
	}
	s = AppendUnique(s, "b")
	if len(s) != 3 {
		t.Fatalf("expected no duplicate, got %v", s)
	}
}

func TestRemoveValue(t *testing.T) {
	got := RemoveValue([]string{"a", "b", "c", "b"}, "b")
	if !reflect.DeepEqual(got, []string{"a", "c"}) {
		t.Fatalf("expected [a c], got %v", got)
	}
	got = RemoveValue([]string{"a"}, "z")
	if !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("expected [a], got %v", got)
	}
}

func TestContains(t *testing.T) {
	if !Contains([]string{"a", "b"}, "b") {
		t.Fatal("expected true")
	}
	if Contains([]string{"a", "b"}, "c") {
		t.Fatal("expected false")
	}
	if Contains(nil, "a") {
		t.Fatal("expected false for nil")
	}
}

func TestParseCSV(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ", []string{"a", "b"}},
		{"", []string{}},
		{",,,", []string{}},
		{"solo", []string{"solo"}},
	}
	for _, tc := range cases {
		got := ParseCSV(tc.input)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("ParseCSV(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestResolveRecordFromList(t *testing.T) {
	records := []ticket.Record{
		{ID: "abc-123"}, {ID: "def-456"}, {ID: "abc-789"},
	}

	// Exact match
	r, ok := ResolveRecordFromList(records, "def-456")
	if !ok || r.ID != "def-456" {
		t.Fatalf("expected exact match def-456, got %v %v", r.ID, ok)
	}

	// Unique partial match
	r, ok = ResolveRecordFromList(records, "def")
	if !ok || r.ID != "def-456" {
		t.Fatalf("expected partial match def-456, got %v %v", r.ID, ok)
	}

	// Ambiguous partial match
	_, ok = ResolveRecordFromList(records, "abc")
	if ok {
		t.Fatal("expected ambiguous match to fail")
	}

	// No match
	_, ok = ResolveRecordFromList(records, "zzz")
	if ok {
		t.Fatal("expected no match")
	}

	// Empty query
	_, ok = ResolveRecordFromList(records, "")
	if ok {
		t.Fatal("expected empty query to fail")
	}
}

func TestIDsFromRecords(t *testing.T) {
	records := []ticket.Record{{ID: "a"}, {ID: "b"}}
	got := IDsFromRecords(records)
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("expected [a b], got %v", got)
	}
}

func TestRenderDepTree(t *testing.T) {
	byID := map[string]ticket.Record{
		"root":  {ID: "root", Front: ticket.Frontmatter{Status: "open", Deps: []string{"child"}}, Body: ticket.Body{Title: "Root"}},
		"child": {ID: "child", Front: ticket.Frontmatter{Status: "closed", Deps: []string{}}, Body: ticket.Body{Title: "Child"}},
	}

	// Without full: closed deps are skipped
	lines := RenderDepTree("root", byID, false)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (closed child hidden), got %d: %v", len(lines), lines)
	}

	// With full: closed deps are shown
	lines = RenderDepTree("root", byID, true)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}

	// Missing ticket
	byID["root"] = ticket.Record{ID: "root", Front: ticket.Frontmatter{Status: "open", Deps: []string{"missing"}}, Body: ticket.Body{Title: "Root"}}
	lines = RenderDepTree("root", byID, true)
	found := false
	for _, l := range lines {
		if strings.Contains(l, "[missing]") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected [missing] marker in output: %v", lines)
	}
}

func TestRenderDepTreeCycleDetection(t *testing.T) {
	byID := map[string]ticket.Record{
		"a": {ID: "a", Front: ticket.Frontmatter{Status: "open", Deps: []string{"b"}}, Body: ticket.Body{Title: "A"}},
		"b": {ID: "b", Front: ticket.Frontmatter{Status: "open", Deps: []string{"a"}}, Body: ticket.Body{Title: "B"}},
	}
	lines := RenderDepTree("a", byID, true)
	hasCycle := false
	for _, l := range lines {
		if len(l) > 0 && l[len(l)-7:] == "(cycle)" {
			hasCycle = true
		}
	}
	if !hasCycle {
		t.Fatalf("expected cycle marker in output: %v", lines)
	}
}

func TestWindowStart(t *testing.T) {
	now := time.Date(2026, 2, 26, 15, 30, 0, 0, time.UTC)

	today := WindowStart("today", now)
	if today != time.Date(2026, 2, 26, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("expected start of day, got %v", today)
	}

	week := WindowStart("week", now)
	expected := now.Add(-7 * 24 * time.Hour)
	if week != expected {
		t.Fatalf("expected %v, got %v", expected, week)
	}
}

func TestMonday(t *testing.T) {
	// Wednesday Feb 26, 2026
	wed := time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC)
	mon := Monday(wed)
	if mon.Weekday() != time.Monday {
		t.Fatalf("expected Monday, got %v", mon.Weekday())
	}
	if mon != time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("expected Feb 23, got %v", mon)
	}

	// Sunday
	sun := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	mon = Monday(sun)
	if mon.Weekday() != time.Monday {
		t.Fatalf("expected Monday, got %v", mon.Weekday())
	}

	// Monday itself
	monDay := time.Date(2026, 2, 23, 8, 0, 0, 0, time.UTC)
	mon = Monday(monDay)
	if mon != time.Date(2026, 2, 23, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("expected same Monday at midnight, got %v", mon)
	}
}

func TestShortSHA(t *testing.T) {
	if got := ShortSHA("abc1234567890"); got != "abc1234" {
		t.Fatalf("expected abc1234, got %s", got)
	}
	if got := ShortSHA("abc"); got != "abc" {
		t.Fatalf("expected abc, got %s", got)
	}
	if got := ShortSHA(""); got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}

func TestFindSection(t *testing.T) {
	sections := []ticket.Section{
		{Heading: "Notes", Content: "some notes"},
		{Heading: "Design", Content: "design doc"},
	}

	s := FindSection(sections, "Notes")
	if s.Content != "some notes" {
		t.Fatalf("expected 'some notes', got %q", s.Content)
	}

	// Case insensitive
	s = FindSection(sections, "notes")
	if s.Content != "some notes" {
		t.Fatalf("expected case-insensitive match, got %q", s.Content)
	}

	// Missing returns empty with heading
	s = FindSection(sections, "Missing")
	if s.Heading != "Missing" || s.Content != "" {
		t.Fatalf("expected empty section with heading, got %+v", s)
	}
}

func TestUpsertSection(t *testing.T) {
	sections := []ticket.Section{
		{Heading: "Notes", Content: "old"},
	}

	// Update existing
	sections = UpsertSection(sections, ticket.Section{Heading: "Notes", Content: "new"})
	if len(sections) != 1 || sections[0].Content != "new" {
		t.Fatalf("expected updated section, got %+v", sections)
	}

	// Append new
	sections = UpsertSection(sections, ticket.Section{Heading: "Design", Content: "design"})
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
}

func TestToSnakeCase(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Acceptance Criteria", "acceptance_criteria"},
		{"Design", "design"},
		{"", ""},
		{"  Multi  Spaces  ", "multi_spaces"},
		{"with-dashes", "with_dashes"},
		{"CamelCase", "camelcase"},
	}
	for _, tc := range cases {
		if got := ToSnakeCase(tc.input); got != tc.want {
			t.Fatalf("ToSnakeCase(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestTicketToMap(t *testing.T) {
	r := ticket.Record{
		ID: "test-1",
		Front: ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
			Deps:     nil,
			Links:    nil,
			Tags:     nil,
			Created:  "2026-01-01T00:00:00Z",
		},
		Body: ticket.Body{
			Title:       "Test ticket",
			Description: "A description",
		},
	}
	m := TicketToMap(r)
	if m["id"] != "test-1" {
		t.Fatalf("expected test-1, got %v", m["id"])
	}
	// nil slices should become empty slices
	if deps, ok := m["deps"].([]string); !ok || len(deps) != 0 {
		t.Fatalf("expected empty deps slice, got %v", m["deps"])
	}
	if links, ok := m["links"].([]string); !ok || len(links) != 0 {
		t.Fatalf("expected empty links slice, got %v", m["links"])
	}
}

func TestTicketsToMaps(t *testing.T) {
	records := []ticket.Record{
		{ID: "a", Front: ticket.Frontmatter{Deps: []string{}, Links: []string{}, Tags: []string{}}},
		{ID: "b", Front: ticket.Frontmatter{Deps: []string{}, Links: []string{}, Tags: []string{}}},
	}
	maps := TicketsToMaps(records)
	if len(maps) != 2 {
		t.Fatalf("expected 2, got %d", len(maps))
	}
	if maps[0]["id"] != "a" || maps[1]["id"] != "b" {
		t.Fatalf("unexpected ids: %v %v", maps[0]["id"], maps[1]["id"])
	}
}

func TestTicketSummaryToMap(t *testing.T) {
	r := ticket.Record{
		ID: "test-1",
		Front: ticket.Frontmatter{
			Status:   "open",
			Type:     "task",
			Priority: 2,
		},
		Body: ticket.Body{
			Title:       "Test ticket",
			Description: "A description",
		},
	}
	m := TicketSummaryToMap(r)
	if m["id"] != "test-1" || m["title"] != "Test ticket" {
		t.Fatalf("unexpected summary map: %#v", m)
	}
	if m["status"] != "open" || m["type"] != "task" || m["priority"] != 2 {
		t.Fatalf("summary map missing expected status/type/priority: %#v", m)
	}
	if _, ok := m["description"]; ok {
		t.Fatalf("summary map should not include description: %#v", m)
	}
}

func TestTicketSummariesToMaps(t *testing.T) {
	records := []ticket.Record{
		{ID: "a", Front: ticket.Frontmatter{Status: "open", Type: "task", Priority: 1}, Body: ticket.Body{Title: "A"}},
		{ID: "b", Front: ticket.Frontmatter{Status: "closed", Type: "bug", Priority: 2}, Body: ticket.Body{Title: "B"}},
	}
	maps := TicketSummariesToMaps(records)
	if len(maps) != 2 {
		t.Fatalf("expected 2, got %d", len(maps))
	}
	if maps[0]["id"] != "a" || maps[1]["id"] != "b" {
		t.Fatalf("unexpected ids: %v %v", maps[0]["id"], maps[1]["id"])
	}
	if _, ok := maps[0]["description"]; ok {
		t.Fatalf("summary map should not include description: %#v", maps[0])
	}
}

func TestSortRecords(t *testing.T) {
	now := time.Now()
	records := []ticket.Record{
		{
			ID:      "c",
			Front:   ticket.Frontmatter{ID: "c", Priority: 2, Created: "2025-01-03T00:00:00Z"},
			Body:    ticket.Body{Title: "Zebra"},
			ModTime: now.Add(-3 * time.Hour),
		},
		{
			ID:      "a",
			Front:   ticket.Frontmatter{ID: "a", Priority: 0, Created: "2025-01-01T00:00:00Z"},
			Body:    ticket.Body{Title: "Apple"},
			ModTime: now.Add(-1 * time.Hour),
		},
		{
			ID:      "b",
			Front:   ticket.Frontmatter{ID: "b", Priority: 1, Created: "2025-01-02T00:00:00Z"},
			Body:    ticket.Body{Title: "Mango"},
			ModTime: now.Add(-2 * time.Hour),
		},
	}

	tests := []struct {
		name    string
		sortBy  string
		wantIDs []string
	}{
		{"by id asc", "id", []string{"a", "b", "c"}},
		{"by id asc explicit", "id:asc", []string{"a", "b", "c"}},
		{"by id desc", "id:desc", []string{"c", "b", "a"}},
		{"by created asc", "created", []string{"a", "b", "c"}},
		{"by created desc", "created:desc", []string{"c", "b", "a"}},
		{"by modified asc", "modified", []string{"c", "b", "a"}},
		{"by modified desc", "modified:desc", []string{"a", "b", "c"}},
		{"by priority asc", "priority", []string{"a", "b", "c"}},
		{"by priority desc", "priority:desc", []string{"c", "b", "a"}},
		{"by title asc", "title", []string{"a", "b", "c"}},
		{"by title desc", "title:desc", []string{"c", "b", "a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recs := make([]ticket.Record, len(records))
			copy(recs, records)
			if err := SortRecords(recs, tt.sortBy); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for i, want := range tt.wantIDs {
				if recs[i].ID != want {
					t.Errorf("position %d: got %s, want %s", i, recs[i].ID, want)
				}
			}
		})
	}
}

func TestSortRecordsUnknownField(t *testing.T) {
	records := []ticket.Record{{ID: "a"}}
	err := SortRecords(records, "bogus")
	if err == nil {
		t.Fatal("expected error for unknown sort field")
	}
	if !strings.Contains(err.Error(), "unknown sort field") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSortRecordsEmpty(t *testing.T) {
	records := []ticket.Record{{ID: "a"}}
	err := SortRecords(records, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSortRecordsNilSlice(t *testing.T) {
	if err := SortRecords(nil, "id"); err != nil {
		t.Fatalf("unexpected error on nil slice: %v", err)
	}
	if err := SortRecords([]ticket.Record{}, "id"); err != nil {
		t.Fatalf("unexpected error on empty slice: %v", err)
	}
}

func TestLimitRecords(t *testing.T) {
	records := []ticket.Record{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}

	got := LimitRecords(records, 2)
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}

	got = LimitRecords(records, 0)
	if len(got) != 4 {
		t.Fatalf("limit 0 should return all, got %d", len(got))
	}

	got = LimitRecords(records, 10)
	if len(got) != 4 {
		t.Fatalf("limit > len should return all, got %d", len(got))
	}

	got = LimitRecords(records, -1)
	if len(got) != 4 {
		t.Fatalf("negative limit should return all, got %d", len(got))
	}
}
