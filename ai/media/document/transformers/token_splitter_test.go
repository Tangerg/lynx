package transformers_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/media/document/transformers"
	"github.com/Tangerg/lynx/ai/tokenizer"
)

func TestTokenSplitter_Process(t *testing.T) {
	doc, err := document.NewDocument(content, nil)
	if err != nil {
		t.Fatal(err)
	}
	config := &transformers.TokenSplitterConfig{
		Tokenizer:     tokenizer.NewTiktokenWithCL100KBase(),
		ChunkSize:     20,
		CopyFormatter: false,
	}
	splitter, err := transformers.NewTokenSplitter(config)
	if err != nil {
		t.Fatal(err)
	}
	process, err := splitter.Transform(context.Background(), []*document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range process {
		t.Log(d.Text)
	}
}
