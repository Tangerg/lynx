package exa

import (
	"testing"
	"time"

	"github.com/Tangerg/scope/tools/web"
)

// TestRecencyMappingIsTotalAndOrdered pins Exa's absolute-date freshness model.
// Every Recency the neutral contract accepts must produce a start time, an
// unset filter must produce none, and a longer window must start strictly
// earlier — otherwise a caller asking for "last year" would silently receive
// "last month".
func TestRecencyMappingIsTotalAndOrdered(t *testing.T) {
	ordered := []web.Recency{
		web.RecencyYear, web.RecencyMonth, web.RecencyWeek, web.RecencyDay, web.RecencyHour,
	}
	var previous time.Time
	for index, recency := range ordered {
		start := recencyToStart(recency)
		if start.IsZero() {
			t.Fatalf("%q produced no start time", recency)
		}
		if index > 0 && !start.After(previous) {
			t.Fatalf("%q starts at %s, not after the longer window's %s", recency, start, previous)
		}
		previous = start
	}

	if start := recencyToStart(""); !start.IsZero() {
		t.Fatalf("an unset Recency produced the start time %s", start)
	}
	if start := recencyToStart(web.Recency("decade")); !start.IsZero() {
		t.Fatalf("an unknown Recency produced the start time %s", start)
	}
}

// TestParseDateRejectsAnythingButRFC3339 keeps an unparsable publication date
// out of the result as the zero time rather than as a wrong instant a caller
// would sort on.
func TestParseDateRejectsAnythingButRFC3339(t *testing.T) {
	parsed := parseDate("2024-03-01T10:00:00Z")
	if parsed.IsZero() || parsed.Year() != 2024 || parsed.Month() != time.March {
		t.Fatalf("parseDate returned %s", parsed)
	}

	for name, value := range map[string]string{
		"empty":       "",
		"date only":   "2024-03-01",
		"unix epoch":  "1709287200",
		"free text":   "March 1, 2024",
		"truncated":   "2024-03-01T10:00",
		"wrong zone":  "2024-03-01T10:00:00 UTC",
		"nonsensical": "not-a-date",
	} {
		t.Run(name, func(t *testing.T) {
			if got := parseDate(value); !got.IsZero() {
				t.Fatalf("parseDate(%q) = %s, want the zero time", value, got)
			}
		})
	}
}
