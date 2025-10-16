package chat

import (
	"errors"
	"maps"
	"slices"

	"github.com/samber/lo"

	"github.com/Tangerg/lynx/pkg/ptr"
)

// Options contains configuration parameters for AI model interactions.
// It includes standard parameters like temperature and token limits,
// as well as provider-specific options stored in the Extra field.
type Options struct {
	// Model The AI model identifier to use
	Model string `json:"model"`

	// FrequencyPenalty Penalty for token frequency (-2.0 to 2.0)
	FrequencyPenalty *float64 `json:"frequency_penalty"`

	// MaxTokens Maximum number of tokens to generate
	MaxTokens *int64 `json:"max_tokens"`

	// PresencePenalty Penalty for token presence (-2.0 to 2.0)
	PresencePenalty *float64 `json:"presence_penalty"`

	// Stop Sequences where generation should stop
	Stop []string `json:"stop"`

	// Temperature Sampling temperature (0.0 to 2.0)
	Temperature *float64 `json:"temperature"`

	// TopK Top-K sampling parameter
	TopK *int64 `json:"top_k"`

	// TopP Nucleus sampling parameter
	TopP *float64 `json:"top_p"`

	// Extra Provider-specific options
	Extra map[string]any `json:"extra"`

	// Tools that can be invoked by LLM models.
	Tools []Tool `json:"-"`
}

// NewOptions creates a new Options instance with the specified model.
// The model parameter is required and cannot be empty.
//
// Parameters:
//   - model: The AI model identifier (must be non-empty)
//
// Returns:
//   - *Options: A new Options instance with the specified model
//   - error If model is an empty string
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("model can not be empty")
	}

	return &Options{
		Model: model,
	}, nil
}

func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

func (o *Options) Get(key string) (any, bool) {
	o.ensureExtra()
	value, exists := o.Extra[key]
	return value, exists
}

func (o *Options) Set(key string, value any) {
	o.ensureExtra()
	o.Extra[key] = value
}

func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}

	return &Options{
		Model:            o.Model,
		FrequencyPenalty: ptr.Clone(o.FrequencyPenalty),
		MaxTokens:        ptr.Clone(o.MaxTokens),
		PresencePenalty:  ptr.Clone(o.PresencePenalty),
		Stop:             slices.Clone(o.Stop),
		Temperature:      ptr.Clone(o.Temperature),
		TopK:             ptr.Clone(o.TopK),
		TopP:             ptr.Clone(o.TopP),
		Tools:            slices.Clone(o.Tools),
		Extra:            maps.Clone(o.Extra),
	}
}

// MergeOptions merges multiple Options into a single Options instance.
// It creates a clone of the base options and applies each subsequent option in order.
// Later options override earlier ones for scalar fields, while slices are accumulated.
//
// Parameters:
//   - options: The base Options to clone (must not be nil)
//   - opts: Additional Options to merge (nil entries are skipped)
//
// Returns:
//   - *Options: The merged result
//   - error: An error if the base options is nil
//
// Merge behavior:
//   - Pointer fields (FrequencyPenalty, MaxTokens, etc.): Later non-nil values override earlier ones
//   - Model field: Later non-empty values override earlier ones
//   - Slice fields (Stop, Tools): Later non-empty slices are appended to existing ones
//   - Map field (Extra): Later entries are merged into the result map
func MergeOptions(options *Options, opts ...*Options) (*Options, error) {
	if options == nil {
		return nil, errors.New("options cannot be nil")
	}

	mergedOpts := ptr.Clone(options)
	if len(opts) == 0 {
		return mergedOpts, nil
	}

	mergedOpts.ensureExtra()

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if opt.Model != "" {
			mergedOpts.Model = opt.Model
		}
		if opt.FrequencyPenalty != nil {
			mergedOpts.FrequencyPenalty = opt.FrequencyPenalty
		}
		if opt.MaxTokens != nil {
			mergedOpts.MaxTokens = opt.MaxTokens
		}
		if opt.PresencePenalty != nil {
			mergedOpts.PresencePenalty = opt.PresencePenalty
		}
		if len(opt.Stop) > 0 {
			mergedOpts.Stop = append(mergedOpts.Stop, opt.Stop...)
		}
		if opt.Temperature != nil {
			mergedOpts.Temperature = opt.Temperature
		}
		if opt.TopK != nil {
			mergedOpts.TopK = opt.TopK
		}
		if opt.TopP != nil {
			mergedOpts.TopP = opt.TopP
		}
		if len(opt.Tools) > 0 {
			mergedOpts.Tools = append(mergedOpts.Tools, opt.Tools...)
		}
		if len(opt.Extra) > 0 {
			maps.Copy(mergedOpts.Extra, opt.Extra)
		}
	}
	mergedOpts.Tools = lo.UniqBy(mergedOpts.Tools, func(tool Tool) string {
		return tool.Definition().Name
	})
	return mergedOpts, nil
}

// Request represents a chat request containing conversation messages,
// model-specific options, and contextual parameters.
type Request struct {
	// Messages The conversation message history
	Messages []Message `json:"messages"`

	// Options Model configuration options
	Options *Options `json:"options"`

	// Params Request parameters like userID, sessionID, etc.
	Params map[string]any `json:"params"`
}

// NewRequest creates a new chat request with the provided messages.
// Nil messages are automatically filtered out before validation.
//
// Parameters:
//   - messages: The list of conversation messages
//
// Returns:
//   - *Request: A new Request instance
//   - error: Non-nil if the message list is empty or contains only nil values
func NewRequest(messages []Message) (*Request, error) {
	validMessages := filterOutNilMessages(messages)
	if len(validMessages) == 0 {
		return nil, errors.New("chat request must contain at least one valid message")
	}

	return &Request{
		Messages: validMessages,
		Params:   make(map[string]any),
	}, nil
}

func (r *Request) ensureParams() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

func (r *Request) Get(key string) (any, bool) {
	r.ensureParams()
	value, exists := r.Params[key]
	return value, exists
}

func (r *Request) Set(key string, value any) {
	r.ensureParams()
	r.Params[key] = value
}

func (r *Request) appendTextToLastUserMessage(textToAppend string) {
	appendTextToLastMessageOfType(r.Messages, MessageTypeUser, textToAppend)
}
