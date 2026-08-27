// Package pagination owns the shared keyset-paging contract used by application
// reads. A token names the query it was minted for and the sort position it
// stopped at, so continuing a page is a bounded seek rather than a scan.
//
// The anchor is the previous page's last sort key, never an offset or an element
// id. An offset shifts when rows are inserted before it, and an id anchor has to
// be located — which means materializing the collection to search it, and
// failing outright once the anchored row is deleted. A sort-key anchor turns
// continuation into `WHERE key > anchor ORDER BY key LIMIT n`, which stays exact
// under concurrent writes and never loads a page it will not return.
//
// A token also carries the query namespace it belongs to. Without that, a cursor
// from one query or filter set silently reinterprets against another and pages skip
// or repeat rows; with it, a mismatched cursor is rejected and the caller starts
// over. That check is the integrity guarantee here — no secret is involved,
// because the runtime has no user boundary to defend across.
package pagination

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/scope/app/runtime/internal/application/opaquetoken"
)

// ErrInvalidCursor reports a cursor that cannot continue this query: damaged,
// minted by an older format, or issued for a different namespace or filter set.
// All of those have one remedy — restart from the first page — so they are one
// sentinel rather than a taxonomy the caller would branch on identically.
var ErrInvalidCursor = errors.New("pagination: cursor cannot continue this query")

// ErrInvalidLimit reports a page size a read will not serve. Separate from
// ErrInvalidCursor because the caller's fix differs: correct the request, rather
// than start the collection over.
var ErrInvalidLimit = errors.New("pagination: page limit is invalid")

// formatVersion changes when the token layout does, so a cursor in flight across
// an upgrade is rejected instead of decoded as something else.
const formatVersion = 2

// Page is one keyset page: the rows, and the token that continues after them.
// An empty NextCursor means the page reached the end of the collection — the
// caller returns it as-is and never truncates a page silently.
type Page[T any] struct {
	Rows       []T
	NextCursor string
}

// token is the decoded cursor. Namespace and Filters identify the query; Key is
// the sort position the previous page ended at.
type token struct {
	Version   int      `json:"v"`
	Namespace string   `json:"n"`
	Filters   []string `json:"f,omitempty"`
	Key       []string `json:"k"`
}

// Encode mints the cursor that continues namespace past key. filters are the
// query's normalized inputs — every value that changes which rows match or the
// order they arrive in, including the sort direction, since a cursor from an
// ascending page cannot continue a descending one.
func Encode(namespace string, filters []string, key []string) string {
	if namespace == "" {
		panic("pagination: encode cursor: namespace is required")
	}
	encoded, err := opaquetoken.Encode(token{
		Version: formatVersion, Namespace: namespace,
		Filters: slices.Clone(filters), Key: key,
	})
	if err != nil {
		// token holds only strings, ints and a string slice, so marshaling it
		// cannot fail; a nil cursor would read as "no more pages" and truncate.
		panic("pagination: encode cursor: " + err.Error())
	}
	return encoded
}

// Decode returns the sort position cursor stopped at, for the same namespace and
// filters that minted it. An empty cursor is the first page and yields a nil key
// with no error.
func Decode(cursor, namespace string, filters []string) ([]string, error) {
	if namespace == "" {
		return nil, ErrInvalidCursor
	}
	if cursor == "" {
		return nil, nil
	}
	var decoded token
	if err := opaquetoken.Decode(cursor, &decoded); err != nil {
		return nil, ErrInvalidCursor
	}
	if decoded.Version != formatVersion || decoded.Namespace != namespace ||
		!slices.Equal(decoded.Filters, filters) || len(decoded.Key) == 0 {
		return nil, ErrInvalidCursor
	}
	return decoded.Key, nil
}

// Limit validates and clamps a requested page size. Zero asks for the default,
// which is also the ceiling; a negative value is invalid.
func Limit(requested, ceiling int) (int, error) {
	if requested < 0 {
		return 0, fmt.Errorf("%w: must not be negative", ErrInvalidLimit)
	}
	if requested == 0 || requested > ceiling {
		return ceiling, nil
	}
	return requested, nil
}

// PageOf splits an over-fetched row set into the page and its continuation.
// Reads ask their store for limit+1 rows: getting the extra one is how "there is
// more" is known without a second count query. nextKey derives the anchor from
// the last row the page actually returns.
func PageOf[T any](rows []T, limit int, namespace string, filters []string, nextKey func(T) []string) Page[T] {
	if namespace == "" {
		panic("pagination: page namespace is required")
	}
	if len(rows) <= limit {
		return Page[T]{Rows: rows}
	}
	page := rows[:limit]
	return Page[T]{
		Rows:       page,
		NextCursor: Encode(namespace, filters, nextKey(page[len(page)-1])),
	}
}
