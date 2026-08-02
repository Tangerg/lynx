package main

import (
	"strings"
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

func TestOpenRPCRequestFramesAreStrictAndPublishUniversalMetadata(t *testing.T) {
	t.Parallel()

	document := newOpenRPC(dispatch.Contract(), dispatch.WireShapes(), walkWireTypes(dispatch.Contract(), dispatch.WireShapes()))
	for _, method := range document.Methods {
		if method.RequestFrame == nil || method.RequestFrame.UnevaluatedProps == nil || *method.RequestFrame.UnevaluatedProps {
			t.Errorf("%s request frame does not reject unknown top-level params", method.Name)
		}
		meta := openRPCParam(t, method, requestMetaField)
		if meta.Required || meta.Schema == nil || !strings.HasSuffix(meta.Schema.Ref, "/RequestMeta") {
			t.Errorf("%s _meta param = %#v, want optional RequestMeta", method.Name, meta)
		}
		foundMeta := false
		for _, branch := range method.RequestFrame.AllOf {
			if branch == nil || branch.Properties == nil {
				continue
			}
			_, foundMeta = branch.Properties[requestMetaField]
			if foundMeta {
				break
			}
		}
		if !foundMeta {
			t.Errorf("%s request frame does not allow %s", method.Name, requestMetaField)
		}
	}
}

func TestOpenRPCParamsPreserveRequestFieldConstraints(t *testing.T) {
	t.Parallel()

	document := newOpenRPC(dispatch.Contract(), dispatch.WireShapes(), walkWireTypes(dispatch.Contract(), dispatch.WireShapes()))
	for _, test := range []struct {
		method    string
		param     string
		minimum   *int64
		minLength *int
	}{
		{method: "sessions.get", param: "sessionId", minLength: new(1)},
		{method: "runs.start", param: "sessionId", minLength: new(1)},
		{method: "skills.drafts.promote", param: "revision", minLength: new(1)},
		{method: "approval.listRules", param: "sessionId", minLength: new(1)},
		{method: "schedules.create", param: "prompt", minLength: new(1)},
		{method: "sessions.update", param: "expectedRevision", minimum: new(int64(1))},
		{method: "runs.start", param: "maxTotalTokens", minimum: new(int64(0))},
		{method: "sessions.list", param: "limit", minimum: new(int64(0))},
		{method: "workspace.files.read", param: "maxBytes", minimum: new(int64(0))},
		{method: "usage.summary", param: "sinceDays", minimum: new(int64(0))},
	} {
		param := openRPCParam(t, openRPCMethod(t, document, test.method), test.param)
		if !equalOptional(param.Schema.Minimum, test.minimum) {
			t.Errorf("%s.%s minimum = %v, want %v", test.method, test.param, param.Schema.Minimum, test.minimum)
		}
		if !equalOptional(param.Schema.MinLength, test.minLength) {
			t.Errorf("%s.%s minLength = %v, want %v", test.method, test.param, param.Schema.MinLength, test.minLength)
		}
	}
}

func openRPCParam(t *testing.T, method openrpcMethod, name string) openrpcParam {
	t.Helper()
	for _, param := range method.Params {
		if param.Name == name {
			return param
		}
	}
	t.Fatalf("OpenRPC method %q has no param %q", method.Name, name)
	return openrpcParam{}
}

func equalOptional[T comparable](left, right *T) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
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
