package protocol

// WorkspaceSubscribeRequest — workspace.subscribe body (AUX_API §3.1). Watches
// registers file-monitoring interest; gated behind features.fileWatch.
type WorkspaceSubscribeRequest struct {
	Watches []WatchSpec `json:"watches,omitempty"`
}

// WatchSpec is one file-watch registration. WatchId is client-chosen (echoed
// in files.changed); Cwd defaults to the serve directory; Path is relative to
// Cwd (jailed like §7.5).
type WatchSpec struct {
	WatchID string `json:"watchId"`
	Cwd     string `json:"cwd,omitempty"`
	Path    string `json:"path"`
}

// WorkspaceSubscribeResponse is the (empty) streaming ack — the first frame of
// the stream, mirroring StartRunResponse's role for runs.
type WorkspaceSubscribeResponse struct{}

// WorkspaceEventType discriminates the WorkspaceEvent union (AUX_API §3.2).
type WorkspaceEventType string

const (
	WorkspaceEventFilesChanged     WorkspaceEventType = "files.changed"
	WorkspaceEventSkillsChanged    WorkspaceEventType = "skills.changed"
	WorkspaceEventMCPServerChanged WorkspaceEventType = "mcp.serverChanged"
	WorkspaceEventSchedulesFired   WorkspaceEventType = "schedules.fired"
	WorkspaceEventResync           WorkspaceEventType = "resync"
)

// WorkspaceEvent is one non-run workspace event (AUX_API §3.2) — a flat
// tag-discriminated struct (single `type`, optional fields per tag, §2.1).
// Types: files.changed | skills.changed | mcp.serverChanged | resync.
type WorkspaceEvent struct {
	Type WorkspaceEventType `json:"type"`
	// files.changed
	WatchID string   `json:"watchId,omitempty"`
	Paths   []string `json:"paths,omitempty"`
	// Cwd scopes a tool-derived files.changed to the session's working
	// directory (paths are relative to it) — set when the change comes from an
	// agent file tool rather than a client-registered watch, so a client can
	// tell whether the change belongs to the project it's showing.
	Cwd string `json:"cwd,omitempty"`
	// mcp.serverChanged
	Server    string       `json:"server,omitempty"`
	Status    McpStatus    `json:"status,omitempty"`
	ToolCount *int         `json:"toolCount,omitempty"`
	Error     *ProblemData `json:"error,omitempty"`
	// schedules.fired — a scheduled run just started; the client refetches its
	// session list (the run lives in a fresh session).
	ScheduleID string `json:"scheduleId,omitempty"`
}
