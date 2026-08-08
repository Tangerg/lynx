package agent2

import (
	"context"
	"sync"
)

type treeOperation struct {
	engine   *Engine
	rootID   ProcessID
	released chan struct{}
	once     sync.Once
}

func (engine *Engine) acquireTreeOperation(
	ctx context.Context,
	rootID ProcessID,
) (*treeOperation, error) {
	ctx = contextOrBackground(ctx)
	for {
		engine.treeOperationsMu.Lock()
		active := engine.treeOperations[rootID]
		if active == nil {
			operation := &treeOperation{
				engine: engine, rootID: rootID, released: make(chan struct{}),
			}
			engine.treeOperations[rootID] = operation
			engine.treeOperationsMu.Unlock()
			return operation, nil
		}
		released := active.released
		engine.treeOperationsMu.Unlock()
		select {
		case <-released:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (operation *treeOperation) release() {
	if operation == nil || operation.engine == nil {
		return
	}
	operation.once.Do(func() {
		engine := operation.engine
		engine.treeOperationsMu.Lock()
		if engine.treeOperations[operation.rootID] == operation {
			delete(engine.treeOperations, operation.rootID)
		}
		close(operation.released)
		engine.treeOperationsMu.Unlock()
	})
}
