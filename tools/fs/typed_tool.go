package fs

import (
	"context"

	toolcontract "github.com/Tangerg/lynx/core/tool"
)

func mustTypedTool[In, Out any](config toolcontract.FuncConfig, function func(context.Context, In) (Out, error)) toolcontract.Func[In, Out] {
	typed, err := toolcontract.NewFunc(config, function)
	if err != nil {
		panic(err)
	}
	return typed
}
