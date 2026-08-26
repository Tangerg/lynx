// Package etl provides explicit extract-transform-load building blocks for
// documents.
//
// Format readers extract source data into core document values. Formatters,
// splitters, identifier assignment, and batching transform those values for a
// downstream index. TextFileWriter provides a concrete filesystem load target;
// vector-store loading remains owned by core/vectorstore capabilities.
//
// The base module owns the root package plus text, JSON, and Markdown packages.
// HTML and PDF remain optional leaf modules because their parser dependencies
// are materially larger. The core document package remains a serializable data
// contract and does not depend on ETL.
package etl
