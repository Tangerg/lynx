package ptr

import (
	"reflect"
	"testing"
)

type person struct {
	Name string
	Age  int
}

func TestTo(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"int", func(t *testing.T) {
			if p := To(42); p == nil || *p != 42 {
				t.Errorf("To(42) = %v, want non-nil 42", p)
			}
		}},
		{"string", func(t *testing.T) {
			if p := To("hello"); p == nil || *p != "hello" {
				t.Errorf(`To("hello") = %v, want non-nil "hello"`, p)
			}
		}},
		{"struct", func(t *testing.T) {
			want := person{"Alice", 30}
			if p := To(want); p == nil || *p != want {
				t.Errorf("To(struct) = %v, want %+v", p, want)
			}
		}},
		{"zero int", func(t *testing.T) {
			if p := To(0); p == nil || *p != 0 {
				t.Errorf("To(0) returned nil or wrong value: %v", p)
			}
		}},
		{"nil slice", func(t *testing.T) {
			var s []int
			p := To(s)
			if p == nil || *p != nil {
				t.Errorf("To(nil slice) = %v, want non-nil pointer to nil slice", p)
			}
		}},
		{"independence", func(t *testing.T) {
			x := 42
			p := To(x)
			*p = 100
			if x != 42 {
				t.Errorf("original mutated: x = %d, want 42", x)
			}
			if *p != 100 {
				t.Errorf("*p = %d, want 100", *p)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestFrom(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"non-nil int", func(t *testing.T) {
			v := 42
			if got := From(&v); got != 42 {
				t.Errorf("From(&42) = %d, want 42", got)
			}
		}},
		{"nil int", func(t *testing.T) {
			var p *int
			if got := From(p); got != 0 {
				t.Errorf("From(nil) = %d, want 0", got)
			}
		}},
		{"nil string", func(t *testing.T) {
			var p *string
			if got := From(p); got != "" {
				t.Errorf("From(nil) = %q, want empty", got)
			}
		}},
		{"nil struct returns zero", func(t *testing.T) {
			var p *person
			if got := From(p); got != (person{}) {
				t.Errorf("From(nil) = %+v, want zero", got)
			}
		}},
		{"non-nil slice", func(t *testing.T) {
			s := []int{1, 2, 3}
			if got := From(&s); !reflect.DeepEqual(got, s) {
				t.Errorf("From(&slice) = %v, want %v", got, s)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestClone(t *testing.T) {
	t.Run("non-nil int", func(t *testing.T) {
		v := 42
		p := &v
		c := Clone(p)
		if c == nil || *c != 42 {
			t.Fatalf("Clone(&42) = %v, want non-nil 42", c)
		}
		if c == p {
			t.Error("Clone returned same pointer")
		}
	})

	t.Run("nil returns nil", func(t *testing.T) {
		var p *int
		if c := Clone(p); c != nil {
			t.Errorf("Clone(nil) = %v, want nil", c)
		}
	})

	t.Run("modifying clone does not affect original", func(t *testing.T) {
		v := 42
		p := &v
		c := Clone(p)
		*c = 100
		if *p != 42 {
			t.Errorf("*p = %d, want 42", *p)
		}
		if *c != 100 {
			t.Errorf("*c = %d, want 100", *c)
		}
	})

	t.Run("modifying original does not affect clone", func(t *testing.T) {
		v := "hello"
		p := &v
		c := Clone(p)
		*p = "world"
		if *c != "hello" {
			t.Errorf("*c = %q, want %q", *c, "hello")
		}
	})

	t.Run("struct", func(t *testing.T) {
		v := person{"Bob", 25}
		c := Clone(&v)
		if c == nil || *c != v {
			t.Errorf("Clone(struct) = %+v, want %+v", c, v)
		}
	})
}

func TestRoundTrip(t *testing.T) {
	t.Run("To then From", func(t *testing.T) {
		if got := From(To(42)); got != 42 {
			t.Errorf("From(To(42)) = %d, want 42", got)
		}
	})
	t.Run("To, Clone, From chain", func(t *testing.T) {
		if got := From(Clone(To("test"))); got != "test" {
			t.Errorf("chain returned %q, want %q", got, "test")
		}
	})
	t.Run("nil chain", func(t *testing.T) {
		var p *int
		if got := From(Clone(p)); got != 0 {
			t.Errorf("From(Clone(nil)) = %d, want 0", got)
		}
	})
}

func BenchmarkTo(b *testing.B) {
	b.Run("int", func(b *testing.B) {
		for b.Loop() {
			_ = To(42)
		}
	})
	b.Run("struct", func(b *testing.B) {
		v := person{"Alice", 30}
		for b.Loop() {
			_ = To(v)
		}
	})
}

func BenchmarkFrom(b *testing.B) {
	v := 42
	p := &v
	b.Run("non-nil", func(b *testing.B) {
		for b.Loop() {
			_ = From(p)
		}
	})
	b.Run("nil", func(b *testing.B) {
		var np *int
		for b.Loop() {
			_ = From(np)
		}
	})
}

func BenchmarkClone(b *testing.B) {
	v := 42
	p := &v
	b.Run("non-nil", func(b *testing.B) {
		for b.Loop() {
			_ = Clone(p)
		}
	})
	b.Run("nil", func(b *testing.B) {
		var np *int
		for b.Loop() {
			_ = Clone(np)
		}
	})
}
