package assert

import (
	"errors"
	"testing"
)

func TestMust_ReturnsValue(t *testing.T) {
	type point struct{ X, Y int }

	tests := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"string", func(t *testing.T) {
			if got := Must("hello", nil); got != "hello" {
				t.Errorf("got %q, want %q", got, "hello")
			}
		}},
		{"int", func(t *testing.T) {
			if got := Must(42, nil); got != 42 {
				t.Errorf("got %d, want %d", got, 42)
			}
		}},
		{"struct", func(t *testing.T) {
			want := point{1, 2}
			if got := Must(want, nil); got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		}},
		{"pointer", func(t *testing.T) {
			v := 7
			if got := Must(&v, nil); got != &v {
				t.Errorf("got %p, want %p", got, &v)
			}
		}},
		{"slice", func(t *testing.T) {
			got := Must([]int{1, 2, 3}, nil)
			if len(got) != 3 || got[0] != 1 || got[2] != 3 {
				t.Errorf("got %v, want [1 2 3]", got)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.run)
	}
}

func TestMust_PanicsOnError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"sentinel", errors.New("boom")},
		{"wrapped", errors.New("error: code 500")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("expected panic, got none")
				}
				err, ok := r.(error)
				if !ok {
					t.Fatalf("panic value type = %T, want error", r)
				}
				if err.Error() != tt.err.Error() {
					t.Errorf("panic err = %q, want %q", err.Error(), tt.err.Error())
				}
			}()
			Must(0, tt.err)
		})
	}
}

func TestEnsure(t *testing.T) {
	tests := []struct {
		name      string
		condition bool
		err       error
		wantPanic bool
	}{
		{"true does not panic", true, errors.New("ok"), false},
		{"false panics with err", false, errors.New("condition failed"), true},
		{"comparison false panics", 1 > 2, errors.New("1 > 2"), true},
		{"empty string check", len("") > 0, errors.New("empty"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				switch {
				case tt.wantPanic && r == nil:
					t.Fatal("expected panic, got none")
				case !tt.wantPanic && r != nil:
					t.Fatalf("unexpected panic: %v", r)
				case tt.wantPanic:
					err, ok := r.(error)
					if !ok {
						t.Fatalf("panic value type = %T, want error", r)
					}
					if err.Error() != tt.err.Error() {
						t.Errorf("panic err = %q, want %q", err.Error(), tt.err.Error())
					}
				}
			}()
			Ensure(tt.condition, tt.err)
		})
	}
}

func BenchmarkMust(b *testing.B) {
	for b.Loop() {
		_ = Must(42, nil)
	}
}

func BenchmarkEnsure(b *testing.B) {
	err := errors.New("ok")
	for b.Loop() {
		Ensure(true, err)
	}
}
