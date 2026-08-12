package terminal

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func TestCompletionGateKeepsARejectedTokenClosedUntilItChanges(t *testing.T) {
	t.Parallel()

	query := completionQuery{
		line:  2,
		token: headless.Token{Start: 9, End: 16, Query: "archive", Trigger: headless.Trigger{Prefix: "@"}},
	}
	var gate completionGate
	if !gate.Allow(query) {
		t.Fatal("fresh completion query was suppressed")
	}
	gate.Suppress(query)
	if gate.Allow(query) {
		t.Fatal("rejected completion query reopened immediately")
	}
	if gate.Allow(query) {
		t.Fatal("rejected completion query reopened without an edit")
	}
	changed := query
	changed.token.Query += "s"
	changed.token.End++
	if !gate.Allow(changed) || !gate.Allow(query) {
		t.Fatal("editing the token did not begin a new completion query")
	}
	gate.Suppress(query)
	gate.Reset()
	if !gate.Allow(query) {
		t.Fatal("reset did not release the suppressed query")
	}
}
