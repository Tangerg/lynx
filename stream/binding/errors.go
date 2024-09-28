package binding

import (
	"errors"
)

var (
	ErrorSendWithinReceiveBinding = errors.New("unable to send within receive binding")
	ErrorReceiveWithinSendBinding = errors.New("unable to receive within send binding")
)
