// Conversation search exposes search_conversations over prior transcripts.
package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	toolcontract "github.com/Tangerg/lynx/core/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

const conversationSearchDefaultLimit = 8

type conversationSearchRequest struct {
	Query string `json:"query" jsonschema:"minLength=1" jsonschema_description:"Keywords that should appear in earlier conversation transcripts."`
	Limit int    `json:"limit,omitempty" jsonschema:"minimum=1,maximum=20" jsonschema_description:"Maximum matching excerpts to return. Defaults to 8."`
}

func (c conversationSearchRequest) normalize() (conversationSearchRequest, error) {
	c.Query = strings.TrimSpace(c.Query)
	if c.Query == "" {
		return conversationSearchRequest{}, errors.New("query is required")
	}
	if c.Limit <= 0 {
		c.Limit = conversationSearchDefaultLimit
	}
	return c, nil
}

// ConversationSearch is the transcript full-text search capability this tool consumes.
type ConversationSearch interface {
	SearchTranscript(ctx context.Context, query string, limit int) ([]transcript.SearchHit, error)
}

type conversationSearcher struct {
	search ConversationSearch
}

// NewConversationSearch builds search_conversations over the given searcher. A nil searcher
// yields a nil tool (the feature is simply omitted), mirroring the other
// optional tools.
func NewConversationSearch(search ConversationSearch) (toolcontract.Tool, error) {
	if search == nil {
		return nil, nil
	}
	return toolcontract.NewFunc[conversationSearchRequest, string](conversationSearchDefinition(), (&conversationSearcher{search: search}).run)
}

func conversationSearchDefinition() toolcontract.FuncConfig {
	return toolcontract.FuncConfig{
		Name: tool.SearchConversations,
		Description: "Search raw transcripts from earlier conversations by keyword and return matching excerpts " +
			"with speaker and date. Use it to determine whether a topic, decision, error, or approach was discussed " +
			"before. Use search_memory for curated durable facts and grep for source code.",
	}
}

func (c *conversationSearcher) run(ctx context.Context, req conversationSearchRequest) (string, error) {
	req, err := req.normalize()
	if err != nil {
		return "", fmt.Errorf("search_conversations: %w", err)
	}
	hits, err := c.search.SearchTranscript(ctx, req.Query, req.Limit)
	if err != nil {
		return "", err
	}
	return conversationSearchResults(hits).String(), nil
}

type conversationSearchResults []transcript.SearchHit

func (c conversationSearchResults) String() string {
	if len(c) == 0 {
		return "No earlier conversation matched. This topic may not have come up before."
	}
	var b strings.Builder
	for i, hit := range c {
		fmt.Fprintf(&b, "%d. [%s · %s] %s\n", i+1, transcriptSpeaker(hit.Kind), hit.CreatedAt.Format("2006-01-02"), strings.TrimSpace(hit.Snippet))
	}
	return strings.TrimRight(b.String(), "\n")
}

func transcriptSpeaker(kind transcript.ItemKind) string {
	if kind == transcript.UserMessage {
		return "user"
	}
	return "agent"
}
