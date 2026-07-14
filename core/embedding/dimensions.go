package embedding

import (
	"context"
	"errors"
	"fmt"
)

// ResolveDimensions uses a model's explicit [Dimensioner] capability when
// available and otherwise performs one uncached probe. Failures are returned;
// zero is never used as an "unknown" sentinel.
func ResolveDimensions(ctx context.Context, model Model) (int, error) {
	if dimensioner, ok := model.(Dimensioner); ok {
		dimensions, err := dimensioner.Dimensions(ctx)
		if err != nil {
			return 0, fmt.Errorf("embedding.ResolveDimensions: explicit capability: %w", err)
		}
		if dimensions <= 0 {
			return 0, fmt.Errorf("embedding.ResolveDimensions: explicit capability returned %d", dimensions)
		}
		return dimensions, nil
	}
	return ProbeDimensions(ctx, model)
}

// ProbeDimensions issues one embedding request and reports its vector width.
// It deliberately does not cache: callers that need caching own its lifetime,
// key, invalidation policy, and errors.
func ProbeDimensions(ctx context.Context, model Model) (int, error) {
	client, err := NewClient(model)
	if err != nil {
		return 0, err
	}
	vector, _, err := client.EmbedText(ctx, "dimension probe")
	if err != nil {
		return 0, fmt.Errorf("embedding.ProbeDimensions: %w", err)
	}
	if len(vector) == 0 {
		return 0, errors.New("embedding.ProbeDimensions: model returned an empty vector")
	}
	return len(vector), nil
}
