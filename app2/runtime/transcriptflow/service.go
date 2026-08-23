// Package transcriptflow projects searchable user-visible Item text and owns
// bounded conversation recall. It never treats Conversation model history or
// reasoning as a substitute for the Transcript.
package transcriptflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Store interface {
	SearchTranscript(context.Context, transcript.SearchQuery) ([]transcript.SearchHit, error)
}

type Service struct {
	store Store
}

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("transcriptflow: store is required")
	}
	return &Service{store: store}, nil
}

func (service *Service) SearchConversations(
	ctx context.Context,
	workspacePath string,
	sessionID string,
	query string,
	limit int,
) ([]transcript.SearchHit, error) {
	request, err := transcript.NewSearchQuery(
		transcript.SearchScope{WorkspacePath: workspacePath, SessionID: sessionID},
		query,
		limit,
	)
	if err != nil {
		return nil, err
	}
	hits, err := service.store.SearchTranscript(ctx, request)
	if err != nil {
		return nil, err
	}
	for _, hit := range hits {
		if err := hit.Validate(); err != nil {
			return nil, fmt.Errorf("transcriptflow: invalid stored hit: %w", err)
		}
	}
	return hits, nil
}

func SearchableItem(item protocol.Item) transcript.SearchableText {
	switch item.Type {
	case protocol.ItemTypeUserMessage, protocol.ItemTypeAgentMessage:
		parts := make([]string, 0, len(item.Content))
		for _, block := range item.Content {
			if block.Type == protocol.ContentBlockText {
				parts = append(parts, block.Text)
			}
		}
		return transcript.NewSearchableText(parts...)
	case protocol.ItemTypeQuestion:
		if item.Question == nil {
			return ""
		}
		parts := make([]string, 0, len(item.Question.Fields))
		for _, field := range item.Question.Fields {
			parts = append(parts, field.Prompt)
		}
		return transcript.NewSearchableText(parts...)
	default:
		return ""
	}
}
