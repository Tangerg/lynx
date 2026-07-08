package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// RunScheduler starts the scheduled-run worker until ctx is canceled.
func (s *Server) RunScheduler(ctx context.Context) {
	s.scheduleWorker.RunScheduleWorker(ctx, scheduleRunner{s})
}

type scheduleRunner struct {
	server *Server
}

func (r scheduleRunner) StartScheduledRun(ctx context.Context, sc schedule.Schedule) (string, error) {
	s := r.server
	cwd := sc.Cwd
	if cwd == "" {
		cwd = s.serverInfo.Cwd
	}
	title := sc.Title
	if title == "" {
		title = "Scheduled run"
	}
	sess, err := s.sessionCreation.CreateSession(ctx, title, cwd)
	if err != nil {
		return "", err
	}

	// Drop the unused event subscription immediately; StartRun detaches the run
	// context, so the pump keeps going and persists without a subscriber.
	fireCtx, cancel := context.WithCancel(ctx)
	_, _, err = s.StartRun(fireCtx, protocol.StartRunRequest{
		SessionID: sess.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: sc.Prompt}},
		Provider:  sc.Provider,
		Model:     sc.Model,
	})
	cancel()
	if err != nil {
		return "", err
	}

	s.PublishWorkspaceEvent(protocol.WorkspaceEvent{
		Type:       protocol.WorkspaceEventSchedulesFired,
		ScheduleID: sc.ID,
	})
	return sess.ID, nil
}
