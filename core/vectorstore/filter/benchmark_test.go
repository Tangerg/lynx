package filter_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func BenchmarkParse(b *testing.B) {
	benchmarks := []struct {
		name   string
		source string
	}{
		{name: "comparison", source: `category == 'tech'`},
		{name: "compound", source: `(category == 'tech' and year >= 2024) or title like 'Go%'`},
	}

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := filter.Parse(benchmark.source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
