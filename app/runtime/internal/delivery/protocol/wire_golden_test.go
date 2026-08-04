package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// samplesDir holds the shared canonical wire samples. They live under the
// frontend tree (its tsconfig rootDir) so the TS `satisfies` test can import
// them directly; the Go side — the protocol SSOT — reads them cross-module.
// See app/desktop/docs/protocol/API.md §14 (machine-readable artifacts / drift
// gate) and app/desktop/docs/protocol/API.md.
const samplesDir = "../../../../desktop/frontend/src/rpc/samples"

// TestWireGoldenRoundTrip is the Go half of the §14 drift gate: every canonical
// sample must unmarshal into the SSOT type and re-marshal to a SEMANTICALLY
// identical object. A Go struct that drops a field (unknown → discarded) or adds
// a non-omitempty zero diverges from the sample and fails here — catching the
// `items` vs `data` class of drift the moment the Go side moves. The TS side
// (frontend rpc/samples.test.ts) pins the SAME files against the hand-written
// wire types, so the two together pin one contract.
func TestWireGoldenRoundTrip(t *testing.T) {
	for _, s := range CanonicalSamples() {
		t.Run(s.File, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(samplesDir, s.File))
			if err != nil {
				t.Fatalf("read sample: %v", err)
			}

			target := reflect.New(s.Type).Interface()
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

// TestEveryCanonicalSampleIsBound checks the directory against the binding.
//
// A sample nobody binds is checked by nothing — it looks like coverage and is not —
// and a binding whose file was renamed fails on read, which is why only this
// direction needs saying. Both are written down because the samples directory is
// the batch: §11.3's "all three check the same fixtures" is a claim about ITS
// contents, not about whatever the table happens to list.
func TestEveryCanonicalSampleIsBound(t *testing.T) {
	entries, err := os.ReadDir(samplesDir)
	if err != nil {
		t.Fatalf("read samples: %v", err)
	}
	samples := CanonicalSamples()
	bound := make(map[string]bool, len(samples))
	for _, sample := range samples {
		if bound[sample.File] {
			t.Errorf("%s is bound twice", sample.File)
		}
		bound[sample.File] = true
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if !bound[entry.Name()] {
			t.Errorf("%s is a canonical sample nothing binds — no side checks it", entry.Name())
		}
	}
}
