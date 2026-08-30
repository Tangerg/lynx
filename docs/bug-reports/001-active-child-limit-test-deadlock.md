# 001 — testActiveChildLimit deadlocks the whole agent package

**Status**: fixed in the test. No implementation change needed — the kernel
behaves as `agent/ARCHITECTURE.md` §8 specifies.

**Module / package**: `agent`
**Symbol**: `testActiveChildLimit`, a subtest of
`TestEngineEnforcesChildDepthFanoutActiveAndTreeLimits` (`agent/child_test.go`)

## Observed behavior

The subtest blocked forever on `<-dispatcher.started` before awaiting the root
Process. Because the receive had no bound, the whole `agent` package ran to the
10-minute test-binary timeout and died with a goroutine dump instead of a
targeted failure — taking the full workspace sweep down with it.

It passed on a normally loaded machine and passed 500+ consecutive runs in
isolation, so it read as a flake. It is not: under `GOMAXPROCS=1` it deadlocks
**every** time.

```
$ GOMAXPROCS=1 go test -count=5 -run TestEngineEnforcesChildDepthFanoutActiveAndTreeLimits -timeout 45s ./
panic: test timed out after 45s
goroutine 27 [chan receive]:
github.com/Tangerg/scope/agent.testActiveChildLimit(...)
	/Users/tangerg/Desktop/scope/agent/child_test.go:312
```

## Root cause

The test raced against a behavior the architecture explicitly forbids relying
on. Instrumenting the same scenario under `GOMAXPROCS=1`:

```
root status=completed
ChildIDs=[process:6255…8e3d] Failures=2
child found=true status=running
STALLED: no dispatch in 15s; child status=canceled
```

The root Definition runs in `fanout_blocking` mode and does **not**
`WaitForChildren`. With `MaxActiveChildren: 1` it admits one child, rejects two
with `engine.child.tree_limit`, and completes. Its one still-active child is then
canceled as a parent cancellation, so the child's leaf effect may never reach the
blocking dispatcher and nothing is ever sent to `started`.

That is exactly the documented contract in `agent/ARCHITECTURE.md` §8:

> A parent terminal state delivers control intent only to direct children still
> active at that moment […] If a parent result depends on a child, the strategy
> must `WaitForChildren` explicitly rather than rely on the scheduling order of
> two concurrent run loops.

On a multi-core machine the child usually wins the race and dispatches first,
which is why the test normally passed. Under `GOMAXPROCS=1` the root always wins.

## Expected behavior

The kernel is correct: it produced `ChildIDs=1, Failures=2` with two
`engine.child.tree_limit` codes on every run, including every deadlocking one.
`MaxActiveChildren` works exactly as specified.

## Blast radius

Test-only. No production symbol is involved and no public API changes.

The same unbounded `<-dispatcher.started` idiom appears in several other tests,
but each of those waits while the parent is still `Waiting` on its children — so
the parent cannot complete out from under the dispatch and the race does not
exist there. `tree_durability_test.go` already uses the bounded `select` form.

## Fix applied

Dropped the pre-await receive. What the test asserts — that
`MaxActiveChildren: 1` admits one child and rejects two — is carried
deterministically by the root output, so the wait added nothing but the race. A
comment records why waiting there is wrong, so it is not reintroduced.

Verified with `GOMAXPROCS=1 go test -count=100` on the subtest and a full
`GOMAXPROCS=1` run of the package.

## Follow-up worth considering

`GOMAXPROCS=1` is a cheap, deterministic way to surface exactly this class of
"passes because the fast goroutine usually wins" test. It is not in the CI gate
set today.
