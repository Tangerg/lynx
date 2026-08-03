package redis_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/redis"
	"github.com/Tangerg/lynx/vectorstores/storetest"
)

// TestVisitor_Conformance runs the shared visitor suite against the
// redis (RediSearch) visitor. Redis is schema-required, so the
// per-field-type declarations below mirror the [storetest] case
// identifiers; redis IN-on-numeric isn't supported by the visitor
// so `in_numbers` is declared via [storetest.Options.Unsupported].
func TestVisitor_Conformance(t *testing.T) {
	fields := map[string]redis.MetadataFieldType{
		"author":         redis.FieldTag, // == / !=
		"year":           redis.FieldNumeric,
		"published":      redis.FieldTag, // bool ==
		"n":              redis.FieldNumeric,
		"a":              redis.FieldNumeric,
		"b":              redis.FieldNumeric,
		"c":              redis.FieldNumeric,
		"d":              redis.FieldNumeric,
		"tags":           redis.FieldTag,  // IN strings
		"flags":          redis.FieldTag,  // IN bools (rendered as tag string)
		"title":          redis.FieldText, // LIKE
		"profile.author": redis.FieldTag,
		"profile.a.b":    redis.FieldTag,
	}

	storetest.VisitorConformance(t,
		func(src string) error {
			expr, err := filter.Parse(src)
			if err != nil {
				return err
			}
			v := redis.NewVisitor(fields)
			return v.Visit(expr)
		},
		storetest.Options{
			// Redis doesn't support IN on NUMERIC fields — the visitor
			// errors with "IN is not supported on field type". This is
			// a real capability gap, not a visitor bug.
			Unsupported: []string{"in_numbers", "collection_membership"},
		},
	)
}

func TestVisitor_RejectsIntegerThatRediSearchCannotRepresentExactly(t *testing.T) {
	visitor := redis.NewVisitor(map[string]redis.MetadataFieldType{"id": redis.FieldNumeric})
	if err := visitor.Visit(filter.EQ("id", uint64(1<<53+1))); err == nil {
		t.Fatal("Redis silently rounded a large integer")
	}
}

func TestVisitor_TranslatesLikeToRedisWildcardQuery(t *testing.T) {
	t.Parallel()

	visitor := redis.NewVisitor(map[string]redis.MetadataFieldType{"title": redis.FieldText})
	if err := visitor.Visit(filter.Like("title", `intro%_literal*?`)); err != nil {
		t.Fatal(err)
	}
	if got, want := visitor.Result(), `@title:(w'intro*?literal\*\?')`; got != want {
		t.Fatalf("Result() = %q, want %q", got, want)
	}
}
