package maps

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// SyncMap Constructor Tests
// =============================================================================

func TestSyncMap_NewSyncMap(t *testing.T) {
	t.Run("create with default HashMap", func(t *testing.T) {
		m := NewSyncMap[string, int]()
		if m == nil {
			t.Fatal("NewSyncMap returned nil")
		}
		syncMap, ok := m.(*SyncMap[string, int])
		if !ok {
			t.Fatal("NewSyncMap did not return *SyncMap")
		}
		if syncMap.inner == nil {
			t.Error("inner map is nil")
		}
	})

	t.Run("wrap existing HashMap", func(t *testing.T) {
		hm := NewHashMap[string, int]()
		hm.Put("test", 123)

		m := NewSyncMap(hm)
		syncMap := m.(*SyncMap[string, int])

		if val, ok := syncMap.Get("test"); !ok || val != 123 {
			t.Errorf("Get() = (%v, %v), want (123, true)", val, ok)
		}
	})

	t.Run("wrap existing LinkedMap preserves order", func(t *testing.T) {
		lm := NewLinkedMap[string, int]()
		lm.Put("a", 1)
		lm.Put("b", 2)
		lm.Put("c", 3)

		m := NewSyncMap(lm)
		keys := m.Keys()

		expected := []string{"a", "b", "c"}
		for i, key := range keys {
			if key != expected[i] {
				t.Errorf("Keys()[%d] = %v, want %v", i, key, expected[i])
			}
		}
	})

	t.Run("avoid double wrapping SyncMap", func(t *testing.T) {
		m1 := NewSyncMap[string, int]()
		m2 := NewSyncMap(m1)

		if m1 != m2 {
			t.Error("NewSyncMap should return same instance when wrapping SyncMap")
		}
	})

	t.Run("avoid double wrapping StdSyncMap", func(t *testing.T) {
		m1 := NewStdSyncMap[string, int]()
		m2 := NewSyncMap[string, int](m1)

		if m1 != m2 {
			t.Error("NewSyncMap should return same instance when wrapping StdSyncMap")
		}
	})

	t.Run("ignore nil maps", func(t *testing.T) {
		m := NewSyncMap[string, int](nil)
		if m == nil {
			t.Fatal("NewSyncMap returned nil")
		}
		if !m.IsEmpty() {
			t.Error("map should be empty")
		}
	})
}

// =============================================================================
// SyncMap Basic Thread Safety Tests
// =============================================================================

func TestSyncMap_ConcurrentPut(t *testing.T) {
	m := NewSyncMap[int, int]()
	const numGoroutines = 100
	const numOpsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				key := id*numOpsPerGoroutine + j
				m.Put(key, key*10)
			}
		}(i)
	}

	wg.Wait()

	expectedSize := numGoroutines * numOpsPerGoroutine
	if m.Size() != expectedSize {
		t.Errorf("Size() = %v, want %v", m.Size(), expectedSize)
	}
}

func TestSyncMap_ConcurrentGet(t *testing.T) {
	m := NewSyncMap[int, int]()

	// Populate map
	for i := 0; i < 1000; i++ {
		m.Put(i, i*10)
	}

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				val, ok := m.Get(j)
				if !ok {
					errors <- fmt.Errorf("key %d not found", j)
					return
				}
				if val != j*10 {
					errors <- fmt.Errorf("Get(%d) = %v, want %v", j, val, j*10)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

func TestSyncMap_ConcurrentRemove(t *testing.T) {
	m := NewSyncMap[int, int]()

	// Populate map
	const numKeys = 1000
	for i := 0; i < numKeys; i++ {
		m.Put(i, i)
	}

	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	var removedCount int32

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := id; j < numKeys; j += numGoroutines {
				if _, removed := m.Remove(j); removed {
					atomic.AddInt32(&removedCount, 1)
				}
			}
		}(i)
	}

	wg.Wait()

	if m.Size() != 0 {
		t.Errorf("Size() = %v, want 0", m.Size())
	}
	if int(removedCount) != numKeys {
		t.Errorf("removed count = %v, want %v", removedCount, numKeys)
	}
}

func TestSyncMap_ConcurrentReadWrite(t *testing.T) {
	m := NewSyncMap[int, int]()

	// Populate initial data
	for i := 0; i < 100; i++ {
		m.Put(i, i)
	}

	const duration = 100 * time.Millisecond
	done := make(chan bool)
	errors := make(chan error, 10)

	// Writers
	for i := 0; i < 5; i++ {
		go func(id int) {
			timer := time.After(duration)
			for {
				select {
				case <-timer:
					done <- true
					return
				default:
					key := rand.Intn(100)
					m.Put(key, id*1000+key)
				}
			}
		}(i)
	}

	// Readers
	for i := 0; i < 10; i++ {
		go func() {
			timer := time.After(duration)
			for {
				select {
				case <-timer:
					done <- true
					return
				default:
					key := rand.Intn(100)
					if _, ok := m.Get(key); !ok {
						errors <- fmt.Errorf("key %d should exist", key)
						return
					}
				}
			}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 15; i++ {
		<-done
	}
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// =============================================================================
// SyncMap Operation Tests
// =============================================================================

func TestSyncMap_BasicOperations(t *testing.T) {
	m := NewSyncMap[string, int]()

	t.Run("Put and Get", func(t *testing.T) {
		m.Put("key1", 100)
		if val, ok := m.Get("key1"); !ok || val != 100 {
			t.Errorf("Get() = (%v, %v), want (100, true)", val, ok)
		}
	})

	t.Run("ContainsKey", func(t *testing.T) {
		if !m.ContainsKey("key1") {
			t.Error("ContainsKey() = false, want true")
		}
		if m.ContainsKey("missing") {
			t.Error("ContainsKey() = true, want false")
		}
	})

	t.Run("Size and IsEmpty", func(t *testing.T) {
		if m.Size() != 1 {
			t.Errorf("Size() = %v, want 1", m.Size())
		}
		if m.IsEmpty() {
			t.Error("IsEmpty() = true, want false")
		}
	})

	t.Run("Remove", func(t *testing.T) {
		val, removed := m.Remove("key1")
		if !removed || val != 100 {
			t.Errorf("Remove() = (%v, %v), want (100, true)", val, removed)
		}
		if !m.IsEmpty() {
			t.Error("map should be empty after removing last element")
		}
	})
}

func TestSyncMap_BulkOperations(t *testing.T) {
	t.Run("PutAll", func(t *testing.T) {
		m1 := NewSyncMap[string, int]()
		m1.Put("a", 1)
		m1.Put("b", 2)

		m2 := NewSyncMap[string, int]()
		m2.PutAll(m1)

		if m2.Size() != 2 {
			t.Errorf("Size() = %v, want 2", m2.Size())
		}
		if val, _ := m2.Get("a"); val != 1 {
			t.Errorf("Get(a) = %v, want 1", val)
		}
	})

	t.Run("Clear", func(t *testing.T) {
		m := NewSyncMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Clear()

		if !m.IsEmpty() {
			t.Error("map should be empty after Clear()")
		}
	})

	t.Run("Keys, Values, Entries", func(t *testing.T) {
		m := NewSyncMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)

		keys := m.Keys()
		if len(keys) != 2 {
			t.Errorf("len(Keys()) = %v, want 2", len(keys))
		}

		values := m.Values()
		if len(values) != 2 {
			t.Errorf("len(Values()) = %v, want 2", len(values))
		}

		entries := m.Entries()
		if len(entries) != 2 {
			t.Errorf("len(Entries()) = %v, want 2", len(entries))
		}
	})
}

func TestSyncMap_ConditionalOperations(t *testing.T) {
	t.Run("PutIfAbsent", func(t *testing.T) {
		m := NewSyncMap[string, int]()

		val, inserted := m.PutIfAbsent("key", 100)
		if !inserted || val != 100 {
			t.Errorf("PutIfAbsent() = (%v, %v), want (100, true)", val, inserted)
		}

		val, inserted = m.PutIfAbsent("key", 200)
		if inserted || val != 100 {
			t.Errorf("PutIfAbsent() = (%v, %v), want (100, false)", val, inserted)
		}
	})

	t.Run("RemoveIf", func(t *testing.T) {
		m := NewSyncMap[string, int]()
		m.Put("key", 100)

		if !m.RemoveIf("key", 100) {
			t.Error("RemoveIf() = false, want true")
		}
		if m.RemoveIf("key", 100) {
			t.Error("RemoveIf() = true, want false after removal")
		}
	})

	t.Run("Replace", func(t *testing.T) {
		m := NewSyncMap[string, int]()
		m.Put("key", 100)

		oldVal, replaced := m.Replace("key", 200)
		if !replaced || oldVal != 100 {
			t.Errorf("Replace() = (%v, %v), want (100, true)", oldVal, replaced)
		}

		_, replaced = m.Replace("missing", 300)
		if replaced {
			t.Error("Replace() on missing key should return false")
		}
	})

	t.Run("ReplaceIf", func(t *testing.T) {
		m := NewSyncMap[string, int]()
		m.Put("key", 100)

		if !m.ReplaceIf("key", 100, 200) {
			t.Error("ReplaceIf() = false, want true")
		}
		if m.ReplaceIf("key", 100, 300) {
			t.Error("ReplaceIf() with wrong old value should return false")
		}
	})
}

func TestSyncMap_ComputeOperations(t *testing.T) {
	t.Run("Compute", func(t *testing.T) {
		m := NewSyncMap[string, int]()

		val, ok := m.Compute("key", func(k string, v int, exists bool) (int, bool) {
			if !exists {
				return 100, true
			}
			return v + 1, true
		})

		if !ok || val != 100 {
			t.Errorf("Compute() = (%v, %v), want (100, true)", val, ok)
		}

		val, ok = m.Compute("key", func(k string, v int, exists bool) (int, bool) {
			return v + 1, true
		})

		if !ok || val != 101 {
			t.Errorf("Compute() = (%v, %v), want (101, true)", val, ok)
		}
	})

	t.Run("ComputeIfAbsent", func(t *testing.T) {
		m := NewSyncMap[string, int]()

		val := m.ComputeIfAbsent("key", func(k string) int {
			return 100
		})

		if val != 100 {
			t.Errorf("ComputeIfAbsent() = %v, want 100", val)
		}

		callCount := 0
		val = m.ComputeIfAbsent("key", func(k string) int {
			callCount++
			return 200
		})

		if val != 100 || callCount != 0 {
			t.Error("ComputeIfAbsent should not call function for existing key")
		}
	})

	t.Run("ComputeIfPresent", func(t *testing.T) {
		m := NewSyncMap[string, int]()
		m.Put("key", 100)

		val, ok := m.ComputeIfPresent("key", func(k string, v int) int {
			return v * 2
		})

		if !ok || val != 200 {
			t.Errorf("ComputeIfPresent() = (%v, %v), want (200, true)", val, ok)
		}

		_, ok = m.ComputeIfPresent("missing", func(k string, v int) int {
			return 999
		})

		if ok {
			t.Error("ComputeIfPresent on missing key should return false")
		}
	})

	t.Run("Merge", func(t *testing.T) {
		m := NewSyncMap[string, int]()

		val := m.Merge("key", 100, func(old, new int) int {
			return old + new
		})

		if val != 100 {
			t.Errorf("Merge() = %v, want 100", val)
		}

		val = m.Merge("key", 50, func(old, new int) int {
			return old + new
		})

		if val != 150 {
			t.Errorf("Merge() = %v, want 150", val)
		}
	})
}

func TestSyncMap_IteratorSafety(t *testing.T) {
	m := NewSyncMap[int, int]()
	for i := 0; i < 100; i++ {
		m.Put(i, i*10)
	}

	t.Run("Iter creates snapshot", func(t *testing.T) {
		count := 0
		for k, v := range m.Iter() {
			count++
			if k*10 != v {
				t.Errorf("Iter() yielded (%v, %v), expected value = key*10", k, v)
			}
			// Modify map during iteration should not affect iteration
			if count == 50 {
				m.Put(1000, 10000)
			}
		}

		if count != 100 {
			t.Errorf("iterated %v items, want 100", count)
		}
	})

	t.Run("concurrent iteration", func(t *testing.T) {
		var wg sync.WaitGroup
		const numIterators = 10

		wg.Add(numIterators)
		for i := 0; i < numIterators; i++ {
			go func() {
				defer wg.Done()
				count := 0
				for range m.Iter() {
					count++
				}
				// Count may vary due to concurrent modifications
			}()
		}

		wg.Wait()
	})
}

func TestSyncMap_Clone(t *testing.T) {
	m := NewSyncMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)

	cloned := m.Clone()

	t.Run("clone has same content", func(t *testing.T) {
		if cloned.Size() != 2 {
			t.Errorf("cloned Size() = %v, want 2", cloned.Size())
		}
		if val, _ := cloned.Get("a"); val != 1 {
			t.Errorf("cloned Get(a) = %v, want 1", val)
		}
	})

	t.Run("clone is independent", func(t *testing.T) {
		cloned.Put("c", 3)
		m.Put("d", 4)

		if m.ContainsKey("c") {
			t.Error("original should not contain key from clone")
		}
		if cloned.ContainsKey("d") {
			t.Error("clone should not contain key from original")
		}
	})

	t.Run("clone is also thread-safe", func(t *testing.T) {
		_, ok := cloned.(*SyncMap[string, int])
		if !ok {
			t.Error("clone should be *SyncMap")
		}
	})
}

// =============================================================================
// StdSyncMap Tests
// =============================================================================

func TestStdSyncMap_NewStdSyncMap(t *testing.T) {
	m := NewStdSyncMap[string, int]()
	if m == nil {
		t.Fatal("NewStdSyncMap returned nil")
	}
	if !m.IsEmpty() {
		t.Error("new map should be empty")
	}
}

func TestStdSyncMap_BasicOperations(t *testing.T) {
	m := NewStdSyncMap[string, int]()

	t.Run("Put and Get", func(t *testing.T) {
		oldVal, existed := m.Put("key1", 100)
		if existed {
			t.Error("Put() on new key should return existed=false")
		}
		if oldVal != 0 {
			t.Errorf("Put() old value = %v, want 0", oldVal)
		}

		val, ok := m.Get("key1")
		if !ok || val != 100 {
			t.Errorf("Get() = (%v, %v), want (100, true)", val, ok)
		}

		oldVal, existed = m.Put("key1", 200)
		if !existed || oldVal != 100 {
			t.Errorf("Put() on existing key = (%v, %v), want (100, true)", oldVal, existed)
		}
	})

	t.Run("Remove", func(t *testing.T) {
		m.Put("key2", 300)
		val, removed := m.Remove("key2")
		if !removed || val != 300 {
			t.Errorf("Remove() = (%v, %v), want (300, true)", val, removed)
		}

		_, removed = m.Remove("key2")
		if removed {
			t.Error("Remove() on non-existent key should return false")
		}
	})

	t.Run("ContainsKey", func(t *testing.T) {
		m.Put("key3", 400)
		if !m.ContainsKey("key3") {
			t.Error("ContainsKey() = false, want true")
		}
		if m.ContainsKey("missing") {
			t.Error("ContainsKey() = true, want false")
		}
	})
}

func TestStdSyncMap_ConcurrentOperations(t *testing.T) {
	m := NewStdSyncMap[int, int]()

	t.Run("concurrent Put", func(t *testing.T) {
		const numGoroutines = 50
		const numOpsPerGoroutine = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < numOpsPerGoroutine; j++ {
					key := id*numOpsPerGoroutine + j
					m.Put(key, key*10)
				}
			}(i)
		}

		wg.Wait()

		expectedSize := numGoroutines * numOpsPerGoroutine
		if m.Size() != expectedSize {
			t.Errorf("Size() = %v, want %v", m.Size(), expectedSize)
		}
	})

	t.Run("concurrent Get", func(t *testing.T) {
		const numReaders = 20
		var wg sync.WaitGroup
		wg.Add(numReaders)
		errors := make(chan error, numReaders)

		for i := 0; i < numReaders; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					val, ok := m.Get(j)
					if ok && val != j*10 {
						errors <- fmt.Errorf("Get(%d) = %v, want %v", j, val, j*10)
						return
					}
				}
			}()
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Error(err)
		}
	})
}

func TestStdSyncMap_PutIfAbsent(t *testing.T) {
	m := NewStdSyncMap[string, int]()

	val, inserted := m.PutIfAbsent("key", 100)
	if !inserted || val != 100 {
		t.Errorf("PutIfAbsent() = (%v, %v), want (100, true)", val, inserted)
	}

	val, inserted = m.PutIfAbsent("key", 200)
	if inserted || val != 100 {
		t.Errorf("PutIfAbsent() on existing key = (%v, %v), want (100, false)", val, inserted)
	}
}

func TestStdSyncMap_ComputeOperations(t *testing.T) {
	t.Run("Compute new value", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()

		val, ok := m.Compute("key", func(k string, v int, exists bool) (int, bool) {
			if !exists {
				return 100, true
			}
			return v + 1, true
		})

		if !ok || val != 100 {
			t.Errorf("Compute() = (%v, %v), want (100, true)", val, ok)
		}
	})

	t.Run("Compute update existing", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
		m.Put("key", 100)

		val, ok := m.Compute("key", func(k string, v int, exists bool) (int, bool) {
			if exists {
				return v * 2, true
			}
			return 0, false
		})

		if !ok || val != 200 {
			t.Errorf("Compute() = (%v, %v), want (200, true)", val, ok)
		}
	})

	t.Run("Compute remove", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
		m.Put("key", 100)

		_, ok := m.Compute("key", func(k string, v int, exists bool) (int, bool) {
			return 0, false
		})

		if ok {
			t.Error("Compute() should return false when removing")
		}
		if m.ContainsKey("key") {
			t.Error("key should be removed")
		}
	})

	t.Run("ComputeIfAbsent", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()

		val := m.ComputeIfAbsent("key", func(k string) int {
			return 100
		})

		if val != 100 {
			t.Errorf("ComputeIfAbsent() = %v, want 100", val)
		}

		// Should not recompute
		val = m.ComputeIfAbsent("key", func(k string) int {
			return 200
		})

		if val != 100 {
			t.Errorf("ComputeIfAbsent() = %v, want 100 (should not recompute)", val)
		}
	})

	t.Run("ComputeIfPresent", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
		m.Put("key", 100)

		val, ok := m.ComputeIfPresent("key", func(k string, v int) int {
			return v * 3
		})

		if !ok || val != 300 {
			t.Errorf("ComputeIfPresent() = (%v, %v), want (300, true)", val, ok)
		}

		_, ok = m.ComputeIfPresent("missing", func(k string, v int) int {
			return 999
		})

		if ok {
			t.Error("ComputeIfPresent() on missing key should return false")
		}
	})
}

func TestStdSyncMap_Merge(t *testing.T) {
	m := NewStdSyncMap[string, int]()

	t.Run("merge into non-existing key", func(t *testing.T) {
		val := m.Merge("key", 100, func(old, new int) int {
			return old + new
		})

		if val != 100 {
			t.Errorf("Merge() = %v, want 100", val)
		}
	})

	t.Run("merge into existing key", func(t *testing.T) {
		val := m.Merge("key", 50, func(old, new int) int {
			return old + new
		})

		if val != 150 {
			t.Errorf("Merge() = %v, want 150", val)
		}
	})
}

func TestStdSyncMap_ReplaceOperations(t *testing.T) {
	t.Run("Replace existing", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
		m.Put("key", 100)

		oldVal, replaced := m.Replace("key", 200)
		if !replaced || oldVal != 100 {
			t.Errorf("Replace() = (%v, %v), want (100, true)", oldVal, replaced)
		}
	})

	t.Run("Replace non-existing", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()

		_, replaced := m.Replace("missing", 100)
		if replaced {
			t.Error("Replace() on missing key should return false")
		}
	})

	t.Run("ReplaceIf matching", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
		m.Put("key", 100)

		if !m.ReplaceIf("key", 100, 200) {
			t.Error("ReplaceIf() = false, want true")
		}

		val, _ := m.Get("key")
		if val != 200 {
			t.Errorf("Get() = %v, want 200", val)
		}
	})

	t.Run("ReplaceIf non-matching", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
		m.Put("key", 100)

		if m.ReplaceIf("key", 999, 200) {
			t.Error("ReplaceIf() with wrong old value should return false")
		}

		val, _ := m.Get("key")
		if val != 100 {
			t.Errorf("Get() = %v, want 100 (value should not change)", val)
		}
	})

	t.Run("ReplaceAll", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
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
	})
}

func TestStdSyncMap_BulkOperations(t *testing.T) {
	t.Run("PutAll", func(t *testing.T) {
		m1 := NewStdSyncMap[string, int]()
		m1.Put("a", 1)
		m1.Put("b", 2)

		m2 := NewStdSyncMap[string, int]()
		m2.PutAll(m1)

		if m2.Size() != 2 {
			t.Errorf("Size() = %v, want 2", m2.Size())
		}
	})

	t.Run("Clear", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Clear()

		if !m.IsEmpty() {
			t.Error("map should be empty after Clear()")
		}
	})

	t.Run("Keys, Values, Entries", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)

		keys := m.Keys()
		if len(keys) != 2 {
			t.Errorf("len(Keys()) = %v, want 2", len(keys))
		}

		values := m.Values()
		if len(values) != 2 {
			t.Errorf("len(Values()) = %v, want 2", len(values))
		}

		entries := m.Entries()
		if len(entries) != 2 {
			t.Errorf("len(Entries()) = %v, want 2", len(entries))
		}
	})

	t.Run("ForEach", func(t *testing.T) {
		m := NewStdSyncMap[string, int]()
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		sum := 0
		m.ForEach(func(k string, v int) {
			sum += v
		})

		if sum != 6 {
			t.Errorf("sum = %v, want 6", sum)
		}
	})
}

func TestStdSyncMap_Iterator(t *testing.T) {
	m := NewStdSyncMap[int, int]()
	for i := 0; i < 10; i++ {
		m.Put(i, i*10)
	}

	t.Run("Iter", func(t *testing.T) {
		count := 0
		for k, v := range m.Iter() {
			if k*10 != v {
				t.Errorf("Iter() yielded (%v, %v), expected value = key*10", k, v)
			}
			count++
		}

		if count != 10 {
			t.Errorf("iterated %v items, want 10", count)
		}
	})

	t.Run("IterKeys", func(t *testing.T) {
		count := 0
		for k := range m.IterKeys() {
			if k < 0 || k >= 10 {
				t.Errorf("IterKeys() yielded invalid key %v", k)
			}
			count++
		}

		if count != 10 {
			t.Errorf("iterated %v keys, want 10", count)
		}
	})

	t.Run("IterValues", func(t *testing.T) {
		count := 0
		for v := range m.IterValues() {
			if v%10 != 0 || v < 0 || v >= 100 {
				t.Errorf("IterValues() yielded invalid value %v", v)
			}
			count++
		}

		if count != 10 {
			t.Errorf("iterated %v values, want 10", count)
		}
	})
}

func TestStdSyncMap_Clone(t *testing.T) {
	m := NewStdSyncMap[string, int]()
	m.Put("a", 1)
	m.Put("b", 2)

	cloned := m.Clone()

	t.Run("clone has same content", func(t *testing.T) {
		if cloned.Size() != 2 {
			t.Errorf("cloned Size() = %v, want 2", cloned.Size())
		}
	})

	t.Run("clone is independent", func(t *testing.T) {
		cloned.Put("c", 3)
		m.Put("d", 4)

		if m.ContainsKey("c") {
			t.Error("original should not contain key from clone")
		}
		if cloned.ContainsKey("d") {
			t.Error("clone should not contain key from original")
		}
	})
}

// =============================================================================
// Stress Tests
// =============================================================================

func TestSyncMap_StressConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	m := NewSyncMap[int, int]()
	const numGoroutines = 100
	const numOperations = 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := rand.Intn(100)
				op := rand.Intn(4)

				switch op {
				case 0: // Put
					m.Put(key, id*1000+j)
				case 1: // Get
					m.Get(key)
				case 2: // Remove
					m.Remove(key)
				case 3: // Compute
					m.Compute(key, func(k int, v int, exists bool) (int, bool) {
						if exists {
							return v + 1, true
						}
						return 1, true
					})
				}
			}
		}(i)
	}

	wg.Wait()
	// Test passes if no panics or deadlocks occur
}

func TestStdSyncMap_StressConcurrentOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	m := NewStdSyncMap[int, int]()
	const numGoroutines = 100
	const numOperations = 1000

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := rand.Intn(100)
				op := rand.Intn(5)

				switch op {
				case 0: // Put
					m.Put(key, id*1000+j)
				case 1: // Get
					m.Get(key)
				case 2: // Remove
					m.Remove(key)
				case 3: // PutIfAbsent
					m.PutIfAbsent(key, id*1000+j)
				case 4: // Merge
					m.Merge(key, 1, func(old, new int) int {
						return old + new
					})
				}
			}
		}(i)
	}

	wg.Wait()
	// Test passes if no panics occur
}

// =============================================================================
// Performance Benchmarks
// =============================================================================

func BenchmarkSyncMap_Put(b *testing.B) {
	m := NewSyncMap[int, int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Put(i, i)
			i++
		}
	})
}

func BenchmarkSyncMap_Get(b *testing.B) {
	m := NewSyncMap[int, int]()
	for i := 0; i < 10000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Get(i % 10000)
			i++
		}
	})
}

func BenchmarkStdSyncMap_Put(b *testing.B) {
	m := NewStdSyncMap[int, int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Put(i, i)
			i++
		}
	})
}

func BenchmarkStdSyncMap_Get(b *testing.B) {
	m := NewStdSyncMap[int, int]()
	for i := 0; i < 10000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Get(i % 10000)
			i++
		}
	})
}

func BenchmarkSyncMap_ReadHeavy(b *testing.B) {
	m := NewSyncMap[int, int]()
	for i := 0; i < 1000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				m.Put(i%1000, i)
			} else {
				m.Get(i % 1000)
			}
			i++
		}
	})
}

func BenchmarkStdSyncMap_ReadHeavy(b *testing.B) {
	m := NewStdSyncMap[int, int]()
	for i := 0; i < 1000; i++ {
		m.Put(i, i)
	}
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 == 0 {
				m.Put(i%1000, i)
			} else {
				m.Get(i % 1000)
			}
			i++
		}
	})
}

func BenchmarkSyncMap_vs_StdSyncMap_ReadHeavy(b *testing.B) {
	b.Run("SyncMap", func(b *testing.B) {
		m := NewSyncMap[int, int]()
		for i := 0; i < 1000; i++ {
			m.Put(i, i)
		}
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%100 == 0 {
					m.Put(i%1000, i)
				} else {
					m.Get(i % 1000)
				}
				i++
			}
		})
	})

	b.Run("StdSyncMap", func(b *testing.B) {
		m := NewStdSyncMap[int, int]()
		for i := 0; i < 1000; i++ {
			m.Put(i, i)
		}
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%100 == 0 {
					m.Put(i%1000, i)
				} else {
					m.Get(i % 1000)
				}
				i++
			}
		})
	})
}

func BenchmarkSyncMap_Compute(b *testing.B) {
	m := NewSyncMap[int, int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Compute(i%1000, func(k int, v int, exists bool) (int, bool) {
				if exists {
					return v + 1, true
				}
				return 1, true
			})
			i++
		}
	})
}

func BenchmarkStdSyncMap_Compute(b *testing.B) {
	m := NewStdSyncMap[int, int]()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Compute(i%1000, func(k int, v int, exists bool) (int, bool) {
				if exists {
					return v + 1, true
				}
				return 1, true
			})
			i++
		}
	})
}
