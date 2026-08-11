package terminal

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/sessionexport"
)

type sessionOutputResult struct {
	sessionID string
	value     string
}

func (a *app) copyLastAssistant() error {
	sessionID := a.session.ID
	started := runOperation(a, sessionOutputOperation, false,
		func(ctx context.Context) (sessionOutputResult, error) {
			snapshot, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return sessionOutputResult{}, err
			}
			text, err := sessionexport.LastAssistantText(snapshot)
			return sessionOutputResult{sessionID: sessionID, value: text}, err
		},
		func(result sessionOutputResult, err error) {
			if err != nil {
				a.message("copy last response failed: " + err.Error())
				return
			}
			if a.session.ID != result.sessionID {
				a.message("copy canceled because the active session changed")
				return
			}
			if !a.loop.Clipboard().Copy(result.value) {
				a.message("the terminal host does not provide a clipboard")
				return
			}
			a.message("copied the last assistant response")
		},
	)
	if !started {
		return errors.New("another session output operation is already running")
	}
	return nil
}

func (a *app) exportSession(argument string) error {
	format, filename, err := parseExportArgument(argument)
	if err != nil {
		return err
	}
	sessionID, workspace := a.session.ID, a.session.Workspace
	started := runOperation(a, sessionOutputOperation, false,
		func(ctx context.Context) (sessionOutputResult, error) {
			snapshot, err := a.runtime.GetSession(ctx, sessionID)
			if err != nil {
				return sessionOutputResult{}, err
			}
			report, err := sessionexport.New(snapshot, format)
			if err != nil {
				return sessionOutputResult{}, err
			}
			path, err := report.Save(workspace, filename)
			return sessionOutputResult{sessionID: sessionID, value: path}, err
		},
		func(result sessionOutputResult, err error) {
			if err != nil {
				a.message("export session failed: " + err.Error())
				return
			}
			a.message("exported session · " + result.value)
		},
	)
	if !started {
		return errors.New("another session output operation is already running")
	}
	return nil
}

func parseExportArgument(argument string) (sessionexport.Format, string, error) {
	argument = strings.TrimSpace(argument)
	formatName, filename, found := strings.Cut(argument, " ")
	if !found {
		formatName, filename = argument, ""
	}
	format, err := sessionexport.ParseFormat(formatName)
	if err != nil {
		return "", "", err
	}
	return format, strings.TrimSpace(filename), nil
}
