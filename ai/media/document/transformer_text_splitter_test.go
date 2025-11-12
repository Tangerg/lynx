package document

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTextSplitter tests the constructor
func TestNewTextSplitter(t *testing.T) {
	t.Run("with nil config uses default", func(t *testing.T) {
		splitter := NewTextSplitter(nil)

		require.NotNil(t, splitter)
		require.NotNil(t, splitter.config)
		assert.Equal(t, "\n", splitter.config.Separator)
		assert.False(t, splitter.config.CopyFormatter)
		require.NotNil(t, splitter.splitter)
	})

	t.Run("with custom separator", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator:     ",",
			CopyFormatter: false,
		}

		splitter := NewTextSplitter(config)

		require.NotNil(t, splitter)
		assert.Equal(t, ",", splitter.config.Separator)
		assert.False(t, splitter.config.CopyFormatter)
	})

	t.Run("with copy formatter enabled", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator:     " ",
			CopyFormatter: true,
		}

		splitter := NewTextSplitter(config)

		require.NotNil(t, splitter)
		assert.True(t, splitter.config.CopyFormatter)
	})

	t.Run("with empty separator", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator:     "",
			CopyFormatter: false,
		}

		splitter := NewTextSplitter(config)

		require.NotNil(t, splitter)
		assert.Equal(t, "", splitter.config.Separator)
	})

	t.Run("with multi-character separator", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator:     "\n\n",
			CopyFormatter: false,
		}

		splitter := NewTextSplitter(config)

		require.NotNil(t, splitter)
		assert.Equal(t, "\n\n", splitter.config.Separator)
	})

	t.Run("with special character separator", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator:     "|",
			CopyFormatter: false,
		}

		splitter := NewTextSplitter(config)

		require.NotNil(t, splitter)
		assert.Equal(t, "|", splitter.config.Separator)
	})
}

// TestNewDefaultTextSplitter tests the default constructor
func TestNewDefaultTextSplitter(t *testing.T) {
	t.Run("creates splitter with default config", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		require.NotNil(t, splitter)
		require.NotNil(t, splitter.config)
		assert.Equal(t, "\n", splitter.config.Separator)
		assert.False(t, splitter.config.CopyFormatter)
	})

	t.Run("multiple calls create independent instances", func(t *testing.T) {
		splitter1 := NewDefaultTextSplitter()
		splitter2 := NewDefaultTextSplitter()

		assert.NotSame(t, splitter1, splitter2)
		assert.NotSame(t, splitter1.config, splitter2.config)
	})
}

// TestTextSplitter_Transform tests the Transform method
func TestTextSplitter_Transform(t *testing.T) {
	ctx := context.Background()

	t.Run("split by newline", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		text := "line1\nline2\nline3"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "line1", result[0].Text)
		assert.Equal(t, "line2", result[1].Text)
		assert.Equal(t, "line3", result[2].Text)
	})

	t.Run("split by space", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator: " ",
		}
		splitter := NewTextSplitter(config)

		text := "hello world test"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "hello", result[0].Text)
		assert.Equal(t, "world", result[1].Text)
		assert.Equal(t, "test", result[2].Text)
	})

	t.Run("split by comma", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator: ",",
		}
		splitter := NewTextSplitter(config)

		text := "a,b,c,d"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 4)
		assert.Equal(t, "a", result[0].Text)
		assert.Equal(t, "b", result[1].Text)
		assert.Equal(t, "c", result[2].Text)
		assert.Equal(t, "d", result[3].Text)
	})

	t.Run("split by double newline (paragraphs)", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator: "\n\n",
		}
		splitter := NewTextSplitter(config)

		text := "paragraph1\n\nparagraph2\n\nparagraph3"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "paragraph1", result[0].Text)
		assert.Equal(t, "paragraph2", result[1].Text)
		assert.Equal(t, "paragraph3", result[2].Text)
	})

	t.Run("split by pipe", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator: "|",
		}
		splitter := NewTextSplitter(config)

		text := "section1|section2|section3"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "section1", result[0].Text)
		assert.Equal(t, "section2", result[1].Text)
		assert.Equal(t, "section3", result[2].Text)
	})

	t.Run("empty string results in empty output", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		doc, err := NewDocument(" ", nil) // Space to avoid NewDocument error
		require.NoError(t, err)
		doc.Text = "" // Set to empty after creation

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("no separator in text", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		text := "single line without newline"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, text, result[0].Text)
	})

	t.Run("multiple documents", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		doc1, err := NewDocument("line1\nline2", nil)
		require.NoError(t, err)

		doc2, err := NewDocument("line3\nline4", nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc1, doc2})

		require.NoError(t, err)
		require.Len(t, result, 4)
		assert.Equal(t, "line1", result[0].Text)
		assert.Equal(t, "line2", result[1].Text)
		assert.Equal(t, "line3", result[2].Text)
		assert.Equal(t, "line4", result[3].Text)
	})

	t.Run("empty document list", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		result, err := splitter.Transform(ctx, []*Document{})

		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("preserves metadata", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		doc, err := NewDocument("line1\nline2", nil)
		require.NoError(t, err)
		doc.Metadata["source"] = "test"
		doc.Metadata["page"] = 1

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 2)

		for _, chunk := range result {
			assert.Equal(t, "test", chunk.Metadata["source"])
			assert.Equal(t, 1, chunk.Metadata["page"])
		}
	})

	t.Run("with copy formatter enabled", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator:     "\n",
			CopyFormatter: true,
		}
		splitter := NewTextSplitter(config)

		formatter := &mockFormatter{prefix: "test: "}
		doc, err := NewDocument("line1\nline2", nil)
		require.NoError(t, err)
		doc.Formatter = formatter

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 2)

		for _, chunk := range result {
			formatted := chunk.Formatter.Format(chunk, MetadataModeAll)
			assert.Contains(t, formatted, "test:")
		}
	})

	t.Run("with copy formatter disabled", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator:     "\n",
			CopyFormatter: false,
		}
		splitter := NewTextSplitter(config)

		formatter := &mockFormatter{prefix: "test: "}
		doc, err := NewDocument("line1\nline2", nil)
		require.NoError(t, err)
		doc.Formatter = formatter

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 2)

		for _, chunk := range result {
			formatted := chunk.Formatter.Format(chunk, MetadataModeAll)
			assert.NotContains(t, formatted, "test:")
		}
	})

	t.Run("filters empty chunks", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		text := "line1\n\n\nline2\n\nline3"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		// Empty strings between separators are filtered out
		assert.Greater(t, len(result), 0)

		for _, chunk := range result {
			assert.NotEmpty(t, chunk.Text)
		}
	})
}

// TestTextSplitter_EdgeCases tests edge cases
func TestTextSplitter_EdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("unicode text", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		text := "你好\n世界\n🌍"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "你好", result[0].Text)
		assert.Equal(t, "世界", result[1].Text)
		assert.Equal(t, "🌍", result[2].Text)
	})

	t.Run("very long text", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		lines := make([]string, 10000)
		for i := range lines {
			lines[i] = "line"
		}
		text := strings.Join(lines, "\n")

		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		assert.Len(t, result, 10000)
	})

	t.Run("whitespace handling", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		text := "  line1  \n  line2  \n  line3  "
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		// Whitespace is preserved
		assert.Equal(t, "  line1  ", result[0].Text)
		assert.Equal(t, "  line2  ", result[1].Text)
		assert.Equal(t, "  line3  ", result[2].Text)
	})

	t.Run("special characters in separator", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator: "|||",
		}
		splitter := NewTextSplitter(config)

		text := "part1|||part2|||part3"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "part1", result[0].Text)
		assert.Equal(t, "part2", result[1].Text)
		assert.Equal(t, "part3", result[2].Text)
	})

	t.Run("tab separator", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator: "\t",
		}
		splitter := NewTextSplitter(config)

		text := "col1\tcol2\tcol3"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "col1", result[0].Text)
		assert.Equal(t, "col2", result[1].Text)
		assert.Equal(t, "col3", result[2].Text)
	})

	t.Run("mixed line endings", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		text := "line1\nline2\rline3\r\nline4"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		// Only splits on \n
		assert.Greater(t, len(result), 0)
	})

	t.Run("consecutive separators", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator: ",",
		}
		splitter := NewTextSplitter(config)

		text := "a,,,b,,c,d"
		doc, err := NewDocument(text, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		// Empty strings are filtered out by Splitter
		for _, chunk := range result {
			assert.NotEmpty(t, chunk.Text)
		}
	})
}

// TestTextSplitter_InterfaceCompliance verifies interface implementation
func TestTextSplitter_InterfaceCompliance(t *testing.T) {
	splitter := NewDefaultTextSplitter()
	var _ Transformer = splitter
}

// TestTextSplitter_Integration tests complete workflows
func TestTextSplitter_Integration(t *testing.T) {
	ctx := context.Background()

	t.Run("markdown document splitting", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		markdown := `# Title
## Subtitle
Content line 1
Content line 2`

		doc, err := NewDocument(markdown, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(result), 0)
		assert.Contains(t, result[0].Text, "# Title")
	})

	t.Run("code file splitting", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		code := `package main
import "fmt"
func main() {
	fmt.Println("Hello")
}`

		doc, err := NewDocument(code, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(result), 0)
	})

	t.Run("CSV data splitting", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator: "\n",
		}
		splitter := NewTextSplitter(config)

		csv := `id,name,email
1,Alice,alice@example.com
2,Bob,bob@example.com`

		doc, err := NewDocument(csv, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Contains(t, result[0].Text, "id,name,email")
	})

	t.Run("log file splitting", func(t *testing.T) {
		splitter := NewDefaultTextSplitter()

		logs := `2025-01-01 10:00:00 INFO Starting
2025-01-01 10:00:01 DEBUG Loading
2025-01-01 10:00:02 ERROR Failed`

		doc, err := NewDocument(logs, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Contains(t, result[0].Text, "INFO")
		assert.Contains(t, result[1].Text, "DEBUG")
		assert.Contains(t, result[2].Text, "ERROR")
	})

	t.Run("article paragraph splitting", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator: "\n\n",
		}
		splitter := NewTextSplitter(config)

		article := `First paragraph.
More content.

Second paragraph.
More content.

Third paragraph.`

		doc, err := NewDocument(article, nil)
		require.NoError(t, err)
		doc.Metadata["title"] = "Test Article"

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3)

		for _, chunk := range result {
			assert.Equal(t, "Test Article", chunk.Metadata["title"])
		}
	})
}

// TestTextSplitter_RealWorldExample tests the provided real-world example
func TestTextSplitter_RealWorldExample(t *testing.T) {
	ctx := context.Background()

	content := `GPT-4o has safety built-in by design across modalities, through techniques such as filtering training data and refining the model's behavior through post-training. We have also created new safety systems to provide guardrails on voice outputs.

We've evaluated GPT-4o according to our Preparedness Framework and in line with our voluntary commitments. Our evaluations of cybersecurity, CBRN, persuasion, and model autonomy show that GPT-4o does not score above Medium risk in any of these categories. This assessment involved running a suite of automated and human evaluations throughout the model training process. We tested both pre-safety-mitigation and post-safety-mitigation versions of the model, using custom fine-tuning and prompts, to better elicit model capabilities.

GPT-4o has also undergone extensive external red teaming with 70+ external experts in domains such as social psychology, bias and fairness, and misinformation to identify risks that are introduced or amplified by the newly added modalities. We used these learnings to build out our safety interventions in order to improve the safety of interacting with GPT-4o. We will continue to mitigate new risks as they're discovered.`

	t.Run("split by newline", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator:     "\n",
			CopyFormatter: true,
		}
		splitter := NewTextSplitter(config)

		doc, err := NewDocument(content, nil)
		require.NoError(t, err)

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		assert.Greater(t, len(result), 0)

		// Verify content is split correctly
		for _, chunk := range result {
			assert.NotEmpty(t, chunk.Text)
			t.Logf("Chunk: %s", chunk.Text)
		}
	})

	t.Run("split by double newline (paragraphs)", func(t *testing.T) {
		config := &TextSplitterConfig{
			Separator:     "\n\n",
			CopyFormatter: false,
		}
		splitter := NewTextSplitter(config)

		doc, err := NewDocument(content, nil)
		require.NoError(t, err)
		doc.Metadata["source"] = "GPT-4o announcement"

		result, err := splitter.Transform(ctx, []*Document{doc})

		require.NoError(t, err)
		require.Len(t, result, 3) // 3 paragraphs

		// Verify paragraphs
		assert.Contains(t, result[0].Text, "safety built-in")
		assert.Contains(t, result[1].Text, "Preparedness Framework")
		assert.Contains(t, result[2].Text, "external red teaming")

		// Verify metadata
		for _, chunk := range result {
			assert.Equal(t, "GPT-4o announcement", chunk.Metadata["source"])
		}
	})
}

// BenchmarkTextSplitter benchmarks text splitting performance
func BenchmarkTextSplitter_TransformSmall(b *testing.B) {
	ctx := context.Background()
	splitter := NewDefaultTextSplitter()

	text := "line1\nline2\nline3"
	doc, _ := NewDocument(text, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, []*Document{doc})
	}
}

func BenchmarkTextSplitter_TransformMedium(b *testing.B) {
	ctx := context.Background()
	splitter := NewDefaultTextSplitter()

	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "This is line number " + string(rune(i))
	}
	text := strings.Join(lines, "\n")
	doc, _ := NewDocument(text, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, []*Document{doc})
	}
}

func BenchmarkTextSplitter_TransformLarge(b *testing.B) {
	ctx := context.Background()
	splitter := NewDefaultTextSplitter()

	lines := make([]string, 10000)
	for i := range lines {
		lines[i] = "This is a line of text with some content"
	}
	text := strings.Join(lines, "\n")
	doc, _ := NewDocument(text, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, []*Document{doc})
	}
}

func BenchmarkTextSplitter_DifferentSeparators(b *testing.B) {
	ctx := context.Background()

	text := strings.Repeat("word ", 1000)
	doc, _ := NewDocument(text, nil)

	separators := []string{" ", ",", "\n", "\t", "|"}

	for _, sep := range separators {
		b.Run("separator_"+sep, func(b *testing.B) {
			config := &TextSplitterConfig{
				Separator: sep,
			}
			splitter := NewTextSplitter(config)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = splitter.Transform(ctx, []*Document{doc})
			}
		})
	}
}

func BenchmarkTextSplitter_WithFormatterCopy(b *testing.B) {
	ctx := context.Background()

	config := &TextSplitterConfig{
		Separator:     "\n",
		CopyFormatter: true,
	}
	splitter := NewTextSplitter(config)

	text := "line1\nline2\nline3\nline4\nline5"
	doc, _ := NewDocument(text, nil)
	doc.Formatter = &mockFormatter{prefix: "test: "}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, []*Document{doc})
	}
}

func BenchmarkTextSplitter_MultipleDocuments(b *testing.B) {
	ctx := context.Background()
	splitter := NewDefaultTextSplitter()

	docs := make([]*Document, 100)
	for i := range docs {
		docs[i], _ = NewDocument("line1\nline2\nline3", nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = splitter.Transform(ctx, docs)
	}
}
