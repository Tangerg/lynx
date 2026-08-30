package eval

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/core/metadata"
)

// MetricName identifies one quality calculation within a namespace.
type MetricName string

const MetricNameComposite MetricName = "composite"

const metricConfigurationKey = "configuration"

// Direction describes how a raw measurement relates to quality. It is kept
// separate from Score, whose direction is always higher-is-better.
type Direction string

const (
	DirectionUnspecified    Direction = ""
	DirectionHigherIsBetter Direction = "higher_is_better"
	DirectionLowerIsBetter  Direction = "lower_is_better"
)

func (d Direction) Validate() error {
	switch d {
	case DirectionUnspecified, DirectionHigherIsBetter, DirectionLowerIsBetter:
		return nil
	default:
		return fmt.Errorf("%w: unsupported direction %q", ErrInvalidMetric, d)
	}
}

// Metric identifies an evaluation without encoding configuration into a
// string. Parameters holds owned, structured identity for calculation and
// decision rules. Unit and Direction describe optional raw measurements;
// normalized scores are always unitless and higher-is-better.
type Metric struct {
	Namespace  string       `json:"namespace,omitzero"`
	Name       MetricName   `json:"name"`
	Unit       string       `json:"unit,omitzero"`
	Direction  Direction    `json:"direction,omitzero"`
	Parameters metadata.Map `json:"parameters,omitzero"`
}

type MetricConfig struct {
	Namespace  string
	Name       MetricName
	Unit       string
	Direction  Direction
	Parameters metadata.Map
}

func NewMetric(config MetricConfig) (Metric, error) {
	metric := Metric{
		Namespace:  config.Namespace,
		Name:       config.Name,
		Unit:       config.Unit,
		Direction:  config.Direction,
		Parameters: config.Parameters.Clone(),
	}
	if err := metric.Validate(); err != nil {
		return Metric{}, err
	}
	return metric, nil
}

func (m Metric) Clone() Metric {
	m.Parameters = m.Parameters.Clone()
	return m
}

func (m Metric) String() string {
	if m.Namespace == "" {
		return string(m.Name)
	}
	return m.Namespace + "/" + string(m.Name)
}

func (m Metric) Validate() error {
	if err := validateMetricPart("namespace", m.Namespace, true); err != nil {
		return err
	}
	if err := validateMetricPart("name", string(m.Name), false); err != nil {
		return err
	}
	if m.Unit != strings.TrimSpace(m.Unit) {
		return fmt.Errorf("%w: unit must not contain surrounding whitespace", ErrInvalidMetric)
	}
	if err := m.Direction.Validate(); err != nil {
		return err
	}
	if err := m.Parameters.Validate(); err != nil {
		return fmt.Errorf("%w: parameters: %w", ErrInvalidMetric, err)
	}
	return nil
}

func (m Metric) identity() (string, error) {
	encoded, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("%w: encode identity: %w", ErrInvalidMetric, err)
	}
	return string(encoded), nil
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
