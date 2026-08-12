package terminal

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
	"github.com/Tangerg/lynx/app/cli/internal/sessiontransfer"
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
			text, err := snapshot.LastAssistantText()
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
	if err := a.requireRuntimeFeature(runtimeprofile.FeatureSessionExport); err != nil {
		return err
	}
	format, filename, err := parseExportArgument(argument)
	if err != nil {
		return err
	}
	sessionID, workspace := a.session.ID, a.session.Workspace
	title := a.session.Title
	started := runOperation(a, sessionOutputOperation, false,
		func(ctx context.Context) (sessionOutputResult, error) {
			document, err := a.transfers.ExportSession(ctx, sessiontransfer.ExportRequest{SessionID: sessionID, Format: format})
			if err != nil {
				return sessionOutputResult{}, err
			}
			path, err := a.artifacts.Publish(workspace, title, filename, document)
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

func parseExportArgument(argument string) (sessiontransfer.Format, string, error) {
	argument = strings.TrimSpace(argument)
	formatName, filename, found := strings.Cut(argument, " ")
	if !found {
		formatName, filename = argument, ""
	}
	format, err := sessiontransfer.ParseFormat(formatName)
	if err != nil {
		return "", "", err
	}
	return format, strings.TrimSpace(filename), nil
}
