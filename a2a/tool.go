package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	corechat "github.com/Tangerg/lynx/core/chat"
	pkgjson "github.com/Tangerg/lynx/pkg/json"
	toolcontract "github.com/Tangerg/lynx/tools"
)

var (
	errNilClient     = errors.New("a2a: client must not be nil")
	errEmptyToolName = errors.New("a2a: tool name must not be empty")
)

type callInput struct {
	Message string `json:"message" jsonschema:"required" jsonschema_description:"The natural-language request to send to the remote agent."`
}

var inputSchema, _ = pkgjson.StringDefSchemaOf(callInput{})

// tool wraps a remote A2A agent as a [tools.Tool]. Each Call sends the
// argument text as an A2A message and returns the agent's reply, so an
// agent can delegate to a remote agent through the ordinary tool-calling
// loop. A non-successful terminal task is mapped to [*RemoteAgentError] (use
// [errors.AsType]) so a remote failure is not fed back as a successful result.
//
// The wrapper is immutable after construction and does not own the client.
type tool struct {
	client     *a2aclient.Client
	definition corechat.ToolDefinition
}

var _ toolcontract.Tool = (*tool)(nil)

type toolConfig struct {
	Client *a2aclient.Client
	Card   *sdka2a.AgentCard
	Name   string
}

func newTool(cfg toolConfig) (*tool, error) {
	if cfg.Client == nil {
		return nil, errNilClient
	}
	if cfg.Card == nil {
		return nil, ErrNilCard
	}
	if cfg.Name == "" {
		cfg.Name = sanitizeToolName(cfg.Card.Name)
	}
	if cfg.Name == "" {
		return nil, errEmptyToolName
	}
	return &tool{
		client: cfg.Client,
		definition: corechat.ToolDefinition{
			Name:        cfg.Name,
			Description: describeAgent(cfg.Card),
			InputSchema: json.RawMessage(inputSchema),
		},
	}, nil
}

func (t *tool) Definition() corechat.ToolDefinition { return t.definition.Clone() }

// Call implements [tools.Tool]: it sends the request text to the remote agent
// and returns its reply. One `a2a.agent.call <name>` span per call
// (kind=Client) carrying gen_ai.agent.name; a remote failure records the
// error and sets the span status to Error.
func (t *tool) Call(ctx context.Context, arguments string) (string, error) {
	ctx, span := a2aTracer.Start(ctx, "a2a.agent.call "+t.definition.Name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String(attrAgentName, t.definition.Name)),
	)
	defer span.End()

	input, err := decodeCallInput(arguments)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("a2a.tool.Call %q: %w", t.definition.Name, err)
	}

	req := &sdka2a.SendMessageRequest{Message: userMessage(input.Message)}
	result, err := t.client.SendMessage(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("a2a.tool.Call %q: %w", t.definition.Name, err)
	}

	text, err := textOfResult(result)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	return text, nil
}

func decodeCallInput(arguments string) (callInput, error) {
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()

	var input callInput
	if err := decoder.Decode(&input); err != nil {
		return callInput{}, fmt.Errorf("decode arguments: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return callInput{}, errors.New("arguments must contain exactly one JSON object")
		}
		return callInput{}, fmt.Errorf("decode trailing arguments: %w", err)
	}
	if strings.TrimSpace(input.Message) == "" {
		return callInput{}, errors.New("message must not be empty")
	}
	return input, nil
}

// describeAgent builds the tool description from the card: its description
// plus a compact list of skill names so the model knows what the remote can
// do.
func describeAgent(card *sdka2a.AgentCard) string {
	var b strings.Builder
	b.WriteString(card.Description)
	if len(card.Skills) > 0 {
		names := make([]string, 0, len(card.Skills))
		for _, skill := range card.Skills {
			names = append(names, skill.Name)
		}
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString("Skills: ")
		b.WriteString(strings.Join(names, ", "))
		b.WriteString(".")
	}
	return b.String()
}

// sanitizeToolName maps an AgentCard name (which may contain spaces or
// punctuation) to a tool identifier: lowercased, with runs of non-alphanumeric
// characters collapsed to single underscores.
func sanitizeToolName(name string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if !prevUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}
