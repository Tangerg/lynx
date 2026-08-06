package agent2

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDeploymentRefBindsContractImplementationAndConfiguration(t *testing.T) {
	descriptor := testDescriptor(t)
	implementation := digestBytes([]byte("interaction implementation v1"))
	configuration := digestBytes([]byte("model and dispatcher configuration v1"))
	reference, err := NewDeploymentRef(descriptor, implementation, configuration)
	if err != nil {
		t.Fatal(err)
	}
	if !reference.Valid() || reference.ContractDigest() != descriptor.Digest() || reference.ImplementationDigest() != implementation || reference.ConfigurationDigest() != configuration {
		t.Fatalf("DeploymentRef = %+v", reference)
	}
	wantText := descriptor.Name() + "@" + descriptor.Version() + "+" + reference.Digest().String()
	if reference.String() != wantText || (DeploymentRef{}).String() != invalidDeploymentRefText {
		t.Fatalf("DeploymentRef text = %q, invalid = %q", reference.String(), (DeploymentRef{}).String())
	}

	changedImplementation, err := NewDeploymentRef(descriptor, digestBytes([]byte("interaction implementation v2")), configuration)
	if err != nil {
		t.Fatal(err)
	}
	changedConfiguration, err := NewDeploymentRef(descriptor, implementation, digestBytes([]byte("model and dispatcher configuration v2")))
	if err != nil {
		t.Fatal(err)
	}
	if reference.Digest() == changedImplementation.Digest() || reference.Digest() == changedConfiguration.Digest() {
		t.Fatal("Deployment digest did not change with exact implementation or configuration")
	}
}

func TestDeploymentRefStrictJSONRejectsTampering(t *testing.T) {
	reference, err := NewDeploymentRef(testDescriptor(t), digestBytes([]byte("implementation")), digestBytes([]byte("configuration")))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(reference)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DeploymentRef
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != reference {
		t.Fatalf("decoded DeploymentRef = %+v, want %+v", decoded, reference)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	wire["version"] = "9.9.9"
	tampered, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(tampered, &decoded); !errors.Is(err, ErrInvalidDeploymentRef) {
		t.Fatalf("tampered DeploymentRef error = %v, want ErrInvalidDeploymentRef", err)
	}
}

func FuzzDeploymentRefJSONRoundTrip(f *testing.F) {
	reference, err := NewDeploymentRef(testDescriptorForFuzz(f), ComputeDigest([]byte("implementation")), ComputeDigest([]byte("configuration")))
	if err != nil {
		f.Fatal(err)
	}
	seed, err := json.Marshal(reference)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		var decoded DeploymentRef
		if err := json.Unmarshal(data, &decoded); err != nil {
			return
		}
		encoded, err := json.Marshal(decoded)
		if err != nil {
			t.Fatal(err)
		}
		var roundTrip DeploymentRef
		if err := json.Unmarshal(encoded, &roundTrip); err != nil {
			t.Fatal(err)
		}
		if roundTrip != decoded {
			t.Fatalf("round trip = %+v, want %+v", roundTrip, decoded)
		}
	})
}

func testDescriptorForFuzz(f *testing.F) Descriptor {
	f.Helper()
	schema, err := SchemaFor[wireFixture]()
	if err != nil {
		f.Fatal(err)
	}
	descriptor, err := NewDescriptor(DescriptorConfig{
		Name:         "deployment.fuzz",
		Description:  "Validates DeploymentRef codec fuzz behavior.",
		Version:      "0.1.0",
		InputSchema:  schema,
		OutputSchema: schema,
	})
	if err != nil {
		f.Fatal(err)
	}
	return descriptor
}
