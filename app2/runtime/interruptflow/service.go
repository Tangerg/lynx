// Package interruptflow owns cold reads of durable human-input requests.
package interruptflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Store interface {
	ListInterruptSets(context.Context, string, string) ([]protocol.PendingInterruptSet, error)
}

type Service struct{ store Store }

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("interruptflow: store is required")
	}
	return &Service{store: store}, nil
}

func (service *Service) List(ctx context.Context, request protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error) {
	if request.SessionID == "" && request.RootRunID == "" {
		return nil, fmt.Errorf("%w: sessionId or rootRunId is required", protocol.ErrInvalidParams)
	}
	values, err := service.store.ListInterruptSets(ctx, request.SessionID, request.RootRunID)
	if err != nil {
		return nil, err
	}
	return protocol.NewPage(values), nil
}
