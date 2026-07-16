package documentpipeline

import (
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/core/document"
)

// SimpleFormatterConfig configures a [SimpleFormatter]'s metadata
// filtering. Each list names keys that should be hidden in the
// corresponding mode — useful for keeping internal ids or timestamps
// out of embeddings while still surfacing them at inference time.
type SimpleFormatterConfig struct {
	// ExcludeFromInference lists metadata keys omitted in inference mode.
	ExcludeFromInference []string
	// ExcludeFromEmbedding lists metadata keys omitted in embedding mode.
	ExcludeFromEmbedding []string
}

var _ Formatter = (*SimpleFormatter)(nil)

// SimpleFormatter renders a [*document.Document] as
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
//	f := documentpipeline.NewSimpleFormatter(documentpipeline.SimpleFormatterConfig{
//	    ExcludeFromEmbedding: []string{"row_id", "internal"},
//	})
type SimpleFormatter struct {
	excludeFromInference []string
	excludeFromEmbedding []string
}

// NewSimpleFormatter builds a [SimpleFormatter]. The zero config emits every
// metadata key in every mode.
func NewSimpleFormatter(config SimpleFormatterConfig) *SimpleFormatter {
	return &SimpleFormatter{
		excludeFromInference: slices.Clone(config.ExcludeFromInference),
		excludeFromEmbedding: slices.Clone(config.ExcludeFromEmbedding),
	}
}

// Format renders doc by emitting filtered metadata as `key: value` lines
// (sorted by key — map iteration order would make the rendered text,
// and thus embedding inputs and token counts, non-deterministic)
// followed by a blank line and the document text. With no metadata
// (filtered empty), the output is just doc.Text — no leading newlines.
func (s *SimpleFormatter) Format(doc *document.Document, mode MetadataMode) (string, error) {
	if doc == nil {
		return "", nil
	}
	values, err := doc.Metadata.Values()
	if err != nil {
		return "", err
	}
	filtered := s.filterMetadataByMode(values, mode)
	if len(filtered) == 0 {
		return doc.Text, nil
	}

	entries := make([]string, 0, len(filtered))
	for _, key := range slices.Sorted(maps.Keys(filtered)) {
		entries = append(entries, key+": "+cast.ToString(filtered[key]))
	}
	return strings.Join(entries, "\n") + "\n\n" + doc.Text, nil
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
			return slices.Contains(s.excludeFromInference, key)
		}
	case MetadataModeEmbed:
		shouldExclude = func(key string, _ any) bool {
			return slices.Contains(s.excludeFromEmbedding, key)
		}
	}

	if shouldExclude != nil {
		maps.DeleteFunc(cloned, shouldExclude)
	}
	return cloned
}
