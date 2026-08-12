package agent

import "testing"

func TestStreamedTextOrdersSparseContentBlocks(t *testing.T) {
	stream := NewStreamedText("")
	second := 1
	mutation, err := stream.Apply(BlockDelta{BlockID: "answer", Text: "second", ContentIndex: &second})
	if err != nil {
		t.Fatal(err)
	}
	if mutation.Replace || mutation.Text != "second" || stream.String() != "second" {
		t.Fatalf("first mutation = %+v, text = %q", mutation, stream.String())
	}

	first := 0
	mutation, err = stream.Apply(BlockDelta{BlockID: "answer", Text: "first", ContentIndex: &first})
	if err != nil {
		t.Fatal(err)
	}
	if !mutation.Replace || mutation.Text != "first\n\nsecond" || stream.String() != mutation.Text {
		t.Fatalf("out-of-order mutation = %+v, text = %q", mutation, stream.String())
	}

	mutation, err = stream.Apply(BlockDelta{BlockID: "answer", Text: " tail", ContentIndex: &first})
	if err != nil {
		t.Fatal(err)
	}
	if !mutation.Replace || mutation.Text != "first tail\n\nsecond" {
		t.Fatalf("earlier-block append = %+v", mutation)
	}

	third := 2
	mutation, err = stream.Apply(BlockDelta{BlockID: "answer", Text: "third", ContentIndex: &third})
	if err != nil {
		t.Fatal(err)
	}
	if mutation.Replace || mutation.Text != "\n\nthird" || stream.String() != "first tail\n\nsecond\n\nthird" {
		t.Fatalf("tail mutation = %+v, text = %q", mutation, stream.String())
	}
}

func TestStreamedTextIgnoresMissingIndicesAndValidatesDeltas(t *testing.T) {
	stream := NewStreamedText("first")
	third := 2
	if _, err := stream.Apply(BlockDelta{BlockID: "answer", Text: "third", ContentIndex: &third}); err != nil {
		t.Fatal(err)
	}
	if got := stream.String(); got != "first\n\nthird" {
		t.Fatalf("sparse text = %q", got)
	}

	negative := -1
	for _, delta := range []BlockDelta{
		{BlockID: "answer"},
		{BlockID: "answer", Text: "invalid", ContentIndex: &negative},
	} {
		if _, err := stream.Apply(delta); err == nil {
			t.Fatalf("Apply accepted %+v", delta)
		}
	}
}
