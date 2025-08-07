package document

import (
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cast"
)

// MetadataMode defines how metadata should be handled when formatting document content.
type MetadataMode string

const (
	// MetadataModeAll includes all available metadata in the formatted content
	MetadataModeAll MetadataMode = "all"
	// MetadataModeEmbed includes only metadata relevant for embedding processes
	MetadataModeEmbed MetadataMode = "embed"
	// MetadataModeInference includes only metadata relevant for inference operations
	MetadataModeInference MetadataMode = "inference"
	// MetadataModeNone excludes all metadata from the formatted content
	MetadataModeNone MetadataMode = "none"
)

// Formatter defines an interface for formatting document content
// according to specific needs and metadata inclusion rules.
type Formatter interface {
	// Format produces a string representation of a document with controlled metadata inclusion.
	Format(document *Document, mode MetadataMode) string
}

const (
	DefaultMetadataSeparator                = "\n"
	DefaultMetadataTemplateKeyPlaceholder   = "{{.key}}"
	DefaultMetadataTemplateValuePlaceholder = "{{.value}}"
	DefaultMetadataTemplate                 = DefaultMetadataTemplateKeyPlaceholder + ": " + DefaultMetadataTemplateValuePlaceholder
	DefaultTextTemplateMetadataPlaceholder  = "{{.metadata}}"
	DefaultTextTemplateContentPlaceholder   = "{{.content}}"
	DefaultTextTemplate                     = DefaultTextTemplateMetadataPlaceholder + "\n\n" + DefaultTextTemplateContentPlaceholder
)

var _ Formatter = (*DefaultFormatter)(nil)

// DefaultFormatter provides a standard implementation of the Formatter interface.
// It formats documents based on configurable templates and filters metadata by mode.
type DefaultFormatter struct {
	metadataTemplate              string
	metadataSeparator             string
	textTemplate                  string
	excludedInferenceMetadataKeys []string
	excludedEmbedMetadataKeys     []string
}

// MetadataTemplate returns the template used to format individual metadata entries.
func (d *DefaultFormatter) MetadataTemplate() string {
	return d.metadataTemplate
}

// MetadataSeparator returns the separator used between metadata entries.
func (d *DefaultFormatter) MetadataSeparator() string {
	return d.metadataSeparator
}

// TextTemplate returns the template used to combine metadata and document content.
func (d *DefaultFormatter) TextTemplate() string {
	return d.textTemplate
}

// ExcludedInferenceMetadataKeys returns metadata keys excluded in Inference mode.
func (d *DefaultFormatter) ExcludedInferenceMetadataKeys() []string {
	return d.excludedInferenceMetadataKeys
}

// ExcludedEmbedMetadataKeys returns metadata keys excluded in Embed mode.
func (d *DefaultFormatter) ExcludedEmbedMetadataKeys() []string {
	return d.excludedEmbedMetadataKeys
}

// Format produces a string representation of a document with controlled metadata inclusion.
// Filters metadata based on mode, formats entries using templates, then combines with content.
func (d *DefaultFormatter) Format(document *Document, mode MetadataMode) string {
	filteredMetadata := d.filterMetadata(document.metadata, mode)

	var formattedEntries []string
	for key, value := range filteredMetadata {
		entry := strings.ReplaceAll(d.metadataTemplate, DefaultMetadataTemplateKeyPlaceholder, key)
		entry = strings.ReplaceAll(entry, DefaultMetadataTemplateValuePlaceholder, cast.ToString(value))
		formattedEntries = append(formattedEntries, entry)
	}

	metadataText := strings.Join(formattedEntries, d.metadataSeparator)
	result := strings.ReplaceAll(d.textTemplate, DefaultTextTemplateMetadataPlaceholder, metadataText)
	result = strings.ReplaceAll(result, DefaultTextTemplateContentPlaceholder, document.text)

	return result
}

// filterMetadata filters document metadata based on the specified mode.
// Returns a filtered copy of the metadata map.
func (d *DefaultFormatter) filterMetadata(metadata map[string]any, mode MetadataMode) map[string]any {
	if mode == MetadataModeAll {
		return metadata
	}
	if mode == MetadataModeNone {
		return make(map[string]any)
	}

	filtered := maps.Clone(metadata)
	var shouldExclude func(key string, value any) bool

	switch mode {
	case MetadataModeInference:
		shouldExclude = func(key string, _ any) bool {
			return slices.Contains(d.excludedInferenceMetadataKeys, key)
		}
	case MetadataModeEmbed:
		shouldExclude = func(key string, _ any) bool {
			return slices.Contains(d.excludedEmbedMetadataKeys, key)
		}
	}

	if shouldExclude != nil {
		maps.DeleteFunc(filtered, shouldExclude)
	}

	return filtered
}

// DefaultFormatterBuilder implements the builder pattern for creating DefaultFormatter instances.
type DefaultFormatterBuilder struct {
	metadataTemplate              string
	metadataSeparator             string
	textTemplate                  string
	excludedInferenceMetadataKeys []string
	excludedEmbedMetadataKeys     []string
}

// NewDefaultFormatterBuilder creates a new builder for DefaultFormatter.
func NewDefaultFormatterBuilder() *DefaultFormatterBuilder {
	return &DefaultFormatterBuilder{}
}

// WithMetadataTemplate sets a custom template for formatting metadata entries.
func (b *DefaultFormatterBuilder) WithMetadataTemplate(template string) *DefaultFormatterBuilder {
	b.metadataTemplate = template
	return b
}

// WithMetadataSeparator sets a custom separator between metadata entries.
func (b *DefaultFormatterBuilder) WithMetadataSeparator(separator string) *DefaultFormatterBuilder {
	b.metadataSeparator = separator
	return b
}

// WithTextTemplate sets a custom template for combining metadata and document content.
func (b *DefaultFormatterBuilder) WithTextTemplate(template string) *DefaultFormatterBuilder {
	b.textTemplate = template
	return b
}

// WithExcludedInferenceMetadataKeys sets metadata keys to exclude in Inference mode.
func (b *DefaultFormatterBuilder) WithExcludedInferenceMetadataKeys(keys []string) *DefaultFormatterBuilder {
	b.excludedInferenceMetadataKeys = keys
	return b
}

// WithExcludedEmbedMetadataKeys sets metadata keys to exclude in Embed mode.
func (b *DefaultFormatterBuilder) WithExcludedEmbedMetadataKeys(keys []string) *DefaultFormatterBuilder {
	b.excludedEmbedMetadataKeys = keys
	return b
}

// Build creates a new DefaultFormatter with the configured settings.
// Uses default values for any unspecified settings.
func (b *DefaultFormatterBuilder) Build() *DefaultFormatter {
	if b.metadataTemplate == "" {
		b.metadataTemplate = DefaultMetadataTemplate
	}
	if b.metadataSeparator == "" {
		b.metadataSeparator = DefaultMetadataSeparator
	}
	if b.textTemplate == "" {
		b.textTemplate = DefaultTextTemplate
	}

	return &DefaultFormatter{
		metadataTemplate:              b.metadataTemplate,
		metadataSeparator:             b.metadataSeparator,
		textTemplate:                  b.textTemplate,
		excludedInferenceMetadataKeys: b.excludedInferenceMetadataKeys,
		excludedEmbedMetadataKeys:     b.excludedEmbedMetadataKeys,
	}
}
