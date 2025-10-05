package processors_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/ai/media/document"
	"github.com/Tangerg/lynx/ai/media/document/processors"
	"github.com/Tangerg/lynx/ai/tokenizer"
)

func TestTokenSplitter_Process(t *testing.T) {
	doc, err := document.NewDocument(content, nil)
	if err != nil {
		t.Fatal(err)
	}

	splitter := &processors.TokenSplitter{
		Tokenizer: tokenizer.NewTiktokenWithCL100KBase(),
	}
	process, err := splitter.Process(context.Background(), []*document.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range process {
		t.Log(d.Text)
	}
}
