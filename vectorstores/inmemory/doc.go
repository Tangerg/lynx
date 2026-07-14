// Package inmemory is an in-process the vectorstore capability interfaces backed by a
// `map[string]record` plus a configurable similarity function. It is
// intended for demos, unit tests, and corpora that fit in RAM.
//
// Concurrency: every public method is safe for concurrent use; reads
// take an RLock, writes take a Lock. The store performs no I/O —
// errors come from the embedding client or the filter parser.
//
// Persistence is out of scope: the store has no durability story.
package inmemory
