package terminal

import (
	"context"
	"fmt"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/program"
)

func (a *app) listenForSearch() {
	results := a.transcript.SearchResults()
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(searchOperation, true, func(ctx context.Context, lease operationLease) {
		for {
			select {
			case result, ok := <-results:
				if !ok {
					return
				}
				if err := a.postSearchResult(ctx, dispatcher, lease, result); err != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})
}

func (a *app) postSearchResult(ctx context.Context, dispatcher program.Dispatcher, lease operationLease, result headless.Result) error {
	return post(ctx, dispatcher, func() {
		if !a.operations.Current(lease) || a.closed {
			return
		}
		a.acceptSearchResult(result)
	})
}

func (a *app) acceptSearchResult(result headless.Result) {
	if result.Err != nil {
		a.message(fmt.Sprintf("search failed: %v", result.Err))
		return
	}
	accepted, announce := a.transcript.AcceptSearch(result)
	if accepted && announce {
		a.message(fmt.Sprintf("%d match(es) for %q", len(result.Matches), result.Query))
	}
}
