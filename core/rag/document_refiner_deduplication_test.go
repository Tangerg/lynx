package rag

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
)

func TestNewDeduplicationRefiner(t *testing.T) {
	refiner := NewDeduplicationDocumentRefiner()
	assert.NotNil(t, refiner)
	assert.Implements(t, (*DocumentRefiner)(nil), refiner)
}

func TestDeduplicationRefiner_Refine(t *testing.T) {
	tests := []struct {
		name      string
		documents []*document.Document
		expected  []*document.Document
		wantErr   bool
	}{
		{
			name:      "empty documents",
			documents: []*document.Document{},
			expected:  []*document.Document{},
			wantErr:   false,
		},
		{
			name: "no duplicates",
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc2", "Content 2", 0.8),
				createDoc(t, "doc3", "Content 3", 0.7),
			},
			expected: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc2", "Content 2", 0.8),
				createDoc(t, "doc3", "Content 3", 0.7),
			},
			wantErr: false,
		},
		{
			name: "all duplicates",
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc1", "Content 1 Duplicate", 0.85),
				createDoc(t, "doc1", "Content 1 Another", 0.8),
			},
			expected: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
			},
			wantErr: false,
		},
		{
			name: "partial duplicates preserves first occurrence",
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc2", "Content 2", 0.85),
				createDoc(t, "doc1", "Content 1 Duplicate", 0.8),
				createDoc(t, "doc3", "Content 3", 0.75),
				createDoc(t, "doc2", "Content 2 Duplicate", 0.7),
			},
			expected: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc2", "Content 2", 0.85),
				createDoc(t, "doc3", "Content 3", 0.75),
			},
			wantErr: false,
		},
		{
			name: "multiple duplicates at different positions",
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc2", "Content 2", 0.85),
				createDoc(t, "doc3", "Content 3", 0.8),
				createDoc(t, "doc1", "Duplicate 1", 0.75),
				createDoc(t, "doc4", "Content 4", 0.7),
				createDoc(t, "doc2", "Duplicate 2", 0.65),
				createDoc(t, "doc3", "Duplicate 3", 0.6),
			},
			expected: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc2", "Content 2", 0.85),
				createDoc(t, "doc3", "Content 3", 0.8),
				createDoc(t, "doc4", "Content 4", 0.7),
			},
			wantErr: false,
		},
		{
			name: "consecutive duplicates",
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc1", "Content 1 Dup", 0.85),
				createDoc(t, "doc2", "Content 2", 0.8),
				createDoc(t, "doc2", "Content 2 Dup", 0.75),
				createDoc(t, "doc3", "Content 3", 0.7),
			},
			expected: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc2", "Content 2", 0.8),
				createDoc(t, "doc3", "Content 3", 0.7),
			},
			wantErr: false,
		},
		{
			name: "single document",
			documents: []*document.Document{
				createDoc(t, "doc1", "Single Content", 0.9),
			},
			expected: []*document.Document{
				createDoc(t, "doc1", "Single Content", 0.9),
			},
			wantErr: false,
		},
		{
			name: "documents with same content but different IDs",
			documents: []*document.Document{
				createDoc(t, "doc1", "Same Content", 0.9),
				createDoc(t, "doc2", "Same Content", 0.8),
				createDoc(t, "doc3", "Same Content", 0.7),
			},
			expected: []*document.Document{
				createDoc(t, "doc1", "Same Content", 0.9),
				createDoc(t, "doc2", "Same Content", 0.8),
				createDoc(t, "doc3", "Same Content", 0.7),
			},
			wantErr: false,
		},
		{
			name: "documents with metadata duplicates removed",
			documents: []*document.Document{
				createDocWithMetadata(t, "doc1", "Content 1", 0.9, map[string]any{"source": "db"}),
				createDocWithMetadata(t, "doc1", "Content 1", 0.8, map[string]any{"source": "cache"}),
				createDocWithMetadata(t, "doc2", "Content 2", 0.7, map[string]any{"source": "api"}),
			},
			expected: []*document.Document{
				createDocWithMetadata(t, "doc1", "Content 1", 0.9, map[string]any{"source": "db"}),
				createDocWithMetadata(t, "doc2", "Content 2", 0.7, map[string]any{"source": "api"}),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refiner := NewDeduplicationDocumentRefiner()
			ctx := context.Background()
			query := &Query{}

			result, err := refiner.Refine(ctx, query, tt.documents)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, len(tt.expected), len(result))

			for i, expectedDoc := range tt.expected {
				assert.Equal(t, expectedDoc.ID, result[i].ID)
				assert.Equal(t, expectedDoc.Text, result[i].Text)
				assert.Equal(t, expectedDoc.Score, result[i].Score)
			}
		})
	}
}

func TestDeduplicationRefiner_Refine_NilQuery(t *testing.T) {
	refiner := NewDeduplicationDocumentRefiner()
	documents := []*document.Document{
		createDoc(t, "doc1", "Content 1", 0.9),
	}

	result, err := refiner.Refine(context.Background(), nil, documents)
	require.NoError(t, err)
	assert.Equal(t, 1, len(result))
}

func TestDeduplicationRefiner_Refine_NilDocuments(t *testing.T) {
	refiner := NewDeduplicationDocumentRefiner()
	ctx := context.Background()
	query := &Query{}

	result, err := refiner.Refine(ctx, query, nil)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result))
}
