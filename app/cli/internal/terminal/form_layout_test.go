package terminal

import "testing"

func TestFormDialogHeightReservesValidationWithoutExceedingItsBound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                             string
		contentRows, fieldCount, maximum int
		want                             int
	}{
		{name: "minimum", contentRows: 1, maximum: 24, want: 8},
		{name: "validation reserve", contentRows: 12, fieldCount: 3, maximum: 24, want: 17},
		{name: "maximum", contentRows: 20, fieldCount: 8, maximum: 24, want: 24},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := formDialogHeight(test.contentRows, test.fieldCount, test.maximum); got != test.want {
				t.Fatalf("formDialogHeight(%d, %d, %d) = %d, want %d", test.contentRows, test.fieldCount, test.maximum, got, test.want)
			}
		})
	}
}
