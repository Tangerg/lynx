package agui

import "sync"

// Permission store — a thread-safe map of in-flight approval requests.
//
// When a DSL script hits an `Approval(...)` step it:
//   1. Generates a requestId
//   2. Emits a `lyra.approval` CUSTOM event carrying the id
//   3. Registers a chan in this store, keyed by id
//   4. Blocks on that chan (with ctx cancellation)
//
// The HTTP handler at POST /permission accepts `{requestId, decision}`
// from the frontend (when the user clicks Approve / Decline on the
// rendered card), finds the matching chan, and pushes the decision —
// unblocking the script.
//
// Used as a package-level singleton so the SSE goroutine that runs a
// script and the HTTP handler that receives the user's click can share
// state without threading a pointer through every Step.

// Decision captures what the user clicked on an approval card.
type Decision string

const (
	DecisionApproved Decision = "approved"
	DecisionDeclined Decision = "declined"
)

// PermissionResponse is the body the frontend POSTs to /permission.
type PermissionResponse struct {
	RequestID string   `json:"requestId"`
	Decision  Decision `json:"decision"`
}

type permissionStore struct {
	mu      sync.Mutex
	pending map[string]chan PermissionResponse
}

var permissions = &permissionStore{pending: make(map[string]chan PermissionResponse)}

// register reserves a chan for the given id. Caller must always defer
// `release(id)` to clean up — even if the chan was already drained or
// the wait got cancelled.
func (p *permissionStore) register(id string) chan PermissionResponse {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan PermissionResponse, 1)
	p.pending[id] = ch
	return ch
}

// release drops the entry. Safe to call multiple times — second call is
// a no-op since the key is gone.
func (p *permissionStore) release(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pending, id)
}

// resolve delivers a decision to the waiting script. Returns false if
// the id isn't known (script aborted / response arrived too late).
func (p *permissionStore) resolve(resp PermissionResponse) bool {
	p.mu.Lock()
	ch, ok := p.pending[resp.RequestID]
	p.mu.Unlock()
	if !ok {
		return false
	}
	// Buffered chan of size 1 + non-blocking send: works even if the
	// receiver already moved on (ctx cancelled before resolve landed).
	select {
	case ch <- resp:
		return true
	default:
		return false
	}
}
