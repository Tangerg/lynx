package interaction_test

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

func streamOf(deltas ...*chat.Response) chat.Streamer {
	return chat.StreamerFunc(func(_ context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) {
			for _, delta := range deltas {
				if !yield(delta, nil) {
					return
				}
			}
		}
	})
}

func streamError(err error) chat.Streamer {
	return chat.StreamerFunc(func(_ context.Context, _ *chat.Request) iter.Seq2[*chat.Response, error] {
		return func(yield func(*chat.Response, error) bool) { yield(nil, err) }
	})
}

func TestStreamCallAccumulatesAndForwardsDeltas(t *testing.T) {
	deltas := []*chat.Response{
		{Usage: chat.Usage{InputTokens: 8}},
		{Usage: chat.Usage{InputTokens: 8, OutputTokens: 3}},
	}
	var forwarded []*chat.Response
	response, err := interaction.StreamCall(t.Context(), streamOf(deltas...), &chat.Request{}, func(delta *chat.Response) {
		forwarded = append(forwarded, delta)
	})
	if err != nil {
		t.Fatalf("StreamCall: %v", err)
	}
	if response == nil {
		t.Fatal("StreamCall returned a nil accumulated response")
	}
	if response.Usage.OutputTokens != 3 || response.Usage.InputTokens != 8 {
		t.Fatalf("accumulated usage = %+v, want the latest snapshot {8,3}", response.Usage)
	}
	if len(forwarded) != len(deltas) {
		t.Fatalf("onDelta fired %d times, want %d", len(forwarded), len(deltas))
	}
	for i, delta := range forwarded {
		if delta != deltas[i] {
			t.Fatalf("onDelta[%d] forwarded the wrong delta", i)
		}
	}
}

func TestStreamCallNilOnDeltaIsAllowed(t *testing.T) {
	response, err := interaction.StreamCall(t.Context(), streamOf(&chat.Response{Usage: chat.Usage{InputTokens: 1}}), &chat.Request{}, nil)
	if err != nil || response == nil {
		t.Fatalf("StreamCall with nil onDelta = (%v, %v)", response, err)
	}
}

func TestStreamCallNilStreamer(t *testing.T) {
	if _, err := interaction.StreamCall(t.Context(), nil, &chat.Request{}, nil); err == nil {
		t.Fatal("StreamCall accepted a nil streamer")
	}
}

func TestStreamCallPropagatesStreamError(t *testing.T) {
	sentinel := errors.New("provider exploded")
	_, err := interaction.StreamCall(t.Context(), streamError(sentinel), &chat.Request{}, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("StreamCall error = %v, want the stream's error", err)
	}
}

func TestStreamCallRejectsNilDelta(t *testing.T) {
	if _, err := interaction.StreamCall(t.Context(), streamOf(nil), &chat.Request{}, nil); err == nil {
		t.Fatal("StreamCall accepted a nil delta")
	}
}

func TestStreamCallEmptyStream(t *testing.T) {
	if _, err := interaction.StreamCall(t.Context(), streamOf(), &chat.Request{}, nil); err == nil {
		t.Fatal("StreamCall accepted an empty stream")
	}
}
