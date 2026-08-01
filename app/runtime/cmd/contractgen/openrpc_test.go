package main

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
)

func TestOpenRPCPublishesAckNullableAndNotificationResults(t *testing.T) {
	t.Parallel()

	registry, shapes := dispatch.Contract(), dispatch.WireShapes()
	document := newOpenRPC(registry, shapes, walkWireTypes(registry, shapes))

	ack := openRPCMethod(t, document, "sessions.delete")
	if ack.Result == nil || ack.Result.Schema == nil || ack.Result.Schema.Type != schemaTypeObject {
		t.Fatalf("sessions.delete result = %#v, want an explicit object acknowledgement", ack.Result)
	}

	nullable := openRPCMethod(t, document, "goals.get")
	if nullable.Result == nil || nullable.Result.Schema == nil || len(nullable.Result.Schema.AnyOf) != 2 {
		t.Fatalf("goals.get result = %#v, want value-or-null", nullable.Result)
	}
	foundNull := false
	for _, option := range nullable.Result.Schema.AnyOf {
		foundNull = foundNull || option.Type == schemaTypeNull
	}
	if !foundNull {
		t.Fatalf("goals.get result anyOf = %#v, want a null option", nullable.Result.Schema.AnyOf)
	}

	if len(document.Notifications) != len(shapes.Notifications()) {
		t.Fatalf("notifications = %d, want %d", len(document.Notifications), len(shapes.Notifications()))
	}
	for _, notification := range document.Notifications {
		if notification.Params == nil || notification.Params.Ref == "" {
			t.Fatalf("notification %q has no published params schema: %#v", notification.Name, notification.Params)
		}
	}
}

func openRPCMethod(t *testing.T, document openrpcDocument, name string) openrpcMethod {
	t.Helper()
	for _, method := range document.Methods {
		if method.Name == name {
			return method
		}
	}
	t.Fatalf("OpenRPC has no method %q", name)
	return openrpcMethod{}
}
