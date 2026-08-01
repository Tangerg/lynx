package documentpipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/metadata"
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
	excludeFromInference map[string]struct{}
	excludeFromEmbedding map[string]struct{}
}

// NewSimpleFormatter builds a [SimpleFormatter]. The zero config emits every
// metadata key in every mode.
func NewSimpleFormatter(config SimpleFormatterConfig) *SimpleFormatter {
	return &SimpleFormatter{
		excludeFromInference: keySet(config.ExcludeFromInference),
		excludeFromEmbedding: keySet(config.ExcludeFromEmbedding),
	}
}

// Format renders doc by emitting filtered metadata as `key: value` lines
// (sorted by key — map iteration order would make the rendered text,
// and thus embedding inputs and token counts, non-deterministic)
// followed by a blank line and the document text. With no metadata
// (filtered empty), the output is just doc.Text — no leading newlines.
func (s *SimpleFormatter) Format(doc *document.Document, mode MetadataMode) (string, error) {
	if doc == nil {
		return "", ErrNilDocument
	}
	mode, err := normalizeMetadataMode(mode)
	if err != nil {
		return "", err
	}
	if err := doc.Metadata.Validate(); err != nil {
		return "", fmt.Errorf("document pipeline: validate metadata: %w", err)
	}
	filtered := s.filterMetadataByMode(doc.Metadata, mode)
	if len(filtered) == 0 {
		return doc.Text, nil
	}

	entries := make([]string, 0, len(filtered))
	for _, key := range slices.Sorted(maps.Keys(filtered)) {
		value, err := metadataValueText(filtered[key])
		if err != nil {
			return "", fmt.Errorf("document pipeline: format metadata %q: %w", key, err)
		}
		entries = append(entries, key+": "+value)
	}
	return strings.Join(entries, "\n") + "\n\n" + doc.Text, nil
}

func (s *SimpleFormatter) filterMetadataByMode(values metadata.Map, mode MetadataMode) metadata.Map {
	switch mode {
	case MetadataModeAll:
		return values.Clone()
	case MetadataModeNone:
		return metadata.Map{}
	}

	filtered := values.Clone()

	var excluded map[string]struct{}
	switch mode {
	case MetadataModeInference:
		excluded = s.excludeFromInference
	case MetadataModeEmbed:
		excluded = s.excludeFromEmbedding
	}

	if len(excluded) > 0 {
		maps.DeleteFunc(filtered, func(key string, _ json.RawMessage) bool {
			_, found := excluded[key]
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

func metadataValueText(value json.RawMessage) (string, error) {
	value = bytes.TrimSpace(value)
	if !json.Valid(value) {
		return "", metadata.ErrInvalidValue
	}
	if bytes.Equal(value, []byte("null")) {
		return "", nil
	}
	if value[0] == '"' {
		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			return "", err
		}
		return text, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return "", err
	}
	return compact.String(), nil
}
