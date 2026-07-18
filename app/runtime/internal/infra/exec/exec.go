// Package exec is the background-process mechanism: it runs shell commands
// detached from the calling turn, buffers their output in a bounded ring so
// the model can read it incrementally, and kills them on demand. It is pure
// infra — no domain knowledge, no upward dependency.
//
// Every command the engine's shell tool runs starts here as a detached job:
// the foreground path races the command's completion ([Shell.Done]) against an
// auto-background window, removing the job ([Shells.Remove]) if it finishes in
// time and otherwise leaving it running and addressable by its shell id. So one
// mechanism backs both the synchronous shell result and the shell_output /
// shell_kill tools — the auto-background design.
//
// No PTY: plain pipes into a bounded ring buffer. Cross-platform with
// no platform-specific features — kill is a plain process kill, so a command
// that itself forks grandchildren may leave them, acceptable for the local
// single-user runtime.
package exec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"time"
)

// maxBuffer caps a background shell's retained output; once exceeded the
// oldest bytes are dropped (a poll that fell behind is told output was lost).
const maxBuffer = 256 * 1024

// Shells owns background shell commands and lets callers poll their output or
// stop them. The process handles and output buffers live here. The zero value is
// not usable; build one with [NewShells].
type Shells struct {
	mu        sync.Mutex
	nextID    int
	shells    map[string]*Shell
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

var (
	// ErrShellsClosed reports a launch attempted after the shell owner shut down.
	ErrShellsClosed = errors.New("exec: shells closed")
	// ErrShellNotFound reports a command addressed outside this owner's shell set.
	ErrShellNotFound = errors.New("exec: shell not found")
)

// NewShells creates an empty background-shell set.
func NewShells() *Shells {
	return &Shells{shells: map[string]*Shell{}}
}

// Shell is one background process: its handle, the tail of its combined
// stdout+stderr (capped), and its completion state. Read its output with
// [Shell.Read], wait for it with [Shell.Done], inspect its terminal state with
// [Shell.Status] / [Shell.Outcome]; the [Shells] set owns its lifecycle.
type Shell struct {
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	started time.Time
	done    chan struct{} // closed once the process finishes

	mu       sync.Mutex
	buf      []byte // tail of stdout+stderr, capped at maxBuffer
	total    int    // absolute bytes ever written (buf holds the last len(buf))
	readPos  int    // absolute offset already returned to the caller
	finished bool
	exitInfo string        // "exit 0" / "exit 2" / "signal: killed" — set on completion
	exitCode int           // process exit code; -1 when it never ran / wasn't an exit
	killed   bool          // terminated by ctx/timeout/kill rather than exiting on its own
	duration time.Duration // wall time from launch to completion
}

// Launch starts command under cwd in the background and returns its shell id.
// It is detached from the tool-call's CANCELLATION so it outlives the turn —
// via context.WithoutCancel(ctx), which drops cancellation but KEEPS ctx's
// values, so the launching turn's trace span still propagates (full-link)
// rather than being severed by a bare context.Background(). A positive timeout
// hard-kills the command when it elapses (0 = no hard timeout; the command
// runs until it exits or is killed).
func (s *Shells) Launch(ctx context.Context, cwd, command string, timeout time.Duration) (string, error) {
	base := context.WithoutCancel(ctx)
	runCtx, cancel := context.WithCancel(base)
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(base, timeout)
	}
	cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", command)
	cmd.Dir = cwd
	// On kill/timeout, force-close the pipes shortly after so the Wait goroutine
	// (and thus Done) returns promptly even when a child the shell spawned still
	// holds them — otherwise Wait blocks until that child exits.
	cmd.WaitDelay = time.Second
	sh := &Shell{cancel: cancel, cmd: cmd, started: time.Now(), done: make(chan struct{})}
	cmd.Stdout = sh
	cmd.Stderr = sh

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		return "", ErrShellsClosed
	}
	s.nextID++
	id := "bg_" + strconv.Itoa(s.nextID)
	// Start while holding the owner lock so shutdown cannot observe a Shell
	// whose exec.Cmd is only partly initialized. Once the shell is published,
	// cmd.Process is immutable and Kill/KillAll may safely use it.
	startErr := cmd.Start()
	if startErr != nil {
		cancel()
		sh.finish("start failed: "+startErr.Error(), -1, false)
		s.shells[id] = sh
		s.mu.Unlock()
		return id, nil
	}
	s.shells[id] = sh
	s.mu.Unlock()
	go func() {
		err := cmd.Wait()
		killed := runCtx.Err() != nil // ctx done = timeout or an explicit Kill
		cancel()
		code, info := 0, "exit 0"
		if err != nil {
			if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
				code = exitErr.ExitCode()
				info = "exit " + strconv.Itoa(code)
			} else {
				code, info = -1, err.Error()
			}
		}
		sh.finish(info, code, killed)
	}()
	return id, nil
}

// Get returns the shell with id and whether it exists.
func (s *Shells) Get(id string) (*Shell, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sh, ok := s.shells[id]
	return sh, ok
}

// Kill stops a background shell and reports whether it was still running.
// Missing ids have the stable [ErrShellNotFound] identity. A process that exits
// between the state snapshot and the kill is an idempotent success.
func (s *Shells) Kill(id string) (running bool, err error) {
	sh, ok := s.Get(id)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrShellNotFound, id)
	}
	sh.mu.Lock()
	running = !sh.finished
	sh.mu.Unlock()
	if !running {
		return false, nil
	}
	sh.cancel()
	if sh.cmd.Process == nil {
		return true, fmt.Errorf("exec: kill shell %q: process is unavailable", id)
	}
	if err := sh.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return true, fmt.Errorf("exec: kill shell %q: %w", id, err)
	}
	return true, nil
}

// Remove drops a shell from the set without killing it. The foreground
// shell race calls it once a command completes within the auto-background
// window, so a finished command isn't left behind as a phantom background job.
// Killing instead would cancel the already-exited process context needlessly.
func (s *Shells) Remove(id string) {
	s.mu.Lock()
	delete(s.shells, id)
	s.mu.Unlock()
}

// KillAll stops and joins every background shell in stable id order. It keeps
// every process-kill failure while still joining the complete set. Safe to call
// repeatedly; subsequent calls return the original shutdown result.
func (s *Shells) KillAll() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		shells := s.shells
		s.shells = map[string]*Shell{}
		s.mu.Unlock()
		ids := make([]string, 0, len(shells))
		for id := range shells {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		var errs []error
		for _, id := range ids {
			sh := shells[id]
			sh.cancel()
			if sh.cmd.Process != nil {
				if err := sh.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					errs = append(errs, fmt.Errorf("exec: kill shell %q during shutdown: %w", id, err))
				}
			}
		}
		for _, id := range ids {
			<-shells[id].done
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}

func (s *Shell) finish(info string, code int, killed bool) {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.exitInfo = info
	s.exitCode = code
	s.killed = killed
	s.duration = time.Since(s.started)
	s.mu.Unlock()
	close(s.done)
}

// Done is closed when the process finishes — the foreground shell race selects
// on it to detect completion without polling.
func (s *Shell) Done() <-chan struct{} { return s.done }

// Outcome reports a finished shell's exit code, whether it was killed
// (timeout / explicit kill) rather than exiting on its own, and its wall-clock
// duration. Meaningful only after [Shell.Done] is closed.
func (s *Shell) Outcome() (exitCode int, killed bool, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode, s.killed, s.duration
}

// Read returns the output not yet returned to the caller and whether earlier
// output had to be dropped (the buffer overflowed before this poll).
func (s *Shell) Read() (out string, dropped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bufStart := s.total - len(s.buf)
	if s.readPos < bufStart {
		dropped = true
		s.readPos = bufStart
	}
	out = string(s.buf[s.readPos-bufStart:])
	s.readPos = s.total
	return out, dropped
}

// Status reports whether the shell finished and its exit info.
func (s *Shell) Status() (done bool, info string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.finished, s.exitInfo
}

// Write funnels the shell's stdout/stderr into its capped ring buffer (the
// process's Stdout/Stderr point straight at the Shell). On overflow the oldest
// bytes are dropped — a poll that fell behind learns so via [Shell.Read].
func (s *Shell) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total += len(p)
	s.buf = append(s.buf, p...)
	if len(s.buf) > maxBuffer {
		s.buf = s.buf[len(s.buf)-maxBuffer:]
	}
	return len(p), nil
}
