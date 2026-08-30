package evaluation

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/scope/core/metadata"
)

// MaxReportDepth bounds recursive detail trees at every public trust boundary.
const MaxReportDepth = 64

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

// Clone validates the complete detail tree before allocating its detached copy.
func (report Report) Clone() (Report, error) {
	if err := report.Validate(); err != nil {
		return Report{}, err
	}
	return report.cloneValid(), nil
}

func (report Report) cloneValid() Report {
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
		report.Details[index] = report.Details[index].cloneValid()
	}
	return report
}

func (report Report) Validate() error {
	return report.validate(1)
}

func (report Report) validate(depth int) error {
	if depth > MaxReportDepth {
		return fmt.Errorf("%w: detail depth exceeds %d", ErrInvalidReport, MaxReportDepth)
	}
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
	if !report.hasOutcome() {
		return fmt.Errorf("%w: at least one verdict, score, measurement, feedback, metadata value, or detail is required", ErrInvalidReport)
	}
	for index, detail := range report.Details {
		if err := detail.validate(depth + 1); err != nil {
			return fmt.Errorf("%w: details[%d]: %w", ErrInvalidReport, index, err)
		}
	}
	return nil
}

func (report Report) MarshalJSON() ([]byte, error) {
	if err := report.Validate(); err != nil {
		return nil, err
	}
	type wireReport Report
	return json.Marshal(wireReport(report))
}

func (report *Report) UnmarshalJSON(data []byte) error {
	if report == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidReport)
	}
	type wireReport Report
	var decoded wireReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidReport, err)
	}
	candidate := Report(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*report = candidate
	return nil
}

func (report Report) hasOutcome() bool {
	return report.Verdict.Decided() || report.Score != nil || report.Measurement != nil ||
		strings.TrimSpace(report.Feedback) != "" || len(report.Metadata) > 0 || len(report.Details) > 0
}
