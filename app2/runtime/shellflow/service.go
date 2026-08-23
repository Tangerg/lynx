// Package shellflow owns background command processes for one Runtime
// instance. Tool calls address jobs through stable IDs; only this service owns
// process handles, retained output, cancellation, and shutdown joining.
package shellflow

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	outputCapacity    = 256 << 10
	finishedRetention = 10 * time.Minute
)

var (
	ErrClosed   = errors.New("shellflow: service is closed")
	ErrNotFound = errors.New("shellflow: job not found")
)

type JobSnapshot struct {
	ID       string
	Output   string
	Dropped  bool
	Finished bool
	ExitCode int
	Killed   bool
	Duration time.Duration
}

type Service struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	jobs    map[string]*job
	nextID  uint64
	closed  bool
	tasks   sync.WaitGroup
	once    sync.Once
}

type job struct {
	id      string
	sessionID string
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	done    chan struct{}
	started time.Time

	mu       sync.Mutex
	buffer   []byte
	total    int
	readAt   int
	finished bool
	exitCode int
	killed   bool
	duration time.Duration
}

func New(lifetime context.Context) (*Service, error) {
	if lifetime == nil {
		return nil, errors.New("shellflow: lifetime is required")
	}
	ctx, cancel := context.WithCancel(lifetime)
	return &Service{ctx: ctx, cancel: cancel, jobs: make(map[string]*job)}, nil
}

func (service *Service) Launch(sessionID, cwd, command string, timeout time.Duration) (string, error) {
	if strings.TrimSpace(sessionID) == "" || !filepath.IsAbs(cwd) || filepath.Clean(cwd) != cwd || strings.TrimSpace(command) == "" || timeout < 0 {
		return "", errors.New("shellflow: canonical cwd, command, and non-negative timeout are required")
	}
	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return "", ErrClosed
	}
	service.nextID++
	id := "bg_" + strconv.FormatUint(service.nextID, 10)
	jobContext := service.ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		jobContext, cancel = context.WithTimeout(jobContext, timeout)
	} else {
		jobContext, cancel = context.WithCancel(jobContext)
	}
	commandProcess := shellCommand(jobContext, command)
	commandProcess.Dir = cwd
	commandProcess.WaitDelay = time.Second
	value := &job{
		id: id, sessionID: sessionID, cancel: cancel, cmd: commandProcess,
		done: make(chan struct{}), started: time.Now(), exitCode: -1,
	}
	commandProcess.Stdout = value
	commandProcess.Stderr = value
	if err := commandProcess.Start(); err != nil {
		cancel()
		service.mu.Unlock()
		return "", fmt.Errorf("shellflow: start job: %w", err)
	}
	service.jobs[id] = value
	service.tasks.Add(1)
	service.mu.Unlock()
	go service.wait(value, jobContext)
	return id, nil
}

func (service *Service) wait(value *job, jobContext context.Context) {
	defer service.tasks.Done()
	err := value.cmd.Wait()
	exitCode := 0
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			exitCode = exit.ExitCode()
		} else {
			exitCode = -1
		}
	}
	value.mu.Lock()
	value.finished = true
	value.exitCode = exitCode
	value.killed = jobContext.Err() != nil
	value.duration = time.Since(value.started)
	value.mu.Unlock()
	value.cancel()
	close(value.done)
	timer := time.NewTimer(finishedRetention)
	defer timer.Stop()
	select {
	case <-timer.C:
		service.forget(value)
	case <-service.ctx.Done():
	}
}

func (service *Service) Await(ctx context.Context, sessionID, id string, timeout time.Duration) error {
	value, err := service.job(sessionID, id)
	if err != nil {
		return err
	}
	if timeout <= 0 {
		select {
		case <-value.done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-value.done:
	case <-timer.C:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (service *Service) Read(sessionID, id string) (JobSnapshot, error) {
	value, err := service.job(sessionID, id)
	if err != nil {
		return JobSnapshot{}, err
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	start := max(value.readAt-(value.total-len(value.buffer)), 0)
	dropped := value.readAt < value.total-len(value.buffer)
	output := string(slices.Clone(value.buffer[start:]))
	value.readAt = value.total
	return JobSnapshot{
		ID: value.id, Output: output, Dropped: dropped,
		Finished: value.finished, ExitCode: value.exitCode,
		Killed: value.killed, Duration: value.duration,
	}, nil
}

func (service *Service) Stop(sessionID, id string) (bool, error) {
	value, err := service.job(sessionID, id)
	if err != nil {
		return false, err
	}
	value.mu.Lock()
	running := !value.finished
	value.mu.Unlock()
	if running {
		value.cancel()
	}
	return running, nil
}

func (service *Service) Forget(sessionID, id string) {
	value, err := service.job(sessionID, id)
	if err == nil {
		service.forget(value)
	}
}

func (service *Service) Running(sessionID string) []string {
	service.mu.Lock()
	values := make([]*job, 0, len(service.jobs))
	for _, value := range service.jobs {
		if sessionID == "" || value.sessionID == sessionID {
			values = append(values, value)
		}
	}
	service.mu.Unlock()
	result := make([]string, 0, len(values))
	for _, value := range values {
		value.mu.Lock()
		if !value.finished {
			result = append(result, value.id)
		}
		value.mu.Unlock()
	}
	slices.Sort(result)
	return result
}

func (service *Service) job(sessionID, id string) (*job, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	value, found := service.jobs[id]
	if !found || value.sessionID != sessionID {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return value, nil
}

func (service *Service) forget(value *job) {
	service.mu.Lock()
	if service.jobs[value.id] == value {
		delete(service.jobs, value.id)
	}
	service.mu.Unlock()
}

func (service *Service) Close() {
	if service == nil {
		return
	}
	service.once.Do(func() {
		service.mu.Lock()
		service.closed = true
		service.mu.Unlock()
		service.cancel()
		service.tasks.Wait()
		service.mu.Lock()
		service.jobs = make(map[string]*job)
		service.mu.Unlock()
	})
}

func (value *job) Write(contents []byte) (int, error) {
	value.mu.Lock()
	defer value.mu.Unlock()
	value.total += len(contents)
	value.buffer = append(value.buffer, contents...)
	if len(value.buffer) > outputCapacity {
		value.buffer = slices.Clone(value.buffer[len(value.buffer)-outputCapacity:])
	}
	return len(contents), nil
}
