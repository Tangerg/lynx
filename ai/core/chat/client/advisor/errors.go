package advisor

import (
	"errors"
)

var (
	ErrorSensitiveUserText    = errors.New("the text entered by the user contains sensitive vocabulary")
	ErrorChainNoAroundAdvisor = errors.New("no AroundAdvisor available to execute")
)
