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
	// ExcludedInferenceMetadataKeys names metadata keys to drop when
	// rendering in [MetadataModeInference].
	ExcludedInferenceMetadataKeys []string

	// ExcludedEmbedMetadataKeys names metadata keys to drop when
	// rendering in [MetadataModeEmbed].
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

// NewSimpleFormatter builds a [SimpleFormatter]. A zero config produces
// an unrestricted formatter that emits every metadata key in every
// mode.
func NewSimpleFormatter(config SimpleFormatterConfig) *SimpleFormatter {
	if config.ExcludedInferenceMetadataKeys == nil {
		config.ExcludedInferenceMetadataKeys = []string{}
	}
	if config.ExcludedEmbedMetadataKeys == nil {
		config.ExcludedEmbedMetadataKeys = []string{}
	}

	return &SimpleFormatter{
		excludedInferenceMetadataKeys: config.ExcludedInferenceMetadataKeys,
		excludedEmbedMetadataKeys:     config.ExcludedEmbedMetadataKeys,
	}
}

// NewDefaultSimpleFormatter returns an unrestricted [SimpleFormatter].
func NewDefaultSimpleFormatter() *SimpleFormatter {
	return NewSimpleFormatter(SimpleFormatterConfig{})
}

// Format renders doc by emitting filtered metadata as `key: value` lines
// followed by a blank line and the document text.
func (s *SimpleFormatter) Format(doc *Document, mode MetadataMode) string {
	filtered := s.filterMetadataByMode(doc.Metadata, mode)

	entries := make([]string, 0, len(filtered))
	for key, value := range filtered {
		entries = append(entries, key+": "+cast.ToString(value))
	}

	metadataBlock := strings.Join(entries, "\n")
	return metadataBlock + "\n\n" + doc.Text
}

// filterMetadataByMode returns a copy of metadata with the appropriate
// keys removed for the supplied mode. Modes that don't filter return
// the live map directly (when [MetadataModeAll]) or an empty map
// (when [MetadataModeNone]).
func (s *SimpleFormatter) filterMetadataByMode(metadata map[string]any, mode MetadataMode) map[string]any {
	switch mode {
	case MetadataModeAll:
		return metadata
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
