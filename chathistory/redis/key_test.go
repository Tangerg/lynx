package redis

import "testing"

func TestScanPatternEscapesRedisGlobCharacters(t *testing.T) {
	prefix := `history:*?[tenant]\\`
	got := scanPatternEscaper.Replace(prefix) + "*"
	want := `history:\*\?\[tenant\]\\\\*`
	if got != want {
		t.Fatalf("scan pattern = %q, want %q", got, want)
	}
}
