package interaction

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/core/chat"
)

func TestInteractionWireBaseline(t *testing.T) {
	shape := interactionWireShape()
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(shape)))
	const want = "73a91aca91d2a968636d90aebd11041c149e0e06afc2f8efc0eac6f4b42b64de"
	if got != want {
		t.Fatalf("Interaction wire changed: got %s, want %s\n%s", got, want, shape)
	}
}

func TestInteractionWireBaselineCoversEveryPrivateJSONStruct(t *testing.T) {
	assertPrivateJSONStructsCovered(t, interactionWireTypes())
}

func TestInteractionRejectsPriorProtocolVersion(t *testing.T) {
	effect, err := newModelEffect(&chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("test prior protocol"))},
	}, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := encodeProtocol(effect)
	if err != nil {
		t.Fatal(err)
	}
	prior := interactionPriorProtocolPayload(t, payload)
	if _, err := decodeEffect(prior); err == nil {
		t.Fatal("decodeEffect accepted the prior Interaction protocol version")
	}

	signalID, err := agent.ParseSignalID("signal:prior-protocol")
	if err != nil {
		t.Fatal(err)
	}
	signalRequest, err := NewSteerSignal(
		signalID,
		chat.NewUserMessage(chat.NewTextPart("steer")),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeSignal(interactionPriorProtocolPayload(t, signalRequest.Payload())); err == nil {
		t.Fatal("decodeSignal accepted the prior Interaction protocol version")
	}

	delta, err := encodeModelResponseDelta(&chat.Response{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseModelResponseDelta(interactionPriorProtocolPayload(t, delta)); err == nil {
		t.Fatal("ParseModelResponseDelta accepted the prior Interaction protocol version")
	}
}

func interactionPriorProtocolPayload(t *testing.T, payload json.RawMessage) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	version, err := json.Marshal(protocolSchemaVersion - 1)
	if err != nil {
		t.Fatal(err)
	}
	fields["schema_version"] = version
	prior, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return prior
}

func interactionWireShape() string {
	types := interactionWireTypes()
	slices.SortFunc(types, func(left, right reflect.Type) int {
		return strings.Compare(left.Name(), right.Name())
	})
	var shape strings.Builder
	fmt.Fprintf(
		&shape, "execution_state=%d protocol=%d\n",
		executionStateSchemaVersion, protocolSchemaVersion,
	)
	for _, wireType := range types {
		fmt.Fprintf(&shape, "%s\n", wireType.Name())
		for index := range wireType.NumField() {
			field := wireType.Field(index)
			fmt.Fprintf(&shape, "  %s %s json=%q\n", field.Name, field.Type.String(), field.Tag.Get("json"))
		}
	}
	return shape.String()
}

func interactionWireTypes() []reflect.Type {
	return []reflect.Type{
		reflect.TypeOf(artifactRecord{}),
		reflect.TypeOf(delegateInvocationState{}),
		reflect.TypeOf(delegateSegmentState{}),
		reflect.TypeOf(effectEnvelope{}),
		reflect.TypeOf(executionState{}),
		reflect.TypeOf(inputRequestWire{}),
		reflect.TypeOf(modelCall{}),
		reflect.TypeOf(modelCallResult{}),
		reflect.TypeOf(modelResponseDeltaWire{}),
		reflect.TypeOf(signalEnvelope{}),
		reflect.TypeOf(steerBatch{}),
		reflect.TypeOf(steerInput{}),
		reflect.TypeOf(toolBatchCall{}),
		reflect.TypeOf(toolBatchResult{}),
		reflect.TypeOf(toolCheckpoint{}),
	}
}

func assertPrivateJSONStructsCovered(t *testing.T, wireTypes []reflect.Type) {
	t.Helper()
	covered := make(map[string]struct{}, len(wireTypes))
	for _, wireType := range wireTypes {
		covered[wireType.Name()] = struct{}{}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, specification := range general.Specs {
				typeSpec, ok := specification.(*ast.TypeSpec)
				if !ok || typeSpec.Name.IsExported() {
					continue
				}
				structure, ok := typeSpec.Type.(*ast.StructType)
				if !ok || !structHasJSONTag(structure) {
					continue
				}
				if _, found := covered[typeSpec.Name.Name]; !found {
					t.Errorf("%s: private JSON struct %s is absent from the Interaction wire baseline", name, typeSpec.Name.Name)
				}
			}
		}
	}
}

func structHasJSONTag(structure *ast.StructType) bool {
	for _, field := range structure.Fields.List {
		if field.Tag != nil && strings.Contains(field.Tag.Value, "json:") {
			return true
		}
	}
	return false
}
