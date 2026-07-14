package chat_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/chat"
)

type callOnlyModel struct{}

func (callOnlyModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return &chat.Response{}, nil
}

var _ chat.Model = callOnlyModel{}

func TestCallOnlyProviderSatisfiesModel(t *testing.T) {
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("hello")))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	response, err := (callOnlyModel{}).Call(context.Background(), request)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if response == nil {
		t.Fatal("Call returned nil response")
	}
}

func TestModelPublicShape(t *testing.T) {
	modelType := reflect.TypeFor[chat.Model]()
	if modelType.NumMethod() != 1 {
		t.Fatalf("Model method count = %d, want 1", modelType.NumMethod())
	}
	method := modelType.Method(0)
	if method.Name != "Call" {
		t.Fatalf("Model method = %q, want Call", method.Name)
	}

	callType := method.Type
	if callType.NumIn() != 2 || callType.In(0) != reflect.TypeFor[context.Context]() || callType.In(1) != reflect.TypeFor[*chat.Request]() {
		t.Fatalf("Call inputs = %v, want (context.Context, *chat.Request)", callType)
	}
	if callType.NumOut() != 2 || callType.Out(0) != reflect.TypeFor[*chat.Response]() || !callType.Out(1).Implements(reflect.TypeFor[error]()) {
		t.Fatalf("Call outputs = %v, want (*chat.Response, error)", callType)
	}
}
