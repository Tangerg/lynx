// Package supervisor owns the independent local Runtime process used by Desktop.
package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/httptransport"
	"github.com/Tangerg/lynx/app2/runtime/localruntime"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

var ErrUnavailable = errors.New("local Runtime is unavailable")

type Config struct {
	RuntimeBinary      string
	DataHome           string
	DefaultWorkspace   string
	UserHome           string
	CORSOrigins        []string
	StartupTimeout     time.Duration
	ProbeTimeout       time.Duration
	ShutdownTimeout    time.Duration
	RestartBackoff     time.Duration
	MaxRestartBackoff  time.Duration
	MaxStartupAttempts int
}

type Connection struct {
	Endpoint             string `json:"endpoint"`
	LocalToken           string `json:"localToken"`
	InstanceID           string `json:"instanceId"`
	ProtocolVersion      string `json:"protocolVersion"`
	IdempotencyNamespace string `json:"idempotencyNamespace"`
	Generation           uint64 `json:"generation"`

	process *os.Process
}

func (connection Connection) redacted() Connection {
	connection.LocalToken = "[redacted]"
	return connection
}

type Supervisor struct {
	config Config

	mu         sync.RWMutex
	started    bool
	connection *Connection
	lastError  error
	root       string

	stop       chan struct{}
	done       chan struct{}
	stopOnce   sync.Once
	finishOnce sync.Once
}

type startResult struct {
	connection Connection
	err        error
}

type generation struct {
	connection Connection
	root       string
	command    *exec.Cmd
	waited     chan error
	logs       *boundedLog
}

func New(config Config) (*Supervisor, error) {
	config = withDefaults(config)
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	return &Supervisor{config: config, stop: make(chan struct{}), done: make(chan struct{})}, nil
}

func (supervisor *Supervisor) Start(ctx context.Context) (Connection, error) {
	if ctx == nil {
		return Connection{}, errors.New("supervisor: context is required")
	}
	supervisor.mu.Lock()
	if supervisor.started {
		supervisor.mu.Unlock()
		return Connection{}, errors.New("supervisor: Start may be called only once")
	}
	select {
	case <-supervisor.stop:
		supervisor.mu.Unlock()
		return Connection{}, errors.New("supervisor: already closed")
	default:
	}
	supervisor.started = true
	supervisor.mu.Unlock()

	first := make(chan startResult, 1)
	go supervisor.run(first)
	select {
	case result := <-first:
		return result.connection, result.err
	case <-ctx.Done():
		_ = supervisor.Close(context.Background())
		return Connection{}, ctx.Err()
	}
}

func (supervisor *Supervisor) Connection() (Connection, error) {
	supervisor.mu.RLock()
	defer supervisor.mu.RUnlock()
	if supervisor.connection == nil {
		if supervisor.lastError != nil {
			return Connection{}, fmt.Errorf("%w: %v", ErrUnavailable, supervisor.lastError)
		}
		return Connection{}, ErrUnavailable
	}
	return *supervisor.connection, nil
}

func (supervisor *Supervisor) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("supervisor: context is required")
	}
	supervisor.stopOnce.Do(func() {
		close(supervisor.stop)
		supervisor.mu.RLock()
		started := supervisor.started
		supervisor.mu.RUnlock()
		if !started {
			supervisor.finish()
		}
	})
	select {
	case <-supervisor.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (supervisor *Supervisor) run(first chan<- startResult) {
	defer supervisor.finish()
	var generationNumber uint64
	attempts := 0
	backoff := supervisor.config.RestartBackoff
	firstPending := true
	for {
		select {
		case <-supervisor.stop:
			if firstPending {
				first <- startResult{err: errors.New("supervisor: closed before Runtime became ready")}
			}
			return
		default:
		}

		generationNumber++
		active, err := supervisor.startGeneration(generationNumber)
		if err != nil {
			attempts++
			supervisor.setUnavailable(err, "")
			if firstPending && attempts >= supervisor.config.MaxStartupAttempts {
				first <- startResult{err: err}
				return
			}
			if !waitFor(supervisor.stop, backoff) {
				if firstPending {
					first <- startResult{err: errors.New("supervisor: closed during Runtime startup")}
				}
				return
			}
			backoff = min(backoff*2, supervisor.config.MaxRestartBackoff)
			continue
		}
		attempts = 0
		backoff = supervisor.config.RestartBackoff
		supervisor.setConnection(active.connection, active.root)
		if firstPending {
			first <- startResult{connection: active.connection}
			firstPending = false
		}

		select {
		case <-supervisor.stop:
			supervisor.setUnavailable(nil, active.root)
			_ = supervisor.stopGeneration(active)
			return
		case err := <-active.waited:
			detail := fmt.Errorf("runtime generation %d exited unexpectedly: %w; logs: %s", generationNumber, err, active.logs.String())
			_ = cleanupGenerationRoot(active.root)
			supervisor.setUnavailable(detail, "")
		}
		if !waitFor(supervisor.stop, backoff) {
			return
		}
		backoff = min(backoff*2, supervisor.config.MaxRestartBackoff)
	}
}

func (supervisor *Supervisor) startGeneration(number uint64) (*generation, error) {
	root, err := os.MkdirTemp("", "lyra-app2-runtime-")
	if err != nil {
		return nil, fmt.Errorf("supervisor: create generation root: %w", err)
	}
	supervisor.setGenerationRoot(root)
	failed := true
	defer func() {
		if failed {
			_ = cleanupGenerationRoot(root)
			supervisor.setGenerationRoot("")
		}
	}()
	nonce, err := localruntime.NewNonce()
	if err != nil {
		return nil, err
	}
	descriptorPath := filepath.Join(root, "ready.json")
	tokenPath := filepath.Join(root, "token")
	arguments := []string{
		"serve", "--listen", "127.0.0.1:0",
		"--data-home", supervisor.config.DataHome,
		"--database-path", filepath.Join(supervisor.config.DataHome, "runtime.sqlite"),
		"--token-path", tokenPath,
		"--bootstrap-descriptor", descriptorPath,
		"--bootstrap-nonce", nonce,
		"--workspace", supervisor.config.DefaultWorkspace,
		"--user-home", supervisor.config.UserHome,
	}
	for _, origin := range supervisor.config.CORSOrigins {
		arguments = append(arguments, "--cors-origin", origin)
	}
	logs := newBoundedLog(64 << 10)
	command := exec.Command(supervisor.config.RuntimeBinary, arguments...)
	command.Stdout = logs
	command.Stderr = logs
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("supervisor: start Runtime: %w", err)
	}
	active := &generation{root: root, command: command, waited: make(chan error, 1), logs: logs}
	go func() { active.waited <- command.Wait() }()

	descriptor, err := supervisor.awaitDescriptor(active, descriptorPath, localruntime.Expectation{
		Root: root, Nonce: nonce, PID: command.Process.Pid, ProtocolVersion: protocol.ProtocolVersion,
	})
	if err != nil {
		_ = supervisor.stopGeneration(active)
		return nil, err
	}
	connection, err := supervisor.verifyConnection(number, command.Process, descriptor)
	if err != nil {
		_ = supervisor.stopGeneration(active)
		return nil, err
	}
	active.connection = connection
	failed = false
	return active, nil
}

func (supervisor *Supervisor) awaitDescriptor(
	active *generation,
	path string,
	expectation localruntime.Expectation,
) (localruntime.Descriptor, error) {
	timer := time.NewTimer(supervisor.config.StartupTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		descriptor, err := localruntime.Consume(path, expectation)
		if err == nil {
			return descriptor, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return localruntime.Descriptor{}, fmt.Errorf("supervisor: consume descriptor: %w", err)
		}
		select {
		case exitErr := <-active.waited:
			active.waited <- exitErr
			return localruntime.Descriptor{}, fmt.Errorf("supervisor: Runtime exited before ready: %w; logs: %s", exitErr, active.logs.String())
		case <-supervisor.stop:
			return localruntime.Descriptor{}, errors.New("supervisor: closed while waiting for Runtime")
		case <-timer.C:
			return localruntime.Descriptor{}, errors.New("supervisor: timed out waiting for Runtime descriptor")
		case <-ticker.C:
		}
	}
}

func (supervisor *Supervisor) verifyConnection(number uint64, process *os.Process, descriptor localruntime.Descriptor) (Connection, error) {
	token, err := localruntime.ReadToken(descriptor.TokenPath)
	if err != nil {
		return Connection{}, fmt.Errorf("supervisor: read Runtime token: %w", err)
	}
	transport := &http.Transport{
		Proxy:       nil,
		DialContext: (&net.Dialer{Timeout: supervisor.config.ProbeTimeout}).DialContext,
	}
	client := &http.Client{
		Transport:     transport,
		Timeout:       supervisor.config.ProbeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	var info httptransport.RuntimeInfo
	if err := getJSON(client, descriptor.BaseURL+httptransport.PathInfo, "", nil, &info); err != nil {
		return Connection{}, fmt.Errorf("supervisor: probe Runtime info: %w", err)
	}
	var live httptransport.LivenessStatus
	if err := getJSON(client, descriptor.BaseURL+httptransport.PathLiveness, "", nil, &live); err != nil {
		return Connection{}, fmt.Errorf("supervisor: probe Runtime liveness: %w", err)
	}
	var ready httptransport.ReadinessStatus
	if err := getJSON(client, descriptor.BaseURL+httptransport.PathReadiness, "", nil, &ready); err != nil {
		return Connection{}, fmt.Errorf("supervisor: probe Runtime readiness: %w", err)
	}
	var rpcResponse struct {
		JSONRPC string                    `json:"jsonrpc"`
		ID      string                    `json:"id"`
		Result  protocol.DiscoverResponse `json:"result"`
		Error   json.RawMessage           `json:"error,omitempty"`
	}
	body := []byte(`{"jsonrpc":"2.0","id":"desktop-bootstrap","method":"runtime.discover","params":{"_meta":{"protocolVersion":"` + protocol.ProtocolVersion + `"}}}`)
	if err := getJSON(client, descriptor.BaseURL+httptransport.PathRPC, token, body, &rpcResponse); err != nil {
		return Connection{}, fmt.Errorf("supervisor: discover Runtime: %w", err)
	}
	if len(rpcResponse.Error) != 0 {
		return Connection{}, errors.New("supervisor: runtime.discover returned an error")
	}
	if err := rpcResponse.Result.Validate(); err != nil {
		return Connection{}, fmt.Errorf("supervisor: validate runtime.discover: %w", err)
	}
	instanceID := descriptor.InstanceID
	if descriptor.ProtocolVersion != protocol.ProtocolVersion || info.ProtocolVersion != protocol.ProtocolVersion || rpcResponse.Result.ProtocolVersion != protocol.ProtocolVersion ||
		info.Server.InstanceID != instanceID || live.InstanceID != instanceID || ready.InstanceID != instanceID || rpcResponse.Result.ServerInfo.InstanceID != instanceID ||
		live.Status != httptransport.HealthOK || ready.Status != httptransport.HealthOK || info.Endpoints.RPC != httptransport.PathRPC {
		return Connection{}, errors.New("supervisor: Runtime bootstrap identities or readiness do not agree")
	}
	return Connection{
		Endpoint: descriptor.BaseURL, LocalToken: token, InstanceID: instanceID,
		ProtocolVersion:      protocol.ProtocolVersion,
		IdempotencyNamespace: rpcResponse.Result.Capabilities.Limits.Idempotency.Namespace,
		Generation:           number, process: process,
	}, nil
}

func getJSON(client *http.Client, endpoint, token string, body []byte, target any) error {
	method := http.MethodGet
	reader := bytes.NewReader(nil)
	if body != nil {
		method = http.MethodPost
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %d", response.StatusCode)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("response contains trailing JSON")
	}
	return nil
}

func (supervisor *Supervisor) stopGeneration(active *generation) error {
	if active == nil {
		return nil
	}
	_ = active.command.Process.Signal(os.Interrupt)
	timer := time.NewTimer(supervisor.config.ShutdownTimeout)
	defer timer.Stop()
	var waitErr error
	select {
	case waitErr = <-active.waited:
	case <-timer.C:
		killErr := active.command.Process.Kill()
		waitErr = errors.Join(killErr, <-active.waited)
	}
	rootErr := cleanupGenerationRoot(active.root)
	supervisor.setGenerationRoot("")
	if exitError := (*exec.ExitError)(nil); errors.As(waitErr, &exitError) && exitError.ProcessState.Exited() {
		waitErr = nil
	}
	return errors.Join(waitErr, rootErr)
}

func (supervisor *Supervisor) setConnection(connection Connection, root string) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	supervisor.connection = &connection
	supervisor.lastError = nil
	supervisor.root = root
}

func (supervisor *Supervisor) setUnavailable(err error, root string) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	supervisor.connection = nil
	supervisor.lastError = err
	supervisor.root = root
}

func (supervisor *Supervisor) setGenerationRoot(root string) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	supervisor.root = root
}

func (supervisor *Supervisor) generationRoot() string {
	supervisor.mu.RLock()
	defer supervisor.mu.RUnlock()
	return supervisor.root
}

func (supervisor *Supervisor) finish() {
	supervisor.finishOnce.Do(func() { close(supervisor.done) })
}

func withDefaults(config Config) Config {
	if len(config.CORSOrigins) == 0 {
		config.CORSOrigins = httptransport.DefaultCORSOrigins()
	}
	if config.StartupTimeout <= 0 {
		config.StartupTimeout = 10 * time.Second
	}
	if config.ProbeTimeout <= 0 {
		config.ProbeTimeout = 2 * time.Second
	}
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = 5 * time.Second
	}
	if config.RestartBackoff <= 0 {
		config.RestartBackoff = 100 * time.Millisecond
	}
	if config.MaxRestartBackoff <= 0 {
		config.MaxRestartBackoff = 2 * time.Second
	}
	if config.MaxStartupAttempts <= 0 {
		config.MaxStartupAttempts = 3
	}
	return config
}

func validateConfig(config Config) error {
	for name, path := range map[string]string{
		"Runtime binary": config.RuntimeBinary, "data home": config.DataHome,
		"default workspace": config.DefaultWorkspace, "user home": config.UserHome,
	} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("supervisor: %s must be absolute", name)
		}
	}
	info, err := os.Stat(config.RuntimeBinary)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("supervisor: Runtime binary must be an executable regular file")
	}
	if err := os.MkdirAll(config.DataHome, 0o700); err != nil {
		return fmt.Errorf("supervisor: create data home: %w", err)
	}
	dataInfo, err := os.Lstat(config.DataHome)
	if err != nil || !dataInfo.IsDir() || dataInfo.Mode().Perm()&0o077 != 0 {
		return errors.New("supervisor: data home must be a private directory")
	}
	if config.RestartBackoff > config.MaxRestartBackoff || config.MaxStartupAttempts < 1 {
		return errors.New("supervisor: restart policy is invalid")
	}
	return nil
}

func cleanupGenerationRoot(root string) error {
	relative, err := filepath.Rel(os.TempDir(), root)
	if err != nil || root == "" || filepath.Dir(relative) != "." || !strings.HasPrefix(filepath.Base(root), "lyra-app2-runtime-") {
		return errors.New("supervisor: refusing to remove an unrecognized generation root")
	}
	return os.RemoveAll(root)
}

func waitFor(stop <-chan struct{}, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-stop:
		return false
	case <-timer.C:
		return true
	}
}

type boundedLog struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func newBoundedLog(limit int) *boundedLog { return &boundedLog{limit: limit} }

func (log *boundedLog) Write(data []byte) (int, error) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.data = append(log.data, data...)
	if len(log.data) > log.limit {
		log.data = append([]byte(nil), log.data[len(log.data)-log.limit:]...)
	}
	return len(data), nil
}

func (log *boundedLog) String() string {
	log.mu.Lock()
	defer log.mu.Unlock()
	return strings.TrimSpace(string(log.data))
}

var _ io.Writer = (*boundedLog)(nil)
