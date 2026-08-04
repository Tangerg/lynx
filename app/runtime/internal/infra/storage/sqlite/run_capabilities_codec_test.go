package sqlite

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestRunCapabilitiesCodecOwnsCanonicalStorageShape(t *testing.T) {
	want := execution.RunCapabilities{
		ChildRuns: true,
		InterruptKinds: []execution.InterruptKind{
			execution.ApprovalInterrupt,
			execution.QuestionInterrupt,
		},
	}
	encoded, err := encodeRunCapabilities(want)
	if err != nil {
		t.Fatalf("encodeRunCapabilities: %v", err)
	}
	if !strings.Contains(encoded, `"interruptKinds"`) || strings.Contains(encoded, `"interruptTypes"`) {
		t.Fatalf("encoded capabilities = %s, want semantic interruptKinds only", encoded)
	}
	got, err := decodeRunCapabilities(encoded)
	if err != nil {
		t.Fatalf("decodeRunCapabilities: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("round trip = %v, want %v", got, want)
	}
}

func TestRunCapabilitiesCodecRejectsNonCanonicalOrFormerShapes(t *testing.T) {
	for name, encoded := range map[string]string{
		"former protocol vocabulary": `{"interruptTypes":["approval"]}`,
		"unknown field":              `{"childRuns":true,"extra":true}`,
		"duplicate kind":             `{"interruptKinds":["approval","approval"]}`,
		"unsorted kinds":             `{"interruptKinds":["question","approval"]}`,
		"trailing value":             `{} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeRunCapabilities(encoded); err == nil {
				t.Fatalf("decodeRunCapabilities(%s) succeeded", encoded)
			}
		})
	}
}
