package toolloop

import (
	"fmt"

	"github.com/Tangerg/lynx/agent/internal/nilvalue"
	"github.com/Tangerg/lynx/core/chat"
)

// ConcurrentTool is the optional capability Runner consumes to schedule
// non-conflicting calls from one model response concurrently. It deliberately
// lives in the consumer package rather than tools: a tool may provide this
// advice structurally without depending on a particular loop driver, and a
// driver remains correct if it ignores the advice and executes serially.
type ConcurrentTool interface {
	// ConcurrencyKey reports whether this call may overlap other calls in the
	// same round and, when non-empty, the resource on which it conflicts:
	//
	//   - concurrent=false: execute alone; this is also the default for tools
	//     that do not implement ConcurrentTool.
	//   - concurrent=true, key="": no known resource conflict.
	//   - concurrent=true, key!="": calls with the same key serialize.
	//
	// A panic aborts the round as a scheduling error; it is never interpreted as
	// permission to execute concurrently.
	ConcurrencyKey(arguments string) (key string, concurrent bool)
}

type callPlan struct {
	hosted     hostedTool
	concurrent bool
	key        string
	direct     bool
}

func planCalls(resolver ToolResolver, calls []chat.ToolCall) ([]callPlan, bool, error) {
	plans := make([]callPlan, len(calls))
	allDirect := len(calls) > 0
	for index, call := range calls {
		if nilvalue.Is(resolver) {
			allDirect = false
			continue
		}
		hosted, ok, err := resolveHosted(resolver, call.Name)
		if err != nil {
			return nil, false, err
		}
		if !ok {
			allDirect = false
			continue
		}
		direct, err := hosted.returnsDirect()
		if err != nil {
			return nil, false, fmt.Errorf("tool %q: %w", call.Name, err)
		}
		plan := callPlan{hosted: hosted, direct: direct}
		plan.key, plan.concurrent, err = hosted.concurrencyKey(call.Arguments)
		if err != nil {
			return nil, false, err
		}
		plans[index] = plan
		allDirect = allDirect && plan.direct
	}
	return plans, allDirect, nil
}

// segmentEnd returns the exclusive end of the longest consecutive call range
// that can start together without violating an exclusive declaration or
// duplicating a non-empty resource key. A single exclusive call forms its own
// segment.
func segmentEnd(plans []callPlan, start int) int {
	if start < 0 || start >= len(plans) || !plans[start].concurrent {
		return start + 1
	}

	claimed := make(map[string]struct{})
	if key := plans[start].key; key != "" {
		claimed[key] = struct{}{}
	}
	end := start + 1
	for end < len(plans) {
		plan := plans[end]
		if !plan.concurrent {
			break
		}
		if plan.key != "" {
			if _, conflict := claimed[plan.key]; conflict {
				break
			}
			claimed[plan.key] = struct{}{}
		}
		end++
	}
	return end
}
