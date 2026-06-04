package transport

import "context"

// Last-Event-Id is a streaming reconnect cursor — transport metadata, not
// a business param (TRANSPORT §2/§9.2). The transport reads it off the
// wire (HTTP Last-Event-Id header / IPC metadata) and carries it on the
// context with WithLastEventID; the runtime's SubscribeRun reads it with
// LastEventIDFrom to replay a run's durable backlog from that point.
// Transports that don't carry it (or a fresh subscribe) leave it empty →
// full replay.
type lastEventIDKey struct{}

// WithLastEventID returns ctx carrying the streaming reconnect cursor.
func WithLastEventID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, lastEventIDKey{}, id)
}

// LastEventIDFrom reads the reconnect cursor, "" when unset (full replay).
func LastEventIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(lastEventIDKey{}).(string)
	return id
}
