// Package embedding defines the stable text-to-vector protocol and its
// single-method provider SPI.
//
// Model implementations own provider defaults at construction. Callers put
// per-call overrides in Request.Options. Dimension discovery is optional via
// Dimensioner or explicit and uncached via ProbeDimensions.
package embedding
