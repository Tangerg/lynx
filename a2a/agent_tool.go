package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	sdka2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/core/model/chat"
)

// agentToolInputSchema is the JSON Schema reported for an [AgentTool]: a
// single free-form natural-language request forwarded to the remote agent.
// A2A's surface is a message, not a typed call, so the tool shape is
// deliberately uniform across every remote agent.
const agentToolInputSchema = `{"type":"object","properties":{"message":{"type":"string","description":"The natural-language request to send to the remote agent."}},"required":["message"]}`

// AgentTool wraps a remote A2A agent as a [chat.Tool]. Each Call sends the
// argument text as an A2A message and returns the agent's reply, so a lynx
// agent can delegate to a remote agent through the ordinary tool-calling
// loop. A non-successful terminal task is mapped to [*RemoteAgentError] (use
// errors.As) so a remote failure is not fed back as a successful result.
//
// The wrapper is immutable after construction and does not own the client.
type AgentTool struct {
	client     *a2aclient.Client
	definition chat.ToolDefinition
	metadata   chat.ToolMetadata
}

// AgentToolConfig configures an [AgentTool]. Client and Card are required.
type AgentToolConfig struct {
	// Client is a live client opened against the remote agent (see [Dial]).
	Client *a2aclient.Client

	// Card is the remote AgentCard; its Name/Description seed the tool
	// definition and its skills enrich the description.
	Card *sdka2a.AgentCard

	// Name overrides the lynx tool name. Empty defaults to a sanitized form
	// of the card's Name.
	Name string

	// Metadata is the chat.ToolMetadata reported by the wrapper. Zero is fine.
	Metadata chat.ToolMetadata
}

// Validate reports whether the config has the required fields. Run
// [AgentToolConfig.ApplyDefaults] first — Name is checked post-default,
// so a card whose name sanitizes to nothing fails loudly here instead
// of registering a tool with an empty name.
func (c *AgentToolConfig) Validate() error {
	if c.Client == nil {
		return ErrNilClient
	}
	if c.Card == nil {
		return ErrNilCard
	}
	if c.Name == "" {
		return ErrEmptyToolName
	}
	return nil
}

// ApplyDefaults fills zero fields: Name defaults to a sanitized form of the
// card's Name. A nil Card is left alone — Validate surfaces it as an error.
func (c *AgentToolConfig) ApplyDefaults() {
	if c.Name == "" && c.Card != nil {
		c.Name = sanitizeToolName(c.Card.Name)
	}
}

// NewAgentTool builds a [chat.Tool] from cfg.
func NewAgentTool(cfg AgentToolConfig) (*AgentTool, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &AgentTool{
		client: cfg.Client,
		definition: chat.ToolDefinition{
			Name:        cfg.Name,
			Description: describeAgent(cfg.Card),
			InputSchema: agentToolInputSchema,
		},
		metadata: cfg.Metadata,
	}, nil
}

func (t *AgentTool) Definition() chat.ToolDefinition { return t.definition }
func (t *AgentTool) Metadata() chat.ToolMetadata     { return t.metadata }

// Call implements [chat.Tool]: it sends the request text to the remote agent
// and returns its reply. One `a2a.agent.call <name>` span per call
// (kind=Client) carrying gen_ai.agent.name; a remote failure records the
// error and sets the span status to Error.
func (t *AgentTool) Call(ctx context.Context, arguments string) (string, error) {
	ctx, span := a2aTracer.Start(ctx, "a2a.agent.call "+t.definition.Name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String(attrAgentName, t.definition.Name)),
	)
	defer span.End()

	req := &sdka2a.SendMessageRequest{Message: userMessage(promptText(arguments))}
	result, err := t.client.SendMessage(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", fmt.Errorf("a2a.AgentTool.Call %q: %w", t.definition.Name, err)
	}

	text, err := resultText(result)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return "", err
	}
	return text, nil
}

// promptText extracts the message to send from the LLM-produced arguments.
// It accepts the conventional {"message": "..."} object but falls back to
// the raw arguments string when the JSON has no message field or isn't an
// object, so a model that passes a bare string still works.
func promptText(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return ""
	}
	var obj struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(trimmed), &obj); err == nil && obj.Message != "" {
		return obj.Message
	}
	return trimmed
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
