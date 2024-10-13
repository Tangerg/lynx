package kv

// KV is a generic key-value map with comparable keys and any type of values.
type KV[K comparable, V any] map[K]V

// KSVA is a type alias for a KV map where the keys are of type string and the values can be of any type.
type KSVA = KV[string, any]

// Size returns the number of key-value pairs in the map.
func (m KV[K, V]) Size() int {
	return len(m)
}

// IsEmpty checks if the map contains no key-value pairs.
func (m KV[K, V]) IsEmpty() bool {
	return m.Size() == 0
}

// Value returns the value associated with the specified key and a boolean indicating whether the key exists.
func (m KV[K, V]) Value(k K) (V, bool) {
	v, ok := m[k]
	return v, ok
}

// Get retrieves the value associated with the specified key.
// If the key does not exist, the zero value for the value type is returned.
func (m KV[K, V]) Get(k K) V {
	return m[k]
}

// GetOrDefault retrieves the value associated with the specified key,
// or returns the provided default value if the key does not exist.
func (m KV[K, V]) GetOrDefault(k K, def V) V {
	if v, ok := m.Value(k); ok {
		return v
	}
	return def
}

// Put inserts or updates a key-value pair in the map.
// It returns the updated map.
func (m KV[K, V]) Put(k K, v V) KV[K, V] {
	m[k] = v
	return m
}

// PutAll inserts or updates all key-value pairs from the provided map into the current map.
// It returns the updated map.
func (m KV[K, V]) PutAll(p KV[K, V]) KV[K, V] {
	for k, v := range p {
		m.Put(k, v)
	}
	return m
}

// PutIfAbsent inserts a key-value pair only if the key does not already exist in the map.
// It returns the updated map.
func (m KV[K, V]) PutIfAbsent(k K, v V) KV[K, V] {
	if !m.ContainsKey(k) {
		m.Put(k, v)
	}
	return m
}

// Remove deletes a key-value pair from the map based on the specified key.
// It returns the removed value.
func (m KV[K, V]) Remove(k K) V {
	get := m.Get(k)
	delete(m, k)
	return get
}

// Clear removes all key-value pairs from the map.
// It returns the updated (empty) map.
func (m KV[K, V]) Clear() KV[K, V] {
	clear(m)
	return m
}

// ContainsKey checks if the map contains the specified key.
func (m KV[K, V]) ContainsKey(k K) bool {
	_, ok := m[k]
	return ok
}

// Keys returns a slice containing all the keys in the map.
func (m KV[K, V]) Keys() []K {
	rv := make([]K, 0, len(m))
	for k := range m {
		rv = append(rv, k)
	}
	return rv
}

// Values returns a slice containing all the values in the map.
func (m KV[K, V]) Values() []V {
	rv := make([]V, 0, len(m))
	for _, v := range m {
		rv = append(rv, v)
	}
	return rv
}

// Clone creates and returns a shallow copy of the current map.
func (m KV[K, V]) Clone() KV[K, V] {
	return New[K, V](m.Size()).
		PutAll(m)
}

// ForEach iterates over all key-value pairs in the map and applies the provided function.
func (m KV[K, V]) ForEach(f func(k K, v V)) {
	for k, v := range m {
		f(k, v)
	}
}

// GetReply retrieves the value associated with the specified key
// and returns it wrapped in a Reply struct. The function returns
// a pointer to the Reply struct.
func (m KV[K, V]) GetReply(k K) *Reply {
	return &Reply{
		v: m.Get(k),
	}
}

// New creates and returns an empty KV map with an optional initial capacity.
func New[K comparable, V any](lens ...int) KV[K, V] {
	var l = 0
	if len(lens) > 0 {
		l = lens[0]
	}
	return make(KV[K, V], l)
}

// NewKSVA creates and returns an empty KSVA map with an optional initial capacity.
func NewKSVA(lens ...int) KSVA {
	return New[string, any](lens...)
}

// Of creates and returns a new KV map containing all the key-value pairs from the provided kv map.
// It initializes a new KV map and populates it by copying all entries from the provided map.
func Of[K comparable, V any](kv KV[K, V]) KV[K, V] {
	return New[K, V](kv.Size()).PutAll(kv)
}
