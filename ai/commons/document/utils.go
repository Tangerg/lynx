package document

import (
	"context"
	"github.com/spf13/cast"
	"maps"
	"slices"
	"strings"
)

// Reader defines an interface for reading documents from a source.
// Implementations can retrieve documents from various data sources such as
// databases, file systems, or external APIs.
type Reader interface {
	// Read retrieves documents from the underlying source.
	//
	// Parameters:
	//   ctx - Context for request cancellation and timeout
	//
	// Returns:
	//   A slice of retrieved Document objects and nil error if successful
	//   nil and an error if the operation fails
	Read(ctx context.Context) ([]*Document, error)
}

// Writer defines an interface for writing documents to a destination.
// Implementations can store documents in various data stores such as
// databases, file systems, or external services.
type Writer interface {
	// Write stores documents in the underlying destination.
	//
	// Parameters:
	//   ctx  - Context for request cancellation and timeout
	//   docs - Slice of Document objects to be stored
	//
	// Returns:
	//   nil if successful, or an error if the operation fails
	Write(ctx context.Context, docs []*Document) error
}

// Transformer defines an interface for processing and transforming documents.
// Implementations can modify, filter, enrich, or otherwise transform documents
// as they pass through a processing pipeline.
type Transformer interface {
	// Transform processes a batch of documents and returns the transformed result.
	//
	// Parameters:
	//   ctx  - Context for request cancellation and timeout
	//   docs - Input documents to be transformed
	//
	// Returns:
	//   A slice of transformed Document objects and nil error if successful
	//   nil and an error if the transformation fails
	Transform(ctx context.Context, docs []*Document) ([]*Document, error)
}

// MetadataMode defines how metadata should be handled when formatting document content.
// Different modes are useful in various contexts such as embedding, inference, or display.
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

// ContentFormatter defines an interface for formatting document content
// according to specific needs and metadata inclusion rules.
type ContentFormatter interface {
	// Format produces a string representation of a document, controlling
	// how metadata is included based on the specified mode.
	//
	// Parameters:
	//   document - The document to be formatted
	//   mode     - Controls which metadata to include in the formatted result
	//
	// Returns:
	//   A formatted string representation of the document
	Format(document *Document, mode MetadataMode) string
}

const (
	// DefaultMetadataSeparator Default separator for metadata entries in formatted content
	DefaultMetadataSeparator = "\n"

	// MetadataTemplateKeyPlaceholder Placeholder for the key in metadata template
	MetadataTemplateKeyPlaceholder = "{{.key}}"

	// MetadataTemplateValuePlaceholder Placeholder for the value in metadata template
	MetadataTemplateValuePlaceholder = "{{.value}}"

	// DefaultMetadataTemplate Default template format for metadata entries
	DefaultMetadataTemplate = MetadataTemplateKeyPlaceholder + ": " + MetadataTemplateValuePlaceholder

	// TextTemplateMetadataPlaceholder Placeholder for all metadata content in text template
	TextTemplateMetadataPlaceholder = "{{.metadata}}"

	// TextTemplateContentPlaceholder Placeholder for document content in text template
	TextTemplateContentPlaceholder = "{{.content}}"

	// DefaultTextTemplate Default template format for the entire document content
	DefaultTextTemplate = TextTemplateMetadataPlaceholder + "\n\n" + TextTemplateContentPlaceholder
)

var _ ContentFormatter = (*DefaultContentFormatter)(nil)

// DefaultContentFormatter provides a standard implementation of the ContentFormatter interface.
// It formats documents based on configurable templates and can filter metadata based on mode.
type DefaultContentFormatter struct {
	metadataTemplate              string   // Template used to format individual metadata entries
	metadataSeparator             string   // Separator between metadata entries
	textTemplate                  string   // Template for combining metadata and document content
	excludedInferenceMetadataKeys []string // Metadata keys to exclude in Inference mode
	excludedEmbedMetadataKeys     []string // Metadata keys to exclude in Embed mode
}

// MetadataTemplate returns the template used to format individual metadata entries
func (d *DefaultContentFormatter) MetadataTemplate() string {
	return d.metadataTemplate
}

// MetadataSeparator returns the separator used between metadata entries
func (d *DefaultContentFormatter) MetadataSeparator() string {
	return d.metadataSeparator
}

// TextTemplate returns the template used to combine metadata and document content
func (d *DefaultContentFormatter) TextTemplate() string {
	return d.textTemplate
}

// ExcludedInferenceMetadataKeys returns a copy of the list of metadata keys excluded in Inference mode
func (d *DefaultContentFormatter) ExcludedInferenceMetadataKeys() []string {
	// Return a copy to prevent external modification
	return slices.Clone(d.excludedInferenceMetadataKeys)
}

// ExcludedEmbedMetadataKeys returns a copy of the list of metadata keys excluded in Embed mode
func (d *DefaultContentFormatter) ExcludedEmbedMetadataKeys() []string {
	// Return a copy to prevent external modification
	return slices.Clone(d.excludedEmbedMetadataKeys)
}

// Format produces a string representation of a document according to the formatter's templates
// and the specified metadata mode.
//
// The method:
// 1. Filters metadata based on the specified mode
// 2. Formats each metadata entry using the metadata template
// 3. Combines the formatted metadata with the document content using the text template
//
// Parameters:
//
//	document - The document to format
//	mode - Controls which metadata to include (All, Embed, Inference, or None)
//
// Returns:
//
//	A formatted string representation of the document
func (d *DefaultContentFormatter) Format(document *Document, mode MetadataMode) string {
	metadata := d.metadataFilter(document.metadata, mode)
	var metaTexts []string
	for k, v := range metadata {
		text := strings.ReplaceAll(d.metadataTemplate, MetadataTemplateKeyPlaceholder, k)
		text = strings.ReplaceAll(text, MetadataTemplateValuePlaceholder, cast.ToString(v))
		metaTexts = append(metaTexts, text)
	}
	text := strings.ReplaceAll(d.textTemplate, TextTemplateMetadataPlaceholder, strings.Join(metaTexts, d.metadataSeparator))
	text = strings.ReplaceAll(text, TextTemplateContentPlaceholder, document.text)
	return text
}

// metadataFilter filters the document metadata based on the specified mode.
//
// Parameters:
//
//	metadata - The original metadata map
//	mode - The filtering mode (All, Embed, Inference, or None)
//
// Returns:
//
//	A filtered copy of the metadata map
func (d *DefaultContentFormatter) metadataFilter(metadata map[string]any, mode MetadataMode) map[string]any {
	if mode == MetadataModeAll {
		return metadata
	}
	if mode == MetadataModeNone {
		return make(map[string]any)
	}
	cloneMetadata := maps.Clone(metadata)
	var deleteFunc func(key string, value any) bool
	if mode == MetadataModeInference {
		deleteFunc = func(key string, _ any) bool {
			return slices.Contains(d.excludedInferenceMetadataKeys, key)
		}
	}
	if mode == MetadataModeEmbed {
		deleteFunc = func(key string, _ any) bool {
			return slices.Contains(d.excludedEmbedMetadataKeys, key)
		}
	}
	maps.DeleteFunc(cloneMetadata, deleteFunc)
	return cloneMetadata
}

// DefaultContentFormatterBuilder implements the builder pattern for creating
// DefaultContentFormatter instances with custom configurations.
type DefaultContentFormatterBuilder struct {
	metadataTemplate              string   // Template for metadata entries
	metadataSeparator             string   // Separator between metadata entries
	textTemplate                  string   // Template for combining metadata and content
	excludedInferenceMetadataKeys []string // Keys to exclude in Inference mode
	excludedEmbedMetadataKeys     []string // Keys to exclude in Embed mode
}

// NewDefaultContentFormatterBuilder creates a new builder for DefaultContentFormatter
func NewDefaultContentFormatterBuilder() *DefaultContentFormatterBuilder {
	return &DefaultContentFormatterBuilder{}
}

// WithMetadataTemplate sets a custom template for formatting metadata entries
func (b *DefaultContentFormatterBuilder) WithMetadataTemplate(metadataTemplate string) *DefaultContentFormatterBuilder {
	b.metadataTemplate = metadataTemplate
	return b
}

// WithMetadataSeparator sets a custom separator between metadata entries
func (b *DefaultContentFormatterBuilder) WithMetadataSeparator(metadataSeparator string) *DefaultContentFormatterBuilder {
	b.metadataSeparator = metadataSeparator
	return b
}

// WithTextTemplate sets a custom template for combining metadata and document content
func (b *DefaultContentFormatterBuilder) WithTextTemplate(textTemplate string) *DefaultContentFormatterBuilder {
	b.textTemplate = textTemplate
	return b
}

// WithExcludedInferenceMetadataKeys sets the list of metadata keys to exclude in Inference mode
func (b *DefaultContentFormatterBuilder) WithExcludedInferenceMetadataKeys(excludedInferenceMetadataKeys []string) *DefaultContentFormatterBuilder {
	b.excludedInferenceMetadataKeys = excludedInferenceMetadataKeys
	return b
}

// WithExcludedEmbedMetadataKeys sets the list of metadata keys to exclude in Embed mode
func (b *DefaultContentFormatterBuilder) WithExcludedEmbedMetadataKeys(excludedEmbedMetadataKeys []string) *DefaultContentFormatterBuilder {
	b.excludedEmbedMetadataKeys = excludedEmbedMetadataKeys
	return b
}

// Build creates a new DefaultContentFormatter with the configured settings
// If any settings are not specified, defaults will be used.
//
// Returns:
//
//	A new DefaultContentFormatter instance
func (b *DefaultContentFormatterBuilder) Build() *DefaultContentFormatter {
	if b.metadataTemplate == "" {
		b.metadataTemplate = DefaultMetadataTemplate
	}
	if b.metadataSeparator == "" {
		b.metadataSeparator = DefaultMetadataSeparator
	}
	if b.textTemplate == "" {
		b.textTemplate = DefaultTextTemplate
	}
	return &DefaultContentFormatter{
		metadataTemplate:              b.metadataTemplate,
		metadataSeparator:             b.metadataSeparator,
		textTemplate:                  b.textTemplate,
		excludedInferenceMetadataKeys: b.excludedInferenceMetadataKeys,
		excludedEmbedMetadataKeys:     b.excludedEmbedMetadataKeys,
	}
}
