package redis_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
	"github.com/Tangerg/lynx/vectorstores/redis"
)

// TestVisitor_Conformance runs the shared visitor suite against the
// redis (RediSearch) visitor. Redis is schema-required, so the
// per-field-type declarations below mirror the [storetest] case
// identifiers; redis IN-on-numeric isn't supported by the visitor
// so `in_numbers` is opted out via [storetest.Options.Skip].
func TestVisitor_Conformance(t *testing.T) {
	fields := map[string]redis.MetadataFieldType{
		"author":           redis.FieldTag, // == / !=
		"year":             redis.FieldNumeric,
		"published":        redis.FieldTag, // bool ==
		"n":                redis.FieldNumeric,
		"a":                redis.FieldNumeric,
		"b":                redis.FieldNumeric,
		"c":                redis.FieldNumeric,
		"d":                redis.FieldNumeric,
		"tags":      redis.FieldTag,  // IN strings
		"flags":     redis.FieldTag,  // IN bools (rendered as tag string)
		"title":     redis.FieldText, // LIKE
		// The visitor strips the base identifier ("metadata") off
		// IndexExpr chains and joins the inner keys with dots, so
		// `metadata['author']` resolves to "author" (already above)
		// and `metadata['a']['b']` resolves to "a.b".
		"a.b": redis.FieldTag,
	}

	storetest.VisitorConformance(t,
		func(src string) error {
			expr, err := filter.ParseAndAnalyze(src)
			if err != nil {
				return err
			}
			v := redis.NewVisitor(fields)
			v.Visit(expr)
			return v.Error()
		},
		storetest.Options{
			// Redis doesn't support IN on NUMERIC fields — the visitor
			// errors with "IN is not supported on field type". This is
			// a real capability gap, not a visitor bug.
			Skip: []string{"in_numbers"},
		},
	)
}
