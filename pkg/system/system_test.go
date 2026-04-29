package system

import (
	"runtime"
	"sync"
	"testing"
)

func TestLineSeparator(t *testing.T) {
	got := LineSeparator()
	want := "\n"
	if runtime.GOOS == "windows" {
		want = "\r\n"
	}
	if got != want {
		t.Errorf("LineSeparator() = %q, want %q on %s", got, want, runtime.GOOS)
	}
}

func TestLineSeparator_Stable(t *testing.T) {
	first := LineSeparator()
	for range 100 {
		if got := LineSeparator(); got != first {
			t.Fatalf("got %q, want %q", got, first)
		}
	}
}

func TestLineSeparator_Concurrent(t *testing.T) {
	const goroutines, calls = 50, 100
	first := LineSeparator()
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range calls {
				if got := LineSeparator(); got != first {
					t.Errorf("got %q, want %q", got, first)
				}
			}
		}()
	}
	wg.Wait()
}

func BenchmarkLineSeparator(b *testing.B) {
	for b.Loop() {
		_ = LineSeparator()
	}
}
