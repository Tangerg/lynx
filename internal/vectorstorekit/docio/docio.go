package docio

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/lynx/core/document"
	"github.com/Tangerg/lynx/core/vectorstore"
)

// ValidateDocuments enforces the provider-independent ingestion contract
// before a batcher, embedding model, or store client can observe the input.
func ValidateDocuments(docs []*document.Document) error {
	if len(docs) == 0 {
		return vectorstore.ErrEmptyDocuments
	}

	seen := make(map[string]int, len(docs))
	for i, doc := range docs {
		if doc == nil {
			return fmt.Errorf("%w: documents[%d] is nil", vectorstore.ErrInvalidDocument, i)
		}
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("%w: documents[%d]: %w", vectorstore.ErrInvalidDocument, i, err)
		}
		if strings.TrimSpace(doc.ID) == "" {
			return fmt.Errorf("%w: documents[%d]", vectorstore.ErrMissingDocumentID, i)
		}
		if doc.Text == "" {
			return fmt.Errorf("%w: documents[%d] has no text to embed", vectorstore.ErrInvalidDocument, i)
		}
		if first, duplicate := seen[doc.ID]; duplicate {
			return fmt.Errorf("%w %q at documents[%d] and documents[%d]",
				vectorstore.ErrDuplicateDocumentID, doc.ID, first, i)
		}
		seen[doc.ID] = i
	}
	return nil
}

// FormatVectorLiteral renders a float32 slice as the textual
// "[v1,v2,...]" form accepted by pgvector / MariaDB / Oracle / TiDB
// / Cassandra. The values use the shortest round-trippable form.
func FormatVectorLiteral(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 8)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
