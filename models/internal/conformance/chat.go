package conformance

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

// ChatSuite describes one provider's happy-path Model and Streamer contract.
// New and Request are called independently for each subtest so provider state
// and request mutation cannot leak between Call and Stream.
type ChatSuite struct {
	New              func(t *testing.T) (chat.Model, chat.Streamer)
	Request          func(t *testing.T) *chat.Request
	AssertCall       func(t *testing.T, response *chat.Response)
	AssertStream     func(t *testing.T, responses []*chat.Response)
	AssertAggregated func(t *testing.T, response *chat.Response)
}

// Run executes the shared synchronous and streaming conformance cases.
func (s ChatSuite) Run(t *testing.T) {
	t.Helper()
	if s.New == nil {
		t.Fatal("conformance.ChatSuite.New must not be nil")
	}
	if s.Request == nil {
		t.Fatal("conformance.ChatSuite.Request must not be nil")
	}

	t.Run("call", func(t *testing.T) {
		model, _ := s.New(t)
		if model == nil {
			t.Fatal("provider returned nil Model")
		}
		request := s.validRequest(t)
		before := requestWire(t, request)
		response, err := model.Call(t.Context(), request)
		if err != nil {
			t.Fatalf("Call: %v", err)
		}
		assertResponse(t, response)
		if after := requestWire(t, request); !bytes.Equal(before, after) {
			t.Fatalf("Call mutated Request\nbefore: %s\nafter:  %s", before, after)
		}
		if s.AssertCall != nil {
			s.AssertCall(t, response)
		}
	})

	t.Run("stream", func(t *testing.T) {
		_, streamer := s.New(t)
		if streamer == nil {
			t.Fatal("provider returned nil Streamer")
		}
		request := s.validRequest(t)
		before := requestWire(t, request)
		var responses []*chat.Response
		var accumulator chat.ResponseAccumulator
		for response, err := range streamer.Stream(t.Context(), request) {
			if err != nil {
				t.Fatalf("Stream: %v", err)
			}
			assertResponse(t, response)
			if err := accumulator.Add(response); err != nil {
				t.Fatalf("ResponseAccumulator.Add: %v", err)
			}
			responses = append(responses, response)
		}
		if len(responses) == 0 {
			t.Fatal("Stream yielded no responses")
		}
		if after := requestWire(t, request); !bytes.Equal(before, after) {
			t.Fatalf("Stream mutated Request\nbefore: %s\nafter:  %s", before, after)
		}
		if s.AssertStream != nil {
			s.AssertStream(t, responses)
		}
		aggregated := accumulator.Response()
		assertResponse(t, aggregated)
		if s.AssertAggregated != nil {
			s.AssertAggregated(t, aggregated)
		}
	})
}

func (s ChatSuite) validRequest(t *testing.T) *chat.Request {
	t.Helper()
	request := s.Request(t)
	if request == nil {
		t.Fatal("provider returned nil Request fixture")
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Request.Validate: %v", err)
	}
	return request
}

func assertResponse(t *testing.T, response *chat.Response) {
	t.Helper()
	if response == nil {
		t.Fatal("provider yielded nil Response without error")
	}
	if err := response.Validate(); err != nil {
		t.Fatalf("Response.Validate: %v", err)
	}
}

func requestWire(t *testing.T, request *chat.Request) []byte {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal Request: %v", err)
	}
	return body
}
