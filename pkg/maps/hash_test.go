package maps

import (
	"fmt"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/pkg/ptr"
)

// =============================================================================
// Basic Functionality Tests
// =============================================================================

func TestHashMap_NewHashMap(t *testing.T) {
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
			m := NewHashMap[string, int](tt.capacity...)
			if m.Size() != tt.wantSize {
				t.Errorf("Size() = %v, want %v", m.Size(), tt.wantSize)
			}
			if m == nil {
				t.Error("NewHashMap returned nil")
			}
		})
	}
}

func TestHashMap_Put(t *testing.T) {
	m := NewHashMap[string, int]()

	t.Run("insert new key", func(t *testing.T) {
		oldVal, existed := m.Put("key1", 100)
		if existed {
			t.Error("expected key not to exist")
		}
		if oldVal != 0 {
			t.Errorf("expected zero value, got %v", oldVal)
		}
		if m.Size() != 1 {
			t.Errorf("Size() = %v, want 1", m.Size())
		}
	})

	t.Run("update existing key", func(t *testing.T) {
		oldVal, existed := m.Put("key1", 200)
		if !existed {
			t.Error("expected key to exist")
		}
		if oldVal != 100 {
			t.Errorf("expected old value 100, got %v", oldVal)
		}
		if m.Size() != 1 {
			t.Errorf("Size() = %v, want 1", m.Size())
		}
	})

	t.Run("multiple keys", func(t *testing.T) {
		m.Put("key2", 300)
		m.Put("key3", 400)
		if m.Size() != 3 {
			t.Errorf("Size() = %v, want 3", m.Size())
		}
	})
}

func TestHashMap_Get(t *testing.T) {
	m := NewHashMap[string, int]()
	m.Put("existing", 42)

	tests := []struct {
		name      string
		key       string
		wantValue int
		wantOk    bool
	}{
		{
			name:      "existing key",
			key:       "existing",
			wantValue: 42,
			wantOk:    true,
		},
		{
			name:      "non-existing key",
			key:       "missing",
			wantValue: 0,
			wantOk:    false,
		},
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

func TestHashMap_Remove(t *testing.T) {
	m := NewHashMap[string, int]()
	m.Put("key1", 100)
	m.Put("key2", 200)

	t.Run("remove existing key", func(t *testing.T) {
		val, existed := m.Remove("key1")
		if !existed {
			t.Error("expected key to exist")
		}
		if val != 100 {
			t.Errorf("Remove() = %v, want 100", val)
		}
		if m.Size() != 1 {
			t.Errorf("Size() = %v, want 1", m.Size())
		}
		if m.ContainsKey("key1") {
			t.Error("key should not exist after removal")
		}
	})

	t.Run("remove non-existing key", func(t *testing.T) {
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

func TestHashMap_ContainsKey(t *testing.T) {
	m := NewHashMap[string, int]()
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

func TestHashMap_ContainsValue(t *testing.T) {
	t.Run("primitive types", func(t *testing.T) {
		m := NewHashMap[string, int]()
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
		m := NewHashMap[string, []int]()
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
		m := NewHashMap[string, Person]()
		m.Put("alice", Person{"Alice", 30})
		m.Put("bob", Person{"Bob", 25})

		if !m.ContainsValue(Person{"Alice", 30}) {
			t.Error("expected to find Alice")
		}
		if m.ContainsValue(Person{"Charlie", 35}) {
			t.Error("expected not to find Charlie")
		}
	})

	t.Run("pointer values", func(t *testing.T) {
		m := NewHashMap[string, *string]()
		val1 := ptr.Pointer("abc")
		val2 := ptr.Pointer("cba")

		m.Put("a", val1)
		if !m.ContainsValue(val1) {
			t.Error("expected to find exact pointer")
		}
		if m.ContainsValue(val2) {
			t.Error("expected not to find different pointer with same value")
		}
	})
}

func TestHashMap_Size(t *testing.T) {
	m := NewHashMap[string, int]()

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

func TestHashMap_IsEmpty(t *testing.T) {
	m := NewHashMap[string, int]()

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

func TestHashMap_Clear(t *testing.T) {
	m := NewHashMap[string, int]()
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
	if m.ContainsKey("a") {
		t.Error("key 'a' should not exist after Clear()")
	}
}

// =============================================================================
// Bulk Operations Tests
// =============================================================================

func TestHashMap_PutAll(t *testing.T) {
	t.Run("from HashMap", func(t *testing.T) {
		m1 := NewHashMap[string, int]()
		m1.Put("a", 1)
		m1.Put("b", 2)

		m2 := NewHashMap[string, int]()
		m2.Put("c", 3)

		m2.PutAll(m1)

		if m2.Size() != 3 {
			t.Errorf("Size() = %v, want 3", m2.Size())
		}
		if val, _ := m2.Get("a"); val != 1 {
			t.Errorf("Get(a) = %v, want 1", val)
		}
		if val, _ := m2.Get("b"); val != 2 {
			t.Errorf("Get(b) = %v, want 2", val)
		}
	})

	t.Run("overwrite existing keys", func(t *testing.T) {
		m1 := NewHashMap[string, int]()
		m1.Put("a", 100)

		m2 := NewHashMap[string, int]()
		m2.Put("a", 1)

		m2.PutAll(m1)

		if val, _ := m2.Get("a"); val != 100 {
			t.Errorf("Get(a) = %v, want 100", val)
		}
	})

	t.Run("empty source map", func(t *testing.T) {
		m1 := NewHashMap[string, int]()
		m2 := NewHashMap[string, int]()
		m2.Put("a", 1)

		m2.PutAll(m1)

		if m2.Size() != 1 {
			t.Errorf("Size() = %v, want 1", m2.Size())
		}
	})
}

func TestHashMap_Keys(t *testing.T) {
	m := NewHashMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	keys := m.Keys()

	if len(keys) != 3 {
		t.Errorf("len(Keys()) = %v, want 3", len(keys))
	}

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}

	if !keySet["a"] || !keySet["b"] || !keySet["c"] {
		t.Errorf("Keys() = %v, missing expected keys", keys)
	}

	m.Put("d", 4)
	if len(keys) != 3 {
		t.Error("Keys() returned slice should be a snapshot")
	}
}

func TestHashMap_Values(t *testing.T) {
	m := NewHashMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	values := m.Values()

	if len(values) != 3 {
		t.Errorf("len(Values()) = %v, want 3", len(values))
	}

	valueSet := make(map[int]bool)
	for _, v := range values {
		valueSet[v] = true
	}

	if !valueSet[1] || !valueSet[2] || !valueSet[3] {
		t.Errorf("Values() = %v, missing expected values", values)
	}
}

func TestHashMap_Entries(t *testing.T) {
	m := NewHashMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)

	entries := m.Entries()

	if len(entries) != 2 {
		t.Errorf("len(Entries()) = %v, want 2", len(entries))
	}

	found := make(map[string]int)
	for _, entry := range entries {
		found[entry.Key()] = entry.Value()
	}

	if found["a"] != 1 || found["b"] != 2 {
		t.Errorf("Entries() = %v, unexpected values", entries)
	}

	if entries[0] == nil {
		t.Error("Entry should not be nil")
	}
}

func TestHashMap_ForEach(t *testing.T) {
	m := NewHashMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	sum := 0
	count := 0
	m.ForEach(func(k string, v int) {
		sum += v
		count++
	})

	if sum != 6 {
		t.Errorf("sum = %v, want 6", sum)
	}
	if count != 3 {
		t.Errorf("count = %v, want 3", count)
	}
}

// =============================================================================
// Conditional Operations Tests
// =============================================================================

func TestHashMap_GetOrDefault(t *testing.T) {
	m := NewHashMap[string, int]()
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

func TestHashMap_PutIfAbsent(t *testing.T) {
	m := NewHashMap[string, int]()

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
}

func TestHashMap_RemoveIf(t *testing.T) {
	t.Run("matching value", func(t *testing.T) {
		m := NewHashMap[string, int]()
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
		m := NewHashMap[string, int]()
		m.Put("key", 100)

		removed := m.RemoveIf("key", 200)
		if removed {
			t.Error("expected key not to be removed")
		}
		if !m.ContainsKey("key") {
			t.Error("key should still exist")
		}
	})

	t.Run("non-existing key", func(t *testing.T) {
		m := NewHashMap[string, int]()

		removed := m.RemoveIf("missing", 100)
		if removed {
			t.Error("expected false for non-existing key")
		}
	})

	t.Run("slice values", func(t *testing.T) {
		m := NewHashMap[string, []int]()
		m.Put("key", []int{1, 2, 3})

		removed := m.RemoveIf("key", []int{1, 2, 3})
		if !removed {
			t.Error("expected slice to be removed")
		}
	})
}

func TestHashMap_Replace(t *testing.T) {
	t.Run("existing key", func(t *testing.T) {
		m := NewHashMap[string, int]()
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
		m := NewHashMap[string, int]()

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
}

func TestHashMap_ReplaceIf(t *testing.T) {
	t.Run("matching old value", func(t *testing.T) {
		m := NewHashMap[string, int]()
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
		m := NewHashMap[string, int]()
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
		m := NewHashMap[string, int]()

		replaced := m.ReplaceIf("missing", 100, 200)
		if replaced {
			t.Error("expected false for non-existing key")
		}
	})
}

// =============================================================================
// Compute Operations Tests
// =============================================================================

func TestHashMap_Compute(t *testing.T) {
	t.Run("compute for existing key", func(t *testing.T) {
		m := NewHashMap[string, int]()
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
		if got, _ := m.Get("key"); got != 150 {
			t.Errorf("Get() = %v, want 150", got)
		}
	})

	t.Run("compute for non-existing key with insert", func(t *testing.T) {
		m := NewHashMap[string, int]()

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
		m := NewHashMap[string, int]()
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

	t.Run("compute with no-op for non-existing", func(t *testing.T) {
		m := NewHashMap[string, int]()

		val, ok := m.Compute("missing", func(k string, v int, exists bool) (int, bool) {
			return 0, false
		})

		if ok {
			t.Error("expected compute to return false")
		}
		if val != 0 {
			t.Errorf("Compute() = %v, want 0", val)
		}
		if m.ContainsKey("missing") {
			t.Error("key should not be added")
		}
	})
}

func TestHashMap_ComputeIfAbsent(t *testing.T) {
	t.Run("compute for non-existing key", func(t *testing.T) {
		m := NewHashMap[string, int]()

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
		m := NewHashMap[string, int]()
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
		if got, _ := m.Get("key"); got != 100 {
			t.Errorf("Get() = %v, want 100", got)
		}
	})

	t.Run("compute zero value", func(t *testing.T) {
		m := NewHashMap[string, int]()

		val := m.ComputeIfAbsent("zero", func(k string) int {
			return 0
		})

		if val != 0 {
			t.Errorf("ComputeIfAbsent() = %v, want 0", val)
		}
		if !m.ContainsKey("zero") {
			t.Error("key should exist even with zero value")
		}
	})
}

func TestHashMap_ComputeIfPresent(t *testing.T) {
	t.Run("compute for existing key", func(t *testing.T) {
		m := NewHashMap[string, int]()
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
		if got, _ := m.Get("key"); got != 200 {
			t.Errorf("Get() = %v, want 200", got)
		}
	})

	t.Run("non-existing key not computed", func(t *testing.T) {
		m := NewHashMap[string, int]()

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

func TestHashMap_Merge(t *testing.T) {
	t.Run("merge into non-existing key", func(t *testing.T) {
		m := NewHashMap[string, int]()

		val := m.Merge("new", 100, func(old, new int) int {
			t.Error("remapping function should not be called")
			return old + new
		})

		if val != 100 {
			t.Errorf("Merge() = %v, want 100", val)
		}
		if got, _ := m.Get("new"); got != 100 {
			t.Errorf("Get() = %v, want 100", got)
		}
	})

	t.Run("merge into existing key", func(t *testing.T) {
		m := NewHashMap[string, int]()
		m.Put("key", 100)

		val := m.Merge("key", 50, func(old, new int) int {
			return old + new
		})

		if val != 150 {
			t.Errorf("Merge() = %v, want 150", val)
		}
		if got, _ := m.Get("key"); got != 150 {
			t.Errorf("Get() = %v, want 150", got)
		}
	})

	t.Run("merge with concatenation", func(t *testing.T) {
		m := NewHashMap[string, string]()
		m.Put("key", "Hello")

		val := m.Merge("key", " World", func(old, new string) string {
			return old + new
		})

		if val != "Hello World" {
			t.Errorf("Merge() = %v, want 'Hello World'", val)
		}
	})
}

func TestHashMap_ReplaceAll(t *testing.T) {
	m := NewHashMap[string, int]()
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
}

// =============================================================================
// Iterator Tests
// =============================================================================

func TestHashMap_Iter(t *testing.T) {
	m := NewHashMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	t.Run("iterate all elements", func(t *testing.T) {
		found := make(map[string]int)
		for k, v := range m.Iter() {
			found[k] = v
		}

		if len(found) != 3 {
			t.Errorf("iterated %v elements, want 3", len(found))
		}
		if found["a"] != 1 || found["b"] != 2 || found["c"] != 3 {
			t.Errorf("Iter() found = %v, unexpected values", found)
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
		empty := NewHashMap[string, int]()
		count := 0
		for range empty.Iter() {
			count++
		}

		if count != 0 {
			t.Errorf("count = %v, want 0", count)
		}
	})
}

func TestHashMap_IterKeys(t *testing.T) {
	m := NewHashMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	keys := make(map[string]bool)
	for k := range m.IterKeys() {
		keys[k] = true
	}

	if len(keys) != 3 {
		t.Errorf("iterated %v keys, want 3", len(keys))
	}
	if !keys["a"] || !keys["b"] || !keys["c"] {
		t.Errorf("IterKeys() keys = %v, missing expected keys", keys)
	}
}

func TestHashMap_IterValues(t *testing.T) {
	m := NewHashMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)
	m.Put("c", 3)

	values := make(map[int]bool)
	for v := range m.IterValues() {
		values[v] = true
	}

	if len(values) != 3 {
		t.Errorf("iterated %v values, want 3", len(values))
	}
	if !values[1] || !values[2] || !values[3] {
		t.Errorf("IterValues() values = %v, missing expected values", values)
	}
}

// =============================================================================
// Clone Tests
// =============================================================================

func TestHashMap_Clone(t *testing.T) {
	t.Run("clone empty map", func(t *testing.T) {
		m := NewHashMap[string, int]()
		cloned := m.Clone()

		if cloned.Size() != 0 {
			t.Errorf("cloned Size() = %v, want 0", cloned.Size())
		}
	})

	t.Run("clone with elements", func(t *testing.T) {
		m := NewHashMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		cloned := m.Clone()

		if cloned.Size() != 3 {
			t.Errorf("cloned Size() = %v, want 3", cloned.Size())
		}

		for k, v := range m.Iter() {
			if cv, ok := cloned.Get(k); !ok || cv != v {
				t.Errorf("cloned.Get(%v) = %v, want %v", k, cv, v)
			}
		}
	})

	t.Run("clone independence", func(t *testing.T) {
		m := NewHashMap[string, int]()
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
		m := NewHashMap[string, []int]()
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

	t.Run("shallow copy of pointer values", func(t *testing.T) {
		type Data struct{ value int }
		m := NewHashMap[string, *Data]()
		data := &Data{value: 42}
		m.Put("key", data)

		cloned := m.Clone()

		data.value = 100

		originalData, _ := m.Get("key")
		clonedData, _ := cloned.Get("key")

		if originalData.value != 100 {
			t.Error("original should see modified data")
		}
		if clonedData.value != 100 {
			t.Error("clone should see modified data (shallow copy)")
		}
	})
}

// =============================================================================
// Edge Cases and Special Scenarios Tests
// =============================================================================

func TestHashMap_NilValues(t *testing.T) {
	t.Run("nil pointer values", func(t *testing.T) {
		m := NewHashMap[string, *int]()
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
		m := NewHashMap[string, []int]()
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

func TestHashMap_ZeroValues(t *testing.T) {
	t.Run("zero int value", func(t *testing.T) {
		m := NewHashMap[string, int]()
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
		m := NewHashMap[string, string]()
		m.Put("empty", "")

		val, exists := m.Get("empty")
		if !exists {
			t.Error("key should exist")
		}
		if val != "" {
			t.Errorf("Get() = %v, want empty string", val)
		}
	})

	t.Run("zero struct value", func(t *testing.T) {
		type Empty struct{}
		m := NewHashMap[string, Empty]()
		m.Put("zero", Empty{})

		val, exists := m.Get("zero")
		if !exists {
			t.Error("key should exist")
		}
		if val != (Empty{}) {
			t.Errorf("Get() = %v, want Empty{}", val)
		}
	})
}

func TestHashMap_LargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping large dataset test in short mode")
	}

	m := NewHashMap[int, int](10000)

	t.Run("insert large dataset", func(t *testing.T) {
		for i := 0; i < 10000; i++ {
			m.Put(i, i*10)
		}

		if m.Size() != 10000 {
			t.Errorf("Size() = %v, want 10000", m.Size())
		}
	})

	t.Run("verify all elements", func(t *testing.T) {
		for i := 0; i < 10000; i++ {
			val, exists := m.Get(i)
			if !exists {
				t.Errorf("key %v should exist", i)
			}
			if val != i*10 {
				t.Errorf("Get(%v) = %v, want %v", i, val, i*10)
			}
		}
	})

	t.Run("remove half elements", func(t *testing.T) {
		for i := 0; i < 5000; i++ {
			m.Remove(i)
		}

		if m.Size() != 5000 {
			t.Errorf("Size() = %v, want 5000", m.Size())
		}
	})
}

func TestHashMap_ComplexKeyTypes(t *testing.T) {
	t.Run("struct keys", func(t *testing.T) {
		type Key struct {
			ID   int
			Name string
		}
		m := NewHashMap[Key, string]()

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
		m := NewHashMap[[3]int, string]()

		k1 := [3]int{1, 2, 3}
		k2 := [3]int{4, 5, 6}

		m.Put(k1, "array1")
		m.Put(k2, "array2")

		if val, _ := m.Get(k1); val != "array1" {
			t.Errorf("Get(k1) = %v, want 'array1'", val)
		}
	})
}

func TestHashMap_ComplexValueTypes(t *testing.T) {
	t.Run("map values", func(t *testing.T) {
		m := NewHashMap[string, map[string]int]()

		inner := map[string]int{"a": 1, "b": 2}
		m.Put("nested", inner)

		val, exists := m.Get("nested")
		if !exists {
			t.Error("key should exist")
		}
		if val["a"] != 1 || val["b"] != 2 {
			t.Errorf("Get() = %v, unexpected values", val)
		}
	})

	t.Run("function values", func(t *testing.T) {
		m := NewHashMap[string, func() int]()

		m.Put("fn", func() int { return 42 })

		val, exists := m.Get("fn")
		if !exists {
			t.Error("key should exist")
		}
		if val() != 42 {
			t.Errorf("function returned %v, want 42", val())
		}
	})

	t.Run("channel values", func(t *testing.T) {
		m := NewHashMap[string, chan int]()

		ch := make(chan int, 1)
		m.Put("channel", ch)

		val, exists := m.Get("channel")
		if !exists {
			t.Error("key should exist")
		}

		val <- 42
		received := <-val
		if received != 42 {
			t.Errorf("received %v, want 42", received)
		}
	})
}

// =============================================================================
// Concurrent Access Tests (expecting failures without synchronization)
// =============================================================================

func TestHashMap_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent access test in short mode")
	}

	t.Run("concurrent reads are safe", func(t *testing.T) {
		m := NewHashMap[int, int]()
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
		m := NewHashMap[int, int]()
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
// Performance Benchmarks
// =============================================================================

func BenchmarkHashMap_Put(b *testing.B) {
	m := NewHashMap[int, int](b.N)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Put(i, i)
	}
}

func BenchmarkHashMap_Get(b *testing.B) {
	m := NewHashMap[int, int](b.N)
	for i := 0; i < b.N; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Get(i)
	}
}

func BenchmarkHashMap_Remove(b *testing.B) {
	m := NewHashMap[int, int](b.N)
	for i := 0; i < b.N; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Remove(i)
	}
}

func BenchmarkHashMap_ContainsValue(b *testing.B) {
	m := NewHashMap[int, int]()
	for i := 0; i < 1000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.ContainsValue(500)
	}
}

func BenchmarkHashMap_Iter(b *testing.B) {
	m := NewHashMap[int, int]()
	for i := 0; i < 1000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for range m.Iter() {
		}
	}
}

func BenchmarkHashMap_Clone(b *testing.B) {
	m := NewHashMap[int, int]()
	for i := 0; i < 1000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.Clone()
	}
}

func BenchmarkHashMap_ComputeIfAbsent(b *testing.B) {
	m := NewHashMap[int, int](b.N)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.ComputeIfAbsent(i, func(k int) int {
			return k * 10
		})
	}
}

func BenchmarkHashMap_ReplaceAll(b *testing.B) {
	m := NewHashMap[int, int]()
	for i := 0; i < 1000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		m.ReplaceAll(func(k int, v int) int {
			return v * 2
		})
	}
}

// =============================================================================
// Example Tests (documentation examples)
// =============================================================================

func ExampleHashMap_basic() {
	m := NewHashMap[string, int]()

	m.Put("alice", 30)
	m.Put("bob", 25)

	age, _ := m.Get("alice")
	fmt.Println(age)

	m.Remove("bob")
	fmt.Println(m.Size())

	// Output:
	// 30
	// 1
}

func ExampleHashMap_Iter() {
	m := NewHashMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)

	sum := 0
	for _, v := range m.Iter() {
		sum += v
	}
	fmt.Println(sum)

	// Output:
	// 3
}

func ExampleHashMap_ComputeIfAbsent() {
	m := NewHashMap[string, []int]()

	m.ComputeIfAbsent("numbers", func(k string) []int {
		return []int{1, 2, 3}
	})

	numbers, _ := m.Get("numbers")
	fmt.Println(numbers)

	// Output:
	// [1 2 3]
}

func ExampleHashMap_Merge() {
	m := NewHashMap[string, string]()
	m.Put("greeting", "Hello")

	result := m.Merge("greeting", " World", func(old, new string) string {
		return old + new
	})

	fmt.Println(result)

	// Output:
	// Hello World
}

// =============================================================================
// Entry Tests
// =============================================================================

func TestEntry(t *testing.T) {
	entry := &Entry[string, int]{
		key:   "test",
		value: 42,
	}

	if entry.Key() != "test" {
		t.Errorf("Key() = %v, want 'test'", entry.Key())
	}

	if entry.Value() != 42 {
		t.Errorf("Value() = %v, want 42", entry.Value())
	}
}
