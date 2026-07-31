package mistral

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

const thinkingFrameHeaderSize = 8

var thinkingFrameMagic = [4]byte{'M', 'S', 'T', 'H'}

func encodeThinkingFrame(raw json.RawMessage) ([]byte, error) {
	if uint64(len(raw)) > uint64(^uint32(0)) {
		return nil, errors.New("mistral: thinking chunk exceeds framing limit")
	}
	frame := make([]byte, thinkingFrameHeaderSize+len(raw))
	copy(frame, thinkingFrameMagic[:])
	binary.BigEndian.PutUint32(frame[4:thinkingFrameHeaderSize], uint32(len(raw)))
	copy(frame[thinkingFrameHeaderSize:], raw)
	return frame, nil
}

func decodeThinkingFrames(signature []byte) ([]json.RawMessage, bool, error) {
	if len(signature) < len(thinkingFrameMagic) || !bytes.Equal(signature[:len(thinkingFrameMagic)], thinkingFrameMagic[:]) {
		return nil, false, nil
	}
	frames := make([]json.RawMessage, 0, 1)
	for offset := 0; offset < len(signature); {
		if len(signature)-offset < thinkingFrameHeaderSize {
			return nil, true, errors.New("truncated thinking frame header")
		}
		if !bytes.Equal(signature[offset:offset+4], thinkingFrameMagic[:]) {
			return nil, true, fmt.Errorf("invalid thinking frame magic at byte %d", offset)
		}
		length := int(binary.BigEndian.Uint32(signature[offset+4 : offset+thinkingFrameHeaderSize]))
		offset += thinkingFrameHeaderSize
		if length > len(signature)-offset {
			return nil, true, fmt.Errorf("thinking frame length %d exceeds remaining %d bytes", length, len(signature)-offset)
		}
		raw := json.RawMessage(bytes.Clone(signature[offset : offset+length]))
		if !json.Valid(raw) {
			return nil, true, errors.New("thinking frame contains invalid JSON")
		}
		frames = append(frames, raw)
		offset += length
	}
	return frames, true, nil
}

func coalesceThinkingFrames(frames []json.RawMessage) (json.RawMessage, error) {
	if len(frames) == 0 {
		return nil, nil
	}
	type wireThinking struct {
		Type     string            `json:"type"`
		Thinking []json.RawMessage `json:"thinking"`
		Closed   *bool             `json:"closed,omitempty"`
	}
	merged := wireThinking{Type: "thinking"}
	for index := range frames {
		var chunk wireThinking
		if err := json.Unmarshal(frames[index], &chunk); err != nil {
			return nil, fmt.Errorf("thinking frame %d: %w", index, err)
		}
		if chunk.Type != "thinking" {
			return nil, fmt.Errorf("thinking frame %d has type %q", index, chunk.Type)
		}
		merged.Thinking = append(merged.Thinking, chunk.Thinking...)
		if chunk.Closed != nil {
			closed := *chunk.Closed
			merged.Closed = &closed
		}
	}
	return json.Marshal(merged)
}
