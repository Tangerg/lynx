import type { WorkspaceEventLike } from "../domain/eventInvalidation";

const RECONNECT_BASE_MS = 1_000;
const RECONNECT_CAP_MS = 30_000;
const EVENT_OPENING_TIMEOUT_MS = 10_000;
const RETARGET = Symbol("workspace-events.retarget");
const ABORTED = Symbol("workspace-events.aborted");

class WorkspaceEventOpeningTimeoutError extends Error {
  override readonly name = "WorkspaceEventOpeningTimeoutError";

  constructor() {
    super("runtime_event_subscription_opening_timeout");
  }
}

export interface WorkspaceEventLoopDeps {
  subscribe(input: {
    target: WorkspaceWatchTarget;
    signal: AbortSignal;
  }): Promise<AsyncIterable<WorkspaceEventLike>>;
  handleEvent(ev: WorkspaceEventLike): void;
  invalidateAll(): void;
  reportDisconnect(connectionGeneration: string, error?: unknown): void;
  openingTimeoutMs?: number;
}

export interface WorkspaceEventLoop {
  start(signal: AbortSignal, connectionGeneration: string): Promise<void>;
  retarget(target: WorkspaceWatchTarget): void;
}

/**
 * `none` means the app-wide topics stay subscribed without a file watch while
 * active-session identity is unresolved. It is intentionally distinct from a
 * resolved workspace with no cwd, which means the Runtime's default workspace.
 */
export type WorkspaceWatchTarget = { type: "none" } | { type: "workspace"; cwd?: string };

function sameTarget(left: WorkspaceWatchTarget, right: WorkspaceWatchTarget): boolean {
  return (
    left.type === right.type &&
    (left.type === "none" || right.type === "none" || left.cwd === right.cwd)
  );
}

export function createWorkspaceEventLoop(deps: WorkspaceEventLoopDeps): WorkspaceEventLoop {
  let watchTarget: WorkspaceWatchTarget = { type: "none" };
  let iterAbort: AbortController | null = null;
  let generationAbort: AbortController | null = null;
  let generation = 0;

  return {
    start(signal, connectionGeneration) {
      // The loop itself owns the one active subscription generation. Callers
      // normally withdraw capability before restarting, but correctness must
      // not depend on that ordering: a repeated start atomically supersedes
      // the prior generation even if its caller forgot to abort its signal.
      generationAbort?.abort();
      const cohort = new AbortController();
      generationAbort = cohort;
      const ownGeneration = ++generation;
      const abortCohort = () => cohort.abort(signal.reason);
      if (signal.aborted) abortCohort();
      else signal.addEventListener("abort", abortCohort, { once: true });
      return subscribeLoop(
        deps,
        cohort.signal,
        connectionGeneration,
        () => watchTarget,
        (next) => {
          if (generation === ownGeneration) iterAbort = next;
        },
      ).finally(() => {
        signal.removeEventListener("abort", abortCohort);
        if (generation !== ownGeneration) return;
        iterAbort = null;
        generationAbort = null;
      });
    },
    retarget(target) {
      if (sameTarget(target, watchTarget)) return;
      watchTarget = target;
      iterAbort?.abort(RETARGET);
    },
  };
}

async function subscribeLoop(
  deps: WorkspaceEventLoopDeps,
  signal: AbortSignal,
  connectionGeneration: string,
  watchTarget: () => WorkspaceWatchTarget,
  setIterAbort: (controller: AbortController | null) => void,
): Promise<void> {
  let attempt = 0;
  while (!signal.aborted) {
    const iter = new AbortController();
    setIterAbort(iter);
    const onOuterAbort = () => iter.abort();
    signal.addEventListener("abort", onOuterAbort, { once: true });
    let failure: unknown;
    try {
      const opening = deps.subscribe({ target: watchTarget(), signal: iter.signal });
      const events = await settleOpening(
        opening,
        iter,
        deps.openingTimeoutMs ?? EVENT_OPENING_TIMEOUT_MS,
      );
      if (events === ABORTED) continue;
      // A transport may resolve its opening promise at the same instant a
      // retarget abort wins. Do not publish the stale subscription's initial
      // resync or any already-buffered event into the new workspace target.
      if (iter.signal.aborted) continue;
      const iterator = events[Symbol.asyncIterator]();
      let iteratorDone = false;
      try {
        attempt = 0;
        deps.invalidateAll();
        let lastSequence = 0;
        while (!iter.signal.aborted) {
          const pendingNext = Promise.resolve(iterator.next());
          const next = await settleBeforeAbort(pendingNext, iter.signal);
          if (next === ABORTED) {
            const lateNext = await settleWithinTurn(pendingNext);
            if (lateNext.status === "fulfilled" && lateNext.value.done) iteratorDone = true;
            break;
          }
          if (next.done) {
            iteratorDone = true;
            break;
          }
          const ev = next.value;
          // Sequence belongs to this subscription generation. Once a forward
          // gap has forced an authoritative resync, a duplicated or delayed
          // lower frame is already covered by that snapshot and must not move
          // the watermark backwards or replace every mounted read model again.
          if (ev.sequence <= lastSequence) continue;
          if (ev.sequence > lastSequence + 1) {
            deps.invalidateAll();
          }
          lastSequence = ev.sequence;
          deps.handleEvent(ev);
        }
      } finally {
        if (!iteratorDone) await disposeIterator(iterator);
      }
    } catch (error) {
      if (!signal.aborted && iter.signal.reason !== RETARGET) failure = error;
    } finally {
      signal.removeEventListener("abort", onOuterAbort);
      setIterAbort(null);
    }
    if (signal.aborted) return;
    if (iter.signal.reason === RETARGET) {
      attempt = 0;
      continue;
    }
    // An RPC stream ending without outer cancellation is also a connection
    // signal. Let the Runtime context withdraw this exact connection and recover
    // instead of allowing this consumer to guess global connection health.
    deps.reportDisconnect(connectionGeneration, failure);
    // The connection owner withdraws the generation synchronously. That aborts
    // this exact loop before its asynchronous recovery inspection begins; do not
    // leave a predecessor reconnect timer behind that boundary.
    if (signal.aborted) return;
    const backoff = new AbortController();
    setIterAbort(backoff);
    const abortBackoff = () => backoff.abort();
    signal.addEventListener("abort", abortBackoff, { once: true });
    await delay(Math.min(RECONNECT_BASE_MS * 2 ** attempt, RECONNECT_CAP_MS), backoff.signal);
    signal.removeEventListener("abort", abortBackoff);
    setIterAbort(null);
    if (signal.aborted) return;
    if (backoff.signal.reason === RETARGET) {
      attempt = 0;
      continue;
    }
    attempt += 1;
  }
}

/** Give the response-stream handshake a terminal lifecycle without applying a
 * wall-clock limit to the accepted stream. Reject the deadline before aborting
 * so this cohort reports a connection failure rather than looking like an
 * ordinary retarget; settleBeforeAbort keeps a non-cooperative late opening
 * observed and retires the foreign iterable when it eventually arrives. */
function settleOpening<T>(
  operation: Promise<AsyncIterable<T>>,
  controller: AbortController,
  timeoutMs: number,
): Promise<AsyncIterable<T> | typeof ABORTED> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  let deadlineSettled = false;
  let releaseDeadline!: () => void;
  const deadline = new Promise<never>((_resolve, reject) => {
    releaseDeadline = () => {
      if (deadlineSettled) return;
      deadlineSettled = true;
      reject();
    };
    timer = setTimeout(() => {
      timer = undefined;
      deadlineSettled = true;
      const error = new WorkspaceEventOpeningTimeoutError();
      reject(error);
      controller.abort(error);
    }, timeoutMs);
  });
  return Promise.race([
    settleBeforeAbort(operation, controller.signal, disposeIterable),
    deadline,
  ]).finally(() => {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
    releaseDeadline();
  });
}

/**
 * Settle an asynchronous boundary without making progress depend on the
 * dependency observing its AbortSignal. The losing operation stays observed so
 * a late rejection cannot become unhandled; a late resource may additionally
 * be retired by its owning callback.
 */
function settleBeforeAbort<T>(
  operation: Promise<T>,
  signal: AbortSignal,
  disposeLateValue?: (value: T) => void,
): Promise<T | typeof ABORTED> {
  return new Promise((resolve, reject) => {
    let settled = false;
    const onAbort = () => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", onAbort);
      resolve(ABORTED);
    };
    if (signal.aborted) onAbort();
    else signal.addEventListener("abort", onAbort, { once: true });

    void operation.then(
      (value) => {
        if (settled) {
          disposeLateValue?.(value);
          return;
        }
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(value);
      },
      (error: unknown) => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

function disposeIterable<T>(iterable: AsyncIterable<T>): void {
  try {
    void disposeIterator(iterable[Symbol.asyncIterator]());
  } catch {
    // The subscription was already superseded, so its signal remains the
    // authoritative teardown path when constructing its iterator fails.
  }
}

async function disposeIterator<T>(iterator: AsyncIterator<T>): Promise<void> {
  try {
    const closing = iterator.return?.();
    if (!closing) return;
    // Cooperative async generators often finish one or two microtasks after
    // their signal fires. Join that ordinary path, but yield after one task so
    // a broken `return()` cannot hold the replacement subscription hostage.
    await settleWithinTurn(Promise.resolve(closing));
  } catch {
    // Cancellation must not let a broken retiring iterator block its successor.
  }
}

type TurnSettlement<T> =
  { status: "fulfilled"; value: T } | { status: "rejected" } | { status: "pending" };

function settleWithinTurn<T>(operation: Promise<T>): Promise<TurnSettlement<T>> {
  return new Promise((resolve) => {
    let settled = false;
    const finish = (result: TurnSettlement<T>) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(result);
    };
    const timer = setTimeout(() => finish({ status: "pending" }), 0);
    void operation.then(
      (value) => finish({ status: "fulfilled", value }),
      () => finish({ status: "rejected" }),
    );
  });
}

function delay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const timer = setTimeout(done, ms);
    function done(): void {
      clearTimeout(timer);
      signal.removeEventListener("abort", done);
      resolve();
    }
    signal.addEventListener("abort", done, { once: true });
  });
}
