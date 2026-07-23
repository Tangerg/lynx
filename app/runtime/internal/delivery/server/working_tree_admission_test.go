package server

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/sessions"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestStartRunRejectsWorkingTreeMutation(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	cwd := t.TempDir()
	ses, err := rt.sess.Create(ctx, "s", cwd)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	mutationAdmission, ok := s.sessions.(*sessions.Coordinator).ClaimWorkingTreeMutation(cwd)
	if !ok {
		t.Fatal("claim mutation")
	}
	defer mutationAdmission.Release()

	_, _, err = s.StartRun(ctx, protocol.StartRunRequest{
		SessionID: ses.ID,
		Input: []protocol.ContentBlock{{
			Type: protocol.ContentBlockText,
			Text: "hello",
		}},
	})
	if !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("start under working-tree mutation = %v, want ErrSessionBusy", err)
	}
	if testRunCoordinator(t, s).ActiveSessions()[ses.ID] {
		t.Fatal("rejected start leaked the session admission claim")
	}
}

func TestRollbackFilesRejectsWorkingTreeRunAdmission(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := context.Background()
	cwd := t.TempDir()
	ses, err := rt.sess.Create(ctx, "s", cwd)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	runAdmission, ok := s.sessions.(*sessions.Coordinator).ClaimWorkingTreeRun(cwd)
	if !ok {
		t.Fatal("claim run")
	}
	defer runAdmission.Release()

	_, err = s.RollbackSession(ctx, protocol.RollbackSessionRequest{
		SessionID:   ses.ID,
		ToRunID:     "run_1",
		RestoreType: protocol.RestoreFiles,
	})
	if !errors.Is(err, protocol.ErrSessionBusy) {
		t.Fatalf("file rollback under run admission = %v, want ErrSessionBusy", err)
	}
	if testRunCoordinator(t, s).ActiveSessions()[ses.ID] {
		t.Fatal("rejected rollback leaked the session mutation claim")
	}
}
