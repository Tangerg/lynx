package evaluation

import (
	"fmt"
	"strings"
)

// Metric identifies the quality dimension measured by a [Report]. String
// values keep reports stable and readable across process boundaries.
type Metric string

const (
	MetricComposite Metric = "composite"
)

func (metric Metric) Validate() error {
	name := string(metric)
	if name == "" || name != strings.TrimSpace(name) {
		return fmt.Errorf("%w: name must be non-empty without surrounding whitespace", ErrInvalidMetric)
	}
	return nil
}
