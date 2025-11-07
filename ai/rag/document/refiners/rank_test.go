package refiners

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/rag"
)

func TestNewRankRefiner(t *testing.T) {
	tests := []struct {
		name     string
		topK     int
		expected int
	}{
		{
			name:     "valid topK",
			topK:     5,
			expected: 5,
		},
		{
			name:     "zero topK defaults to 1",
			topK:     0,
			expected: 1,
		},
		{
			name:     "negative topK defaults to 1",
			topK:     -5,
			expected: 1,
		},
		{
			name:     "topK equals 1",
			topK:     1,
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refiner := NewRankRefiner(tt.topK)
			assert.NotNil(t, refiner)
			assert.Equal(t, tt.expected, refiner.topK)
			assert.Implements(t, (*rag.DocumentRefiner)(nil), refiner)
		})
	}
}

func TestRankRefiner_Refine(t *testing.T) {
	tests := []struct {
		name      string
		topK      int
		documents []*document.Document
		expected  []*document.Document
		wantErr   bool
	}{
		{
			name:      "empty documents",
			topK:      3,
			documents: []*document.Document{},
			expected:  []*document.Document{},
			wantErr:   false,
		},
		{
			name: "documents less than topK",
			topK: 5,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.7),
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc3", "Content 3", 0.8),
			},
			expected: []*document.Document{
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc3", "Content 3", 0.8),
				createDoc(t, "doc1", "Content 1", 0.7),
			},
			wantErr: false,
		},
		{
			name: "documents equal to topK",
			topK: 3,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.7),
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc3", "Content 3", 0.8),
			},
			expected: []*document.Document{
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc3", "Content 3", 0.8),
				createDoc(t, "doc1", "Content 1", 0.7),
			},
			wantErr: false,
		},
		{
			name: "documents more than topK",
			topK: 3,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.5),
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc3", "Content 3", 0.7),
				createDoc(t, "doc4", "Content 4", 0.6),
				createDoc(t, "doc5", "Content 5", 0.8),
			},
			expected: []*document.Document{
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc5", "Content 5", 0.8),
				createDoc(t, "doc3", "Content 3", 0.7),
			},
			wantErr: false,
		},
		{
			name: "topK equals 1 returns highest score",
			topK: 1,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.5),
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc3", "Content 3", 0.7),
			},
			expected: []*document.Document{
				createDoc(t, "doc2", "Content 2", 0.9),
			},
			wantErr: false,
		},
		{
			name: "documents with same scores maintain stable sort",
			topK: 3,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.8),
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc3", "Content 3", 0.8),
				createDoc(t, "doc4", "Content 4", 0.7),
			},
			expected: []*document.Document{
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc1", "Content 1", 0.8),
				createDoc(t, "doc3", "Content 3", 0.8),
			},
			wantErr: false,
		},
		{
			name: "single document",
			topK: 3,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
			},
			expected: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
			},
			wantErr: false,
		},
		{
			name: "documents already sorted",
			topK: 3,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc2", "Content 2", 0.8),
				createDoc(t, "doc3", "Content 3", 0.7),
				createDoc(t, "doc4", "Content 4", 0.6),
			},
			expected: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.9),
				createDoc(t, "doc2", "Content 2", 0.8),
				createDoc(t, "doc3", "Content 3", 0.7),
			},
			wantErr: false,
		},
		{
			name: "documents reverse sorted",
			topK: 3,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.5),
				createDoc(t, "doc2", "Content 2", 0.6),
				createDoc(t, "doc3", "Content 3", 0.7),
				createDoc(t, "doc4", "Content 4", 0.9),
			},
			expected: []*document.Document{
				createDoc(t, "doc4", "Content 4", 0.9),
				createDoc(t, "doc3", "Content 3", 0.7),
				createDoc(t, "doc2", "Content 2", 0.6),
			},
			wantErr: false,
		},
		{
			name: "documents with zero scores",
			topK: 2,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.0),
				createDoc(t, "doc2", "Content 2", 0.5),
				createDoc(t, "doc3", "Content 3", 0.0),
			},
			expected: []*document.Document{
				createDoc(t, "doc2", "Content 2", 0.5),
				createDoc(t, "doc1", "Content 1", 0.0),
			},
			wantErr: false,
		},
		{
			name: "documents with negative scores",
			topK: 3,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", -0.5),
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc3", "Content 3", 0.0),
				createDoc(t, "doc4", "Content 4", -0.3),
			},
			expected: []*document.Document{
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc3", "Content 3", 0.0),
				createDoc(t, "doc4", "Content 4", -0.3),
			},
			wantErr: false,
		},
		{
			name: "large topK with few documents",
			topK: 100,
			documents: []*document.Document{
				createDoc(t, "doc1", "Content 1", 0.7),
				createDoc(t, "doc2", "Content 2", 0.9),
			},
			expected: []*document.Document{
				createDoc(t, "doc2", "Content 2", 0.9),
				createDoc(t, "doc1", "Content 1", 0.7),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refiner := NewRankRefiner(tt.topK)
			ctx := context.Background()
			query := &rag.Query{}

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

func TestRankRefiner_Refine_DoesNotModifyOriginal(t *testing.T) {
	refiner := NewRankRefiner(2)
	original := []*document.Document{
		createDoc(t, "doc1", "Content 1", 0.5),
		createDoc(t, "doc2", "Content 2", 0.9),
		createDoc(t, "doc3", "Content 3", 0.7),
	}

	originalCopy := make([]*document.Document, len(original))
	copy(originalCopy, original)

	ctx := context.Background()
	query := &rag.Query{}

	result, err := refiner.Refine(ctx, query, original)
	require.NoError(t, err)

	assert.Equal(t, 2, len(result))
	assert.Equal(t, len(originalCopy), len(original))
	for i := range original {
		assert.Equal(t, originalCopy[i].ID, original[i].ID)
		assert.Equal(t, originalCopy[i].Score, original[i].Score)
	}
}

func TestRankRefiner_Refine_NilContext(t *testing.T) {
	refiner := NewRankRefiner(2)
	documents := []*document.Document{
		createDoc(t, "doc1", "Content 1", 0.9),
		createDoc(t, "doc2", "Content 2", 0.7),
	}

	result, err := refiner.Refine(nil, &rag.Query{}, documents)
	require.NoError(t, err)
	assert.Equal(t, 2, len(result))
}

func TestRankRefiner_Refine_NilQuery(t *testing.T) {
	refiner := NewRankRefiner(2)
	documents := []*document.Document{
		createDoc(t, "doc1", "Content 1", 0.9),
		createDoc(t, "doc2", "Content 2", 0.7),
	}

	result, err := refiner.Refine(context.Background(), nil, documents)
	require.NoError(t, err)
	assert.Equal(t, 2, len(result))
}

func TestRankRefiner_Refine_NilDocuments(t *testing.T) {
	refiner := NewRankRefiner(3)
	ctx := context.Background()
	query := &rag.Query{}

	result, err := refiner.Refine(ctx, query, nil)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result))
}
