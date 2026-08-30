package eval

import "errors"

var (
	ErrInvalidEvaluatorConfig = errors.New("eval: evaluator configuration is invalid")
	ErrInvalidMetric          = errors.New("eval: invalid metric")
	ErrInvalidScore           = errors.New("eval: invalid score")
	ErrInvalidReport          = errors.New("eval: invalid report")
	ErrInvalidCase            = errors.New("eval: invalid case")
	ErrInvalidDataset         = errors.New("eval: invalid dataset")
	ErrInvalidExperiment      = errors.New("eval: invalid experiment")
	ErrInvalidComparison      = errors.New("eval: invalid comparison")
	ErrCaseNotEvaluated       = errors.New("eval: case was not evaluated")
)
