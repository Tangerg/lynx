// Package maps provides a small family of [Map] implementations behind
// a single generic interface — useful when callers need a map abstraction
// that lets them swap ordered / unordered / concurrent backings without
// touching call sites.
//
// Four implementations ship in this package:
//
//   - [HashMap]    — Go's native map under a Map interface; default
//     choice when ordering and concurrency don't matter.
//   - [LinkedMap]  — insertion-order preserving; backs LRU caches and
//     ordered-iteration scenarios.
//   - [SyncMap]    — wraps any other [Map] with a sync.RWMutex; use it
//     to make HashMap / LinkedMap safe under concurrent writes.
//   - [StdSyncMap] — thin wrapper over [sync.Map]; pick this for the
//     classic "read-mostly, write-rarely" concurrent workload.
//
// The interface uses Go 1.23 range-over-func iterators ([Map.Iter],
// [Map.IterKeys], [Map.IterValues]) and exposes functional helpers
// ([Map.Compute], [Map.ComputeIfAbsent], [Map.Merge]) so atomic
// read-modify-write updates don't require a manual lock dance.
//
// Concurrency notes:
//   - [HashMap] and [LinkedMap] are NOT safe for concurrent use;
//     wrap with [SyncMap] (or pick [StdSyncMap]) when sharing
//     across goroutines.
//   - Concurrent maps snapshot during iteration; mutations made
//     after Iter() starts are not observed by that iterator.
//
// Use Go's native `map[K]V` directly when none of these flavors add
// value — this package exists for code that wants the Map interface,
// not as a replacement for the language primitive.
package maps
