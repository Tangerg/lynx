package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/program"
	"github.com/mattn/go-shellwords"
)

const maxExternalDraftBytes = 4 << 20

type promptEditor interface {
	Edit(context.Context, program.Session, string, string) (string, error)
}

type draftEditor struct {
	command []string
}

func configuredDraftEditor() (*draftEditor, error) {
	configured := ""
	for _, name := range []string{"LYRA_EDITOR", "VISUAL", "EDITOR"} {
		if configured = strings.TrimSpace(os.Getenv(name)); configured != "" {
			break
		}
	}
	if configured == "" {
		configured = "vi"
	}
	command, err := shellwords.Parse(configured)
	if err != nil {
		return nil, fmt.Errorf("parse editor command: %w", err)
	}
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return nil, errors.New("editor command is empty")
	}
	return &draftEditor{command: command}, nil
}

func (e *draftEditor) Edit(ctx context.Context, session program.Session, workspace, original string) (string, error) {
	if e == nil || len(e.command) == 0 {
		return "", errors.New("external editor is unavailable")
	}
	temporary, err := os.CreateTemp("", "lyra-prompt-*.md")
	if err != nil {
		return "", fmt.Errorf("create editor draft: %w", err)
	}
	path := temporary.Name()
	defer os.Remove(path)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("protect editor draft: %w", err)
	}
	if _, err := io.WriteString(temporary, original); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("write editor draft: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("sync editor draft: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close editor draft: %w", err)
	}
	arguments := append(slices.Clone(e.command[1:]), path)
	command := exec.CommandContext(ctx, e.command[0], arguments...) //nolint:gosec // The user explicitly configures their editor command.
	command.Dir = workspace
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := session.Hand(command.Run); err != nil {
		return "", fmt.Errorf("run external editor: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open edited draft: %w", err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxExternalDraftBytes+1))
	if err != nil {
		return "", fmt.Errorf("read edited draft: %w", err)
	}
	if len(content) > maxExternalDraftBytes {
		return "", fmt.Errorf("edited draft exceeds %d bytes", maxExternalDraftBytes)
	}
	return strings.TrimRight(string(content), "\r\n"), nil
}

func (a *app) editPromptExternally() error {
	if a.conversation.Busy() || a.following || a.pendingCancel != nil {
		return errors.New("finish or cancel the active run before opening an external editor")
	}
	message, err := a.composerMessage()
	if err != nil {
		return err
	}
	edited, err := a.editor.Edit(a.ctx, a.loop.Session(), a.session.Workspace.Path, message.Text)
	if err != nil {
		return err
	}
	message.Text = edited
	a.restoreComposer(message)
	a.persistDraft()
	a.message("updated prompt from external editor")
	return nil
}
