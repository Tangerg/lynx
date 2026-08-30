package tavily

import (
	"testing"

	"github.com/Tangerg/scope/tools/web"
)

func TestRecencyMappingMatchesTavilyTimeRange(t *testing.T) {
	tests := []struct {
		name    string
		recency web.Recency
		want    string
	}{
		{name: "hour uses minimum granularity", recency: web.RecencyHour, want: "day"},
		{name: "day", recency: web.RecencyDay, want: "day"},
		{name: "week", recency: web.RecencyWeek, want: "week"},
		{name: "month", recency: web.RecencyMonth, want: "month"},
		{name: "year", recency: web.RecencyYear, want: "year"},
		{name: "unset"},
		{name: "unsupported", recency: web.Recency("unsupported")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := recencyToTimeRange(test.recency); got != test.want {
				t.Fatalf("recencyToTimeRange(%q) = %q, want %q", test.recency, got, test.want)
			}
		})
	}
}
