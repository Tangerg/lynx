// Package sets provides a comprehensive collection of set data structures and operations
// for Go, implementing mathematical set theory with type safety through generics.
//
// # Overview
//
// This package offers multiple set implementations optimized for different use cases:
//
//   - HashSet: Fast, unordered set implementation using hash maps (O(1) operations)
//   - LinkedSet: Ordered set that maintains insertion order (O(1) operations with ordering)
//   - SyncSet: Thread-safe wrapper for concurrent access (uses RWMutex)
//
// All implementations satisfy the Set[T comparable] interface, ensuring consistent
// behavior and allowing seamless switching between implementations.
package sets
