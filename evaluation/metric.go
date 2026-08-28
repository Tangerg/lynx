package evaluation

import (
	"fmt"
	"strings"

	"github.com/Tangerg/scope/core/metadata"
)

// MetricName identifies one quality calculation within a namespace.
type MetricName string

const MetricNameComposite MetricName = "composite"

// Metric identifies a quality calculation without encoding configuration into
// a string. Parameters holds owned, structured identity such as a retrieval
// cutoff or judge rubric version.
type Metric struct {
	Namespace  string       `json:"namespace,omitzero"`
	Name       MetricName   `json:"name"`
	Parameters metadata.Map `json:"parameters,omitzero"`
}

func NewMetric(namespace string, name MetricName, parameters metadata.Map) (Metric, error) {
	metric := Metric{Namespace: namespace, Name: name, Parameters: parameters.Clone()}
	if err := metric.Validate(); err != nil {
		return Metric{}, err
	}
	return metric, nil
}

func (metric Metric) Clone() Metric {
	metric.Parameters = metric.Parameters.Clone()
	return metric
}

func (metric Metric) String() string {
	if metric.Namespace == "" {
		return string(metric.Name)
	}
	return metric.Namespace + "/" + string(metric.Name)
}

func (metric Metric) Validate() error {
	if err := validateMetricPart("namespace", metric.Namespace, true); err != nil {
		return err
	}
	if err := validateMetricPart("name", string(metric.Name), false); err != nil {
		return err
	}
	if err := metric.Parameters.Validate(); err != nil {
		return fmt.Errorf("%w: parameters: %w", ErrInvalidMetric, err)
	}
	return nil
}

func validateMetricPart(label, value string, optional bool) error {
	if value == "" && optional {
		return nil
	}
	if value == "" || value != strings.TrimSpace(value) || strings.Contains(value, "/") {
		return fmt.Errorf("%w: %s must be non-empty without surrounding whitespace or slashes", ErrInvalidMetric, label)
	}
	return nil
}
