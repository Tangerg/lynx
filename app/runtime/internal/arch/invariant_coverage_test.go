package arch

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type transactionBoundary string

const (
	boundaryRunAdmission               transactionBoundary = "runs.admission"
	boundarySegmentOpening             transactionBoundary = "runsegment.opening"
	boundarySegmentEvent               transactionBoundary = "runsegment.event"
	boundaryWaitingSubtreeCancellation transactionBoundary = "runsegment.waiting_subtree_cancel"
	boundaryRunRecovery                transactionBoundary = "runs.recovery"
	boundaryParkedTermination          transactionBoundary = "sessions.parked_terminal"
	boundarySessionRollback            transactionBoundary = "sessions.rollback"
	boundarySessionDelete              transactionBoundary = "sessions.delete"
	boundarySessionImport              transactionBoundary = "sessions.import"
	boundaryGoalLifecycle              transactionBoundary = "goals.lifecycle"
)

type systemInvariantSpec struct {
	Key        string                `json:"key"`
	Why        string                `json:"why"`
	Boundaries []transactionBoundary `json:"boundaries"`
}

func readSystemInvariantSpecs(t *testing.T) []systemInvariantSpec {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(moduleRoot(t), "contract", "manifest.json"))
	if err != nil {
		t.Fatalf("read generated contract manifest: %v", err)
	}
	var manifest struct {
		SystemInvariants []systemInvariantSpec `json:"systemInvariants"`
	}
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("decode generated contract manifest: %v", err)
	}
	return manifest.SystemInvariants
}

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

	declared := make(map[string]map[transactionBoundary]bool)
	for _, spec := range readSystemInvariantSpecs(t) {
		boundaries := make(map[transactionBoundary]bool, len(spec.Boundaries))
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
var invariantFixtures = map[string]map[transactionBoundary]fixtureRef{
	"session_has_at_most_one_open_run": {
		boundaryRunAdmission:   {"internal/infra/sqlite", "TestRunAdmitEnforcesOneActivePerSession"},
		boundarySegmentOpening: {"internal/adapter/runsegment", "TestCommitOpeningRefusesASecondOpenRun"},
	},
	"terminal_run_explains_how_it_ended": {
		boundarySegmentEvent: {"internal/adapter/runsegment", "TestCommitEventPersistsTheTerminalRunsResult"},
		boundaryRunRecovery:  {"internal/adapter/runrecovery", "TestRecoveryRepairsWholeDurableLifecycle"},
		boundarySessionImport: {
			"internal/delivery/server", "TestSessionImportRejectsAFailedRunWithoutItsFailure",
		},
	},
	"run_capabilities_are_immutable": {
		boundaryRunAdmission: {"internal/infra/sqlite", "TestRunCapabilitiesAreImmutable"},
	},
	"parked_tree_has_exactly_one_open_interrupt_set": {
		boundarySegmentEvent: {"internal/adapter/runsegment", "TestCommitTreeBarrierProducesDurableTriplet"},
		boundaryRunRecovery: {
			"internal/adapter/runrecovery", "TestRecoveryRejectsPartialParkWithoutMutatingIt",
		},
	},
	"parked_continuation_matches_run_facts": {
		boundarySegmentOpening: {
			"internal/application/runs", "TestResumeRejectsContinuationFactDriftBeforeExecutorPreparation",
		},
		boundarySegmentEvent: {
			"internal/adapter/runsegment", "TestCommitTreeBarrierRejectsRunContinuationFactDriftBeforeTransaction",
		},
		boundaryWaitingSubtreeCancellation: {
			"internal/adapter/runsegment", "TestCommitWaitingSubtreeCancellationRejectsRunContinuationFactDriftWithoutMutation",
		},
		boundaryRunRecovery: {
			"internal/application/runs", "TestRecoveryRejectsContinuationFactDriftWithoutProbingCheckpoint",
		},
		boundaryParkedTermination: {
			"internal/application/sessions", "TestApplyRunLostRejectsContinuationFactDriftBeforeTerminalCommit",
		},
	},
	"dropped_run_leaves_nothing_behind": {
		boundarySessionRollback: {"internal/bootstrap", "TestApplyRollbackDropsRunsAndFreesAdmission"},
		boundarySessionDelete:   {"internal/bootstrap", "TestApplyDeleteRemovesRunRows"},
	},
	"imported_session_keeps_its_identity": {
		boundarySessionImport: {"internal/delivery/server", "TestSessionExportImport_RoundTrip"},
	},
	"goal_never_outlives_its_session": {
		boundaryGoalLifecycle:   {"internal/infra/sqlite", "TestGoalStoreRejectsMissingSession"},
		boundarySessionDelete:   {"internal/bootstrap", "TestApplyDeleteClearsSessionGoal"},
		boundarySessionRollback: {"internal/bootstrap", "TestApplyRollbackDropsRunsAndFreesAdmission"},
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

// TestEveryPlanLifecycleClaimHasAFixture is contract §11.4 gate 18: the Plan
// fixtures prove that the query and the event reducer do not go backwards, that
// session and root ownership is not exceeded, and that the segment's final snapshot
// fence and the runtime invalidation both hold.
//
// The four claims are the gate's own text rather than a code declaration, because
// they are not transaction invariants — two of them are facts about a wire
// projection. What makes the list more than a checklist is the same two-way link the
// other coverage gates use: the claim names the fixture, and the fixture's doc
// comment names the claim. Three of these four had no dedicated fixture before this
// gate existed, and the cold read had none at any layer — the same blind spot that
// left plan.get with no caller on the client.
func TestEveryPlanLifecycleClaimHasAFixture(t *testing.T) {
	root := moduleRoot(t)

	for _, claim := range slices.Sorted(maps.Keys(planLifecycleFixtures)) {
		fixtures := planLifecycleFixtures[claim]
		if len(fixtures) == 0 {
			t.Errorf("state lifecycle claim %q names no fixture", claim)
			continue
		}
		for _, fixture := range fixtures {
			assertFixtureProves(t, root, fixture, claim)
		}
	}
}

// planLifecycleFixtures is the evidence index for the Plan lifecycle claims. A
// claim may need more than one fixture: "ownership" is enforced in the store that
// keeps the value and in the projection that publishes it, and a fixture at either
// layer alone would leave the other free to leak.
var planLifecycleFixtures = map[string][]fixtureRef{
	"plan_revision_never_goes_backwards": {
		{"internal/infra/sqlite", "TestPlanIsOwnedByItsSession"},
		{"internal/delivery/server", "TestPlanQueryAnswersWithTheStreamsOwnSnapshot"},
		{"internal/bootstrap", "TestApplyRollbackRepublishesBoundaryPlan"},
	},
	"session_plan_is_owned_by_its_session": {
		{"internal/infra/sqlite", "TestPlanIsOwnedByItsSession"},
		{"internal/delivery/server", "TestPlanChangeKeepsSessionScope"},
	},
	"segment_fences_its_final_plan": {
		{"internal/application/runs", "TestSegmentFencesItsFinalPlanBeforeFinishing"},
	},
	"committed_plan_change_reaches_other_windows": {
		{"internal/delivery/server", "TestPlanChangeKeepsSessionScope"},
		{"internal/application/plans", "TestCommittedPlanChangeReachesOtherWindows"},
	},
}
