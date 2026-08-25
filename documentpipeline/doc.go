// Package documentpipeline provides explicit document ingestion policies.
//
// The package owns formatting, splitting, batching, identifier assignment, and
// concrete ingestion I/O. Format-aware extensions live in
// optional submodules such as documentpipeline/markdown so this base module
// remains independent of document parsers. The core document package remains
// a serializable data contract and does not depend on this module.
package documentpipeline
