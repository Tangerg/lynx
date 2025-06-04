package response

import (
	"time"
)

// RateLimit encapsulates metadata from an AI provider's API rate limits
// granted to the API key in use and the API key's current balance.
type RateLimit struct {
	requestsLimit     int64
	requestsRemaining int64
	requestsReset     time.Duration
	tokensLimit       int64
	tokensRemaining   int64
	tokensReset       time.Duration
}

// RequestsLimit returns the maximum number of requests that are permitted
// before exhausting the rate limit.
func (r *RateLimit) RequestsLimit() int64 {
	return r.requestsLimit
}

// RequestsRemaining returns the remaining number of requests that are permitted
// before exhausting the rate limit.
func (r *RateLimit) RequestsRemaining() int64 {
	return r.requestsRemaining
}

// RequestsReset returns the time until the rate limit (based on requests) resets
// to its initial state.
func (r *RateLimit) RequestsReset() time.Duration {
	return r.requestsReset
}

// TokensLimit returns the maximum number of tokens that are permitted
// before exhausting the rate limit.
func (r *RateLimit) TokensLimit() int64 {
	return r.tokensLimit
}

// TokensRemaining returns the remaining number of tokens that are permitted
// before exhausting the rate limit.
func (r *RateLimit) TokensRemaining() int64 {
	return r.tokensRemaining
}

// TokensReset returns the time until the rate limit (based on tokens) resets
// to its initial state.
func (r *RateLimit) TokensReset() time.Duration {
	return r.tokensReset
}

// RateLimitBuilder provides a builder pattern for creating RateLimit instances.
type RateLimitBuilder struct {
	requestsLimit     int64
	requestsRemaining int64
	requestsReset     time.Duration
	tokensLimit       int64
	tokensRemaining   int64
	tokensReset       time.Duration
}

// NewRateLimitBuilder creates a new RateLimitBuilder instance.
func NewRateLimitBuilder() *RateLimitBuilder {
	return &RateLimitBuilder{}
}

// WithRequestsLimit sets the maximum number of requests permitted.
func (r *RateLimitBuilder) WithRequestsLimit(requestsLimit int64) *RateLimitBuilder {
	r.requestsLimit = requestsLimit
	return r
}

// WithRequestsRemaining sets the remaining number of requests permitted.
func (r *RateLimitBuilder) WithRequestsRemaining(requestsRemaining int64) *RateLimitBuilder {
	r.requestsRemaining = requestsRemaining
	return r
}

// WithRequestsReset sets the time until the requests rate limit resets.
func (r *RateLimitBuilder) WithRequestsReset(requestsReset time.Duration) *RateLimitBuilder {
	r.requestsReset = requestsReset
	return r
}

// WithTokensLimit sets the maximum number of tokens permitted.
func (r *RateLimitBuilder) WithTokensLimit(tokensLimit int64) *RateLimitBuilder {
	r.tokensLimit = tokensLimit
	return r
}

// WithTokensRemaining sets the remaining number of tokens permitted.
func (r *RateLimitBuilder) WithTokensRemaining(tokensRemaining int64) *RateLimitBuilder {
	r.tokensRemaining = tokensRemaining
	return r
}

// WithTokensReset sets the time until the tokens rate limit resets.
func (r *RateLimitBuilder) WithTokensReset(tokensReset time.Duration) *RateLimitBuilder {
	r.tokensReset = tokensReset
	return r
}

// Build creates a new RateLimit instance with the configured values.
func (r *RateLimitBuilder) Build() *RateLimit {
	return &RateLimit{
		requestsLimit:     r.requestsLimit,
		requestsRemaining: r.requestsRemaining,
		requestsReset:     r.requestsReset,
		tokensLimit:       r.tokensLimit,
		tokensRemaining:   r.tokensRemaining,
		tokensReset:       r.tokensReset,
	}
}
