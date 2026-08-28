package evaluation

import (
	"fmt"
	"slices"

	"github.com/Tangerg/scope/core/metadata"
)

// Report is one normalized evaluation verdict. Details contains the owned
// child reports of a composite evaluation instead of flattening them into
// convention-based metadata keys.
type Report struct {
	Metric   Metric       `json:"metric"`
	Passed   bool         `json:"passed"`
	Score    Score        `json:"score"`
	Feedback string       `json:"feedback,omitzero"`
	Metadata metadata.Map `json:"metadata,omitzero"`
	Details  []Report     `json:"details,omitzero"`
}

func (report Report) Clone() Report {
	report.Metric = report.Metric.Clone()
	report.Metadata = report.Metadata.Clone()
	report.Details = slices.Clone(report.Details)
	for index := range report.Details {
		report.Details[index] = report.Details[index].Clone()
	}
	return report
}

func (report Report) Validate() error {
	if err := report.Metric.Validate(); err != nil {
		return fmt.Errorf("%w: metric: %w", ErrInvalidReport, err)
	}
	if err := report.Score.Validate(); err != nil {
		return fmt.Errorf("%w: score: %w", ErrInvalidReport, err)
	}
	if err := report.Metadata.Validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidReport, err)
	}
	for index, detail := range report.Details {
		if err := detail.Validate(); err != nil {
			return fmt.Errorf("%w: details[%d]: %w", ErrInvalidReport, index, err)
		}
	}
	return nil
}
