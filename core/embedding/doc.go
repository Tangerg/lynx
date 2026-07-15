// Package embedding defines the stable text-to-vector protocol and its
// single-method provider SPI.
//
// Build input with NewRequest and attach Options only for per-call overrides.
// Provider defaults and identity are fixed when constructing an implementation,
// not exposed through Model. Dimension discovery is optional via Dimensioner;
// ProbeDimensions performs an explicit uncached request when needed.
package embedding
