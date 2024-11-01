package kv

import (
	"bytes"
	"encoding/json"
	"iter"
	"reflect"
	"slices"
	"strings"

	xstrings "github.com/Tangerg/lynx/pkg/strings"
)

var _ json.Marshaler = (*OrderedKV[any, any])(nil)
var _ json.Unmarshaler = (*OrderedKV[any, any])(nil)

// OrderedKV is a key-value map that maintains the order of keys.
type OrderedKV[K comparable, V any] struct {
	kv   KV[K, V]
	keys []K
}

// UnmarshalJSON implement [json.Unmarshaler]
func (m *OrderedKV[K, V]) UnmarshalJSON(bs []byte) error {
	if len(bs) == 0 {
		return nil
	}
	m.Clear()

	err := json.Unmarshal(bs, &m.kv)
	if err != nil {
		return err
	}

	keys := make(map[string]K, m.kv.Size())
	m.kv.ForEach(func(k K, _ V) {
		strKey, _ := json.Marshal(k)
		keys[xstrings.UnQuote(string(strKey))] = k
	})

	decoder := json.NewDecoder(bytes.NewReader(bs))
	_, _ = decoder.Token()

	var (
		token    json.Token
		rawValue json.RawMessage
	)
	for decoder.More() {
		// key
		token, _ = decoder.Token()
		keyStr := token.(string)
		key := keys[keyStr]
		m.keys = append(m.keys, key)

		// need to consume value token to ensure next token is key
		_ = decoder.Decode(&rawValue)
		m.tryUnmarshalMap(key, rawValue)
	}

	return nil
}

func (m *OrderedKV[K, V]) tryUnmarshalMap(key K, bs []byte) {
	if len(bs) < 2 {
		return
	}
	if !(bs[0] == '{' && bs[len(bs)-1] == '}') {
		return
	}
	mv := m.Value(key)
	kind := reflect.ValueOf(mv).Kind()
	if kind != reflect.Map {
		return
	}

	om := NewOrderedKV[string, any]()
	_ = json.Unmarshal(bs, om)

	// only put value while V's Type is any|interface|OrderedKV
	v, ok := reflect.ValueOf(om).Interface().(V)
	if ok {
		m.Put(key, v)
	}
}

// MarshalJSON implement [json.Marshaler]
func (m *OrderedKV[K, V]) MarshalJSON() ([]byte, error) {
	sb := strings.Builder{}

	sb.WriteByte('{')
	for i, key := range m.keys {
		k, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		sb.WriteByte('"')
		sb.WriteString(xstrings.UnQuote(string(k)))
		sb.WriteByte('"')

		sb.WriteByte(':')

		v, err := json.Marshal(m.Value(key))
		if err != nil {
			return nil, err
		}
		sb.WriteString(string(v))

		if i < m.Size()-1 {
			sb.WriteByte(',')
		}
	}
	sb.WriteByte('}')

	return []byte(sb.String()), nil
}

// Size returns the number of key-value pairs in the map.
func (m *OrderedKV[K, V]) Size() int {
	return len(m.kv)
}

// IsEmpty checks if the map contains no key-value pairs.
func (m *OrderedKV[K, V]) IsEmpty() bool {
	return m.Size() == 0
}

// Get returns the value associated with the specified key and a boolean indicating whether the key exists.
func (m *OrderedKV[K, V]) Get(k K) (V, bool) {
	return m.kv.Get(k)
}

// Value retrieves the value associated with the specified key.
// If the key does not exist, the zero value for the value type is returned.
func (m *OrderedKV[K, V]) Value(k K) V {
	return m.kv.Value(k)
}

// GetOrDefault retrieves the value associated with the specified key,
// or returns the provided default value if the key does not exist.
func (m *OrderedKV[K, V]) GetOrDefault(k K, def V) V {
	return m.kv.GetOrDefault(k, def)
}

// Put inserts or updates a key-value pair in the map.
// It returns the updated map.
func (m *OrderedKV[K, V]) Put(k K, v V) *OrderedKV[K, V] {
	if m.ContainsKey(k) {
		m.removeKeyIfExist(k)
	}
	m.kv.Put(k, v)
	m.keys = append(m.keys, k)
	return m
}

// PutAll inserts or updates all key-value pairs from the provided OrderedKV into the current map.
// It returns the updated map.
func (m *OrderedKV[K, V]) PutAll(p *OrderedKV[K, V]) *OrderedKV[K, V] {
	for _, key := range p.keys {
		m.Put(key, p.Value(key))
	}
	return m
}

// PutIfAbsent inserts a key-value pair only if the key does not already exist in the map.
// It returns the updated map.
func (m *OrderedKV[K, V]) PutIfAbsent(k K, v V) *OrderedKV[K, V] {
	if !m.ContainsKey(k) {
		m.Put(k, v)
	}
	return m
}

// Remove deletes a key-value pair from the map based on the specified key.
// It returns the removed value.
func (m *OrderedKV[K, V]) Remove(k K) V {
	if m.ContainsKey(k) {
		m.removeKeyIfExist(k)
	}
	return m.kv.Remove(k)
}

// removeKeyIfExist removes the key from the order slice if it exists.
func (m *OrderedKV[K, V]) removeKeyIfExist(k K) {
	idx := slices.Index(m.keys, k)
	if idx != -1 {
		if idx == 0 {
			m.keys = m.keys[1:]
		} else if idx == m.Size()-1 {
			m.keys = m.keys[:idx]
		} else {
			m.keys = append(m.keys[:idx], m.keys[idx+1:]...)
		}
	}
}

// Clear removes all key-value pairs from the map.
// It returns the updated (empty) map.
func (m *OrderedKV[K, V]) Clear() *OrderedKV[K, V] {
	clear(m.kv)
	clear(m.keys)
	return m
}

// ContainsKey checks if the map contains the specified key.
func (m *OrderedKV[K, V]) ContainsKey(k K) bool {
	return m.kv.ContainsKey(k)
}

// Keys returns a slice containing all the keys in the map in the order of insertion.
func (m *OrderedKV[K, V]) Keys() []K {
	return m.keys
}

// Values returns a slice containing all the values in the map in the order of keys.
func (m *OrderedKV[K, V]) Values() []V {
	rv := make([]V, 0, len(m.keys))
	for _, key := range m.keys {
		rv = append(rv, m.Value(key))
	}
	return rv
}

// Clone creates and returns a shallow copy of the current OrderedKV.
func (m *OrderedKV[K, V]) Clone() *OrderedKV[K, V] {
	return NewOrderedKV[K, V](m.Size()).PutAll(m)
}

// Iterator returns a sequence function that iterates over the key-value pairs
// in the OrderedKV map. The iteration stops if the yield function returns false.
func (m *OrderedKV[K, V]) Iterator() iter.Seq2[K, V] {
	return func(yield func(key K, value V) bool) {
		for _, key := range m.keys {
			if !yield(key, m.Value(key)) {
				return
			}
		}
	}
}

// ForEach iterates over all key-value pairs in the map and applies the provided function.
func (m *OrderedKV[K, V]) ForEach(f func(k K, v V)) {
	for _, key := range m.keys {
		f(key, m.Value(key))
	}
}

// Reverse reverses the order of keys in the OrderedKV map and returns the updated map.
func (m *OrderedKV[K, V]) Reverse() *OrderedKV[K, V] {
	for i := range m.keys {
		if i == m.Size()/2 {
			break
		}
		j := m.Size() - i - 1
		m.keys[i], m.keys[j] = m.keys[j], m.keys[i]
	}
	return m
}

// GetReply retrieves the value associated with the specified key
// and returns it wrapped in a Reply struct. The function returns
// a pointer to the Reply struct.
func (m *OrderedKV[K, V]) GetReply(k K) *Reply {
	return &Reply{
		v: m.Value(k),
	}
}

// NewOrderedKV creates and returns an initialized OrderedKV with an optional initial capacity.
func NewOrderedKV[K comparable, V any](lens ...int) *OrderedKV[K, V] {
	var l = 0
	if len(lens) > 0 {
		l = lens[0]
	}
	return &OrderedKV[K, V]{
		kv:   New[K, V](l),
		keys: make([]K, 0, l),
	}
}
