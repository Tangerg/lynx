package settingsflow

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

type SessionCreator interface {
	Create(context.Context, protocol.CreateSessionRequest) (*protocol.Session, error)
}

type RunStarter interface {
	Start(context.Context, runflow.StartCommand) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error)
}

type Launcher struct {
	sessions SessionCreator
	runs RunStarter
}

func NewLauncher(sessions SessionCreator, runs RunStarter) *Launcher {
	return &Launcher{sessions: sessions, runs: runs}
}

func (launcher *Launcher) RunSchedule(ctx context.Context, schedule protocol.Schedule) (string, string, error) {
	created, err := launcher.sessions.Create(ctx, protocol.CreateSessionRequest{
		Workspace: schedule.Workspace, Title: schedule.Title,
	})
	if err != nil {
		return "", "", err
	}
	started, _, err := launcher.runs.Start(ctx, runflow.StartCommand{Request: protocol.StartRunRequest{
		SessionID: created.ID,
		Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: schedule.Instructions}},
		Provider: schedule.Provider, Model: schedule.Model,
	}})
	if err != nil {
		return created.ID, "", err
	}
	return created.ID, started.RunID, nil
}
