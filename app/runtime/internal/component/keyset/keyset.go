// Package keyset encodes the page anchors a cursor-paginated read hands back to
// its caller. A token names the query it was minted for and the sort position it
// stopped at, so continuing a page is a bounded seek rather than a scan.
//
// The anchor is the previous page's last sort key, never an offset or an element
// id. An offset shifts when rows are inserted before it, and an id anchor has to
// be located — which means materializing the collection to search it, and
// failing outright once the anchored row is deleted. A sort-key anchor turns
// continuation into `WHERE key > anchor ORDER BY key LIMIT n`, which stays exact
// under concurrent writes and never loads a page it will not return.
//
// A token also carries the query it belongs to. Without that, a cursor from one
// method or one filter set silently reinterprets against another and pages skip
// or repeat rows; with it, a mismatched cursor is rejected and the caller starts
// over. That check is the integrity guarantee here — no secret is involved,
// because the runtime has no user boundary to defend across.
package keyset

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
)

// ErrInvalidCursor reports a cursor that cannot continue this query: damaged,
// minted by an older format, or issued for a different method or filter set.
// All of those have one remedy — restart from the first page — so they are one
// sentinel rather than a taxonomy the caller would branch on identically.
var ErrInvalidCursor = errors.New("keyset: cursor cannot continue this query")

// ErrInvalidLimit reports a page size a read will not serve. Separate from
// ErrInvalidCursor because the caller's fix differs: correct the request, rather
// than start the collection over.
var ErrInvalidLimit = errors.New("keyset: page limit is invalid")

// formatVersion changes when the token layout does, so a cursor in flight across
// an upgrade is rejected instead of decoded as something else.
const formatVersion = 1

// unitSeparator joins filter values into one fingerprint input. It cannot appear
// in an id, a path, or a sort mode, so no two distinct filter sets collide by
// concatenation.
const unitSeparator = "\x1f"

// Page is one keyset page: the rows, and the token that continues after them.
// An empty NextCursor means the page reached the end of the collection — the
// caller returns it as-is and never truncates a page silently.
type Page[T any] struct {
	Rows       []T
	NextCursor string
}

// token is the decoded cursor. Method and Filters identify the query; Key is the
// sort position the previous page ended at.
type token struct {
	Version int      `json:"v"`
	Method  string   `json:"m"`
	Filters uint64   `json:"f"`
	Key     []string `json:"k"`
}

// Encode mints the cursor that continues method past key. filters are the
// query's normalized inputs — every value that changes which rows match or the
// order they arrive in, including the sort direction, since a cursor from an
// ascending page cannot continue a descending one.
func Encode(method string, filters []string, key []string) string {
	payload, err := json.Marshal(token{
		Version: formatVersion, Method: method,
		Filters: fingerprint(filters), Key: key,
	})
	if err != nil {
		// token holds only strings, ints and a string slice, so marshaling it
		// cannot fail; a nil cursor would read as "no more pages" and truncate.
		panic("keyset: encode cursor: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

// Decode returns the sort position cursor stopped at, for the same method and
// filters that minted it. An empty cursor is the first page and yields a nil key
// with no error.
func Decode(cursor, method string, filters []string) ([]string, error) {
	if cursor == "" {
		return nil, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var decoded token
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil, ErrInvalidCursor
	}
	if decoded.Version != formatVersion || decoded.Method != method ||
		decoded.Filters != fingerprint(filters) || len(decoded.Key) == 0 {
		return nil, ErrInvalidCursor
	}
	return decoded.Key, nil
}

// Limit clamps a requested page size. Zero or negative asks for the default,
// which is also the ceiling: a caller cannot widen a page beyond what the read
// is willing to serve, and gets a cursor instead.
func Limit(requested, max int) (int, error) {
	if requested < 0 {
		return 0, fmt.Errorf("%w: must not be negative", ErrInvalidLimit)
	}
	if requested == 0 || requested > max {
		return max, nil
	}
	return requested, nil
}

// PageOf splits an over-fetched row set into the page and its continuation.
// Reads ask their store for limit+1 rows: getting the extra one is how "there is
// more" is known without a second count query. nextKey derives the anchor from
// the last row the page actually returns.
func PageOf[T any](rows []T, limit int, method string, filters []string, nextKey func(T) []string) Page[T] {
	if len(rows) <= limit {
		return Page[T]{Rows: rows}
	}
	page := rows[:limit]
	return Page[T]{
		Rows:       page,
		NextCursor: Encode(method, filters, nextKey(page[len(page)-1])),
	}
}

func fingerprint(filters []string) uint64 {
	digest := fnv.New64a()
	// Join rather than hashing each value in turn: separate writes make
	// {"a","bc"} and {"ab","c"} the same input, and those are different queries.
	_, _ = digest.Write([]byte(strings.Join(filters, unitSeparator)))
	return digest.Sum64()
}
