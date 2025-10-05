package formatters

import (
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/cast"

	"github.com/Tangerg/lynx/ai/media/document"
)

const (
	DefaultMetadataSeparator                = "\n"
	defaultMetadataTemplateKeyPlaceholder   = "{{.key}}"
	defaultMetadataTemplateValuePlaceholder = "{{.value}}"
	DefaultMetadataTemplate                 = defaultMetadataTemplateKeyPlaceholder + ": " + defaultMetadataTemplateValuePlaceholder
	defaultTextTemplateMetadataPlaceholder  = "{{.metadata}}"
	defaultTextTemplateContentPlaceholder   = "{{.content}}"
	DefaultTextTemplate                     = defaultTextTemplateMetadataPlaceholder + "\n\n" + defaultTextTemplateContentPlaceholder
)

var _ document.Formatter = (*SimpleFormatter)(nil)

type SimpleFormatter struct {
	once                          sync.Once
	MetadataTemplate              string
	MetadataSeparator             string
	TextTemplate                  string
	ExcludedInferenceMetadataKeys []string
	ExcludedEmbedMetadataKeys     []string
}

func (s *SimpleFormatter) initializeDefaultValues() {
	s.once.Do(func() {
		if s.MetadataTemplate == "" {
			s.MetadataTemplate = DefaultMetadataTemplate
		}
		if s.MetadataSeparator == "" {
			s.MetadataSeparator = DefaultMetadataSeparator
		}
		if s.TextTemplate == "" {
			s.TextTemplate = DefaultTextTemplate
		}
	})
}

func (s *SimpleFormatter) Format(doc *document.Document, mode document.MetadataMode) string {
	s.initializeDefaultValues()

	filteredMetadata := s.filterMetadataByMode(doc.Metadata, mode)

	var metadataEntries []string
	for key, value := range filteredMetadata {
		formattedEntry := strings.ReplaceAll(s.MetadataTemplate, defaultMetadataTemplateKeyPlaceholder, key)
		formattedEntry = strings.ReplaceAll(formattedEntry, defaultMetadataTemplateValuePlaceholder, cast.ToString(value))
		metadataEntries = append(metadataEntries, formattedEntry)
	}

	metadataText := strings.Join(metadataEntries, s.MetadataSeparator)
	finalResult := strings.ReplaceAll(s.TextTemplate, defaultTextTemplateMetadataPlaceholder, metadataText)
	finalResult = strings.ReplaceAll(finalResult, defaultTextTemplateContentPlaceholder, doc.Text)

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
			return slices.Contains(s.ExcludedInferenceMetadataKeys, key)
		}
	case document.MetadataModeEmbed:
		excludeKeyFunc = func(key string, _ any) bool {
			return slices.Contains(s.ExcludedEmbedMetadataKeys, key)
		}
	}

	if excludeKeyFunc != nil {
		maps.DeleteFunc(clonedMetadata, excludeKeyFunc)
	}

	return clonedMetadata
}
