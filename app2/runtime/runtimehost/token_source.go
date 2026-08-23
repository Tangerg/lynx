package runtimehost

import (
	"context"

	"github.com/Tangerg/lynx/app2/runtime/localruntime"
)

// fileTokenSource intentionally reads the protected token for each RPC. A
// complete atomic replacement rotates credentials without restarting the
// Runtime or retaining the retired secret in a long-lived object.
type fileTokenSource struct { path string }

func (source fileTokenSource) Token(context.Context)(string,error){
	return localruntime.ReadToken(source.path)
}
