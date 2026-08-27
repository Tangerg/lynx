package evaluation

import (
	"fmt"
	"strings"
)

// Metric identifies the quality dimension measured by a [Report]. String
// values keep reports stable and readable across process boundaries.
type Metric string

const (
	MetricGroundedness    Metric = "groundedness"
	MetricAnswerRelevance Metric = "answer_relevance"
	MetricComposite       Metric = "composite"
)

func NewMetric(name string) (Metric, error) {
	metric := Metric(name)
	if err := metric.Validate(); err != nil {
		return "", err
	}
	return metric, nil
}

// MustMetric is intended for declaration-time identities that cannot recover
// from an invalid source literal.
func MustMetric(name string) Metric {
	metric, err := NewMetric(name)
	if err != nil {
		panic(err)
	}
	return metric
}

func (metric Metric) Validate() error {
	name := string(metric)
	if name == "" || name != strings.TrimSpace(name) {
		return fmt.Errorf("%w: name must be non-empty without surrounding whitespace", ErrInvalidMetric)
	}
	return nil
}
