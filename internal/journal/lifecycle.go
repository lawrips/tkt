package journal

import "time"

// LifecycleSummary captures key lifecycle moments and derived durations.
type LifecycleSummary struct {
	Opened          string
	FirstCommit     string
	LastCommit      string
	WorkStarted     string
	WorkEnded       string
	ClosedAt        string
	CalendarSeconds int
	WorkSeconds     int
	IdleSeconds     int
}

// Lifecycle computes lifecycle timestamps and duration summaries from journal data.
func Lifecycle(created, status string, entries []Entry, now time.Time) LifecycleSummary {
	var firstCommit, lastCommit, firstWorkStart, lastWorkEnd, closedAt time.Time
	workSeconds := 0

	for _, entry := range entries {
		if t, ok := parseRFC3339(entry.TS); ok {
			if firstCommit.IsZero() || t.Before(firstCommit) {
				firstCommit = t
			}
			if lastCommit.IsZero() || t.After(lastCommit) {
				lastCommit = t
			}
			if entry.Action == "close" && (closedAt.IsZero() || t.After(closedAt)) {
				closedAt = t
			}
		}
		if t, ok := parseRFC3339(entry.WorkStarted); ok {
			if firstWorkStart.IsZero() || t.Before(firstWorkStart) {
				firstWorkStart = t
			}
		}
		if t, ok := parseRFC3339(entry.WorkEnded); ok {
			if lastWorkEnd.IsZero() || t.After(lastWorkEnd) {
				lastWorkEnd = t
			}
		}
		if entry.DurationSecs > 0 {
			workSeconds += entry.DurationSecs
		}
	}

	opened := created
	createdTS, createdOK := parseRFC3339(created)
	if createdOK {
		opened = createdTS.UTC().Format(time.RFC3339)
	}

	if closedAt.IsZero() && status == "closed" && !lastCommit.IsZero() {
		closedAt = lastCommit
	}

	if workSeconds == 0 && !firstWorkStart.IsZero() && !lastWorkEnd.IsZero() && lastWorkEnd.After(firstWorkStart) {
		workSeconds = int(lastWorkEnd.Sub(firstWorkStart).Seconds())
	}

	idleSeconds := 0
	windowStart := firstWorkStart
	windowEnd := lastWorkEnd
	if windowStart.IsZero() || windowEnd.IsZero() {
		windowStart = firstCommit
		windowEnd = lastCommit
	}
	if !windowStart.IsZero() && !windowEnd.IsZero() && windowEnd.After(windowStart) {
		activityWindow := int(windowEnd.Sub(windowStart).Seconds())
		idleSeconds = activityWindow - workSeconds
		if idleSeconds < 0 {
			idleSeconds = 0
		}
	}

	calendarSeconds := 0
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if createdOK {
		end := now
		if !closedAt.IsZero() {
			end = closedAt
		}
		if end.After(createdTS) {
			calendarSeconds = int(end.Sub(createdTS).Seconds())
		}
	}

	return LifecycleSummary{
		Opened:          opened,
		FirstCommit:     formatRFC3339(firstCommit),
		LastCommit:      formatRFC3339(lastCommit),
		WorkStarted:     formatRFC3339(firstWorkStart),
		WorkEnded:       formatRFC3339(lastWorkEnd),
		ClosedAt:        formatRFC3339(closedAt),
		CalendarSeconds: calendarSeconds,
		WorkSeconds:     workSeconds,
		IdleSeconds:     idleSeconds,
	}
}

func parseRFC3339(ts string) (time.Time, bool) {
	if ts == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func formatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// FormatSeconds renders a duration in seconds as a human-readable string (e.g. "2h30m0s").
func FormatSeconds(seconds int) string {
	if seconds <= 0 {
		return "0s"
	}
	return (time.Duration(seconds) * time.Second).String()
}
