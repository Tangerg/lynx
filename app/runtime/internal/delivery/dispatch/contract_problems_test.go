package dispatch

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestProblemCatalogOwnsEveryFirstPartyUnionVariant(t *testing.T) {
	t.Parallel()

	var union UnionSpec
	for _, candidate := range shapes.Unions() {
		if candidate.GoType == typeOf[protocol.ProblemData]() {
			union = candidate
			break
		}
	}
	if union.GoType == nil {
		t.Fatal("ProblemData has no union contract")
	}

	contracts := ProblemContracts()
	want := make([]string, 0, len(contracts))
	for _, contract := range contracts {
		want = append(want, contract.Type)
	}
	got := make([]string, 0, len(union.Variants))
	for _, variant := range union.Variants {
		got = append(got, variant.Tag)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ProblemData variants = %v, catalog = %v", got, want)
	}

	rpcTypes := ProblemTypesFor(ProblemChannelRPC)
	codeTypes := make([]string, 0, len(ProblemCodes()))
	for problemType := range ProblemCodes() {
		codeTypes = append(codeTypes, problemType)
	}
	slices.Sort(codeTypes)
	if !slices.Equal(rpcTypes, codeTypes) {
		t.Fatalf("RPC problem catalog = %v, coded problems = %v", rpcTypes, codeTypes)
	}
}

func TestProblemCatalogPublishesExactChannelSemantics(t *testing.T) {
	t.Parallel()

	execution := ProblemTypesFor(ProblemChannelExecution)
	if !slices.Contains(execution, protocol.ProblemChildRunCanceled) {
		t.Fatalf("execution problems omit %q: %v", protocol.ProblemChildRunCanceled, execution)
	}

	inline := ProblemTypesFor(ProblemChannelInlineStatus)
	for _, contract := range ProblemContracts() {
		if !slices.Contains(inline, contract.Type) {
			continue
		}
		if len(contract.Required) != 0 || len(contract.Optional) != 0 {
			t.Fatalf("inline problem %q carries UI or structured fields: %+v", contract.Type, contract)
		}
	}
}

func TestProblemCatalogViewsAreSnapshots(t *testing.T) {
	t.Parallel()

	contracts := ProblemContracts()
	contracts[0].Type = "corrupted"
	contracts[0].Channels[0] = ProblemChannel("corrupted")
	if got := ProblemContracts()[0]; got.Type == "corrupted" || got.Channels[0] == "corrupted" {
		t.Fatalf("ProblemContracts exposed registry storage: %+v", got)
	}
}
