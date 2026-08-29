package evaluation

import "errors"

var (
	ErrInvalidEvaluatorConfig = errors.New("evaluation: evaluator configuration is invalid")
	ErrInvalidMetric          = errors.New("evaluation: invalid metric")
	ErrInvalidScore           = errors.New("evaluation: invalid score")
	ErrInvalidReport          = errors.New("evaluation: invalid report")
	ErrInvalidCase            = errors.New("evaluation: invalid case")
	ErrInvalidRunConfig       = errors.New("evaluation: runner configuration is invalid")
	ErrCaseNotEvaluated       = errors.New("evaluation: case was not evaluated")
)
