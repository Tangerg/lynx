// Package exec is the background-process mechanism: it runs shell commands
// detached from the calling turn, buffers their output in a bounded ring so
// the model can read it incrementally, and kills them on demand. It is pure
// infra — no domain knowledge, no upward dependency.
//
// Every command the engine's bash tool runs starts here as a detached job:
// the foreground path races the command's completion ([Shell.Done]) against an
// auto-background window, removing the job ([Manager.Remove]) if it finishes in
// time and otherwise leaving it running and addressable by its shell id. So one
// mechanism backs both the synchronous bash result and the bash_output /
// kill_shell tools — the auto-background design.
//
// No PTY: plain pipes into a bounded ring buffer. Cross-platform with
// no platform-specific features — kill is a plain process kill, so a command
// that itself forks grandchildren may leave them, acceptable for the local
// single-user runtime.
package exec

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// maxBuffer caps a background shell's retained output; once exceeded the
// oldest bytes are dropped (a poll that fell behind is told output was lost).
const maxBuffer = 256 * 1024

// Manager runs shell commands in the background and lets callers poll their
// output and stop them — the long-running-command counterpart to a synchronous
// bash executor. The process handles + output buffers live here. The zero
// value is not usable; build one with [NewManager].
type Manager struct {
	mu     sync.Mutex
	nextID int
	shells map[string]*Shell
}

func NewManager() *Manager {
	return &Manager{shells: map[string]*Shell{}}
}

// Shell is one background process: its handle, the tail of its combined
// stdout+stderr (capped), and its completion state. Read its output with
// [Shell.Read], wait for it with [Shell.Done], inspect its terminal state with
// [Shell.Status] / [Shell.Outcome]; the [Manager] owns its lifecycle.
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

// Launch starts command under cwd in the background, detached from any
// tool-call context so it outlives the turn, and returns its shell id. A
// positive timeout hard-kills the command when it elapses (0 = no hard
// timeout; the command runs until it exits or is killed).
func (m *Manager) Launch(cwd, command string, timeout time.Duration) string {
	runCtx, cancel := context.WithCancel(context.Background())
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(context.Background(), timeout)
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

	m.mu.Lock()
	m.nextID++
	id := "bg_" + strconv.Itoa(m.nextID)
	m.shells[id] = sh
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		sh.finish("start failed: "+err.Error(), -1, false)
		return id
	}
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
	return id
}

// Get returns the shell with id and whether it exists.
func (m *Manager) Get(id string) (*Shell, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sh, ok := m.shells[id]
	return sh, ok
}

// Kill stops a background shell; reports whether it was still running and
// whether it existed.
func (m *Manager) Kill(id string) (running, ok bool) {
	sh, ok := m.Get(id)
	if !ok {
		return false, false
	}
	sh.mu.Lock()
	running = !sh.finished
	sh.mu.Unlock()
	if running {
		sh.cancel()
		_ = sh.cmd.Process.Kill()
	}
	return running, true
}

// Remove drops a shell from the manager without killing it. The foreground
// bash race calls it once a command completes within the auto-background
// window, so a finished command isn't left behind as a phantom background job.
// Killing instead would cancel the already-exited process context needlessly.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	delete(m.shells, id)
	m.mu.Unlock()
}

// KillAll stops every background shell — called on engine shutdown.
func (m *Manager) KillAll() {
	m.mu.Lock()
	shells := m.shells
	m.shells = map[string]*Shell{}
	m.mu.Unlock()
	for _, sh := range shells {
		sh.cancel()
		if sh.cmd.Process != nil {
			_ = sh.cmd.Process.Kill()
		}
	}
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

// Done is closed when the process finishes — the foreground bash race selects
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
