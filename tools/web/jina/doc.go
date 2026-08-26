// Package jina integrates Jina Search and Jina Reader with the
// provider-neutral web contracts.
//
// A [Client] implements both web.Searcher and web.Fetcher. The two capabilities
// have distinct upstream base URLs, configured through [Config.SearchBaseURL]
// and [Config.FetchBaseURL], while sharing credentials and HTTP transport.
package jina
