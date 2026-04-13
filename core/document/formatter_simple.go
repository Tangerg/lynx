package document

import (
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cast"
)

// SimpleFormatterConfig holds the configuration for SimpleFormatter.
type SimpleFormatterConfig struct {
	// ExcludedInferenceMetadataKeys specifies metadata keys to exclude when formatting
	// documents in MetadataModeInference mode.
	// Optional. Defaults to empty slice if not provided.
	// Use this to filter out metadata that shouldn't be included during model inference,
	// such as internal IDs or processing timestamps.
	ExcludedInferenceMetadataKeys []string

	// ExcludedEmbedMetadataKeys specifies metadata keys to exclude when formatting
	// documents in MetadataModeEmbed mode.
	// Optional. Defaults to empty slice if not provided.
	// Use this to filter out metadata that shouldn't affect vector embeddings,
	// such as UI-related fields or temporary processing flags.
	ExcludedEmbedMetadataKeys []string
}

var _ Formatter = (*SimpleFormatter)(nil)

// SimpleFormatter provides a basic implementation of the Formatter interface
// that converts documents to a simple key-value text format.
//
// This formatter is useful for:
//   - Creating human-readable document representations
//   - Generating consistent text format for embedding generation
//   - Controlling metadata visibility based on processing context
//   - Building simple RAG pipelines without complex formatting requirements
//
// The formatter produces output in the following structure:
//
//	key1: value1
//	key2: value2
//
//	[document content]
//
// Metadata keys can be selectively excluded based on the MetadataMode to optimize
// the output for different use cases (embedding vs inference).
type SimpleFormatter struct {
	excludedInferenceMetadataKeys []string
	excludedEmbedMetadataKeys     []string
}

func NewSimpleFormatter(config *SimpleFormatterConfig) *SimpleFormatter {
	if config == nil {
		config = &SimpleFormatterConfig{}
	}
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

func NewDefaultSimpleFormatter() *SimpleFormatter {
	return NewSimpleFormatter(nil)
}

func (s *SimpleFormatter) Format(doc *Document, mode MetadataMode) string {
	const (
		metadataSeparator                = "\n"
		metadataTemplateKeyPlaceholder   = "{{.key}}"
		metadataTemplateValuePlaceholder = "{{.value}}"
		metadataTemplate                 = metadataTemplateKeyPlaceholder + ": " + metadataTemplateValuePlaceholder
		textTemplateMetadataPlaceholder  = "{{.metadata}}"
		textTemplateContentPlaceholder   = "{{.content}}"
		textTemplate                     = textTemplateMetadataPlaceholder + "\n\n" + textTemplateContentPlaceholder
	)

	filteredMetadata := s.filterMetadataByMode(doc.Metadata, mode)

	metadataEntries := make([]string, 0, len(filteredMetadata))
	for key, value := range filteredMetadata {
		formattedEntry := strings.ReplaceAll(metadataTemplate, metadataTemplateKeyPlaceholder, key)
		formattedEntry = strings.ReplaceAll(formattedEntry, metadataTemplateValuePlaceholder, cast.ToString(value))
		metadataEntries = append(metadataEntries, formattedEntry)
	}

	metadataText := strings.Join(metadataEntries, metadataSeparator)
	finalResult := strings.ReplaceAll(textTemplate, textTemplateMetadataPlaceholder, metadataText)
	finalResult = strings.ReplaceAll(finalResult, textTemplateContentPlaceholder, doc.Text)

	return finalResult
}

func (s *SimpleFormatter) filterMetadataByMode(metadata map[string]any, mode MetadataMode) map[string]any {
	if mode == MetadataModeAll {
		return metadata
	}
	if mode == MetadataModeNone {
		return make(map[string]any)
	}

	clonedMetadata := maps.Clone(metadata)
	var excludeKeyFunc func(key string, value any) bool

	switch mode {
	case MetadataModeInference:
		excludeKeyFunc = func(key string, _ any) bool {
			return slices.Contains(s.excludedInferenceMetadataKeys, key)
		}
	case MetadataModeEmbed:
		excludeKeyFunc = func(key string, _ any) bool {
			return slices.Contains(s.excludedEmbedMetadataKeys, key)
		}
	}

	if excludeKeyFunc != nil {
		maps.DeleteFunc(clonedMetadata, excludeKeyFunc)
	}

	return clonedMetadata
}
