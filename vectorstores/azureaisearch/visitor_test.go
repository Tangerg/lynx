package azureaisearch

import "testing"

func TestAzureWildcardPattern(t *testing.T) {
	t.Parallel()

	if got, want := azureWildcardPattern(`50%_off*?\sale's`), `50*?off\*\?\\sale''s`; got != want {
		t.Fatalf("azureWildcardPattern() = %q, want %q", got, want)
	}
}
