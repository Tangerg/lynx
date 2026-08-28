package runmaintenance

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

const (
	// asciiBytesPerEstimatedToken is the text-only approximation shared by
	// model-context maintenance. Non-ASCII runes are counted individually so
	// CJK and emoji cannot inherit an English-only byte ratio.
	asciiBytesPerEstimatedToken = 4

	// The media placeholders are the smallest valid values for their Source
	// variants. A provider prices image/audio/document content by modality, not
	// by the size of its transport encoding, so raw payload bytes must not be
	// mistaken for text tokens in the provider-neutral estimate.
	mediaURIPlaceholder       = "data:,x"
	mediaReferencePlaceholder = "x"
)

// modelContextBudget owns both compaction triggers and the exact immutable
// request material that survives every candidate rewrite. Keeping the fixed
// prefix, mutable conversation, pending tail, Tool manifest, and Options in one
// value prevents independently rounded "fixed" and "history" token ledgers.
type modelContextBudget struct {
	maxMessages  int
	maxTokens    int
	instructions []chat.Message
	tail         []chat.Message
	tools        []chat.ToolDefinition
	options      chat.Options
	adjustment   int
}

func newModelContextBudget(
	maxMessages int,
	maxTokens int,
	instructions []chat.Message,
	tail []chat.Message,
	tools []chat.ToolDefinition,
	options chat.Options,
	adjustment int,
) modelContextBudget {
	return modelContextBudget{
		maxMessages:  maxMessages,
		maxTokens:    maxTokens,
		instructions: cloneMessages(instructions),
		tail:         cloneMessages(tail),
		tools:        cloneToolDefinitions(tools),
		options:      options.Clone(),
		adjustment:   adjustment,
	}
}

func (m modelContextBudget) triggered(messages []chat.Message) (bool, error) {
	if saturatedAdd(len(messages), len(m.tail)) >= m.maxMessages {
		return true, nil
	}
	tokens, err := m.estimatedTokens(messages)
	if err != nil {
		return false, err
	}
	return calibratedTokenEstimate(tokens, m.adjustment) >= m.maxTokens, nil
}

func (m modelContextBudget) exceeded(messages []chat.Message) (bool, error) {
	if saturatedAdd(len(messages), len(m.tail)) >= m.maxMessages {
		return true, nil
	}
	tokens, err := m.estimatedTokens(messages)
	if err != nil {
		return false, err
	}
	return tokens >= m.maxTokens, nil
}

func (m modelContextBudget) estimatedTokens(messages []chat.Message) (int, error) {
	effective := make([]chat.Message, 0, len(m.instructions)+len(messages)+len(m.tail))
	effective = append(effective, m.instructions...)
	effective = append(effective, messages...)
	effective = append(effective, m.tail...)
	if len(effective) == 0 {
		return 0, nil
	}
	tokens, err := estimateModelContextTokens(effective, m.tools, m.options)
	if err != nil {
		return 0, fmt.Errorf("runmaintenance: estimate model context: %w", err)
	}
	return tokens, nil
}

func calibratedTokenEstimate(estimate, adjustment int) int {
	if adjustment > 0 && estimate > math.MaxInt-adjustment {
		return math.MaxInt
	}
	if adjustment < 0 && estimate < -adjustment {
		return 0
	}
	return estimate + adjustment
}

// estimateModelContextTokens measures the complete provider-neutral request:
// messages of every Part kind, Tool definitions, Options, metadata, provider
// replay signatures, Tool identities, arguments, and results. It intentionally
// does not use renderTranscript: that lossy auxiliary-model view is a different
// artifact and omits model-visible protocol state.
func estimateModelContextTokens(
	messages []chat.Message,
	tools []chat.ToolDefinition,
	options chat.Options,
) (int, error) {
	request := chat.Request{
		Messages: mediaNormalizedMessages(messages),
		Tools:    cloneToolDefinitions(tools),
		Options:  options.Clone(),
	}
	payload, err := json.Marshal(&request)
	if err != nil {
		return 0, err
	}
	return estimateTextTokens(string(payload)), nil
}

func mediaNormalizedMessages(messages []chat.Message) []chat.Message {
	normalized := make([]chat.Message, len(messages))
	for messageIndex := range messages {
		message := messages[messageIndex]
		normalized[messageIndex] = chat.Message{
			Role:     message.Role,
			Parts:    make([]chat.Part, len(message.Parts)),
			Metadata: message.Metadata.Clone(),
		}
		for partIndex := range message.Parts {
			part := message.Parts[partIndex]
			value := part.Media
			part.Media = nil
			normalized[messageIndex].Parts[partIndex] = part.Clone()
			if value == nil {
				continue
			}
			mediaValue := *value
			mediaValue.Source.Bytes = nil
			mediaValue.Metadata = value.Metadata.Clone()
			switch mediaValue.Source.Kind {
			case media.SourceBytes:
				mediaValue.Source.Bytes = []byte{0}
			case media.SourceURI:
				mediaValue.Source.URI = mediaURIPlaceholder
			case media.SourceReference:
				mediaValue.Source.Ref = mediaReferencePlaceholder
			}
			normalized[messageIndex].Parts[partIndex].Media = &mediaValue
		}
	}
	return normalized
}

func cloneToolDefinitions(tools []chat.ToolDefinition) []chat.ToolDefinition {
	cloned := make([]chat.ToolDefinition, len(tools))
	for index := range tools {
		cloned[index] = tools[index].Clone()
	}
	return cloned
}

func estimateTextTokens(text string) int {
	ascii := 0
	tokens := 0
	for _, value := range text {
		if value <= 0x7f {
			ascii++
		} else {
			tokens++
		}
	}
	asciiTokens := ascii / asciiBytesPerEstimatedToken
	if ascii%asciiBytesPerEstimatedToken != 0 {
		asciiTokens++
	}
	return saturatedAdd(tokens, asciiTokens)
}
