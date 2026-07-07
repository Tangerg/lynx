package runtime

import (
	"errors"
)

// maxSpawnDepth is the hard backstop on sub-agent delegation depth (a top-level
// process is depth 0, its child 1, and so on). A spawn that would exceed it
// fails fast with [ErrMaxSpawnDepth] BEFORE the child process is created —
// structural insurance against pathological recursive delegation (an agent that
// keeps delegating to itself) that holds even when no token / step budget is
// set. Generous on purpose: real recursive task decomposition nests only a few
// levels, so this never trips legitimate use — it only stops runaways.
const maxSpawnDepth = 8

// ErrMaxSpawnDepth reports that a sub-agent spawn was refused because it would
// exceed [maxSpawnDepth]. All spawn entry points fail with it before creating a
// child; the agent-as-tool wrapper returns it as a tool error, so the
// delegating model re-plans instead of recursing without bound.
var ErrMaxSpawnDepth = errors.New("spawn child: max delegation depth exceeded")
