// Package mime provides comprehensive MIME type handling with advanced parsing,
// detection, manipulation, and file extension mapping capabilities.
//
// # Overview
//
// This package offers a complete solution for working with MIME (Multipurpose
// Internet Mail Extensions) types in Go applications. It supports RFC-compliant
// parsing, content-based type detection, type normalization, and extensive file
// extension to MIME type mapping.
//
// # Key Features
//
//   - RFC 2045/2046 compliant MIME type parsing
//   - Content-based MIME type detection using magic bytes
//   - Hierarchical type matching and comparison
//   - X-prefix normalization (RFC 6648)
//   - Comprehensive file extension mapping (600+ extensions)
//   - Thread-safe operations with concurrent access support
//   - Extensible registration system for custom types
//   - Structured suffix handling (+xml, +json, etc.)
package mime
