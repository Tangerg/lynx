package agent

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestDescriptorOwnsContractAndValidatesValues(t *testing.T) {
	inputSchema, err := SchemaFor[wireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := SchemaFor[wireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := NewDescriptor(DescriptorConfig{
		Name:         "interaction.chat",
		Description:  "Runs one model and tool interaction.",
		Version:      "1.2.3",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !descriptor.Valid() || !descriptor.Digest().Valid() {
		t.Fatalf("descriptor is not valid: %+v", descriptor)
	}
	input, err := EncodeInput(wireFixture{Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if err := descriptor.ValidateInput(input); err != nil {
		t.Fatal(err)
	}
	output, err := ParseOutput([]byte(`{"message":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := descriptor.ValidateOutput(output); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("ValidateOutput error = %v, want ErrInvalidOutput", err)
	}
}

func TestDescriptorDigestChangesWithContract(t *testing.T) {
	inputSchema, err := SchemaFor[wireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := SchemaFor[wireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	base := DescriptorConfig{
		Name:         "interaction.chat",
		Description:  "Runs one model and tool interaction.",
		Version:      "1.0.0",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
	first, err := NewDescriptor(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Version = "1.0.1"
	second, err := NewDescriptor(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest() == second.Digest() {
		t.Fatal("descriptor digest did not change with version")
	}
	countSchema, err := SchemaFor[struct {
		Count int `json:"count"`
	}]()
	if err != nil {
		t.Fatal(err)
	}
	base.Version = "1.0.0"
	base.OutputSchema = countSchema
	third, err := NewDescriptor(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest() == third.Digest() {
		t.Fatal("descriptor digest did not change with output schema")
	}
}

func TestDescriptorJSONRejectsDrift(t *testing.T) {
	descriptor := testDescriptor(t)
	data, err := json.Marshal(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Descriptor
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Digest() != descriptor.Digest() {
		t.Fatalf("decoded digest = %q, want %q", decoded.Digest(), descriptor.Digest())
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	wire["version"] = "1.0.1"
	tampered, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(tampered, &decoded); !errors.Is(err, ErrInvalidDescriptor) {
		t.Fatalf("unmarshal tampered descriptor error = %v, want ErrInvalidDescriptor", err)
	}
}

func TestDescriptorRejectsInvalidIdentity(t *testing.T) {
	valid := descriptorConfig(t)
	for name, mutate := range map[string]func(*DescriptorConfig){
		"empty name":        func(config *DescriptorConfig) { config.Name = "" },
		"uppercase name":    func(config *DescriptorConfig) { config.Name = "Chat" },
		"empty description": func(config *DescriptorConfig) { config.Description = "" },
		"noncanonical semver": func(config *DescriptorConfig) {
			config.Version = "v1.0.0"
		},
	} {
		t.Run(name, func(t *testing.T) {
			config := valid
			mutate(&config)
			if _, err := NewDescriptor(config); !errors.Is(err, ErrInvalidDescriptor) {
				t.Fatalf("NewDescriptor error = %v, want ErrInvalidDescriptor", err)
			}
		})
	}
}

func testDescriptor(t *testing.T) Descriptor {
	t.Helper()
	descriptor, err := NewDescriptor(descriptorConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	return descriptor
}

func descriptorConfig(t *testing.T) DescriptorConfig {
	t.Helper()
	inputSchema, err := SchemaFor[wireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	outputSchema, err := SchemaFor[wireFixture]()
	if err != nil {
		t.Fatal(err)
	}
	return DescriptorConfig{
		Name:         "interaction.chat",
		Description:  "Runs one model and tool interaction.",
		Version:      "1.0.0",
		InputSchema:  inputSchema,
		OutputSchema: outputSchema,
	}
}
