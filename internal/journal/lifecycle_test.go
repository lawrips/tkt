package journal

import (
	"testing"
	"time"
)

func TestLifecycleUsesCloseActionAndDurationFields(t *testing.T) {
	now := time.Date(2026, 2, 22, 13, 0, 0, 0, time.UTC)
	entries := []Entry{
		{
			TS:           "2026-02-21T08:00:00+05:30",
			Action:       "ref",
			WorkStarted:  "2026-02-21T01:30:00Z",
			WorkEnded:    "2026-02-21T02:30:00Z",
			DurationSecs: 3600,
		},
		{
			TS:           "2026-02-22T12:00:00Z",
			Action:       "close",
			WorkStarted:  "2026-02-22T11:30:00Z",
			WorkEnded:    "2026-02-22T12:00:00Z",
			DurationSecs: 1800,
		},
	}

	got := Lifecycle("2026-02-20T10:00:00Z", "closed", entries, now)

	if got.FirstCommit != "2026-02-21T02:30:00Z" {
		t.Fatalf("FirstCommit: want 2026-02-21T02:30:00Z, got %s", got.FirstCommit)
	}
	if got.ClosedAt != "2026-02-22T12:00:00Z" {
		t.Fatalf("ClosedAt: want 2026-02-22T12:00:00Z, got %s", got.ClosedAt)
	}
	if got.WorkSeconds != 5400 {
		t.Fatalf("WorkSeconds: want 5400, got %d", got.WorkSeconds)
	}
	if got.CalendarSeconds != 180000 {
		t.Fatalf("CalendarSeconds: want 180000, got %d", got.CalendarSeconds)
	}
	if got.IdleSeconds != 118800 {
		t.Fatalf("IdleSeconds: want 118800, got %d", got.IdleSeconds)
	}
}

func TestLifecycleFallsBackClosedAtToLastCommit(t *testing.T) {
	now := time.Date(2026, 2, 22, 13, 0, 0, 0, time.UTC)
	entries := []Entry{
		{TS: "2026-02-22T12:00:00Z", Action: "ref"},
	}

	got := Lifecycle("2026-02-22T10:00:00Z", "closed", entries, now)

	if got.ClosedAt != "2026-02-22T12:00:00Z" {
		t.Fatalf("ClosedAt fallback: want 2026-02-22T12:00:00Z, got %s", got.ClosedAt)
	}
	if got.CalendarSeconds != 7200 {
		t.Fatalf("CalendarSeconds: want 7200, got %d", got.CalendarSeconds)
	}
}

func TestLifecycleEmptyEntries(t *testing.T) {
	now := time.Date(2026, 2, 22, 13, 0, 0, 0, time.UTC)
	got := Lifecycle("2026-02-22T10:00:00Z", "open", nil, now)

	if got.Opened != "2026-02-22T10:00:00Z" {
		t.Fatalf("Opened: want 2026-02-22T10:00:00Z, got %s", got.Opened)
	}
	if got.FirstCommit != "" {
		t.Fatalf("FirstCommit: want empty, got %s", got.FirstCommit)
	}
	if got.ClosedAt != "" {
		t.Fatalf("ClosedAt: want empty for open ticket, got %s", got.ClosedAt)
	}
	if got.CalendarSeconds != 10800 {
		t.Fatalf("CalendarSeconds: want 10800 (3h), got %d", got.CalendarSeconds)
	}
	if got.WorkSeconds != 0 {
		t.Fatalf("WorkSeconds: want 0, got %d", got.WorkSeconds)
	}
}

func TestFormatSecondsZero(t *testing.T) {
	if got := FormatSeconds(0); got != "0s" {
		t.Fatalf("want 0s, got %q", got)
	}
}

func TestFormatSecondsNegative(t *testing.T) {
	if got := FormatSeconds(-5); got != "0s" {
		t.Fatalf("want 0s for negative, got %q", got)
	}
}

func TestFormatSecondsMinutes(t *testing.T) {
	if got := FormatSeconds(90); got != "1m30s" {
		t.Fatalf("want 1m30s, got %q", got)
	}
}

func TestFormatSecondsHours(t *testing.T) {
	if got := FormatSeconds(9000); got != "2h30m0s" {
		t.Fatalf("want 2h30m0s, got %q", got)
	}
}
