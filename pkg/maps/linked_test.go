package maps

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
)

// =============================================================================
// Constructor Tests
// =============================================================================

func TestLinkedMap_NewLinkedMap(t *testing.T) {
	tests := []struct {
		name     string
		capacity []int
		wantSize int
	}{
		{
			name:     "no capacity specified",
			capacity: nil,
			wantSize: 0,
		},
		{
			name:     "zero capacity",
			capacity: []int{0},
			wantSize: 0,
		},
		{
			name:     "negative capacity",
			capacity: []int{-1},
			wantSize: 0,
		},
		{
			name:     "positive capacity",
			capacity: []int{100},
			wantSize: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewLinkedMap[string, int](tt.capacity...)
			if m == nil {
				t.Fatal("NewLinkedMap returned nil")
			}
			if m.Size() != tt.wantSize {
				t.Errorf("Size() = %v, want %v", m.Size(), tt.wantSize)
			}
			if m.nodes == nil {
				t.Error("nodes map is nil")
			}
		})
	}
}

// =============================================================================
// Basic Operations Tests
// =============================================================================

func TestLinkedMap_Put(t *testing.T) {
	m := NewLinkedMap[string, int]()

	t.Run("insert first element", func(t *testing.T) {
		oldVal, existed := m.Put("first", 1)
		if existed {
			t.Error("expected key not to exist")
		}
		if oldVal != 0 {
			t.Errorf("expected zero value, got %v", oldVal)
		}
		if m.Size() != 1 {
			t.Errorf("Size() = %v, want 1", m.Size())
		}
		if m.head == nil || m.tail == nil {
			t.Error("head or tail is nil after first insert")
		}
		if m.head != m.tail {
			t.Error("head should equal tail for single element")
		}
	})

	t.Run("insert second element", func(t *testing.T) {
		oldVal, existed := m.Put("second", 2)
		if existed {
			t.Error("expected key not to exist")
		}
		if oldVal != 0 {
			t.Errorf("expected zero value, got %v", oldVal)
		}
		if m.Size() != 2 {
			t.Errorf("Size() = %v, want 2", m.Size())
		}
		if m.head == m.tail {
			t.Error("head should not equal tail for two elements")
		}
	})

	t.Run("update existing key preserves order", func(t *testing.T) {
		oldVal, existed := m.Put("first", 100)
		if !existed {
			t.Error("expected key to exist")
		}
		if oldVal != 1 {
			t.Errorf("expected old value 1, got %v", oldVal)
		}
		if m.Size() != 2 {
			t.Errorf("Size() = %v, want 2", m.Size())
		}

		keys := m.Keys()
		if keys[0] != "first" {
			t.Error("order should be preserved when updating")
		}
	})

	t.Run("insertion order maintained", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		order := []string{"a", "b", "c", "d", "e"}
		for i, key := range order {
			m.Put(key, i)
		}

		keys := m.Keys()
		for i, key := range keys {
			if key != order[i] {
				t.Errorf("key[%d] = %v, want %v", i, key, order[i])
			}
		}
	})
}

func TestLinkedMap_Get(t *testing.T) {
	m := NewLinkedMap[string, int]()
	m.Put("exists", 42)

	tests := []struct {
		name      string
		key       string
		wantValue int
		wantOk    bool
	}{
		{"existing key", "exists", 42, true},
		{"non-existing key", "missing", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotOk := m.Get(tt.key)
			if gotValue != tt.wantValue {
				t.Errorf("Get() value = %v, want %v", gotValue, tt.wantValue)
			}
			if gotOk != tt.wantOk {
				t.Errorf("Get() ok = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}

func TestLinkedMap_Remove(t *testing.T) {
	t.Run("remove from middle", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		val, existed := m.Remove("b")
		if !existed {
			t.Error("expected key to exist")
		}
		if val != 2 {
			t.Errorf("Remove() = %v, want 2", val)
		}
		if m.Size() != 2 {
			t.Errorf("Size() = %v, want 2", m.Size())
		}

		keys := m.Keys()
		if len(keys) != 2 || keys[0] != "a" || keys[1] != "c" {
			t.Errorf("Keys() = %v, want [a c]", keys)
		}
	})

	t.Run("remove head", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		val, existed := m.Remove("a")
		if !existed {
			t.Error("expected key to exist")
		}
		if val != 1 {
			t.Errorf("Remove() = %v, want 1", val)
		}
		if m.head.key != "b" {
			t.Errorf("head.key = %v, want 'b'", m.head.key)
		}
		if m.head.prev != nil {
			t.Error("new head.prev should be nil")
		}
	})

	t.Run("remove tail", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		val, existed := m.Remove("c")
		if !existed {
			t.Error("expected key to exist")
		}
		if val != 3 {
			t.Errorf("Remove() = %v, want 3", val)
		}
		if m.tail.key != "b" {
			t.Errorf("tail.key = %v, want 'b'", m.tail.key)
		}
		if m.tail.next != nil {
			t.Error("new tail.next should be nil")
		}
	})

	t.Run("remove last element", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("only", 1)

		val, existed := m.Remove("only")
		if !existed {
			t.Error("expected key to exist")
		}
		if val != 1 {
			t.Errorf("Remove() = %v, want 1", val)
		}
		if m.head != nil || m.tail != nil {
			t.Error("head and tail should be nil after removing last element")
		}
		if m.Size() != 0 {
			t.Errorf("Size() = %v, want 0", m.Size())
		}
	})

	t.Run("remove non-existing key", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)

		val, existed := m.Remove("missing")
		if existed {
			t.Error("expected key not to exist")
		}
		if val != 0 {
			t.Errorf("Remove() = %v, want 0", val)
		}
		if m.Size() != 1 {
			t.Errorf("Size() = %v, want 1", m.Size())
		}
	})
}

func TestLinkedMap_ContainsKey(t *testing.T) {
	m := NewLinkedMap[string, int]()
	m.Put("exists", 42)

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"existing key", "exists", true},
		{"non-existing key", "missing", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.ContainsKey(tt.key); got != tt.want {
				t.Errorf("ContainsKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinkedMap_ContainsValue(t *testing.T) {
	t.Run("primitive types", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		if !m.ContainsValue(2) {
			t.Error("expected to find value 2")
		}
		if m.ContainsValue(999) {
			t.Error("expected not to find value 999")
		}
	})

	t.Run("slice values", func(t *testing.T) {
		m := NewLinkedMap[string, []int]()
		m.Put("a", []int{1, 2, 3})
		m.Put("b", []int{4, 5, 6})

		if !m.ContainsValue([]int{1, 2, 3}) {
			t.Error("expected to find slice [1,2,3]")
		}
		if m.ContainsValue([]int{1, 2}) {
			t.Error("expected not to find slice [1,2]")
		}
	})

	t.Run("struct values", func(t *testing.T) {
		type Person struct {
			Name string
			Age  int
		}
		m := NewLinkedMap[string, Person]()
		m.Put("alice", Person{"Alice", 30})
		m.Put("bob", Person{"Bob", 25})

		if !m.ContainsValue(Person{"Alice", 30}) {
			t.Error("expected to find Alice")
		}
		if m.ContainsValue(Person{"Charlie", 35}) {
			t.Error("expected not to find Charlie")
		}
	})
}

func TestLinkedMap_Size(t *testing.T) {
	m := NewLinkedMap[string, int]()

	if m.Size() != 0 {
		t.Errorf("initial Size() = %v, want 0", m.Size())
	}

	m.Put("a", 1)
	if m.Size() != 1 {
		t.Errorf("Size() = %v, want 1", m.Size())
	}

	m.Put("b", 2)
	m.Put("c", 3)
	if m.Size() != 3 {
		t.Errorf("Size() = %v, want 3", m.Size())
	}

	m.Remove("b")
	if m.Size() != 2 {
		t.Errorf("Size() = %v, want 2", m.Size())
	}

	m.Clear()
	if m.Size() != 0 {
		t.Errorf("Size() after Clear() = %v, want 0", m.Size())
	}
}

func TestLinkedMap_IsEmpty(t *testing.T) {
	m := NewLinkedMap[string, int]()

	if !m.IsEmpty() {
		t.Error("new map should be empty")
	}

	m.Put("key", 1)
	if m.IsEmpty() {
		t.Error("map with elements should not be empty")
	}

	m.Remove("key")
	if !m.IsEmpty() {
		t.Error("map after removing all elements should be empty")
	}
}

func TestLinkedMap_Clear(t *testing.T) {
	m := NewLinkedMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	m.Clear()

	if !m.IsEmpty() {
		t.Error("map should be empty after Clear()")
	}
	if m.Size() != 0 {
		t.Errorf("Size() = %v, want 0", m.Size())
	}
	if m.head != nil {
		t.Error("head should be nil after Clear()")
	}
	if m.tail != nil {
		t.Error("tail should be nil after Clear()")
	}
	if m.ContainsKey("a") {
		t.Error("key 'a' should not exist after Clear()")
	}
}

// =============================================================================
// Bulk Operations Tests
// =============================================================================

func TestLinkedMap_PutAll(t *testing.T) {
	t.Run("from LinkedMap preserves order", func(t *testing.T) {
		m1 := NewLinkedMap[string, int]()
		m1.Put("a", 1)
		m1.Put("b", 2)
		m1.Put("c", 3)

		m2 := NewLinkedMap[string, int]()
		m2.PutAll(m1)

		if m2.Size() != 3 {
			t.Errorf("Size() = %v, want 3", m2.Size())
		}

		keys1 := m1.Keys()
		keys2 := m2.Keys()
		for i := range keys1 {
			if keys1[i] != keys2[i] {
				t.Errorf("order not preserved: got %v, want %v", keys2, keys1)
			}
		}
	})

	t.Run("from HashMap maintains insertion order", func(t *testing.T) {
		hm := NewHashMap[string, int]()
		hm.Put("a", 1)
		hm.Put("b", 2)

		lm := NewLinkedMap[string, int]()
		lm.PutAll(hm)

		if lm.Size() != 2 {
			t.Errorf("Size() = %v, want 2", lm.Size())
		}
	})

	t.Run("overwrite existing keys preserves original position", func(t *testing.T) {
		m1 := NewLinkedMap[string, int]()
		m1.Put("a", 1)
		m1.Put("b", 2)
		m1.Put("c", 3)

		m2 := NewLinkedMap[string, int]()
		m2.Put("b", 200)

		m1.PutAll(m2)

		keys := m1.Keys()
		if keys[1] != "b" {
			t.Error("'b' should maintain its original position")
		}
		if val, _ := m1.Get("b"); val != 200 {
			t.Errorf("Get(b) = %v, want 200", val)
		}
	})
}

func TestLinkedMap_Keys(t *testing.T) {
	m := NewLinkedMap[string, int]()
	order := []string{"first", "second", "third", "fourth"}
	for i, key := range order {
		m.Put(key, i)
	}

	keys := m.Keys()

	if len(keys) != len(order) {
		t.Errorf("len(Keys()) = %v, want %v", len(keys), len(order))
	}

	for i, key := range keys {
		if key != order[i] {
			t.Errorf("Keys()[%d] = %v, want %v", i, key, order[i])
		}
	}

	m.Put("fifth", 5)
	if len(keys) != 4 {
		t.Error("Keys() returned slice should be a snapshot")
	}
}

func TestLinkedMap_Values(t *testing.T) {
	m := NewLinkedMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	values := m.Values()

	if len(values) != 3 {
		t.Errorf("len(Values()) = %v, want 3", len(values))
	}

	expected := []int{1, 2, 3}
	for i, v := range values {
		if v != expected[i] {
			t.Errorf("Values()[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

func TestLinkedMap_Entries(t *testing.T) {
	m := NewLinkedMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)

	entries := m.Entries()

	if len(entries) != 2 {
		t.Errorf("len(Entries()) = %v, want 2", len(entries))
	}

	if entries[0].Key() != "a" || entries[0].Value() != 1 {
		t.Errorf("Entries()[0] = (%v, %v), want (a, 1)", entries[0].Key(), entries[0].Value())
	}
	if entries[1].Key() != "b" || entries[1].Value() != 2 {
		t.Errorf("Entries()[1] = (%v, %v), want (b, 2)", entries[1].Key(), entries[1].Value())
	}
}

func TestLinkedMap_ForEach(t *testing.T) {
	m := NewLinkedMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	sum := 0
	count := 0
	order := []string{}

	m.ForEach(func(k string, v int) {
		sum += v
		count++
		order = append(order, k)
	})

	if sum != 6 {
		t.Errorf("sum = %v, want 6", sum)
	}
	if count != 3 {
		t.Errorf("count = %v, want 3", count)
	}
	if order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Errorf("order = %v, want [a b c]", order)
	}
}

// =============================================================================
// Conditional Operations Tests
// =============================================================================

func TestLinkedMap_GetOrDefault(t *testing.T) {
	m := NewLinkedMap[string, int]()
	m.Put("exists", 42)

	tests := []struct {
		name         string
		key          string
		defaultValue int
		want         int
	}{
		{"existing key", "exists", 999, 42},
		{"non-existing key", "missing", 999, 999},
		{"zero default", "missing", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.GetOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("GetOrDefault() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinkedMap_PutIfAbsent(t *testing.T) {
	m := NewLinkedMap[string, int]()

	t.Run("insert into empty map", func(t *testing.T) {
		val, inserted := m.PutIfAbsent("new", 100)
		if !inserted {
			t.Error("expected key to be inserted")
		}
		if val != 100 {
			t.Errorf("PutIfAbsent() = %v, want 100", val)
		}
		if m.Size() != 1 {
			t.Errorf("Size() = %v, want 1", m.Size())
		}
	})

	t.Run("key already exists", func(t *testing.T) {
		val, inserted := m.PutIfAbsent("new", 200)
		if inserted {
			t.Error("expected key not to be inserted")
		}
		if val != 100 {
			t.Errorf("PutIfAbsent() = %v, want 100", val)
		}
		if m.Size() != 1 {
			t.Errorf("Size() = %v, want 1", m.Size())
		}
	})

	t.Run("maintains insertion order", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.PutIfAbsent("b", 2)
		m.Put("c", 3)

		keys := m.Keys()
		expected := []string{"a", "b", "c"}
		for i, key := range keys {
			if key != expected[i] {
				t.Errorf("Keys()[%d] = %v, want %v", i, key, expected[i])
			}
		}
	})
}

func TestLinkedMap_RemoveIf(t *testing.T) {
	t.Run("matching value", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		removed := m.RemoveIf("key", 100)
		if !removed {
			t.Error("expected key to be removed")
		}
		if m.ContainsKey("key") {
			t.Error("key should not exist after RemoveIf")
		}
	})

	t.Run("non-matching value", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		removed := m.RemoveIf("key", 200)
		if removed {
			t.Error("expected key not to be removed")
		}
		if !m.ContainsKey("key") {
			t.Error("key should still exist")
		}
	})

	t.Run("maintains order after removal", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		m.RemoveIf("b", 2)

		keys := m.Keys()
		if len(keys) != 2 || keys[0] != "a" || keys[1] != "c" {
			t.Errorf("Keys() = %v, want [a c]", keys)
		}
	})
}

func TestLinkedMap_Replace(t *testing.T) {
	t.Run("existing key", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		oldVal, replaced := m.Replace("key", 200)
		if !replaced {
			t.Error("expected key to be replaced")
		}
		if oldVal != 100 {
			t.Errorf("Replace() old value = %v, want 100", oldVal)
		}
		if val, _ := m.Get("key"); val != 200 {
			t.Errorf("Get() = %v, want 200", val)
		}
	})

	t.Run("non-existing key", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		oldVal, replaced := m.Replace("missing", 100)
		if replaced {
			t.Error("expected key not to be replaced")
		}
		if oldVal != 0 {
			t.Errorf("Replace() = %v, want 0", oldVal)
		}
		if m.ContainsKey("missing") {
			t.Error("key should not be added by Replace")
		}
	})

	t.Run("preserves order", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		m.Replace("b", 200)

		keys := m.Keys()
		expected := []string{"a", "b", "c"}
		for i, key := range keys {
			if key != expected[i] {
				t.Errorf("order changed after Replace")
			}
		}
	})
}

func TestLinkedMap_ReplaceIf(t *testing.T) {
	t.Run("matching old value", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		replaced := m.ReplaceIf("key", 100, 200)
		if !replaced {
			t.Error("expected key to be replaced")
		}
		if val, _ := m.Get("key"); val != 200 {
			t.Errorf("Get() = %v, want 200", val)
		}
	})

	t.Run("non-matching old value", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		replaced := m.ReplaceIf("key", 999, 200)
		if replaced {
			t.Error("expected key not to be replaced")
		}
		if val, _ := m.Get("key"); val != 100 {
			t.Errorf("Get() = %v, want 100", val)
		}
	})

	t.Run("non-existing key", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		replaced := m.ReplaceIf("missing", 100, 200)
		if replaced {
			t.Error("expected false for non-existing key")
		}
	})
}

// =============================================================================
// Compute Operations Tests
// =============================================================================

func TestLinkedMap_Compute(t *testing.T) {
	t.Run("compute for existing key", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		val, ok := m.Compute("key", func(k string, v int, exists bool) (int, bool) {
			if !exists {
				t.Error("expected key to exist")
			}
			if v != 100 {
				t.Errorf("existing value = %v, want 100", v)
			}
			return v + 50, true
		})

		if !ok {
			t.Error("expected compute to succeed")
		}
		if val != 150 {
			t.Errorf("Compute() = %v, want 150", val)
		}
	})

	t.Run("compute for non-existing key with insert", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		val, ok := m.Compute("new", func(k string, v int, exists bool) (int, bool) {
			if exists {
				t.Error("expected key not to exist")
			}
			return 42, true
		})

		if !ok {
			t.Error("expected compute to succeed")
		}
		if val != 42 {
			t.Errorf("Compute() = %v, want 42", val)
		}
		if !m.ContainsKey("new") {
			t.Error("key should exist after compute")
		}
	})

	t.Run("compute with removal", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		val, ok := m.Compute("key", func(k string, v int, exists bool) (int, bool) {
			return 0, false
		})

		if ok {
			t.Error("expected compute to return false")
		}
		if val != 0 {
			t.Errorf("Compute() = %v, want 0", val)
		}
		if m.ContainsKey("key") {
			t.Error("key should be removed")
		}
	})

	t.Run("preserves order", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		m.Compute("b", func(k string, v int, exists bool) (int, bool) {
			return v * 10, true
		})

		keys := m.Keys()
		expected := []string{"a", "b", "c"}
		for i, key := range keys {
			if key != expected[i] {
				t.Error("order changed after Compute")
			}
		}
	})
}

func TestLinkedMap_ComputeIfAbsent(t *testing.T) {
	t.Run("compute for non-existing key", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		val := m.ComputeIfAbsent("new", func(k string) int {
			return 42
		})

		if val != 42 {
			t.Errorf("ComputeIfAbsent() = %v, want 42", val)
		}
		if got, _ := m.Get("new"); got != 42 {
			t.Errorf("Get() = %v, want 42", got)
		}
	})

	t.Run("existing key not recomputed", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		callCount := 0
		val := m.ComputeIfAbsent("key", func(k string) int {
			callCount++
			return 999
		})

		if val != 100 {
			t.Errorf("ComputeIfAbsent() = %v, want 100", val)
		}
		if callCount != 0 {
			t.Error("mapping function should not be called for existing key")
		}
	})

	t.Run("maintains order", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.ComputeIfAbsent("b", func(k string) int { return 2 })
		m.Put("c", 3)

		keys := m.Keys()
		expected := []string{"a", "b", "c"}
		for i, key := range keys {
			if key != expected[i] {
				t.Errorf("Keys()[%d] = %v, want %v", i, key, expected[i])
			}
		}
	})
}

func TestLinkedMap_ComputeIfPresent(t *testing.T) {
	t.Run("compute for existing key", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		val, ok := m.ComputeIfPresent("key", func(k string, v int) int {
			return v * 2
		})

		if !ok {
			t.Error("expected compute to succeed")
		}
		if val != 200 {
			t.Errorf("ComputeIfPresent() = %v, want 200", val)
		}
	})

	t.Run("non-existing key not computed", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		callCount := 0
		val, ok := m.ComputeIfPresent("missing", func(k string, v int) int {
			callCount++
			return 999
		})

		if ok {
			t.Error("expected compute to return false")
		}
		if val != 0 {
			t.Errorf("ComputeIfPresent() = %v, want 0", val)
		}
		if callCount != 0 {
			t.Error("remapping function should not be called for missing key")
		}
	})
}

func TestLinkedMap_Merge(t *testing.T) {
	t.Run("merge into non-existing key", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		val := m.Merge("new", 100, func(old, new int) int {
			t.Error("remapping function should not be called")
			return old + new
		})

		if val != 100 {
			t.Errorf("Merge() = %v, want 100", val)
		}
	})

	t.Run("merge into existing key", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("key", 100)

		val := m.Merge("key", 50, func(old, new int) int {
			return old + new
		})

		if val != 150 {
			t.Errorf("Merge() = %v, want 150", val)
		}
	})

	t.Run("maintains order", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Merge("b", 2, func(old, new int) int { return old + new })
		m.Put("c", 3)

		keys := m.Keys()
		expected := []string{"a", "b", "c"}
		for i, key := range keys {
			if key != expected[i] {
				t.Errorf("Keys()[%d] = %v, want %v", i, key, expected[i])
			}
		}
	})
}

func TestLinkedMap_ReplaceAll(t *testing.T) {
	m := NewLinkedMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	m.ReplaceAll(func(k string, v int) int {
		return v * 10
	})

	if val, _ := m.Get("a"); val != 10 {
		t.Errorf("Get(a) = %v, want 10", val)
	}
	if val, _ := m.Get("b"); val != 20 {
		t.Errorf("Get(b) = %v, want 20", val)
	}
	if val, _ := m.Get("c"); val != 30 {
		t.Errorf("Get(c) = %v, want 30", val)
	}

	keys := m.Keys()
	expected := []string{"a", "b", "c"}
	for i, key := range keys {
		if key != expected[i] {
			t.Error("order should be preserved after ReplaceAll")
		}
	}
}

// =============================================================================
// Iterator Tests
// =============================================================================

func TestLinkedMap_Iter(t *testing.T) {
	m := NewLinkedMap[string, int]()
	order := []string{"first", "second", "third"}
	for i, key := range order {
		m.Put(key, i+1)
	}

	t.Run("iterate all elements in order", func(t *testing.T) {
		idx := 0
		for k, v := range m.Iter() {
			if k != order[idx] {
				t.Errorf("key = %v, want %v", k, order[idx])
			}
			if v != idx+1 {
				t.Errorf("value = %v, want %v", v, idx+1)
			}
			idx++
		}

		if idx != 3 {
			t.Errorf("iterated %v elements, want 3", idx)
		}
	})

	t.Run("early break", func(t *testing.T) {
		count := 0
		for range m.Iter() {
			count++
			if count == 2 {
				break
			}
		}

		if count != 2 {
			t.Errorf("count = %v, want 2", count)
		}
	})

	t.Run("empty map", func(t *testing.T) {
		empty := NewLinkedMap[string, int]()
		count := 0
		for range empty.Iter() {
			count++
		}

		if count != 0 {
			t.Errorf("count = %v, want 0", count)
		}
	})
}

func TestLinkedMap_IterKeys(t *testing.T) {
	m := NewLinkedMap[string, int]()
	order := []string{"first", "second", "third"}
	for i, key := range order {
		m.Put(key, i+1)
	}

	idx := 0
	for k := range m.IterKeys() {
		if k != order[idx] {
			t.Errorf("key = %v, want %v", k, order[idx])
		}
		idx++
	}

	if idx != 3 {
		t.Errorf("iterated %v keys, want 3", idx)
	}
}

func TestLinkedMap_IterValues(t *testing.T) {
	m := NewLinkedMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	expected := []int{1, 2, 3}
	idx := 0
	for v := range m.IterValues() {
		if v != expected[idx] {
			t.Errorf("value = %v, want %v", v, expected[idx])
		}
		idx++
	}

	if idx != 3 {
		t.Errorf("iterated %v values, want 3", idx)
	}
}

// =============================================================================
// Order-Specific Methods Tests
// =============================================================================

func TestLinkedMap_First(t *testing.T) {
	t.Run("non-empty map", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("first", 1)
		m.Put("second", 2)

		k, v, ok := m.First()
		if !ok {
			t.Error("expected First to succeed")
		}
		if k != "first" {
			t.Errorf("First() key = %v, want 'first'", k)
		}
		if v != 1 {
			t.Errorf("First() value = %v, want 1", v)
		}
	})

	t.Run("empty map", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		k, v, ok := m.First()
		if ok {
			t.Error("expected First to return false for empty map")
		}
		if k != "" || v != 0 {
			t.Errorf("First() = (%v, %v), want zero values", k, v)
		}
	})

	t.Run("single element", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("only", 42)

		k, v, ok := m.First()
		if !ok {
			t.Error("expected First to succeed")
		}
		if k != "only" || v != 42 {
			t.Errorf("First() = (%v, %v), want (only, 42)", k, v)
		}
	})
}

func TestLinkedMap_Last(t *testing.T) {
	t.Run("non-empty map", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("first", 1)
		m.Put("last", 2)

		k, v, ok := m.Last()
		if !ok {
			t.Error("expected Last to succeed")
		}
		if k != "last" {
			t.Errorf("Last() key = %v, want 'last'", k)
		}
		if v != 2 {
			t.Errorf("Last() value = %v, want 2", v)
		}
	})

	t.Run("empty map", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		k, v, ok := m.Last()
		if ok {
			t.Error("expected Last to return false for empty map")
		}
		if k != "" || v != 0 {
			t.Errorf("Last() = (%v, %v), want zero values", k, v)
		}
	})

	t.Run("single element", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("only", 42)

		k, v, ok := m.Last()
		if !ok {
			t.Error("expected Last to succeed")
		}
		if k != "only" || v != 42 {
			t.Errorf("Last() = (%v, %v), want (only, 42)", k, v)
		}
	})
}

func TestLinkedMap_RemoveFirst(t *testing.T) {
	t.Run("remove from non-empty map", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("first", 1)
		m.Put("second", 2)
		m.Put("third", 3)

		k, v, ok := m.RemoveFirst()
		if !ok {
			t.Error("expected RemoveFirst to succeed")
		}
		if k != "first" || v != 1 {
			t.Errorf("RemoveFirst() = (%v, %v), want (first, 1)", k, v)
		}
		if m.Size() != 2 {
			t.Errorf("Size() = %v, want 2", m.Size())
		}

		newFirst, _, _ := m.First()
		if newFirst != "second" {
			t.Errorf("new first key = %v, want 'second'", newFirst)
		}
	})

	t.Run("remove from empty map", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		k, v, ok := m.RemoveFirst()
		if ok {
			t.Error("expected RemoveFirst to return false for empty map")
		}
		if k != "" || v != 0 {
			t.Errorf("RemoveFirst() = (%v, %v), want zero values", k, v)
		}
	})

	t.Run("remove last element", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("only", 42)

		k, v, ok := m.RemoveFirst()
		if !ok {
			t.Error("expected RemoveFirst to succeed")
		}
		if k != "only" || v != 42 {
			t.Errorf("RemoveFirst() = (%v, %v), want (only, 42)", k, v)
		}
		if !m.IsEmpty() {
			t.Error("map should be empty")
		}
		if m.head != nil || m.tail != nil {
			t.Error("head and tail should be nil")
		}
	})
}

func TestLinkedMap_RemoveLast(t *testing.T) {
	t.Run("remove from non-empty map", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("first", 1)
		m.Put("second", 2)
		m.Put("last", 3)

		k, v, ok := m.RemoveLast()
		if !ok {
			t.Error("expected RemoveLast to succeed")
		}
		if k != "last" || v != 3 {
			t.Errorf("RemoveLast() = (%v, %v), want (last, 3)", k, v)
		}
		if m.Size() != 2 {
			t.Errorf("Size() = %v, want 2", m.Size())
		}

		newLast, _, _ := m.Last()
		if newLast != "second" {
			t.Errorf("new last key = %v, want 'second'", newLast)
		}
	})

	t.Run("remove from empty map", func(t *testing.T) {
		m := NewLinkedMap[string, int]()

		k, v, ok := m.RemoveLast()
		if ok {
			t.Error("expected RemoveLast to return false for empty map")
		}
		if k != "" || v != 0 {
			t.Errorf("RemoveLast() = (%v, %v), want zero values", k, v)
		}
	})

	t.Run("remove last element", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("only", 42)

		k, v, ok := m.RemoveLast()
		if !ok {
			t.Error("expected RemoveLast to succeed")
		}
		if k != "only" || v != 42 {
			t.Errorf("RemoveLast() = (%v, %v), want (only, 42)", k, v)
		}
		if !m.IsEmpty() {
			t.Error("map should be empty")
		}
		if m.head != nil || m.tail != nil {
			t.Error("head and tail should be nil")
		}
	})
}

// =============================================================================
// Clone Tests
// =============================================================================

func TestLinkedMap_Clone(t *testing.T) {
	t.Run("clone empty map", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		cloned := m.Clone()

		if cloned.Size() != 0 {
			t.Errorf("cloned Size() = %v, want 0", cloned.Size())
		}
	})

	t.Run("clone with elements preserves order", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		order := []string{"first", "second", "third"}
		for i, key := range order {
			m.Put(key, i+1)
		}

		cloned := m.Clone()

		if cloned.Size() != 3 {
			t.Errorf("cloned Size() = %v, want 3", cloned.Size())
		}

		keys := cloned.(*LinkedMap[string, int]).Keys()
		for i, key := range keys {
			if key != order[i] {
				t.Errorf("cloned order differs at index %d: got %v, want %v", i, key, order[i])
			}
		}
	})

	t.Run("clone independence", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)

		cloned := m.Clone()
		cloned.Put("b", 2)
		m.Put("c", 3)

		if m.ContainsKey("b") {
			t.Error("original should not contain key from clone")
		}
		if cloned.ContainsKey("c") {
			t.Error("clone should not contain key from original")
		}
	})

	t.Run("shallow copy of slice values", func(t *testing.T) {
		m := NewLinkedMap[string, []int]()
		slice := []int{1, 2, 3}
		m.Put("key", slice)

		cloned := m.Clone()

		slice[0] = 999

		originalSlice, _ := m.Get("key")
		clonedSlice, _ := cloned.Get("key")

		if originalSlice[0] != 999 {
			t.Error("original should see modified slice")
		}
		if clonedSlice[0] != 999 {
			t.Error("clone should see modified slice (shallow copy)")
		}
	})
}

// =============================================================================
// Edge Cases and Special Scenarios Tests
// =============================================================================

func TestLinkedMap_NilValues(t *testing.T) {
	t.Run("nil pointer values", func(t *testing.T) {
		m := NewLinkedMap[string, *int]()
		m.Put("nil", nil)

		val, exists := m.Get("nil")
		if !exists {
			t.Error("key should exist")
		}
		if val != nil {
			t.Errorf("Get() = %v, want nil", val)
		}

		if !m.ContainsKey("nil") {
			t.Error("ContainsKey should return true for nil value")
		}
	})

	t.Run("nil slice values", func(t *testing.T) {
		m := NewLinkedMap[string, []int]()
		m.Put("nil", nil)

		val, exists := m.Get("nil")
		if !exists {
			t.Error("key should exist")
		}
		if val != nil {
			t.Errorf("Get() = %v, want nil", val)
		}
	})
}

func TestLinkedMap_ZeroValues(t *testing.T) {
	t.Run("zero int value", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("zero", 0)

		val, exists := m.Get("zero")
		if !exists {
			t.Error("key should exist")
		}
		if val != 0 {
			t.Errorf("Get() = %v, want 0", val)
		}
	})

	t.Run("empty string value", func(t *testing.T) {
		m := NewLinkedMap[string, string]()
		m.Put("empty", "")

		val, exists := m.Get("empty")
		if !exists {
			t.Error("key should exist")
		}
		if val != "" {
			t.Errorf("Get() = %v, want empty string", val)
		}
	})
}

func TestLinkedMap_LargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large dataset test in short mode")
	}

	m := NewLinkedMap[int, int](10000)

	t.Run("insert large dataset maintains order", func(t *testing.T) {
		for i := 0; i < 10000; i++ {
			m.Put(i, i*10)
		}

		if m.Size() != 10000 {
			t.Errorf("Size() = %v, want 10000", m.Size())
		}
	})

	t.Run("verify order", func(t *testing.T) {
		idx := 0
		for k, v := range m.Iter() {
			if k != idx {
				t.Errorf("key = %v, want %v", k, idx)
			}
			if v != idx*10 {
				t.Errorf("value = %v, want %v", v, idx*10)
			}
			idx++
			if idx > 100 {
				break
			}
		}
	})

	t.Run("remove elements maintains order", func(t *testing.T) {
		for i := 0; i < 5000; i++ {
			m.Remove(i)
		}

		if m.Size() != 5000 {
			t.Errorf("Size() = %v, want 5000", m.Size())
		}

		keys := m.Keys()
		for i := 0; i < len(keys)-1; i++ {
			if keys[i] >= keys[i+1] {
				t.Error("order not maintained after removals")
				break
			}
		}
	})
}

func TestLinkedMap_ComplexKeyTypes(t *testing.T) {
	t.Run("struct keys", func(t *testing.T) {
		type Key struct {
			ID   int
			Name string
		}
		m := NewLinkedMap[Key, string]()

		k1 := Key{1, "alice"}
		k2 := Key{2, "bob"}

		m.Put(k1, "value1")
		m.Put(k2, "value2")

		if val, _ := m.Get(k1); val != "value1" {
			t.Errorf("Get(k1) = %v, want 'value1'", val)
		}
		if val, _ := m.Get(k2); val != "value2" {
			t.Errorf("Get(k2) = %v, want 'value2'", val)
		}
	})

	t.Run("array keys", func(t *testing.T) {
		m := NewLinkedMap[[3]int, string]()

		k1 := [3]int{1, 2, 3}
		k2 := [3]int{4, 5, 6}

		m.Put(k1, "array1")
		m.Put(k2, "array2")

		if val, _ := m.Get(k1); val != "array1" {
			t.Errorf("Get(k1) = %v, want 'array1'", val)
		}
	})
}

func TestLinkedMap_LRUCachePattern(t *testing.T) {
	t.Run("LRU cache simulation", func(t *testing.T) {
		const maxSize = 3
		m := NewLinkedMap[string, int]()

		put := func(key string, value int) {
			if m.Size() >= maxSize && !m.ContainsKey(key) {
				m.RemoveFirst()
			}
			m.Put(key, value)
		}

		put("a", 1)
		put("b", 2)
		put("c", 3)
		put("d", 4)

		if m.ContainsKey("a") {
			t.Error("oldest entry 'a' should have been evicted")
		}
		if !m.ContainsKey("d") {
			t.Error("newest entry 'd' should exist")
		}
		if m.Size() != 3 {
			t.Errorf("Size() = %v, want 3", m.Size())
		}
	})
}

func TestLinkedMap_QueuePattern(t *testing.T) {
	t.Run("FIFO queue simulation", func(t *testing.T) {
		m := NewLinkedMap[int, string]()

		for i := 0; i < 5; i++ {
			m.Put(i, fmt.Sprintf("item%d", i))
		}

		for i := 0; i < 3; i++ {
			k, v, ok := m.RemoveFirst()
			if !ok {
				t.Error("RemoveFirst should succeed")
			}
			if k != i {
				t.Errorf("dequeued key = %v, want %v", k, i)
			}
			if v != fmt.Sprintf("item%d", i) {
				t.Errorf("dequeued value = %v, want item%d", v, i)
			}
		}

		if m.Size() != 2 {
			t.Errorf("Size() = %v, want 2", m.Size())
		}
	})
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestLinkedMap_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent access test in short mode")
	}

	t.Run("concurrent reads are safe", func(t *testing.T) {
		m := NewLinkedMap[int, int]()
		for i := 0; i < 100; i++ {
			m.Put(i, i*10)
		}

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					m.Get(j)
				}
			}()
		}
		wg.Wait()
	})

	t.Run("concurrent writes require external synchronization", func(t *testing.T) {
		m := NewLinkedMap[int, int]()
		var mu sync.Mutex

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					mu.Lock()
					m.Put(id*100+j, j)
					mu.Unlock()
				}
			}(i)
		}
		wg.Wait()

		if m.Size() != 1000 {
			t.Errorf("Size() = %v, want 1000", m.Size())
		}
	})
}

// =============================================================================
// Internal Structure Tests
// =============================================================================

func TestLinkedMap_InternalConsistency(t *testing.T) {
	t.Run("verify bidirectional links", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		current := m.head
		for current != nil {
			if current.next != nil && current.next.prev != current {
				t.Error("forward and backward links are inconsistent")
			}
			current = current.next
		}
	})

	t.Run("head has no prev", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)

		if m.head.prev != nil {
			t.Error("head.prev should be nil")
		}
	})

	t.Run("tail has no next", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)

		if m.tail.next != nil {
			t.Error("tail.next should be nil")
		}
	})

	t.Run("nodes map and linked list are consistent", func(t *testing.T) {
		m := NewLinkedMap[string, int]()
		for i := 0; i < 10; i++ {
			m.Put(strconv.Itoa(i), i)
		}

		linkedListSize := 0
		current := m.head
		for current != nil {
			linkedListSize++
			if _, exists := m.nodes[current.key]; !exists {
				t.Errorf("node with key %v in linked list but not in map", current.key)
			}
			current = current.next
		}

		if linkedListSize != len(m.nodes) {
			t.Errorf("linked list size = %v, map size = %v", linkedListSize, len(m.nodes))
		}
	})
}

// =============================================================================
// Performance Benchmarks
// =============================================================================

func BenchmarkLinkedMap_Put(b *testing.B) {
	m := NewLinkedMap[int, int](b.N)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Put(i, i)
	}
}

func BenchmarkLinkedMap_Get(b *testing.B) {
	m := NewLinkedMap[int, int](b.N)
	for i := 0; i < b.N; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Get(i)
	}
}

func BenchmarkLinkedMap_Remove(b *testing.B) {
	m := NewLinkedMap[int, int](b.N)
	for i := 0; i < b.N; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Remove(i)
	}
}

func BenchmarkLinkedMap_Iter(b *testing.B) {
	m := NewLinkedMap[int, int]()
	for i := 0; i < 1000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for range m.Iter() {
		}
	}
}

func BenchmarkLinkedMap_First(b *testing.B) {
	m := NewLinkedMap[int, int]()
	for i := 0; i < 1000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.First()
	}
}
