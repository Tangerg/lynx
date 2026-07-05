package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/pkg/mime"

	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

func TestPlanTurnStartCreatesSessionForColdStart(t *testing.T) {
	store := &sessionRuntimeStore{}
	rt := &Runtime{session: store}

	sess, req, err := rt.PlanTurnStart(context.Background(), "", "/repo", turn.StartTurnRequest{Message: "hello", MaxSteps: 3})
	if err != nil {
		t.Fatalf("PlanTurnStart: %v", err)
	}
	if sess.ID != "ses_created" || store.createCwd != "/repo" {
		t.Fatalf("session = %+v createCwd=%q, want created in /repo", sess, store.createCwd)
	}
	if req.SessionID != "ses_created" || req.Cwd != "/repo" || req.Message != "hello" || req.MaxSteps != 3 {
		t.Fatalf("request = %+v, want bound turn draft", req)
	}
}

func TestPlanTurnStartBindsExistingSession(t *testing.T) {
	store := &sessionRuntimeStore{}
	rt := &Runtime{session: store}

	sess, req, err := rt.PlanTurnStart(context.Background(), "ses_1", "/ignored", turn.StartTurnRequest{Message: "hello"})
	if err != nil {
		t.Fatalf("PlanTurnStart: %v", err)
	}
	if store.getID != "ses_1" || sess.ID != "ses_1" {
		t.Fatalf("getID=%q session=%+v", store.getID, sess)
	}
	if req.SessionID != "ses_1" || req.Cwd != "/repo" {
		t.Fatalf("request = %+v, want existing session binding", req)
	}
}

func TestPlanTurnStartOwnsSessionBinding(t *testing.T) {
	store := &sessionRuntimeStore{}
	rt := &Runtime{session: store}

	_, req, err := rt.PlanTurnStart(context.Background(), "ses_1", "/ignored", turn.StartTurnRequest{
		SessionID: "ses_stale",
		Cwd:       "/stale",
		Message:   "hello",
	})
	if err != nil {
		t.Fatalf("PlanTurnStart: %v", err)
	}
	if req.SessionID != "ses_1" || req.Cwd != "/repo" {
		t.Fatalf("request = %+v, want runtime-owned session binding", req)
	}
}

func TestPlanTurnStartRejectsInvalidDraftBeforeCreatingSession(t *testing.T) {
	store := &sessionRuntimeStore{}
	rt := &Runtime{session: store}

	if _, _, err := rt.PlanTurnStart(context.Background(), "", "/repo", turn.StartTurnRequest{}); !errors.Is(err, turn.ErrInputRequired) {
		t.Fatalf("empty input err = %v, want ErrInputRequired", err)
	}
	if store.createCwd != "" {
		t.Fatalf("empty input created session in %q", store.createCwd)
	}

	if _, _, err := rt.PlanTurnStart(context.Background(), "", "/repo", turn.StartTurnRequest{Message: "hello", Provider: "anthropic"}); !errors.Is(err, turn.ErrIncompleteModelSelection) {
		t.Fatalf("partial model err = %v, want ErrIncompleteModelSelection", err)
	}
}

func TestPlanTurnStartRejectsUnsupportedMediaForKnownModel(t *testing.T) {
	store := &sessionRuntimeStore{}
	rt := &Runtime{session: store}
	img := testImage(t)

	_, _, err := rt.PlanTurnStart(context.Background(), "ses_1", "/repo", turn.StartTurnRequest{
		Message:  "describe",
		Media:    []*media.Media{img},
		Provider: "openai",
		Model:    "gpt-3.5-turbo",
	})
	if !errors.Is(err, turn.ErrUnsupportedMedia) {
		t.Fatalf("image err = %v, want ErrUnsupportedMedia", err)
	}
	if store.getID != "" {
		t.Fatalf("unsupported media looked up session %q before rejecting", store.getID)
	}
}

func testImage(t *testing.T) *media.Media {
	t.Helper()
	mt, err := mime.Parse("image/png")
	if err != nil {
		t.Fatalf("parse mime: %v", err)
	}
	img, err := media.NewMedia(mt, "iVBORw0KGgo=")
	if err != nil {
		t.Fatalf("new media: %v", err)
	}
	return img
}
