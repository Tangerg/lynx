package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	appcontract "github.com/Tangerg/lynx/app/runtime/internal/application/contract"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
)

// TestEverySystemInvariantHasAnIntegrationFixture is contract §11.4 gate 8: every
// transaction system invariant has a cross-projection integration fixture.
//
// It is one fixture per (invariant, BOUNDARY) pair, not one per invariant. The
// declaration lists boundaries in the plural and says why — more than one means the
// invariant has several ways to be broken and all of them must hold it — so a single
// test somewhere is not coverage. Two of the pairs below had none until this gate was
// built, and a third was proven only by a pure-function test on a validator, never
// through the write set that would actually admit the artifact.
//
// The link is checked in both directions: the pair names its fixture, and the
// fixture's own doc comment names the invariant. That second half matters more than
// it looks — without it a fixture reads as an ordinary test, and the next person to
// simplify it has no way to know it is the only thing standing behind a declared
// invariant.
func TestEverySystemInvariantHasAnIntegrationFixture(t *testing.T) {
	root := moduleRoot(t)

	declared := make(map[string]map[appcontract.TransactionBoundary]bool)
	for _, spec := range appcontract.SystemInvariants() {
		boundaries := make(map[appcontract.TransactionBoundary]bool, len(spec.Boundaries))
		for _, boundary := range spec.Boundaries {
			boundaries[boundary] = true
		}
		declared[spec.Key] = boundaries
	}

	for key, boundaries := range declared {
		covered, ok := invariantFixtures[key]
		if !ok {
			t.Errorf("system invariant %q has no integration fixture", key)
			continue
		}
		for boundary := range boundaries {
			fixture, ok := covered[boundary]
			if !ok {
				t.Errorf("system invariant %q is unproven at the %s boundary", key, boundary)
				continue
			}
			assertFixtureProves(t, root, fixture, key)
		}
		for boundary := range covered {
			if !boundaries[boundary] {
				t.Errorf("a fixture claims %q at the %s boundary, which the invariant does not name", key, boundary)
			}
		}
	}
	for key := range invariantFixtures {
		if _, ok := declared[key]; !ok {
			t.Errorf("a fixture claims invariant %q, which nothing declares", key)
		}
	}
}

// invariantFixtures is the evidence index: which integration fixture proves that a
// transaction boundary maintains a declared invariant.
//
// The fixtures cannot register this themselves. A Go test binary is per package, so
// a runtime registry would only ever see the one package it was compiled into, and
// reading the claims out of source would mean mapping a boundary CONSTANT back to
// its value — a second table of exactly what this one replaces.
var invariantFixtures = map[string]map[appcontract.TransactionBoundary]fixtureRef{
	"session_has_at_most_one_open_run": {
		appcontract.BoundaryRunAdmission:   {"internal/infra/storage/sqlite", "TestRunAdmitEnforcesOneActivePerSession"},
		appcontract.BoundarySegmentOpening: {"internal/adapter/runsegment", "TestCommitOpeningRefusesASecondOpenRun"},
	},
	"terminal_run_explains_how_it_ended": {
		appcontract.BoundarySegmentEvent: {"internal/adapter/runsegment", "TestCommitEventPersistsTheTerminalRunsResult"},
		appcontract.BoundaryRunRecovery:  {"internal/infra/storage/sqlite", "TestReconcileOrphansRepairsWholeDurableLifecycle"},
		appcontract.BoundarySessionImport: {
			"internal/delivery/server", "TestSessionImportRejectsAFailedRunWithoutItsFailure",
		},
	},
	"run_protocol_profile_is_immutable": {
		appcontract.BoundaryRunAdmission: {"internal/infra/storage/sqlite", "TestRunProtocolProfileIsImmutable"},
	},
	"parked_run_has_exactly_one_open_interrupt_set": {
		appcontract.BoundarySegmentEvent: {"internal/adapter/runsegment", "TestCommitEventParkProducesBootResumableTriplet"},
		appcontract.BoundaryRunRecovery: {
			"internal/infra/storage/sqlite", "TestReconcileOrphansRejectsPartialParkWithoutMutatingIt",
		},
	},
	"dropped_run_leaves_nothing_behind": {
		appcontract.BoundarySessionRollback: {"internal/bootstrap", "TestApplyRollbackDropsRunsAndFreesAdmission"},
		appcontract.BoundarySessionDelete:   {"internal/bootstrap", "TestApplyDeleteRemovesRunRows"},
	},
	"imported_session_keeps_its_identity": {
		appcontract.BoundarySessionImport: {"internal/delivery/server", "TestSessionExportImport_RoundTrip"},
	},
	"goal_never_outlives_its_session": {
		appcontract.BoundaryGoalLifecycle:   {"internal/infra/storage/sqlite", "TestGoalStoreRejectsMissingSession"},
		appcontract.BoundarySessionDelete:   {"internal/bootstrap", "TestApplyDeleteClearsSessionGoal"},
		appcontract.BoundarySessionRollback: {"internal/bootstrap", "TestApplyRollbackDropsRunsAndFreesAdmission"},
	},
}

// fixtureRef points at one test function. The package is named by directory rather
// than import path because this gate reads source, not symbols.
type fixtureRef struct {
	dir  string
	name string
}

func (f fixtureRef) String() string { return f.dir + ":" + f.name }

// assertFixtureProves finds the named test and checks its doc comment names the
// invariant it stands behind.
func assertFixtureProves(t *testing.T, root string, fixture fixtureRef, key string) {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(root, fixture.dir, "*_test.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", fixture.dir, err)
	}
	for _, path := range files {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != fixture.name {
				continue
			}
			if !strings.Contains(fn.Doc.Text(), key) {
				t.Errorf("%s is the fixture for %q and its doc comment does not say so", fixture, key)
			}
			return
		}
	}
	t.Errorf("%s is named as the fixture for %q and does not exist", fixture, key)
}

// TestEveryStateKeyHasAShapeFixture is the achievable half of contract §11.4
// gate 14: every first-party state key's published payload is the one the runtime
// actually emits.
//
// The rest of the gate is already structural. The recovery method must be a
// registered method — the registration refuses one that is not, so a key cannot
// promise a call no client can make — and scope, writer, feature and stability are
// declared and projected. What the declaration alone cannot establish is that the
// PRODUCER agrees: the envelope is a map[string]any, so a key whose value drifted
// from its published shape looks correct from both ends.
//
// The revision / lifecycle half and the state.changed fixture wait for C: today's
// wire carries state.snapshot with no revision, so there is no policy to pin yet.
func TestEveryStateKeyHasAShapeFixture(t *testing.T) {
	root := moduleRoot(t)

	for _, spec := range dispatch.WireShapes().StateKeys() {
		fixture, ok := stateKeyFixtures[spec.Key]
		if !ok {
			t.Errorf("state key %q publishes a payload shape and nothing proves the runtime emits it", spec.Key)
			continue
		}
		assertFixtureProves(t, root, fixture, spec.Key)
	}
	for key := range stateKeyFixtures {
		if !slices.ContainsFunc(dispatch.WireShapes().StateKeys(), func(spec dispatch.StateKeySpec) bool {
			return spec.Key == key
		}) {
			t.Errorf("a fixture claims state key %q, which nothing registers", key)
		}
	}
}

// stateKeyFixtures is the evidence index for the state envelope's keys.
var stateKeyFixtures = map[string]fixtureRef{
	"todos": {"internal/delivery/server", "TestStateSnapshotCarriesItsDeclaredTodosPayload"},
}
