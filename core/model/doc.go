// Package model defines the small response values genuinely shared across AI
// modalities. Calling capabilities live in their concrete modality packages;
// this package does not provide a generic model hierarchy or middleware layer.
//
// Usage reports token totals and optional provider breakdowns. RateLimit holds
// best-effort quota observations. Provider-specific raw payloads belong in the
// concrete adapter rather than this stable shared vocabulary.
package model
