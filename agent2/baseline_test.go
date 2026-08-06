package agent2

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestExportedAPIBaseline(t *testing.T) {
	tests := []struct {
		name      string
		directory string
		want      string
	}{
		{name: "kernel", directory: ".", want: "9cfc3728580548301732bdc8cc9a33e9b5bde083215498e39c455d921730b617"},
		{name: "interaction", directory: "interaction", want: "9678f94265b227e7d085cc18a264ad3be4cac98709d94638f47c9ee7960e3fee"},
		{name: "planning", directory: "planning", want: "bb3a3fee5315afba3cc1f70ecc0486b4b91f88d4d4160aa93bf896b09ffc28a1"},
		{name: "goap", directory: "planning/goap", want: "da348e298e6976318b317873b44ec60829020fdea82947fae4bbc8e0d865b419"},
		{name: "workflow", directory: "workflow", want: "0493f8f7ae6e4cc5a3190735c5d02952ec0e0fdb230794bbb01735b8ecfae055"},
		{name: "otel", directory: "otel", want: "aed81360b2fdedda8b08a2c27e7570a4f06f4584ff84e3f0904016d517c038ec"},
		{name: "platform", directory: "platform", want: "9fef1861154434ca2f4fdd4d3b6bfad2b4c8326b15c49eb0d3c82fc0062f8713"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.CommandContext(t.Context(), "go", "doc", "-all", ".")
			command.Dir = test.directory
			command.Env = append(os.Environ(), "GOWORK=off")
			output, err := command.Output()
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("%x", sha256.Sum256(output))
			if got != test.want {
				t.Fatalf(
					"exported API/GoDoc changed: got %s, want %s; audit the change and update the accepted baseline",
					got, test.want,
				)
			}
		})
	}
}

func TestSnapshotWireBaseline(t *testing.T) {
	shape := snapshotWireShape()
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(shape)))
	const want = "6f4a919ed0c681e9fb021f5571de0cdaf4e97d0cad8e4170fede3453a31c0c9d"
	if got != want {
		t.Fatalf("snapshot wire changed: got %s, want %s\n%s", got, want, shape)
	}
}

func TestObservationWireBaseline(t *testing.T) {
	shape := observationWireShape()
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(shape)))
	const want = "b5a32c7dd19858ee0e256f19973010cffff30f6e961d2b04d7cb80947f121ad4"
	if got != want {
		t.Fatalf("observation wire changed: got %s, want %s\n%s", got, want, shape)
	}
}

func observationWireShape() string {
	types := []reflect.Type{reflect.TypeOf(eventWire{}), reflect.TypeOf(deltaWire{})}
	slices.SortFunc(types, func(left, right reflect.Type) int {
		return strings.Compare(left.Name(), right.Name())
	})
	var shape strings.Builder
	for _, wireType := range types {
		fmt.Fprintf(&shape, "%s\n", wireType.Name())
		for index := range wireType.NumField() {
			field := wireType.Field(index)
			fmt.Fprintf(
				&shape, "  %s %s json=%q\n",
				field.Name, field.Type.String(), field.Tag.Get("json"),
			)
		}
	}
	return shape.String()
}

func snapshotWireShape() string {
	types := []reflect.Type{
		reflect.TypeOf(processSnapshotWire{}),
		reflect.TypeOf(processRelationWire{}),
		reflect.TypeOf(preparedStepWire{}),
		reflect.TypeOf(preparedEffectWire{}),
		reflect.TypeOf(pendingControlWire{}),
		reflect.TypeOf(mailboxWire{}),
		reflect.TypeOf(signalRecordWire{}),
		reflect.TypeOf(waitRecordWire{}),
		reflect.TypeOf(treeSnapshotWire{}),
		reflect.TypeOf(childWaitSnapshotWire{}),
		reflect.TypeOf(executionStateWire{}),
		reflect.TypeOf(transitionWire{}),
		reflect.TypeOf(effectWire{}),
		reflect.TypeOf(settlementWire{}),
		reflect.TypeOf(signalWire{}),
		reflect.TypeOf(deploymentIdentityWire{}),
		reflect.TypeOf(deploymentRefWire{}),
		reflect.TypeOf(terminationWire{}),
		reflect.TypeOf(failureWire{}),
		reflect.TypeOf(childWaitConditionWire{}),
		reflect.TypeOf(childWaitSpecWire{}),
		reflect.TypeOf(childOutcomeWire{}),
		reflect.TypeOf(resultWire{}),
		reflect.TypeOf(Budget{}),
		reflect.TypeOf(Limits{}),
		reflect.TypeOf(TreeLimits{}),
		reflect.TypeOf(Usage{}),
	}
	slices.SortFunc(types, func(left, right reflect.Type) int {
		return strings.Compare(left.Name(), right.Name())
	})
	var shape strings.Builder
	fmt.Fprintf(
		&shape, "process=%d tree=%d child=%d framework_effect=%d\n",
		processSnapshotSchemaVersion, treeSnapshotSchemaVersion,
		childProtocolSchemaVersion, frameworkEffectSchemaVersion,
	)
	for _, wireType := range types {
		fmt.Fprintf(&shape, "%s\n", wireType.Name())
		for index := range wireType.NumField() {
			field := wireType.Field(index)
			fmt.Fprintf(
				&shape, "  %s %s json=%q\n",
				field.Name, field.Type.String(), field.Tag.Get("json"),
			)
		}
	}
	return shape.String()
}
