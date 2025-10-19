package formatters

import (
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/media/document"
)

type SimpleFormatterConfig struct {
	ExcludedInferenceMetadataKeys []string
	ExcludedEmbedMetadataKeys     []string
}

var _ document.Formatter = (*SimpleFormatter)(nil)

type SimpleFormatter struct {
	config *SimpleFormatterConfig
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
		config: config,
	}
}

func NewDefaultSimpleFormatter() *SimpleFormatter {
	return NewSimpleFormatter(nil)
}

func (s *SimpleFormatter) Format(doc *document.Document, mode document.MetadataMode) string {
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

func (s *SimpleFormatter) filterMetadataByMode(metadata map[string]any, mode document.MetadataMode) map[string]any {
	if mode == document.MetadataModeAll {
		return metadata
	}
	if mode == document.MetadataModeNone {
		return make(map[string]any)
	}

	clonedMetadata := maps.Clone(metadata)
	var excludeKeyFunc func(key string, value any) bool

	switch mode {
	case document.MetadataModeInference:
		excludeKeyFunc = func(key string, _ any) bool {
			return slices.Contains(s.config.ExcludedInferenceMetadataKeys, key)
		}
	case document.MetadataModeEmbed:
		excludeKeyFunc = func(key string, _ any) bool {
			return slices.Contains(s.config.ExcludedEmbedMetadataKeys, key)
		}
	}

	if excludeKeyFunc != nil {
		maps.DeleteFunc(clonedMetadata, excludeKeyFunc)
	}

	return clonedMetadata
}
