package operationsflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	feedbackdomain "github.com/Tangerg/lynx/app2/runtime/domain/feedback"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func (service *Service) Feedback(
	ctx context.Context,
	request protocol.FeedbackRequest,
) error {
	if len(request.Text) > feedbackdomain.MaxTextBytes {
		return fmt.Errorf("%w: feedback text is too long", protocol.ErrInvalidParams)
	}
	rating := feedbackdomain.Rating(request.Rating)
	if !rating.Valid() {
		return fmt.Errorf("%w: rating is invalid", protocol.ErrInvalidParams)
	}
	subject := feedbackdomain.Subject{
		SessionID: strings.TrimSpace(request.SessionID),
		RunID:     strings.TrimSpace(request.RunID),
		ItemID:    strings.TrimSpace(request.ItemID),
	}
	attribution, found, err := service.resolveFeedbackAttribution(ctx, subject)
	if err != nil {
		return err
	}
	if !found {
		return feedbackSubjectNotFound(subject.MostSpecific())
	}
	if !attribution.Matches(subject) {
		return fmt.Errorf("%w: feedback subject ownership is inconsistent", protocol.ErrInvalidParams)
	}
	id, err := service.ids.New("fb_")
	if err != nil {
		return err
	}
	record, err := feedbackdomain.New(feedbackdomain.Create{
		ID:          id,
		Attribution: attribution,
		Rating:      rating,
		Text:        request.Text,
		Now:         service.now(),
	})
	if errors.Is(err, feedbackdomain.ErrInvalid) {
		return fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	if err != nil {
		return err
	}
	return service.store.CreateFeedbackRecord(ctx, record)
}

func (service *Service) resolveFeedbackAttribution(
	ctx context.Context,
	subject feedbackdomain.Subject,
) (feedbackdomain.Attribution, bool, error) {
	if subject.Empty() {
		return feedbackdomain.Attribution{}, true, nil
	}
	return service.store.ResolveFeedbackAttribution(ctx, subject)
}

func feedbackSubjectNotFound(kind string) error {
	switch kind {
	case "item":
		return protocol.ErrItemNotFound
	case "run":
		return protocol.ErrRunNotFound
	case "session":
		return protocol.ErrSessionNotFound
	default:
		return fmt.Errorf("%w: feedback has no subject or text", protocol.ErrInvalidParams)
	}
}
