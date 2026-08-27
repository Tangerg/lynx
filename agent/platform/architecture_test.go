package platform

import (
	"reflect"
	"testing"

	agent "github.com/Tangerg/scope/agent"
)

func TestDeploymentCandidateContainsOnlyDiscoveryContracts(t *testing.T) {
	typeOf := reflect.TypeFor[DeploymentCandidate]()
	if typeOf.NumField() != 2 || typeOf.Field(0).Name != "reference" ||
		typeOf.Field(0).Type != reflect.TypeFor[agent.DeploymentRef]() ||
		typeOf.Field(1).Name != "descriptor" ||
		typeOf.Field(1).Type != reflect.TypeFor[agent.Descriptor]() {
		t.Fatalf("DeploymentCandidate fields changed: %v", typeOf)
	}
	for index := range typeOf.NumField() {
		if typeOf.Field(index).IsExported() {
			t.Fatalf("DeploymentCandidate exposes mutable field %s", typeOf.Field(index).Name)
		}
	}
}
