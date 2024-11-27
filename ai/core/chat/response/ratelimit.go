package response

import "time"

type RateLimit interface {
	RequestsLimit() int64
	RequestsRemaining() int64
	RequestsReset() time.Duration
	TokensLimit() int64
	TokensRemaining() int64
	TokensReset() time.Duration
}

var _ RateLimit = (*EmptyRateLimit)(nil)

type EmptyRateLimit struct{}

func (e *EmptyRateLimit) RequestsLimit() int64 {
	return 0
}

func (e *EmptyRateLimit) RequestsRemaining() int64 {
	return 0
}

func (e *EmptyRateLimit) RequestsReset() time.Duration {
	return 0
}

func (e *EmptyRateLimit) TokensLimit() int64 {
	return 0
}

func (e *EmptyRateLimit) TokensRemaining() int64 {
	return 0
}

func (e *EmptyRateLimit) TokensReset() time.Duration {
	return 0
}
