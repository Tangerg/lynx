package kv

import (
	"encoding/json"
	"iter"
	"testing"
)

func TestNewOrderedKV0(t *testing.T) {
	k := NewOrderedKV[string, string]()
	k.Put("a", "1").
		Put("b", "2").
		Put("c", "3").
		Put("d", "4").
		Put("e", "5")
	k.Reverse()
	k.ForEach(func(k string, v string) {
		t.Log(k, v)
	})
}

func TestNewOrderedKV(t *testing.T) {
	k := NewOrderedKV[string, string]()
	k.Put("a", "1").
		Put("b", "2").
		Put("c", "3").
		Put("d", "4").
		Put("e", "5")

	k.ForEach(func(k string, v string) {
		t.Log(k, v)
	})
	seq := k.Iterator()
	next, stop := iter.Pull2(seq)
	for {
		key, val, ok := next()
		if !ok {
			break
		}
		t.Log(key, val)
		if key == "c" {
			stop()
		}
	}
}

func TestNewOrderedKV2(t *testing.T) {
	k := NewOrderedKV[string, string]()
	k.Put("a", "1").
		Put("b", "2").
		Put("c", "3").
		Put("d", "4").
		Put("e", "5")

	b, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))
}

func TestNewOrderedKV3(t *testing.T) {
	k := NewOrderedKV[string, string]()
	k.Put("a", "1").
		Put("b", "2").
		Put("c", "3").
		Put("d", "4").
		Put("e", "5")

	b, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))

	k1 := NewOrderedKV[string, string]()
	err = json.Unmarshal(b, &k1)
	if err != nil {
		t.Fatal(err)
	}
	k.ForEach(func(k string, v string) {
		t.Log(k, v)
	})
}

func TestNewOrderedKV4(t *testing.T) {
	k := NewOrderedKV[int, bool]()
	k.Put(1, false).
		Put(2, true)

	b, err := json.Marshal(k)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))

	k1 := NewOrderedKV[int, bool]()
	err = json.Unmarshal(b, &k1)
	if err != nil {
		t.Fatal(err)
	}
	k1.ForEach(func(k int, v bool) {
		t.Log(k, v)
	})
}

func TestNewOrderedKV5(t *testing.T) {
	m1 := map[string]int{
		"1a": 1,
		"2b": 2,
	}
	v, _ := json.Marshal(m1)
	t.Log(string(v))

	m2 := NewOrderedKV[string, int]()
	err := json.Unmarshal(v, &m2)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(m2)
}

func TestNewOrderedKV6(t *testing.T) {
	m1 := NewOrderedKV[string, any]()
	m1.Put("name", "Alice")
	m1.Put("age", 30)
	m1.Put("is_admin", true)
	m1.Put("scores", []int{95, 88, 76})
	m1.Put("address", map[string]string{
		"city":    "New York",
		"zipcode": "10001",
	})
	m1.Put("balance", "1234.56")
	v, _ := json.Marshal(m1)
	t.Log(string(v))
	m2 := NewOrderedKV[string, any]()
	err := json.Unmarshal(v, &m2)
	if err != nil {
		t.Fatal(err)
	}
	m2.Reverse()
	m2.ForEach(func(k string, v interface{}) {
		t.Log(k, v)
	})
}

func TestNewOrderedKV7(t *testing.T) {
	m1 := NewOrderedKV[string, map[string]string]()
	m1.Put("address", map[string]string{
		"city":    "New York",
		"zipcode": "10001",
	})
	v, _ := json.Marshal(m1)
	t.Log(string(v))
	m2 := NewOrderedKV[string, map[string]string]()
	err := json.Unmarshal(v, &m2)
	if err != nil {
		t.Fatal(err)
	}
	m2.ForEach(func(k string, v map[string]string) {
		t.Log(k, v)
	})

	m3 := NewOrderedKV[string, any]()
	err = json.Unmarshal(v, &m3)
	if err != nil {
		t.Fatal(err)
	}
	m3.ForEach(func(k string, v any) {
		t.Log(k, v)
	})

	m4 := NewOrderedKV[string, *OrderedKV[string, string]]()
	err = json.Unmarshal(v, &m4)
	if err != nil {
		t.Fatal(err)
	}
	m4.ForEach(func(k string, v *OrderedKV[string, string]) {
		t.Log(k, v)
	})
}

func BenchmarkOrderedKVMarshal(b *testing.B) {
	m1 := NewOrderedKV[string, any]()
	m1.Put("name", "Alice")
	m1.Put("age", 30)
	m1.Put("is_admin", true)
	m1.Put("scores", []int{95, 88, 76})
	m1.Put("address", map[string]string{
		"city":    "New York",
		"zipcode": "10001",
	})
	m1.Put("balance", "1234.56")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m1.MarshalJSON()
	}
	b.StopTimer()
}

func BenchmarkOrderedKVUnmarshal(b *testing.B) {
	m1 := NewOrderedKV[string, any]()
	m1.Put("name", "Alice")
	m1.Put("age", 30)
	m1.Put("is_admin", true)
	m1.Put("scores", []int{95, 88, 76})
	m1.Put("address", map[string]string{
		"city":    "New York",
		"zipcode": "10001",
	})
	m1.Put("balance", "1234.56")
	v, _ := json.Marshal(m1)
	m2 := NewOrderedKV[string, any]()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m2.UnmarshalJSON(v)
	}
	b.StopTimer()
}
