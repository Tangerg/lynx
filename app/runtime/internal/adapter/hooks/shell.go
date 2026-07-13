package hooks

import (
	"bytes"
	"context"
	"errors"
	"os/exec"

	domainhooks "github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
)

// Shell executes hook commands with the host shell.
type Shell struct{}

// RunHookCommand runs req.Command via `sh -c`, feeding req.Stdin to stdin.
func (Shell) RunHookCommand(ctx context.Context, req domainhooks.CommandRequest) domainhooks.CommandResult {
	cctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, "sh", "-c", req.Command)
	cmd.Stdin = bytes.NewReader(req.Stdin)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return domainhooks.CommandResult{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.String(),
		ExitCode: exitCodeOf(err),
		Err:      err,
		TimedOut: cctx.Err() == context.DeadlineExceeded,
	}
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		return ee.ExitCode()
	}
	return -1
}
