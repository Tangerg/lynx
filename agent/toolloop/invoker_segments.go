package toolloop

import (
	"context"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/core/model/chat"
)

// maxConcurrentToolCalls bounds how many concurrency-safe tool calls run at
// once within one round's parallel batch. A round rarely emits more than a
// handful of parallelizable calls; the cap keeps a model that fans out wide
// (many `task` sub-agents, many reads) from stampeding provider rate limits.
const maxConcurrentToolCalls = 8

type limiter struct {
	slots chan struct{}
}

func newLimiter(cap int) *limiter {
	if cap <= 0 {
		panic("toolloop: concurrent tool call limit must be > 0")
	}
	return &limiter{slots: make(chan struct{}, cap)}
}

func (l *limiter) Acquire() {
	l.slots <- struct{}{}
}

func (l *limiter) Release() {
	<-l.slots
}

// segmentEnd returns the exclusive upper bound of the segment starting at
// start: the longest run of consecutive calls that may execute together. The
// run stops at the first exclusive call (which runs alone) and at the first
// keyed call whose resource is already claimed by an earlier call in the run
// (it must serialize against it). A single exclusive call yields a one-element
// segment.
func (i *invoker) segmentEnd(calls []*chat.ToolCallPart, start int) int {
	concurrent, key := i.concurrencyOf(calls[start])
	if !concurrent {
		return start + 1
	}
	claimed := map[string]struct{}{}
	if key != "" {
		claimed[key] = struct{}{}
	}
	end := start + 1
	for end < len(calls) {
		concurrent, key = i.concurrencyOf(calls[end])
		if !concurrent {
			break
		}
		if key != "" {
			if _, dup := claimed[key]; dup {
				break
			}
			claimed[key] = struct{}{}
		}
		end++
	}
	return end
}

// concurrencyOf reports whether a call may run concurrently with others and the
// resource key it conflicts on, read from the tool's optional [ConcurrentTool]
// capability. A tool that doesn't implement it — or an unregistered one — is
// exclusive (run alone).
func (i *invoker) concurrencyOf(call *chat.ToolCallPart) (concurrent bool, key string) {
	t, ok := i.registry.find(call.Name)
	if !ok {
		return false, ""
	}
	if c, ok := t.(ConcurrentTool); ok {
		key, concurrent = c.ConcurrencyKey(call.Arguments)
		return concurrent, key
	}
	return false, ""
}

// runSegment executes calls[start:end] — a one-element segment inline, a
// multi-element segment concurrently (bounded by [maxConcurrentToolCalls]) —
// writing each call's result into returns[idx] / direct[idx]. It returns the
// lowest-index HITL interrupt and the lowest-index abort among the segment's
// calls; abort takes precedence (the caller propagates it), an interrupt parks
// the round. Both nil on a clean segment. By policy only an exclusive call
// (its own segment) interrupts or aborts, but a parallel batch tolerates either
// defensively.
func (i *invoker) runSegment(ctx context.Context, calls []*chat.ToolCallPart, start, end int, returns []*chat.ToolReturn, direct []bool) (interrupt, abort error) {
	// run executes one call into its result slot, returning a control-flow
	// signal (HITL interrupt or abort) when the call produced no result. runOne
	// folds a recoverable failure into the result itself, so a non-nil return
	// here is always interrupt-or-abort.
	run := func(ctx context.Context, idx int) error {
		out, err := i.runOne(ctx, calls[idx])
		if err != nil {
			return err
		}
		returns[idx], direct[idx] = out.ret, out.direct
		return nil
	}

	errs := make([]error, end-start) // control-flow signal per call; nil = produced a result
	if len(errs) == 1 {
		errs[0] = run(ctx, start) // exclusive call: inline, no goroutine
	} else {
		// Parallel batch, bounded by maxConcurrentToolCalls. Cancel siblings on
		// the first abort so a torn-down run stops promptly; a HITL interrupt
		// does NOT cancel — the other calls finish and their results join the
		// done-set.
		bctx, cancel := context.WithCancel(ctx)
		defer cancel()
		lim := newLimiter(maxConcurrentToolCalls)
		var wg sync.WaitGroup
		for idx := start; idx < end; idx++ {
			lim.Acquire()
			wg.Go(func() {
				defer lim.Release()
				if err := run(bctx, idx); err != nil {
					errs[idx-start] = err
					if i.abortsToolLoop(err) {
						cancel()
					}
				}
			})
		}
		wg.Wait()
	}

	// Classify in call order: an abort takes precedence (the run can't
	// continue); otherwise the lowest-index HITL interrupt parks the round.
	for off, err := range errs {
		switch {
		case err == nil:
		case i.abortsToolLoop(err):
			return nil, fmt.Errorf("toolloop.invoker.invokeToolCalls: tool %q failed: %w", calls[start+off].Name, err)
		case interrupt == nil:
			interrupt = err
		}
	}
	return interrupt, nil
}

// filledReturns drops the nil holes a parked round leaves in the indexed
// results slice (the pending calls that didn't complete), yielding the
// done-set in call order.
func filledReturns(returns []*chat.ToolReturn) []*chat.ToolReturn {
	out := make([]*chat.ToolReturn, 0, len(returns))
	for _, r := range returns {
		if r != nil {
			out = append(out, r)
		}
	}
	return out
}
