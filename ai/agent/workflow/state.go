package workflow

import (
	"github.com/Tangerg/lynx/pkg/sync"
)

type State struct {
	Futures []sync.Future[any]
}
