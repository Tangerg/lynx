package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// samplesDir holds the shared canonical wire samples. They live under the
// frontend tree (its tsconfig rootDir) so the TS `satisfies` test can import
// them directly; the Go side — the protocol SSOT — reads them cross-module.
// See app/desktop/docs/protocol/API.md §14 (machine-readable artifacts / drift
// gate) + app/runtime/doc/PRIOR_ART.md B-protocol.
const samplesDir = "../../../../desktop/frontend/src/rpc/samples"

// wireSamples pins each canonical sample to the protocol type it must
// round-trip through. Extend this table as coverage grows (methods, more event
// variants); today it covers the highest-drift surface — the RunEvent /
// StreamEvent flat-struct union.
var wireSamples = []struct {
	file   string
	target func() any
}{
	{"run.started.json", func() any { return new(RunEvent) }},
	{"item.delta.json", func() any { return new(RunEvent) }},
	{"run.finished.json", func() any { return new(RunEvent) }},
	{"item.completed.json", func() any { return new(RunEvent) }},
}

// TestWireGoldenRoundTrip is the Go half of the §14 drift gate: every canonical
// sample must unmarshal into the SSOT type and re-marshal to a SEMANTICALLY
// identical object. A Go struct that drops a field (unknown → discarded) or adds
// a non-omitempty zero diverges from the sample and fails here — catching the
// `items` vs `data` class of drift the moment the Go side moves. The TS side
// (frontend rpc/samples.test.ts) pins the SAME files against the hand-written
// wire types, so the two together pin one contract.
func TestWireGoldenRoundTrip(t *testing.T) {
	for _, s := range wireSamples {
		t.Run(s.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(samplesDir, s.file))
			if err != nil {
				t.Fatalf("read sample: %v", err)
			}

			target := s.target()
			if err := json.Unmarshal(raw, target); err != nil {
				t.Fatalf("unmarshal into %T: %v", target, err)
			}
			reencoded, err := json.Marshal(target)
			if err != nil {
				t.Fatalf("re-marshal %T: %v", target, err)
			}

			// Compare as generic maps: order-independent + semantic (a field the
			// Go type can't represent is dropped on re-marshal → the maps differ).
			var want, got map[string]any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatalf("decode sample as map: %v", err)
			}
			if err := json.Unmarshal(reencoded, &got); err != nil {
				t.Fatalf("decode re-encoded as map: %v", err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Errorf("wire drift — sample and Go round-trip disagree\n sample:    %s\n re-marshal: %s", raw, reencoded)
			}
		})
	}
}
