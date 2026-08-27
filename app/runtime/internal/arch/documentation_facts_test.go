package arch

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/Tangerg/scope/app/runtime/protocol"
)

func TestRuntimeDocumentationFactsHaveOneVersionOwner(t *testing.T) {
	root := moduleRoot(t)
	architecture := readRuntimeDocument(t, root, "ARCHITECTURE.md")
	capabilities := readRuntimeDocument(t, root, "CAPABILITY_LEDGER.md")
	baseline := readRuntimeDocument(t, root, "CONTRACT_BASELINE.md")

	progressStatus := regexp.MustCompile(`(?m)^> 状态：P[0-9]+`)
	for name, content := range map[string][]byte{
		"ARCHITECTURE.md":      architecture,
		"CAPABILITY_LEDGER.md": capabilities,
		"CONTRACT_BASELINE.md": baseline,
	} {
		if progressStatus.Match(content) {
			t.Errorf("%s embeds mutable phase progress in a stable status header", name)
		}
	}
	if regexp.MustCompile(`(?m)^> 实施状态：`).Match(architecture) {
		t.Error("ARCHITECTURE.md embeds mutable implementation progress")
	}
	if regexp.MustCompile(`Agent Framework Baseline [0-9]+`).Match(architecture) {
		t.Error("ARCHITECTURE.md embeds a mutable Agent Framework baseline")
	}

	agentBaseline := captureDocumentFact(
		t,
		baseline,
		regexp.MustCompile(`Agent Framework[^\n]*Baseline ([0-9]+)`),
		"Agent Framework baseline",
	)
	assertDocumentFactsMatch(
		t,
		capabilities,
		regexp.MustCompile(`Agent Framework Baseline ([0-9]+)`),
		agentBaseline,
		"Agent Framework baseline",
	)

	schemaEpoch := captureDocumentFact(
		t,
		baseline,
		regexp.MustCompile("`schemaEpoch = ([0-9]+)`"),
		"SQLite schema epoch",
	)
	assertDocumentFactsMatch(
		t,
		capabilities,
		regexp.MustCompile(`epoch ([0-9]+)`),
		schemaEpoch,
		"SQLite schema epoch",
	)

	artifactVersion := captureDocumentFact(
		t,
		baseline,
		regexp.MustCompile(`Session Artifact 当前唯一版本为 ([0-9]+)`),
		"Session artifact version",
	)
	parsedArtifactVersion, err := strconv.Atoi(artifactVersion)
	if err != nil {
		t.Fatalf("parse Session artifact version %q: %v", artifactVersion, err)
	}
	if parsedArtifactVersion != protocol.SessionArtifactVersion {
		t.Errorf("documented Session artifact version = %d, runtime = %d", parsedArtifactVersion, protocol.SessionArtifactVersion)
	}
}

func TestContractBaselineDigestsMatchGeneratedArtifacts(t *testing.T) {
	root := moduleRoot(t)
	baseline := readRuntimeDocument(t, root, "CONTRACT_BASELINE.md")
	pattern := regexp.MustCompile("(?m)^\\| `contract/([^`]+)` \\| `([0-9a-f]{64})` \\|$")
	matches := pattern.FindAllSubmatch(baseline, -1)
	if len(matches) != 4 {
		t.Fatalf("contract baseline contains %d generated artifact digests, want 4", len(matches))
	}
	for _, match := range matches {
		name, want := string(match[1]), string(match[2])
		content, err := os.ReadFile(filepath.Join(root, "contract", name))
		if err != nil {
			t.Fatalf("read generated contract artifact %s: %v", name, err)
		}
		got := fmt.Sprintf("%x", sha256.Sum256(content))
		if got != want {
			t.Errorf("contract/%s digest = %s, baseline = %s", name, got, want)
		}
	}
}

func TestFrameworkExecutionVocabularyIsUnambiguous(t *testing.T) {
	root := moduleRoot(t)
	for _, name := range []string{
		"ARCHITECTURE.md",
		"DECISIONS.md",
		"ENGINEERING_STANDARDS.md",
		"EXECUTION_PLAN.md",
		"CAPABILITY_LEDGER.md",
		"CONTRACT_BASELINE.md",
	} {
		content := readRuntimeDocument(t, root, name)
		for _, forbidden := range []string{
			"原生 Interaction",
			"原生 `interaction.Definition`",
			"Agent Framework 原生",
			"native Interaction",
		} {
			if regexp.MustCompile(regexp.QuoteMeta(forbidden)).Match(content) {
				t.Errorf("%s uses ambiguous Framework execution vocabulary %q", name, forbidden)
			}
		}
	}
}

func readRuntimeDocument(t *testing.T, root, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "doc", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return content
}

func captureDocumentFact(t *testing.T, content []byte, pattern *regexp.Regexp, name string) string {
	t.Helper()
	match := pattern.FindSubmatch(content)
	if len(match) != 2 {
		t.Fatalf("contract baseline does not own %s", name)
	}
	return string(match[1])
}

func assertDocumentFactsMatch(
	t *testing.T,
	content []byte,
	pattern *regexp.Regexp,
	want string,
	name string,
) {
	t.Helper()
	matches := pattern.FindAllSubmatch(content, -1)
	if len(matches) == 0 {
		t.Fatalf("capability ledger does not record current %s", name)
	}
	for _, match := range matches {
		if len(match) != 2 || string(match[1]) != want {
			t.Errorf("capability ledger %s = %q, want %q", name, match[1], want)
		}
	}
}
