package evaluation

import "errors"

var (
	ErrInvalidEvaluatorConfig = errors.New("evaluation: evaluator configuration is invalid")
	ErrInvalidMetric          = errors.New("evaluation: invalid metric")
	ErrInvalidScore           = errors.New("evaluation: invalid score")
	ErrInvalidReport          = errors.New("evaluation: invalid report")
)
