package ptr

import "testing"

func TestClone(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := Clone[int](nil); got != nil {
			t.Fatalf("Clone(nil) = %v, want nil", got)
		}
	})

	t.Run("independent copy", func(t *testing.T) {
		original := 42
		cloned := Clone(&original)
		if cloned == nil || *cloned != original {
			t.Fatalf("Clone(&%d) = %v, want pointer to %d", original, cloned, original)
		}
		if cloned == &original {
			t.Fatal("Clone returned the original pointer")
		}

		*cloned = 7
		if original != 42 {
			t.Fatalf("mutating clone changed original to %d", original)
		}
	})
}
