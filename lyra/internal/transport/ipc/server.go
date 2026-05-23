// Package ipc hosts Lyra's stdio JSON-RPC transport — a thin
// line-delimited protocol designed for embedding lyra as a child
// process inside a TUI / editor / desktop client.
//
// Wire format (newline-delimited JSON):
//
//	→  {"id":"r1", "method":"agent.run", "params":{...}}
//	←  {"id":"r1", "event":{...AG-UI event...}}
//	←  {"id":"r1", "event":{...}}
//	←  {"id":"r1", "done":true}
//
// Streaming methods (agent.run) push zero or more {id,event} frames
// then a single {id,done:true} terminator. Non-streaming methods
// return {id,result} on success or {id,error:{code,message}} on
// failure. The shape stays uniform across both flows so clients
// can use one dispatch loop.
//
// Multiple requests can be in flight; each is handled in its own
// goroutine. The single stdout writer is mutex-serialised so frames
// from different requests don't interleave at the byte level.
package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	lyraruntime "github.com/Tangerg/lynx/lyra/internal/runtime"
)

// Server is the IPC transport. Build with [NewServer] and call
// [Server.Serve] to start the read/dispatch loop.
type Server struct {
	runtime *lyraruntime.Runtime
	in      io.Reader
	out     io.Writer

	// writeMu serialises stdout writes so two in-flight requests'
	// event frames don't interleave mid-line.
	writeMu sync.Mutex
}

// Config bundles the construction-time inputs.
type Config struct {
	Runtime *lyraruntime.Runtime

	// In / Out default to os.Stdin / os.Stdout when nil. Tests
	// inject buffers / pipes here.
	In  io.Reader
	Out io.Writer
}

// NewServer constructs a Server.
func NewServer(cfg Config) (*Server, error) {
	if cfg.Runtime == nil {
		return nil, errors.New("ipc: Runtime is required")
	}
	if cfg.In == nil || cfg.Out == nil {
		return nil, errors.New("ipc: In and Out are required (use os.Stdin / os.Stdout for the default lyra serve --stdio path)")
	}
	return &Server{
		runtime: cfg.Runtime,
		in:      cfg.In,
		out:     cfg.Out,
	}, nil
}

// Serve runs the read-dispatch loop until ctx is cancelled or the
// input stream closes. Returns nil on clean EOF; any other read
// error propagates.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	// Allow JSON-RPC frames up to 4 MiB — large enough for messages
	// that include big inline arguments (e.g. file contents).
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError("", "PARSE_ERROR", "invalid JSON: "+err.Error())
			continue
		}
		// One goroutine per request so a slow streaming method
		// doesn't head-of-line block subsequent requests.
		go s.dispatch(ctx, req)
	}
	return scanner.Err()
}

// request is the inbound frame schema. Method dispatches the
// handler; Params is decoded by each method against its own struct.
type request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// response is the outbound frame schema. Exactly one of Result /
// Event / Done / Error is populated per frame. Streaming methods
// emit multiple Event frames followed by one Done frame.
type response struct {
	ID     string          `json:"id"`
	Result any             `json:"result,omitempty"`
	Event  json.RawMessage `json:"event,omitempty"`
	Done   bool            `json:"done,omitempty"`
	Error  *errorFrame     `json:"error,omitempty"`
}

type errorFrame struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeFrame serialises one response onto the shared stdout. The
// mutex ensures multi-byte frames write atomically vs concurrent
// goroutines.
func (s *Server) writeFrame(frame response) {
	data, err := json.Marshal(frame)
	if err != nil {
		return // best-effort; can't surface the error in-band
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.out.Write(data)
	_, _ = s.out.Write([]byte{'\n'})
}

func (s *Server) writeResult(id string, result any) {
	s.writeFrame(response{ID: id, Result: result})
}

func (s *Server) writeError(id, code, message string) {
	s.writeFrame(response{ID: id, Error: &errorFrame{Code: code, Message: message}})
}

func (s *Server) writeDone(id string) {
	s.writeFrame(response{ID: id, Done: true})
}

// writeEvent emits a streaming event frame. eventJSON is the
// already-marshalled AG-UI event payload.
func (s *Server) writeEvent(id string, eventJSON []byte) {
	s.writeFrame(response{ID: id, Event: json.RawMessage(eventJSON)})
}

// dispatch routes one request to its handler. Unknown methods
// reply with METHOD_NOT_FOUND.
func (s *Server) dispatch(ctx context.Context, req request) {
	switch req.Method {
	case "agent.run":
		s.handleAgentRun(ctx, req)
	case "agent.steer":
		s.handleAgentSteer(ctx, req)
	case "agent.cancel":
		s.handleAgentCancel(ctx, req)
	case "sessions.list":
		s.handleSessionsList(ctx, req)
	case "sessions.create":
		s.handleSessionsCreate(ctx, req)
	case "sessions.get":
		s.handleSessionsGet(ctx, req)
	case "sessions.delete":
		s.handleSessionsDelete(ctx, req)
	case "approvals.list":
		s.handleApprovalsList(ctx, req)
	case "approvals.decide":
		s.handleApprovalsDecide(ctx, req)
	case "approvals.getMode":
		s.handleApprovalsGetMode(ctx, req)
	case "approvals.setMode":
		s.handleApprovalsSetMode(ctx, req)
	case "healthz":
		s.writeResult(req.ID, map[string]string{"status": "ok"})
	default:
		s.writeError(req.ID, "METHOD_NOT_FOUND", fmt.Sprintf("unknown method %q", req.Method))
	}
}

// decodeParams unmarshals req.Params into target. On failure
// writes an error frame and returns false so the caller can stop.
func (s *Server) decodeParams(req request, target any) bool {
	if len(req.Params) == 0 {
		return true
	}
	if err := json.Unmarshal(req.Params, target); err != nil {
		s.writeError(req.ID, "INVALID_PARAMS", err.Error())
		return false
	}
	return true
}
