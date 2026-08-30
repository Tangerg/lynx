package firecrawl

import (
	"testing"

	"github.com/Tangerg/scope/tools/web"
)

// TestRecencyMappingIsTotal pins this provider's native freshness vocabulary.
// Every Recency the neutral contract accepts must map to a distinct provider
// value, and an unset filter must stay unset — a silent default would narrow
// results the caller never asked to narrow.
func TestRecencyMappingIsTotal(t *testing.T) {
	recencies := []web.Recency{
		web.RecencyHour, web.RecencyDay, web.RecencyWeek, web.RecencyMonth, web.RecencyYear,
	}
	for _, recency := range recencies {
		t.Run(string(recency), func(t *testing.T) {
			if err := recency.Validate(); err != nil {
				t.Fatal(err)
			}
			if mapped := recencyToTbs(recency); mapped == "" {
				t.Fatalf("%q mapped to the empty provider value", recency)
			}
		})
	}
	if mapped := recencyToTbs(""); mapped != "" {
		t.Fatalf("an unset Recency mapped to %q", mapped)
	}
	if mapped := recencyToTbs(web.Recency("decade")); mapped != "" {
		t.Fatalf("an unknown Recency mapped to %q", mapped)
	}
}

// TestRecencyMappingIsMonotonic keeps the coarse ordering intact: a longer
// window must never map to a narrower provider value than a shorter one.
func TestRecencyMappingIsMonotonic(t *testing.T) {
	ordered := []web.Recency{
		web.RecencyHour, web.RecencyDay, web.RecencyWeek, web.RecencyMonth, web.RecencyYear,
	}
	seen := map[string]int{}
	for index, recency := range ordered {
		mapped := recencyToTbs(recency)
		if previous, repeated := seen[mapped]; repeated && index-previous > 1 {
			t.Fatalf("%q reuses the provider value %q from a much shorter window", recency, mapped)
		}
		seen[mapped] = index
	}
}
