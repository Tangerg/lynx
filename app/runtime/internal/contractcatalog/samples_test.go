package contractcatalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// samplesDir holds the runtime-owned canonical wire samples beside the published
// TypeScript binding. Client modules own how they consume or vendor both; protocol
// verification never reaches into a client's source tree.
const samplesDir = "../../contract/typescript/samples"

// TestWireGoldenRoundTrip is the Go half of the §14 drift gate: every canonical
// sample must unmarshal into the authoritative Go type and re-marshal to a SEMANTICALLY
// identical object. A Go struct that drops a field (unknown → discarded) or adds
// a non-omitempty zero diverges from the sample and fails here — catching the
// `items` vs `data` class of drift the moment the Go side moves. The generated
// binding publishes the same sample index, so a TypeScript consumer can run an
// equivalent check without becoming an input to Runtime tests.
func TestWireGoldenRoundTrip(t *testing.T) {
	for _, s := range Samples() {
		t.Run(s.File, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(samplesDir, s.File))
			if err != nil {
				t.Fatalf("read sample: %v", err)
			}

			target := reflect.New(s.Type).Interface()
			if unmarshalErr := json.Unmarshal(raw, target); unmarshalErr != nil {
				t.Fatalf("unmarshal into %T: %v", target, unmarshalErr)
			}
			if validator, ok := target.(interface{ ValidateWire() error }); ok {
				if validateWireErr := validator.ValidateWire(); validateWireErr != nil {
					t.Fatalf("canonical sample violates %s wire constraints: %v", s.Type, validateWireErr)
				}
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
	samples := Samples()
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

func TestSamplesReturnsCallerOwnedCatalog(t *testing.T) {
	samples := Samples()
	original := samples[0]
	samples[0] = Sample{}
	if got := Samples()[0]; got != original {
		t.Fatal("mutating one Samples result rewrote the artifact catalog")
	}
}
