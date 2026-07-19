package toolsearch

import "errors"

// ErrEmptyQuery reports a search_tools call with a blank query. Surfaced to the
// model as a tool error so it retries with an actual query rather than aborting
// the run.
var ErrEmptyQuery = errors.New("toolsearch: query must not be empty")
