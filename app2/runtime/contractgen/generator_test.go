package contractgen_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Tangerg/lynx/app2/runtime/contractgen"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func TestArtifactsComeFromTheImplementedContract(t *testing.T) {
	t.Parallel()

	artifacts, err := contractgen.Artifacts()
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	var manifest contractgen.Manifest
	if err := json.Unmarshal(artifacts["manifest.json"], &manifest); err != nil {
		t.Fatalf("decode manifest error = %v", err)
	}
	if manifest.ProtocolVersion != protocol.ProtocolVersion || len(manifest.Methods) != 1 || manifest.Methods[0].Name != "runtime.discover" {
		t.Fatalf("manifest = %+v", manifest)
	}
	if len(manifest.Endpoints) != 4 {
		t.Fatalf("endpoint count = %d", len(manifest.Endpoints))
	}
	wire := artifacts["typescript/wire.generated.ts"]
	client := artifacts["typescript/client.generated.ts"]
	for _, required := range [][]byte{
		[]byte(`export interface DiscoverResponse`),
		[]byte(`"runtime.discover": { params: EmptyObject; result: DiscoverResponse }`),
		[]byte(`export function isDiscoverResponse`),
	} {
		if !bytes.Contains(wire, required) {
			t.Fatalf("wire artifact is missing %q", required)
		}
	}
	if !bytes.Contains(client, []byte(`async discover`)) || bytes.Contains(client, []byte("2026-08-21")) {
		t.Fatal("client must be generated from protocolVersion rather than duplicating its literal")
	}
}
