// Package exa integrates Exa's Search and Contents APIs with the
// provider-neutral web contracts.
//
// A [Client] implements both web.Searcher and web.Fetcher. Search requests use
// POST /search; page fetching uses POST /contents. Exa transport DTOs, search
// tuning, and response normalization remain private to this package.
package exa
