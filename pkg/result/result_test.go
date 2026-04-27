package result

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

var errSentinel = errors.New("sentinel")

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		v       int
		err     error
		wantV   int
		wantErr error
	}{
		{"value only", 42, nil, 42, nil},
		{"value + err", 42, errSentinel, 42, errSentinel},
		{"zero + err", 0, errSentinel, 0, errSentinel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.v, tt.err)
			v, err := r.Get()
			if v != tt.wantV || err != tt.wantErr {
				t.Errorf("Get() = (%v, %v), want (%v, %v)", v, err, tt.wantV, tt.wantErr)
			}
		})
	}
}

func TestValue(t *testing.T) {
	r := Value(42)
	if v, err := r.Get(); v != 42 || err != nil {
		t.Errorf("Get() = (%d, %v), want (42, nil)", v, err)
	}
	if r.Error() != nil {
		t.Errorf("Error() = %v, want nil", r.Error())
	}
	if r.Value() != 42 {
		t.Errorf("Value() = %d, want 42", r.Value())
	}
}

func TestError(t *testing.T) {
	r := Error[int](errSentinel)
	v, err := r.Get()
	if v != 0 || err != errSentinel {
		t.Errorf("Get() = (%d, %v), want (0, %v)", v, err, errSentinel)
	}
	if r.Value() != 0 {
		t.Errorf("Value() = %d, want zero", r.Value())
	}

	// Error[string] returns empty string zero value.
	rs := Error[string](errSentinel)
	if rs.Value() != "" {
		t.Errorf("Value() = %q, want empty", rs.Value())
	}
}

type stringer struct{ s string }

func (s stringer) String() string { return s.s }

func TestResult_String(t *testing.T) {
	rInt := Value(42)
	rStr := Value("hi")
	rStringer := Value(stringer{"X"})
	rErr := Error[int](errSentinel)

	tests := []struct {
		name, got, want string
	}{
		{"success int", rInt.String(), "value: 42"},
		{"success string", rStr.String(), "value: hi"},
		{"success stringer", rStringer.String(), "value: X"},
		{"error", rErr.String(), "error: sentinel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("String() = %q, want %q", tt.got, tt.want)
			}
		})
	}

	// Struct without Stringer falls back to %+v.
	r := Value(struct{ Name string }{"Bob"})
	if s := r.String(); !strings.HasPrefix(s, "value: ") || !strings.Contains(s, "Bob") {
		t.Errorf("String() = %q", s)
	}
}

func TestMap(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := Map(Value(10), func(x int) int { return x * 2 })
		if v, err := r.Get(); v != 20 || err != nil {
			t.Errorf("Get() = (%d, %v), want (20, nil)", v, err)
		}
	})
	t.Run("type change", func(t *testing.T) {
		r := Map(Value(42), func(x int) string { return fmt.Sprintf("n=%d", x) })
		if v, _ := r.Get(); v != "n=42" {
			t.Errorf("Get() value = %q, want %q", v, "n=42")
		}
	})
	t.Run("propagates error", func(t *testing.T) {
		r := Map(Error[int](errSentinel), func(x int) int { return x * 2 })
		v, err := r.Get()
		if v != 0 || err != errSentinel {
			t.Errorf("Get() = (%d, %v), want (0, %v)", v, err, errSentinel)
		}
	})
	t.Run("chained", func(t *testing.T) {
		r := Map(Map(Value(5), func(x int) int { return x * 2 }),
			func(x int) string { return fmt.Sprintf("%d", x) })
		if v, err := r.Get(); v != "10" || err != nil {
			t.Errorf("chained Get() = (%q, %v)", v, err)
		}
	})
}

func TestUsage_LiftAtoiAndMap(t *testing.T) {
	parse := func(s string) Result[int] {
		var n int
		_, err := fmt.Sscanf(s, "%d", &n)
		return New(n, err)
	}
	r := Map(parse("42"), func(n int) int { return n * 2 })
	v, err := r.Get()
	if v != 84 || err != nil {
		t.Errorf("Get() = (%d, %v), want (84, nil)", v, err)
	}
	r2 := Map(parse("abc"), func(n int) int { return n * 2 })
	if _, err := r2.Get(); err == nil {
		t.Error("expected parse error")
	}
}

func BenchmarkResult(b *testing.B) {
	b.Run("New", func(b *testing.B) {
		for b.Loop() {
			_ = New(42, nil)
		}
	})
	b.Run("Value", func(b *testing.B) {
		for b.Loop() {
			_ = Value(42)
		}
	})
	b.Run("Get", func(b *testing.B) {
		r := Value(42)
		for b.Loop() {
			_, _ = r.Get()
		}
	})
	b.Run("Map", func(b *testing.B) {
		r := Value(42)
		fn := func(x int) int { return x * 2 }
		for b.Loop() {
			_ = Map(r, fn)
		}
	})
}
