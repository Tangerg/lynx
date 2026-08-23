package transcript

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxSearchableTextBytes = 64 << 10
	MaxSearchQueryRunes    = 500
	MaxSearchHits          = 20
	MaxSearchSnippetBytes  = 2 << 10
)

var ErrInvalidSearch = errors.New("transcript: invalid search")

// SearchableText is the bounded, user-visible projection indexed from an Item.
// It deliberately excludes reasoning and Tool arguments/results.
type SearchableText string

func NewSearchableText(parts ...string) SearchableText {
	value := strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
	if len(value) <= MaxSearchableTextBytes {
		return SearchableText(value)
	}
	end := MaxSearchableTextBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return SearchableText(value[:end])
}

type SearchScope struct {
	WorkspacePath string
	SessionID     string
}

type SearchQuery struct {
	Scope SearchScope
	Text  string
	Limit int
}

func NewSearchQuery(scope SearchScope, text string, limit int) (SearchQuery, error) {
	workspace := filepath.Clean(strings.TrimSpace(scope.WorkspacePath))
	text = strings.TrimSpace(text)
	if !filepath.IsAbs(workspace) || workspace == string(filepath.Separator) ||
		text == "" || utf8.RuneCountInString(text) > MaxSearchQueryRunes {
		return SearchQuery{}, fmt.Errorf("%w: workspace and bounded query are required", ErrInvalidSearch)
	}
	if limit < 0 {
		return SearchQuery{}, fmt.Errorf("%w: limit must be non-negative", ErrInvalidSearch)
	}
	if limit == 0 {
		limit = 8
	}
	if limit > MaxSearchHits {
		return SearchQuery{}, fmt.Errorf("%w: limit exceeds %d", ErrInvalidSearch, MaxSearchHits)
	}
	return SearchQuery{
		Scope: SearchScope{WorkspacePath: workspace, SessionID: strings.TrimSpace(scope.SessionID)},
		Text: text,
		Limit: limit,
	}, nil
}

type SearchHit struct {
	ItemID       string
	SessionID    string
	RunID        string
	SessionTitle string
	Kind         string
	Snippet      string
	CreatedAt    time.Time
}

func (hit SearchHit) Validate() error {
	if hit.ItemID == "" || hit.SessionID == "" || hit.RunID == "" ||
		hit.SessionTitle == "" || hit.Kind == "" || hit.CreatedAt.IsZero() ||
		!utf8.ValidString(hit.Snippet) || len(hit.Snippet) > MaxSearchSnippetBytes {
		return fmt.Errorf("%w: malformed search hit", ErrInvalidSearch)
	}
	return nil
}
