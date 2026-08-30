package brave

import (
	"testing"

	"github.com/Tangerg/scope/tools/web"
)

func TestRecencyMappingMatchesBraveFreshness(t *testing.T) {
	tests := []struct {
		name    string
		recency web.Recency
		want    string
	}{
		{name: "hour uses minimum granularity", recency: web.RecencyHour, want: "pd"},
		{name: "day", recency: web.RecencyDay, want: "pd"},
		{name: "week", recency: web.RecencyWeek, want: "pw"},
		{name: "month", recency: web.RecencyMonth, want: "pm"},
		{name: "year", recency: web.RecencyYear, want: "py"},
		{name: "unset"},
		{name: "unsupported", recency: web.Recency("unsupported")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := recencyToFreshness(test.recency); got != test.want {
				t.Fatalf("recencyToFreshness(%q) = %q, want %q", test.recency, got, test.want)
			}
		})
	}
}
