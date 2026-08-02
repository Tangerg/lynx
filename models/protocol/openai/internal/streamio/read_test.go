package streamio_test

import (
	"bytes"
	"testing"

	"github.com/Tangerg/lynx/models/protocol/openai/internal/streamio"
)

func TestReadYieldsOwnedChunks(t *testing.T) {
	input := bytes.Repeat([]byte("a"), 16*1024+3)
	var chunks [][]byte
	for chunk, err := range streamio.Read(bytes.NewReader(input)) {
		if err != nil {
			t.Fatal(err)
		}
		chunks = append(chunks, chunk)
	}

	if len(chunks) != 2 {
		t.Fatalf("chunk count = %d, want 2", len(chunks))
	}
	if len(chunks[0]) != 16*1024 || len(chunks[1]) != 3 {
		t.Fatalf("chunk lengths = [%d %d], want [16384 3]", len(chunks[0]), len(chunks[1]))
	}
	if !bytes.Equal(bytes.Join(chunks, nil), input) {
		t.Fatal("joined chunks differ from input")
	}
}
