package transformer

import (
	"context"
	"github.com/Tangerg/lynx/ai/commons/document"
)

var _ document.Transformer = (*ContentFormatTransformer)(nil)

// ContentFormatTransformer processes a list of documents by applying a content formatter
// to each document.
type ContentFormatTransformer struct {
	// disableTemplateRewrite disables the content-formatter template rewrite
	disableTemplateRewrite bool
	contentFormatter       document.ContentFormatter
}

// NewContentFormatTransformer creates a new ContentFormatTransformer with the specified
// content formatter and template rewrite settings.
func NewContentFormatTransformer(contentFormatter document.ContentFormatter, disableTemplateRewrite bool) *ContentFormatTransformer {
	return &ContentFormatTransformer{
		contentFormatter:       contentFormatter,
		disableTemplateRewrite: disableTemplateRewrite,
	}
}

// processDocument applies the content formatter to a single document.
// If both the document's formatter and the transformer's formatter are DefaultContentFormatter,
// it merges their configurations. Otherwise, it replaces the document's formatter entirely.
func (t *ContentFormatTransformer) processDocument(doc *document.Document) {
	docFormatter, docIsDefault := doc.ContentFormatter().(*document.DefaultContentFormatter)
	toUpdateFormatter, updateIsDefault := t.contentFormatter.(*document.DefaultContentFormatter)

	if docIsDefault && updateIsDefault {
		t.updateFormatter(doc, docFormatter, toUpdateFormatter)
	} else {
		t.overrideFormatter(doc)
	}
}

// updateFormatter merges the configuration from two DefaultContentFormatter instances.
// It combines excluded metadata keys from both formatters and preserves the original
// document's template settings unless template rewrite is disabled.
func (t *ContentFormatTransformer) updateFormatter(doc *document.Document, docFormatter, toUpdateFormatter *document.DefaultContentFormatter) {
	// Merge excluded embed metadata keys
	updatedEmbedExcludeKeys := make([]string, 0)
	updatedEmbedExcludeKeys = append(updatedEmbedExcludeKeys, docFormatter.ExcludedEmbedMetadataKeys()...)
	updatedEmbedExcludeKeys = append(updatedEmbedExcludeKeys, toUpdateFormatter.ExcludedEmbedMetadataKeys()...)

	// Merge excluded inference metadata keys
	updatedInferenceExcludeKeys := make([]string, 0)
	updatedInferenceExcludeKeys = append(updatedInferenceExcludeKeys, docFormatter.ExcludedInferenceMetadataKeys()...)
	updatedInferenceExcludeKeys = append(updatedInferenceExcludeKeys, toUpdateFormatter.ExcludedInferenceMetadataKeys()...)

	builder := document.NewDefaultContentFormatterBuilder().
		WithExcludedEmbedMetadataKeys(updatedEmbedExcludeKeys).
		WithExcludedInferenceMetadataKeys(updatedInferenceExcludeKeys).
		WithMetadataTemplate(docFormatter.MetadataTemplate()).
		WithMetadataSeparator(docFormatter.MetadataSeparator())

	if !t.disableTemplateRewrite {
		builder = builder.WithTextTemplate(docFormatter.TextTemplate())
	}

	doc.SetContentFormatter(builder.Build())
}

// overrideFormatter replaces the document's content formatter with the transformer's formatter.
func (t *ContentFormatTransformer) overrideFormatter(doc *document.Document) {
	doc.SetContentFormatter(t.contentFormatter)
}

// Transform applies the content formatter to each document in the list.
// Allows transformers to be chained for sequential processing.
func (t *ContentFormatTransformer) Transform(_ context.Context, docs []*document.Document) ([]*document.Document, error) {
	if t.contentFormatter != nil {
		for _, doc := range docs {
			t.processDocument(doc)
		}
	}
	return docs, nil
}
