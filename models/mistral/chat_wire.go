package mistral

import "encoding/json"

type chatCompletionRequest struct {
	Model            string        `json:"model"`
	Messages         []chatMessage `json:"messages"`
	Temperature      *float64      `json:"temperature,omitempty"`
	TopP             *float64      `json:"top_p,omitempty"`
	MaxTokens        *int64        `json:"max_tokens,omitempty"`
	Stream           bool          `json:"stream"`
	Stop             []string      `json:"stop,omitempty"`
	PresencePenalty  *float64      `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64      `json:"frequency_penalty,omitempty"`
	Tools            []chatTool    `json:"tools,omitempty"`
	ChatRequestOptions
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type textChunk struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type thinkChunk struct {
	Type     string      `json:"type"`
	Thinking []textChunk `json:"thinking"`
	Closed   bool        `json:"closed"`
}

type imageURLChunk struct {
	Type     string        `json:"type"`
	ImageURL imageURLValue `json:"image_url"`
}

type imageURLValue string

func (value imageURLValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(value))
}

func (value *imageURLValue) UnmarshalJSON(data []byte) error {
	var direct string
	if err := json.Unmarshal(data, &direct); err == nil {
		*value = imageURLValue(direct)
		return nil
	}
	var object struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	*value = imageURLValue(object.URL)
	return nil
}

type documentURLChunk struct {
	Type         string `json:"type"`
	DocumentURL  string `json:"document_url"`
	DocumentName string `json:"document_name,omitempty"`
}

type audioChunk struct {
	Type       string `json:"type"`
	InputAudio string `json:"input_audio"`
}

type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}

type chatFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type chatToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function chatFunctionCall `json:"function"`
	Index    int              `json:"index,omitempty"`
}

type chatFunctionCall struct {
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   chatUsage              `json:"usage"`
}

type chatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      chatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type chatCompletionMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"`
	ToolCalls []chatToolCall  `json:"tool_calls"`
}

type chatCompletionChunk struct {
	ID      string                       `json:"id"`
	Model   string                       `json:"model"`
	Choices []chatCompletionStreamChoice `json:"choices"`
	Usage   chatUsage                    `json:"usage"`
}

type chatCompletionStreamChoice struct {
	Index        int                   `json:"index"`
	Delta        chatCompletionMessage `json:"delta"`
	FinishReason string                `json:"finish_reason"`
}

type chatUsage struct {
	PromptTokens        int64 `json:"prompt_tokens"`
	CompletionTokens    int64 `json:"completion_tokens"`
	TotalTokens         int64 `json:"total_tokens"`
	NumCachedTokens     int64 `json:"num_cached_tokens"`
	PromptTokensDetails *struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}
