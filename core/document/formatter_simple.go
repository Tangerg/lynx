package document

import (
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cast"
)

// SimpleFormatterConfig configures a [SimpleFormatter]'s metadata
// filtering. Each list names keys that should be hidden in the
// corresponding mode — useful for keeping internal ids or timestamps
// out of embeddings while still surfacing them at inference time.
type SimpleFormatterConfig struct {
	ExcludedInferenceMetadataKeys []string
	ExcludedEmbedMetadataKeys []string
}

var _ Formatter = (*SimpleFormatter)(nil)

// SimpleFormatter renders a [*Document] as
//
//	key1: value1
//	key2: value2
//
//	<document text>
//
// Metadata keys can be filtered per-mode to keep embeddings clean while
// still showing extras at inference time.
//
// Example:
//
//	f := document.NewSimpleFormatter(document.SimpleFormatterConfig{
//	    ExcludedEmbedMetadataKeys: []string{"row_id", "internal"},
//	})
type SimpleFormatter struct {
	excludedInferenceMetadataKeys []string
	excludedEmbedMetadataKeys     []string
}

// NewSimpleFormatter builds a [SimpleFormatter]. nil config produces an
// unrestricted formatter that emits every metadata key in every mode.
func NewSimpleFormatter(config SimpleFormatterConfig) *SimpleFormatter {
	return &SimpleFormatter{
		excludedInferenceMetadataKeys: config.ExcludedInferenceMetadataKeys,
		excludedEmbedMetadataKeys:     config.ExcludedEmbedMetadataKeys,
	}
}

// Format renders doc by emitting filtered metadata as `key: value` lines
// (sorted by key — map iteration order would make the rendered text,
// and thus embedding inputs and token counts, non-deterministic)
// followed by a blank line and the document text. With no metadata
// (filtered empty), the output is just doc.Text — no leading newlines.
func (s *SimpleFormatter) Format(doc *Document, mode MetadataMode) string {
	if doc == nil {
		return ""
	}
	filtered := s.filterMetadataByMode(doc.Metadata, mode)
	if len(filtered) == 0 {
		return doc.Text
	}

	entries := make([]string, 0, len(filtered))
	for _, key := range slices.Sorted(maps.Keys(filtered)) {
		entries = append(entries, key+": "+cast.ToString(filtered[key]))
	}
	return strings.Join(entries, "\n") + "\n\n" + doc.Text
}

func (s *SimpleFormatter) filterMetadataByMode(metadata map[string]any, mode MetadataMode) map[string]any {
	switch mode {
	case MetadataModeAll:
		return maps.Clone(metadata)
	case MetadataModeNone:
		return make(map[string]any)
	}

	cloned := maps.Clone(metadata)

	var shouldExclude func(key string, value any) bool
	switch mode {
	case MetadataModeInference:
		shouldExclude = func(key string, _ any) bool {
			return slices.Contains(s.excludedInferenceMetadataKeys, key)
		}
	case MetadataModeEmbed:
		shouldExclude = func(key string, _ any) bool {
			return slices.Contains(s.excludedEmbedMetadataKeys, key)
		}
	}

	if shouldExclude != nil {
		maps.DeleteFunc(cloned, shouldExclude)
	}
	return cloned
}
