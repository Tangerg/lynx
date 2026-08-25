package documentpipeline

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
)

// SimpleFormatterConfig configures a [SimpleFormatter]'s metadata policy.
type SimpleFormatterConfig struct {
	// ExcludedMetadata lists metadata keys omitted from rendered output.
	ExcludedMetadata []string
}

var _ Formatter = SimpleFormatter{}

// SimpleFormatter renders a [*document.Document] as
//
//	key1: value1
//	key2: value2
//
//	<document text>
//
// Metadata keys can be excluded when constructing the formatter. Use
// independently configured formatters when different consumers need different
// representations.
//
// Example:
//
//	f := documentpipeline.NewSimpleFormatter(documentpipeline.SimpleFormatterConfig{
//	    ExcludedMetadata: []string{"row_id", "internal"},
//	})
type SimpleFormatter struct {
	excludedMetadata map[string]struct{}
}

// NewSimpleFormatter builds a [SimpleFormatter]. The zero config emits every
// metadata key.
func NewSimpleFormatter(config SimpleFormatterConfig) SimpleFormatter {
	return SimpleFormatter{excludedMetadata: keySet(config.ExcludedMetadata)}
}

// Format renders doc by emitting filtered metadata as `key: value` lines
// (sorted by key — map iteration order would make the rendered text,
// and thus embedding inputs and token counts, non-deterministic)
// followed by a blank line and the document text. With no metadata
// (filtered empty), the output is just doc.Text — no leading newlines.
func (s SimpleFormatter) Format(doc *document.Document) (string, error) {
	if doc == nil {
		return "", ErrNilDocument
	}
	if err := doc.Validate(); err != nil {
		return "", fmt.Errorf("document pipeline: format document: %w", err)
	}
	filtered := s.filterMetadata(doc.Metadata)
	if len(filtered) == 0 {
		return doc.Text, nil
	}

	entries := make([]string, 0, len(filtered))
	for _, key := range slices.Sorted(maps.Keys(filtered)) {
		value, err := metadataValue(filtered[key]).text()
		if err != nil {
			return "", fmt.Errorf("document pipeline: format metadata %q: %w", key, err)
		}
		entries = append(entries, key+": "+value)
	}
	return strings.Join(entries, "\n") + "\n\n" + doc.Text, nil
}

func (s SimpleFormatter) filterMetadata(values metadata.Map) metadata.Map {
	filtered := values.Clone()
	if len(s.excludedMetadata) > 0 {
		maps.DeleteFunc(filtered, func(key string, _ json.RawMessage) bool {
			_, found := s.excludedMetadata[key]
			return found
		})
	}
	return filtered
}

func keySet(keys []string) map[string]struct{} {
	if len(keys) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		set[key] = struct{}{}
	}
	return set
}
