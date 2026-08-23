package sqlite

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
)

const maxTranscriptSearchTerms = 12

func (database *Database) SearchTranscript(
	ctx context.Context,
	query transcript.SearchQuery,
) ([]transcript.SearchHit, error) {
	expression := transcriptSearchExpression(query.Text)
	if expression == "" {
		return []transcript.SearchHit{}, nil
	}
	statement := `
		SELECT i.id,i.session_id,i.run_id,s.title,json_extract(i.body,'$.type'),
			snippet(transcript_search,0,'‹','›',' … ',20),i.created_at
		FROM transcript_search
		JOIN items AS i ON i.rowid=transcript_search.rowid
		JOIN sessions AS s ON s.id=i.session_id
		WHERE transcript_search MATCH ? AND s.workspace_path=?`
	arguments := []any{expression, query.Scope.WorkspacePath}
	if query.Scope.SessionID != "" {
		statement += ` AND i.session_id=?`
		arguments = append(arguments, query.Scope.SessionID)
	}
	statement += ` ORDER BY bm25(transcript_search),i.created_at DESC,i.id LIMIT ?`
	arguments = append(arguments, query.Limit)

	rows, err := database.database.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: search transcript: %w", err)
	}
	defer rows.Close()
	hits := make([]transcript.SearchHit, 0, query.Limit)
	for rows.Next() {
		var hit transcript.SearchHit
		var createdAt string
		if err := rows.Scan(
			&hit.ItemID,
			&hit.SessionID,
			&hit.RunID,
			&hit.SessionTitle,
			&hit.Kind,
			&hit.Snippet,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("sqlite: scan transcript search hit: %w", err)
		}
		hit.Snippet = boundedTranscriptSnippet(hit.Snippet)
		hit.CreatedAt, err = decodeTime(createdAt)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate transcript search: %w", err)
	}
	return hits, nil
}

func transcriptSearchExpression(value string) string {
	terms := strings.FieldsFunc(value, func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsNumber(character)
	})
	seen := make(map[string]bool, len(terms))
	expression := make([]string, 0, min(len(terms), maxTranscriptSearchTerms))
	for _, term := range terms {
		term = strings.ToLower(term)
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		expression = append(expression, `"`+strings.ReplaceAll(term, `"`, `""`)+`"*`)
		if len(expression) == maxTranscriptSearchTerms {
			break
		}
	}
	return strings.Join(expression, " AND ")
}

func boundedTranscriptSnippet(value string) string {
	if len(value) <= transcript.MaxSearchSnippetBytes {
		return value
	}
	end := transcript.MaxSearchSnippetBytes - len("…")
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return strings.TrimSpace(value[:end]) + "…"
}
