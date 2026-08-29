package evaluation

import (
	"fmt"
	"math"
	"slices"

	"github.com/Tangerg/scope/core/metadata"
)

// Report is one evaluation result. Verdict, normalized Score, and raw
// Measurement are independent and optional so measurement-only and qualitative
// evaluations do not need to invent a pass threshold or quality score. Details
// contains owned child reports instead of convention-based metadata keys.
type Report struct {
	Metric      Metric       `json:"metric"`
	Verdict     Verdict      `json:"verdict,omitzero"`
	Score       *Score       `json:"score,omitzero"`
	Measurement *float64     `json:"measurement,omitzero"`
	Feedback    string       `json:"feedback,omitzero"`
	Metadata    metadata.Map `json:"metadata,omitzero"`
	Details     []Report     `json:"details,omitzero"`
}

func (report Report) Clone() Report {
	report.Metric = report.Metric.Clone()
	if report.Score != nil {
		score := *report.Score
		report.Score = &score
	}
	if report.Measurement != nil {
		measurement := *report.Measurement
		report.Measurement = &measurement
	}
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
	if err := report.Verdict.Validate(); err != nil {
		return err
	}
	if report.Score != nil {
		if err := report.Score.Validate(); err != nil {
			return fmt.Errorf("%w: score: %w", ErrInvalidReport, err)
		}
	}
	if report.Measurement != nil {
		value := *report.Measurement
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("%w: measurement must be finite", ErrInvalidReport)
		}
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
