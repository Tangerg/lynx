package trajectory

import "errors"

var (
	ErrInvalidTrajectory = errors.New("eval/trajectory: invalid trajectory")
	ErrInvalidSample     = errors.New("eval/trajectory: invalid sample")
)
