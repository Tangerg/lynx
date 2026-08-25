package textread

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestScanValidatesUnselectedTailAndCountsTrailingLine(t *testing.T) {
	input := "\xef\xbb\xbffirst\r\nsecond\r\n"
	got, err := Scan(t.Context(), strings.NewReader(input), Options{
		InputBytes: int64(len(input)), LineBytes: 32, OutputBytes: 32, MaxLines: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "first" || got.StartLine != 0 || got.EndLine != 1 || got.TotalLines != 3 || !got.Truncated {
		t.Fatalf("Scan = %+v, want normalized first line and complete total", got)
	}

	invalid := "first\n" + string([]byte{0xff})
	_, err = Scan(t.Context(), strings.NewReader(invalid), Options{
		InputBytes: int64(len(invalid)), LineBytes: 32, OutputBytes: 32, MaxLines: 1,
	})
	if !errors.Is(err, ErrInvalidText) {
		t.Fatalf("invalid unselected tail error = %v, want ErrInvalidText", err)
	}
}

func TestScanEnforcesInputAndContextLimits(t *testing.T) {
	_, err := Scan(t.Context(), strings.NewReader("12345"), Options{
		InputBytes: 4, LineBytes: 8, OutputBytes: 8,
	})
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("oversized input error = %v, want ErrInputTooLarge", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = Scan(ctx, strings.NewReader("text"), Options{
		InputBytes: 4, LineBytes: 4, OutputBytes: 4,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled scan error = %v, want context.Canceled", err)
	}
}

func TestScanSupportsConsumerSpecificOutputBoundaries(t *testing.T) {
	input := "abcd\nefgh"
	complete, err := Scan(t.Context(), strings.NewReader(input), Options{
		InputBytes: int64(len(input)), LineBytes: 8, OutputBytes: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if complete.Content != "abcd" || complete.EndLine != 1 || !complete.Truncated {
		t.Fatalf("complete-line result = %+v", complete)
	}

	partial, err := Scan(t.Context(), strings.NewReader(input), Options{
		InputBytes: int64(len(input)), LineBytes: 8, OutputBytes: 7, PartialLine: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Content != "abcd\nef" || partial.EndLine != 2 || !partial.Truncated {
		t.Fatalf("partial-line result = %+v", partial)
	}
}

func TestVisitLinesSharesNormalizedBoundedValidation(t *testing.T) {
	var numbers []int
	var lines []string
	err := VisitLines(t.Context(), strings.NewReader("\xef\xbb\xbfneedle\r\nlast\r\n"), Limits{
		InputBytes: 32,
		LineBytes:  16,
	}, func(number int, line []byte) error {
		numbers = append(numbers, number)
		lines = append(lines, string(line))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(numbers, []int{1, 2, 3}) || !slices.Equal(lines, []string{"needle", "last", ""}) {
		t.Fatalf("VisitLines = %v %q, want normalized one-based lines", numbers, lines)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := VisitLines(ctx, strings.NewReader("text"), Limits{InputBytes: 4, LineBytes: 4}, func(int, []byte) error {
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled VisitLines error = %v, want context.Canceled", err)
	}
}
