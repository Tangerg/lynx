package transformers

import (
	"context"

	"github.com/Tangerg/lynx/ai/content/document"
)

var _ document.Transformer = (*FormatTransformer)(nil)

type FormatTransformer struct {
	disableTemplateRewrite bool
	formatter              document.Formatter
}

func NewFormatTransformer(formatter document.Formatter, disableTemplateRewrite bool) *FormatTransformer {
	return &FormatTransformer{
		formatter:              formatter,
		disableTemplateRewrite: disableTemplateRewrite,
	}
}

func (t *FormatTransformer) processDocument(doc *document.Document) {
	docFormatter, isDocDefault := doc.Formatter().(*document.DefaultFormatter)
	transformerFormatter, isTransformerDefault := t.formatter.(*document.DefaultFormatter)

	if isDocDefault && isTransformerDefault {
		t.mergeFormatters(doc, docFormatter, transformerFormatter)
	} else {
		t.replaceFormatter(doc)
	}
}

func (t *FormatTransformer) mergeFormatters(doc *document.Document, docFormatter, transformerFormatter *document.DefaultFormatter) {
	embedExcludeKeys := make([]string, 0)
	embedExcludeKeys = append(embedExcludeKeys, docFormatter.ExcludedEmbedMetadataKeys()...)
	embedExcludeKeys = append(embedExcludeKeys, transformerFormatter.ExcludedEmbedMetadataKeys()...)

	inferenceExcludeKeys := make([]string, 0)
	inferenceExcludeKeys = append(inferenceExcludeKeys, docFormatter.ExcludedInferenceMetadataKeys()...)
	inferenceExcludeKeys = append(inferenceExcludeKeys, transformerFormatter.ExcludedInferenceMetadataKeys()...)

	builder := document.NewDefaultFormatterBuilder().
		WithExcludedEmbedMetadataKeys(embedExcludeKeys).
		WithExcludedInferenceMetadataKeys(inferenceExcludeKeys).
		WithMetadataTemplate(docFormatter.MetadataTemplate()).
		WithMetadataSeparator(docFormatter.MetadataSeparator())

	if !t.disableTemplateRewrite {
		builder = builder.WithTextTemplate(docFormatter.TextTemplate())
	}

	doc.SetFormatter(builder.Build())
}

func (t *FormatTransformer) replaceFormatter(doc *document.Document) {
	doc.SetFormatter(t.formatter)
}

func (t *FormatTransformer) Transform(_ context.Context, docs []*document.Document) ([]*document.Document, error) {
	if t.formatter == nil {
		return docs, nil
	}

	for _, doc := range docs {
		t.processDocument(doc)
	}

	return docs, nil
}
