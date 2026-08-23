// Package operationsflow owns cross-run usage reporting and write-only user
// feedback. Neither concern participates in Run execution state.
package operationsflow

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/accounting"
	feedbackdomain "github.com/Tangerg/lynx/app2/runtime/domain/feedback"
)

type Store interface {
	ListUsageRunRecords(context.Context, string, time.Time) ([]accounting.RunRecord, error)
	SessionExists(context.Context, string) (bool, error)
	ResolveFeedbackAttribution(
		context.Context,
		feedbackdomain.Subject,
	) (feedbackdomain.Attribution, bool, error)
	CreateFeedbackRecord(context.Context, feedbackdomain.Record) error
}

type IDs interface {
	New(string) (string, error)
}

type Service struct {
	store Store
	ids   IDs
	now   func() time.Time
}

func New(store Store, ids IDs) (*Service, error) {
	if store == nil || ids == nil {
		return nil, errors.New("operationsflow: store and ids are required")
	}
	return &Service{store: store, ids: ids, now: time.Now}, nil
}
