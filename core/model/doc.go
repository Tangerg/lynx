// Package model defines the small response values genuinely shared across AI
// modalities. Calling capabilities live in their concrete modality packages;
// this package does not provide a generic model hierarchy or middleware layer.
//
// Usage reports token totals and optional provider breakdowns. RateLimit holds
// best-effort quota observations. Provider-specific raw usage may be retained in
// Usage.OriginalUsage, but callers should prefer stable fields when available.
package model
