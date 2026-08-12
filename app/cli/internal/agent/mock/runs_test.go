package mock

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

func TestRunCatalogReadsFiltersAndPaginatesNewestFirst(t *testing.T) {
	runtime := New()
	runtime.Script = func(string) Script {
		return Script{Prelude: []Step{{
			Delay: time.Hour,
			Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}},
		}}}
	}
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{
		SessionID: "ses_demo_1", Message: agent.Message{Text: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := runtime.GetRun(t.Context(), opened.RunID)
	if err != nil || got.Status != agent.RunStatusRunning {
		t.Fatalf("GetRun = %+v, %v", got, err)
	}
	page, err := runtime.ListRuns(t.Context(), agent.RunQuery{SessionID: "ses_demo_1", Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != opened.RunID || page.NextCursor == "" {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	next, err := runtime.ListRuns(t.Context(), agent.RunQuery{SessionID: "ses_demo_1", Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != "run_demo_history" || next.NextCursor != "" {
		t.Fatalf("second page = %+v, %v", next, err)
	}
	waiting, err := runtime.ListRuns(t.Context(), agent.RunQuery{Statuses: []agent.RunStatus{agent.RunStatusWaiting}})
	if err != nil || len(waiting.Items) != 0 {
		t.Fatalf("waiting page = %+v, %v", waiting, err)
	}
	if _, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID}); err != nil {
		t.Fatal(err)
	}
}

func TestRunCatalogDoesNotRetainDeletedSessionRuns(t *testing.T) {
	runtime := New()
	if err := runtime.DeleteSession(t.Context(), agent.DeleteSession{SessionID: "ses_demo_1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.GetRun(t.Context(), "run_demo_history"); !errors.Is(err, agent.ErrRunNotFound) {
		t.Fatalf("GetRun after session deletion = %v", err)
	}
	page, err := runtime.ListRuns(t.Context(), agent.RunQuery{SessionID: "ses_demo_1"})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("ListRuns after session deletion = %+v, %v", page, err)
	}
}
