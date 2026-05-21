// Package tokenizer defines text/media token-counting and encoding
// interfaces. The synchronous shapes ([Encoder], [Decoder]) live next
// to the cheap-estimate shapes ([TextEstimator], [MediaEstimator]) so
// callers can mix and match: a token-count gate before submitting a
// chat request, an exact encoder for batching strategies, etc.
package tokenizer
