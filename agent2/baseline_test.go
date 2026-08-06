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
		{name: "kernel", directory: ".", want: "3a53aee10161912baf58b506bc7d3b24f47c09d3e274e910034a422109ac0fd8"},
		{name: "interaction", directory: "interaction", want: "6861bf74171fb6ece89bdda06627ef6cb06ab3e7353c68f95ebd4c3ce4d91e26"},
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
