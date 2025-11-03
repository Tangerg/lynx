// Package maps provides a comprehensive collection of thread-safe and non-thread-safe
// Map implementations with a unified interface, inspired by Java's Map interface but
// designed idiomatically for Go.
//
// # Overview
//
// This package offers multiple Map implementations, each optimized for different use cases:
//
//   - HashMap: Fast, unordered key-value storage using Go's native map
//   - LinkedMap: Maintains insertion order while providing O(1) access
//   - SyncMap: Thread-safe wrapper for any Map implementation
//   - StdSyncMap: High-performance concurrent map based on sync.Map
//
// All implementations share a common Map[K, V] interface, making them interchangeable
// and allowing you to choose the right implementation for your specific needs.
//
// # Map Interface
//
// The Map[K comparable, V any] interface defines a complete set of operations for
// working with key-value mappings:
//
//   - Basic Operations: Put, Get, Remove, ContainsKey, ContainsValue
//   - Query Operations: Size, IsEmpty
//   - Bulk Operations: Clear, PutAll, Keys, Values, Entries, ForEach
//   - Conditional Operations: GetOrDefault, PutIfAbsent, RemoveIf, Replace, ReplaceIf
//   - Functional Operations: Compute, ComputeIfAbsent, ComputeIfPresent, Merge, ReplaceAll
//   - Iteration: Iter, IterKeys, IterValues (using Go 1.23+ iterators)
//   - Cloning: Clone for creating independent copies
//
// # HashMap - Fast Unordered Storage
//
// HashMap provides O(1) average-case performance for basic operations using Go's
// native map. It's the default choice when you don't need specific ordering or
// thread-safety guarantees.
//
// Usage:
//
//	// Create a new HashMap
//	m := make(maps.HashMap[string, int])
//	// or with capacity hint
//	m := maps.NewHashMap[string, int](100)
//
//	// Basic operations
//	m.Put("age", 30)
//	age, exists := m.Get("age")
//	m.Remove("age")
//
//	// Functional operations
//	m.Compute("counter", func(k string, v int, exists bool) (int, bool) {
//	    if exists {
//	        return v + 1, true
//	    }
//	    return 1, true
//	})
//
//	// Iteration
//	for key, value := range m.Iter() {
//	    fmt.Printf("%s: %d\n", key, value)
//	}
//
// Characteristics:
//   - O(1) average-case for Put, Get, Remove, ContainsKey
//   - O(n) for ContainsValue, iteration
//   - No ordering guarantees
//   - Not thread-safe (use SyncMap wrapper for thread safety)
//   - Memory efficient
//
// # LinkedMap - Ordered Key-Value Storage
//
// LinkedMap combines a hash table with a doubly-linked list to maintain insertion
// order while providing O(1) access to elements. It's ideal for LRU caches, ordered
// iteration, and scenarios where insertion order matters.
//
// Usage:
//
//	// Create a new LinkedMap
//	m := maps.NewLinkedMap[string, int]()
//	// or with capacity hint
//	m := maps.NewLinkedMap[string, int](100)
//
//	// Maintains insertion order
//	m.Put("first", 1)
//	m.Put("second", 2)
//	m.Put("third", 3)
//
//	// Access first and last elements
//	firstKey, firstVal, ok := m.First()  // ("first", 1, true)
//	lastKey, lastVal, ok := m.Last()     // ("third", 3, true)
//
//	// Remove from ends (useful for queues/deques)
//	m.RemoveFirst()  // removes "first"
//	m.RemoveLast()   // removes "third"
//
//	// Ordered iteration
//	for key, value := range m.Iter() {
//	    // Iterates in insertion order
//	}
//
//	// LRU cache pattern
//	if m.Size() >= maxSize {
//	    m.RemoveFirst()  // Remove oldest entry
//	}
//	m.Put(newKey, newValue)
//
// Characteristics:
//   - O(1) for Put, Get, Remove, ContainsKey, First, Last, RemoveFirst, RemoveLast
//   - O(n) for ContainsValue
//   - Maintains insertion order
//   - Updating existing keys preserves their position
//   - Not thread-safe (use SyncMap wrapper for thread safety)
//   - Higher memory overhead than HashMap due to linked list nodes
//
// Use Cases:
//   - LRU/LFU caches
//   - Maintaining command history
//   - Ordered configuration settings
//   - FIFO/LIFO queues with O(1) lookup
//   - Preserving API response order
//
// # SyncMap - Thread-Safe Wrapper
//
// SyncMap wraps any Map implementation with a read-write mutex (sync.RWMutex) to
// provide thread-safe concurrent access. It preserves all characteristics of the
// wrapped map, including ordering for LinkedMap.
//
// Usage:
//
//	// Wrap a HashMap (default)
//	m := maps.NewSyncMap[string, int]()
//
//	// Wrap a LinkedMap for ordered + thread-safe
//	orderedMap := maps.NewSyncMap(maps.NewLinkedMap[string, int]())
//
//	// Safe concurrent access
//	var wg sync.WaitGroup
//	for i := 0; i < 100; i++ {
//	    wg.Add(1)
//	    go func(id int) {
//	        defer wg.Done()
//	        m.Put(fmt.Sprintf("key%d", id), id)
//	    }(i)
//	}
//	wg.Wait()
//
//	// Iteration creates a snapshot
//	for k, v := range m.Iter() {
//	    // Safe to iterate while other goroutines modify the map
//	    // Iterates over a snapshot taken at Iter() call time
//	}
//
// Concurrency Model:
//   - Read operations (Get, ContainsKey, Size, etc.) use RLock - multiple readers allowed
//   - Write operations (Put, Remove, etc.) use Lock - exclusive access
//   - Iteration creates a snapshot to avoid holding locks during iteration
//   - Prevents double-wrapping (returns same instance if already wrapped)
//
// Characteristics:
//   - Thread-safe for all operations
//   - Read operations can run concurrently
//   - Write operations are serialized
//   - Preserves wrapped map's characteristics (order, performance)
//   - Snapshot-based iteration avoids deadlocks
//   - Good balance of performance and flexibility
//
// When to use:
//   - General-purpose thread-safe map
//   - When you need ordering + thread-safety (wrap LinkedMap)
//   - Moderate concurrency with mixed read/write workloads
//   - When you want control over the underlying implementation
//
// # StdSyncMap - High-Performance Concurrent Map
//
// StdSyncMap is based on Go's sync.Map and optimized for specific concurrent access
// patterns. It uses lock-free operations and is ideal for read-heavy workloads or
// when different goroutines work with disjoint key sets.
//
// Usage:
//
//	// Create a new StdSyncMap
//	m := maps.NewStdSyncMap[string, int]()
//
//	// Optimized for concurrent reads
//	var wg sync.WaitGroup
//	for i := 0; i < 1000; i++ {
//	    wg.Add(1)
//	    go func(id int) {
//	        defer wg.Done()
//	        // Many concurrent reads
//	        for j := 0; j < 1000; j++ {
//	            m.Get(fmt.Sprintf("key%d", j))
//	        }
//	        // Occasional write
//	        if id % 10 == 0 {
//	            m.Put(fmt.Sprintf("key%d", id), id)
//	        }
//	    }(i)
//	}
//	wg.Wait()
//
//	// Atomic operations
//	value, inserted := m.PutIfAbsent("key", 100)
//	m.Merge("counter", 1, func(old, new int) int {
//	    return old + new  // Atomic increment
//	})
//
// Optimization Patterns:
//   - Entries written once, read many times
//   - Multiple goroutines working with disjoint key sets
//   - Read-heavy workloads with occasional writes
//   - Lock-free operations for better scalability
//
// Characteristics:
//   - Lock-free reads for existing keys
//   - Optimized for read-heavy workloads
//   - No ordering guarantees
//   - Compare-and-swap operations for consistency
//   - Higher memory overhead than SyncMap
//   - May be slower for write-heavy workloads
//
// When to use:
//   - High-concurrency read-heavy scenarios
//   - Goroutines working with separate key sets
//   - Need for lock-free operations
//   - Don't need ordering guarantees
//
// # Entry Type
//
// The Entry[K, V] type represents a key-value pair in the map. Entries are returned
// by the Entries() method and provide immutable access to map contents.
//
//	entries := m.Entries()
//	for _, entry := range entries {
//	    key := entry.Key()
//	    value := entry.Value()
//	    fmt.Printf("%v: %v\n", key, value)
//	}
//
// # Iteration
//
// All map implementations support Go 1.23+ range-over-func iteration, providing
// a clean and idiomatic way to iterate over map contents.
//
//	// Iterate over key-value pairs
//	for key, value := range m.Iter() {
//	    fmt.Printf("%v: %v\n", key, value)
//	}
//
//	// Iterate over keys only
//	for key := range m.IterKeys() {
//	    fmt.Println(key)
//	}
//
//	// Iterate over values only
//	for value := range m.IterValues() {
//	    fmt.Println(value)
//	}
//
//	// Early break is supported
//	for k, v := range m.Iter() {
//	    if k == "target" {
//	        break
//	    }
//	}
//
// For thread-safe maps (SyncMap, StdSyncMap), iteration creates a snapshot to avoid
// holding locks during iteration, preventing deadlocks and allowing concurrent modifications.
//
// # Functional Operations
//
// The package provides powerful functional operations for atomic map updates:
//
//	// Compute - atomically compute new value
//	m.Compute("counter", func(k string, v int, exists bool) (int, bool) {
//	    if exists {
//	        return v + 1, true  // increment
//	    }
//	    return 1, true  // initialize
//	})
//
//	// ComputeIfAbsent - lazy initialization
//	value := m.ComputeIfAbsent("config", func(k string) Config {
//	    return loadConfig(k)  // only called if key is absent
//	})
//
//	// ComputeIfPresent - update existing values only
//	m.ComputeIfPresent("user", func(k string, v User) User {
//	    v.LastSeen = time.Now()
//	    return v
//	})
//
//	// Merge - combine values
//	m.Merge("wordCount", 1, func(oldCount, newCount int) int {
//	    return oldCount + newCount
//	})
//
//	// ReplaceAll - transform all values
//	m.ReplaceAll(func(k string, v int) int {
//	    return v * 2  // double all values
//	})
//
// These operations are particularly useful with thread-safe maps as they ensure
// atomic updates without race conditions.
//
// # Choosing the Right Map
//
// Decision Guide:
//
//	Need thread safety?
//	├─ No
//	│  ├─ Need ordering?
//	│  │  ├─ Yes → LinkedMap
//	│  │  └─ No  → HashMap
//	│  └─ ...
//	└─ Yes
//	   ├─ Need ordering?
//	   │  ├─ Yes → SyncMap(LinkedMap)
//	   │  └─ No  → Continue...
//	   ├─ Read-heavy workload?
//	   │  ├─ Yes (>90% reads) → StdSyncMap
//	   │  └─ No               → SyncMap
//	   └─ Disjoint key sets?
//	      ├─ Yes → StdSyncMap
//	      └─ No  → SyncMap
//
// Performance Characteristics Summary:
//
//	Operation           HashMap    LinkedMap   SyncMap        StdSyncMap
//	─────────────────────────────────────────────────────────────────────
//	Put                 O(1)       O(1)        O(1)+lock      Lock-free*
//	Get                 O(1)       O(1)        O(1)+rlock     Lock-free
//	Remove              O(1)       O(1)        O(1)+lock      Lock-free*
//	ContainsKey         O(1)       O(1)        O(1)+rlock     Lock-free
//	ContainsValue       O(n)       O(n)        O(n)+rlock     O(n)
//	First/Last          -          O(1)        O(1)+rlock     -
//	Iteration           Unordered  Ordered     Snapshot       Unordered
//	Memory Overhead     Low        Medium      Low-Medium     Medium-High
//	Thread Safety       No         No          Yes            Yes
//	Concurrency         -          -           RWMutex        Lock-free
//
//	* Lock-free for common cases, may use locks for contention
//
// # Example: LRU Cache Implementation
//
//	type LRUCache struct {
//	    capacity int
//	    cache    *maps.LinkedMap[string, any]
//	}
//
//	func NewLRUCache(capacity int) *LRUCache {
//	    return &LRUCache{
//	        capacity: capacity,
//	        cache:    maps.NewLinkedMap[string, any](capacity),
//	    }
//	}
//
//	func (c *LRUCache) Get(key string) (any, bool) {
//	    value, exists := c.cache.Get(key)
//	    if exists {
//	        // Move to end (most recently used)
//	        c.cache.Remove(key)
//	        c.cache.Put(key, value)
//	    }
//	    return value, exists
//	}
//
//	func (c *LRUCache) Put(key string, value any) {
//	    if c.cache.Size() >= c.capacity && !c.cache.ContainsKey(key) {
//	        c.cache.RemoveFirst() // Evict least recently used
//	    }
//	    c.cache.Put(key, value)
//	}
//
// # Example: Thread-Safe Counter
//
//	// Using SyncMap for thread-safe counting
//	counter := maps.NewSyncMap[string, int]()
//
//	// Increment counter atomically
//	func incrementCounter(category string) {
//	    counter.Compute(category, func(k string, v int, exists bool) (int, bool) {
//	        if exists {
//	            return v + 1, true
//	        }
//	        return 1, true
//	    })
//	}
//
//	// Or using StdSyncMap with Merge
//	counter := maps.NewStdSyncMap[string, int]()
//
//	func incrementCounter(category string) {
//	    counter.Merge(category, 1, func(old, new int) int {
//	        return old + new
//	    })
//	}
//
// # Example: Configuration Manager
//
//	type Config struct {
//	    settings maps.Map[string, string]
//	    mu       sync.RWMutex
//	}
//
//	func NewConfig() *Config {
//	    return &Config{
//	        settings: maps.NewLinkedMap[string, string](),
//	    }
//	}
//
//	func (c *Config) Get(key string) string {
//	    return c.settings.GetOrDefault(key, "")
//	}
//
//	func (c *Config) Set(key, value string) {
//	    c.settings.Put(key, value)
//	}
//
//	func (c *Config) GetAll() []string {
//	    // Returns settings in insertion order
//	    return c.settings.Keys()
//	}
//
// # Thread Safety Considerations
//
// Non-thread-safe maps (HashMap, LinkedMap):
//   - Fastest performance for single-threaded use
//   - Can be wrapped with SyncMap for thread safety
//   - Must use external synchronization if accessed from multiple goroutines
//
// Thread-safe maps (SyncMap, StdSyncMap):
//   - Safe for concurrent access without external synchronization
//   - SyncMap uses RWMutex - good for general use
//   - StdSyncMap uses lock-free techniques - best for read-heavy workloads
//   - Iteration creates snapshots to avoid lock contention
//
// # Memory Management
//
// All map implementations properly handle memory for their elements:
//   - Removed entries are eligible for garbage collection
//   - Clone creates deep copies of map structure (shallow copy of values)
//   - Clear removes all entries and allows GC to reclaim memory
//   - No memory leaks from cyclic references in the map structure
//
// # Type Constraints
//
// Keys (K) must be comparable:
//   - Basic types: int, string, float64, etc.
//   - Structs with comparable fields
//   - Arrays (not slices)
//   - Pointers
//
// Values (V) can be any type:
//   - No constraints on value types
//   - Can store interfaces, slices, maps, functions, etc.
//   - nil values are supported for pointer and interface types
//
// # Best Practices
//
// 1. Choose the right implementation:
//   - Default to HashMap for single-threaded use
//   - Use LinkedMap when order matters
//   - Wrap with SyncMap for thread safety
//   - Use StdSyncMap for high-concurrency reads
//
// 2. Capacity hints:
//   - Provide capacity hints when size is known
//   - Reduces allocations and improves performance
//   - Example: NewHashMap[string, int](expectedSize)
//
// 3. Functional operations:
//   - Use Compute/Merge for atomic updates in concurrent scenarios
//   - Use ComputeIfAbsent for lazy initialization
//   - Prefer functional operations over Get-Modify-Put patterns
//
// 4. Iteration:
//   - Use Iter() for modern idiomatic Go
//   - Remember that iteration order is only guaranteed for LinkedMap
//   - Thread-safe maps create snapshots during iteration
//
// 5. Error handling:
//   - Check boolean return values (e.g., Get, Remove)
//   - Use GetOrDefault to avoid nil checks
//   - ContainsKey before operations if existence matters
//
// # Package Structure
//
// The package is organized as follows:
//
//	maps/
//	├── map.go          # Map interface definition
//	├── entry.go        # Entry type implementation
//	├── hashmap.go      # HashMap implementation
//	├── linkedmap.go    # LinkedMap implementation
//	├── sync.go         # SyncMap and StdSyncMap implementations
//	├── doc.go          # This documentation file
//	└── *_test.go       # Comprehensive test suites
//
// # Compatibility
//
// Requires:
//   - Go 1.23 or later (for range-over-func iterators)
//   - Compatible with all Go platforms
//
// # Performance Notes
//
// Benchmark results (approximate, varies by workload):
//
//	HashMap:
//	  - Put: ~50-100 ns/op
//	  - Get: ~30-50 ns/op
//	  - Ideal for: Single-threaded, unordered operations
//
//	LinkedMap:
//	  - Put: ~100-150 ns/op
//	  - Get: ~50-80 ns/op
//	  - Ideal for: Ordered iteration, LRU caches
//
//	SyncMap:
//	  - Put: ~200-300 ns/op (with lock contention)
//	  - Get: ~100-150 ns/op (read lock)
//	  - Ideal for: General concurrent use, moderate contention
//
//	StdSyncMap:
//	  - Put: ~150-250 ns/op (lock-free fast path)
//	  - Get: ~50-100 ns/op (lock-free)
//	  - Ideal for: Read-heavy concurrent workloads
//
// # License
//
// This package is provided as-is for use in Go applications. See the repository
// for specific license terms.
//
// # Contributing
//
// Contributions are welcome! Please ensure:
//   - All tests pass: go test ./...
//   - Code is formatted: go fmt ./...
//   - New features include tests and documentation
//   - Benchmarks show performance characteristics
//
// # Additional Resources
//
// For more information:
//   - Package documentation: go doc maps
//   - Examples: go test -run Example
//   - Benchmarks: go test -bench=. -benchmem
//   - Source code: See individual implementation files
package maps
