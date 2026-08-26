package interaction

import (
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/chat"
)

// CompletionSource identifies the semantic value that completed an
// Interaction. It is Strategy-owned and does not add a Framework lifecycle
// status.
type CompletionSource string

const (
	// CompletionSourceModelResponse means the model produced a final response
	// without requesting another tool round.
	CompletionSourceModelResponse CompletionSource = "model_response"

	// CompletionSourceDirectToolResults means every call in one model-requested
	// batch targeted a DirectResultTool and returned successfully.
	CompletionSourceDirectToolResults CompletionSource = "direct_tool_results"
)

// Valid reports whether source is a supported Interaction completion source.
func (source CompletionSource) Valid() bool {
	return source == CompletionSourceModelResponse || source == CompletionSourceDirectToolResults
}

// Output is the final semantic Interaction result. Response is accumulated
// independently of best-effort stream Delta delivery, so it remains complete
// after observer loss or snapshot restoration.
type Output struct {
	// Source identifies which mutually exclusive result field is authoritative.
	Source CompletionSource `json:"source"`

	// ModelResponse is the authoritative accumulated response when Source is
	// CompletionSourceModelResponse.
	ModelResponse *chat.Response `json:"model_response,omitempty"`

	// DirectToolResults preserves model ToolCall order when Source is
	// CompletionSourceDirectToolResults.
	DirectToolResults []chat.ToolResult `json:"direct_tool_results,omitempty"`

	// ModelCalls is the number of model Effects issued by this Interaction.
	ModelCalls uint32 `json:"model_calls"`
}

// Validate verifies that Output contains exactly the semantic result selected
// by Source and a positive model-call count.
func (output Output) Validate() error {
	if !output.Source.Valid() {
		return errors.New("interaction: output source is invalid")
	}
	if output.ModelCalls == 0 {
		return errors.New("interaction: output model_calls must be positive")
	}
	switch output.Source {
	case CompletionSourceModelResponse:
		if output.ModelResponse == nil || len(output.DirectToolResults) != 0 {
			return errors.New("interaction: model_response output requires only ModelResponse")
		}
		if err := output.ModelResponse.Validate(); err != nil {
			return fmt.Errorf("interaction: output model response: %w", err)
		}
		modelOutput := output.ModelResponse.Output
		if modelOutput == nil || modelOutput.Message == nil || modelOutput.FinishReason == "" {
			return errors.New("interaction: output has no finished assistant response")
		}
	case CompletionSourceDirectToolResults:
		if output.ModelResponse != nil || len(output.DirectToolResults) == 0 {
			return errors.New("interaction: direct_tool_results output requires only DirectToolResults")
		}
		seen := make(map[string]struct{}, len(output.DirectToolResults))
		for index := range output.DirectToolResults {
			result := output.DirectToolResults[index]
			if err := result.Validate(); err != nil {
				return fmt.Errorf("interaction: direct tool result %d: %w", index, err)
			}
			if _, duplicate := seen[result.ID]; duplicate {
				return fmt.Errorf("interaction: duplicate direct tool result ID %q", result.ID)
			}
			seen[result.ID] = struct{}{}
		}
	}
	return nil
}
