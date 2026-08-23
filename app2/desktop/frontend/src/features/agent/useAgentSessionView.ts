import { useQueryClient } from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";

import type {
  ContentBlock,
  Item,
  OpenRuntimeStream,
  Plan,
  RunEvent,
  RunProgress,
  RunRef,
  RuntimeConnection,
  SessionSnapshot,
} from "@lyra/runtime-contract";

import {
  cancelRun,
  runtimeQueryKeys,
  startRun,
  steerRun,
  subscribeRun,
} from "../../runtime/runtimeQueries";

const seenEventLimit = 8_192;

interface AgentSessionState {
  identity: string;
  runsById: Record<string, RunRef>;
  runOrder: string[];
  itemsById: Record<string, Item>;
  itemOrder: string[];
  progressByRunId: Record<string, RunProgress>;
  plan?: Plan;
}

type AgentSessionAction =
  | { type: "hydrate"; identity: string; snapshot: SessionSnapshot }
  | { type: "event"; event: RunEvent }
  | { type: "run"; run: RunRef };

interface RunLease {
  key: string;
  identity: string;
  runId: string;
  segmentId: string;
  controller: AbortController;
  lastEventId?: string;
}

type SendCommand =
  | { key: string; type: "start" }
  | { key: string; type: "steer"; runId: string; segmentId: string };

type RunStream = OpenRuntimeStream<unknown, RunEvent>;

export interface AgentSessionView {
  items: Item[];
  runs: RunRef[];
  plan?: Plan;
  activeRootRun?: RunRef;
  focusRootRun?: RunRef;
  progress?: RunProgress;
  contextTokens?: number;
  actionPending: boolean;
  actionError?: string;
  streamError?: string;
  send(input: ContentBlock[], idempotencyKey: string): Promise<void>;
  stop(): Promise<void>;
}

const emptyState: AgentSessionState = {
  identity: "",
  runsById: {},
  runOrder: [],
  itemsById: {},
  itemOrder: [],
  progressByRunId: {},
};

export function useAgentSessionView(
  connection: RuntimeConnection,
  sessionId: string | undefined,
  snapshot: SessionSnapshot | undefined,
): AgentSessionView {
  const queryClient = useQueryClient();
  const identity = sessionId
    ? `${connection.instanceId}:${connection.generation}:${sessionId}`
    : "";
  const [state, dispatch] = useReducer(agentSessionReducer, emptyState);
  const [actionPending, setActionPending] = useState(false);
  const [actionError, setActionError] = useState<string>();
  const [streamError, setStreamError] = useState<string>();
  const leases = useRef(new Map<string, RunLease>());
  const actionInFlight = useRef(false);
  const pendingAction = useRef<AbortController | undefined>(undefined);
  const sendCommand = useRef<SendCommand | undefined>(undefined);
  const currentIdentity = useRef(identity);
  const seenEvents = useRef(new Set<string>());
  const seenOrder = useRef<string[]>([]);

  currentIdentity.current = identity;

  useLayoutEffect(() => {
    if (identity !== "" && snapshot !== undefined) {
      dispatch({ type: "hydrate", identity, snapshot });
    }
  }, [identity, snapshot]);

  useEffect(() => {
    setActionPending(false);
    setActionError(undefined);
    setStreamError(undefined);
    seenEvents.current.clear();
    seenOrder.current = [];
    return () => {
      pendingAction.current?.abort();
      pendingAction.current = undefined;
      sendCommand.current = undefined;
      actionInFlight.current = false;
      for (const lease of leases.current.values()) {
        lease.controller.abort();
      }
      leases.current.clear();
    };
  }, [identity]);

  const visible = state.identity === identity ? state : emptyState;
  const runs = useMemo(
    () => visible.runOrder.flatMap((id) => valueOf(visible.runsById, id)),
    [visible.runOrder, visible.runsById],
  );
  const items = useMemo(
    () => visible.itemOrder.flatMap((id) => valueOf(visible.itemsById, id)),
    [visible.itemOrder, visible.itemsById],
  );
  const activeRootRun = useMemo(
    () =>
      runs
        .filter(
          (run) =>
            run.parentRunId === undefined &&
            (run.status === "running" || run.status === "waiting"),
        )
        .toSorted((left, right) =>
          (right.createdAt ?? "").localeCompare(left.createdAt ?? ""),
        )[0],
    [runs],
  );
  const focusRootRun = useMemo(
    () =>
      activeRootRun ??
      runs
        .filter((run) => run.parentRunId === undefined)
        .toSorted((left, right) =>
          (right.createdAt ?? "").localeCompare(left.createdAt ?? ""),
        )[0],
    [activeRootRun, runs],
  );

  const invalidateMaterial = useCallback(() => {
    if (!sessionId) return;
    void queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.snapshot(connection, sessionId),
    });
    void queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.sessions(connection),
    });
  }, [connection, queryClient, sessionId]);

  const fold = useCallback(
    (frame: { event: RunEvent; eventId?: string }, owner: string) => {
      if (currentIdentity.current !== owner) return;
      const eventId = frame.event.eventId;
      if (seenEvents.current.has(eventId)) return;
      seenEvents.current.add(eventId);
      seenOrder.current.push(eventId);
      if (seenOrder.current.length > seenEventLimit) {
        const expired = seenOrder.current.shift();
        if (expired !== undefined) seenEvents.current.delete(expired);
      }
      setStreamError(undefined);
      dispatch({ type: "event", event: frame.event });
    },
    [],
  );

  const runLease = useCallback(
    async (lease: RunLease, initial?: RunStream) => {
      let stream = initial;
      let retry = 0;
      let terminal = false;
      try {
        while (!lease.controller.signal.aborted && !terminal) {
          try {
            stream ??= await subscribeRun(
              connection,
              lease.runId,
              lease.segmentId,
              lease.controller.signal,
              lease.lastEventId,
            );
            for await (const frame of stream) {
              if (frame.eventId !== undefined) {
                lease.lastEventId = frame.eventId;
              }
              fold(frame, lease.identity);
              if (frame.event.event.type === "segment.finished") {
                terminal = true;
                break;
              }
            }
            stream.close();
            stream = undefined;
            if (!terminal) {
              retry += 1;
              if (currentIdentity.current === lease.identity) {
                setStreamError("Runtime closed the live stream before the segment finished");
              }
              invalidateMaterial();
              await abortableDelay(
                Math.min(400 * 2 ** Math.min(retry - 1, 4), 6_400),
                lease.controller.signal,
              );
            }
          } catch (error) {
            stream?.close(error);
            stream = undefined;
            if (lease.controller.signal.aborted || isAbort(error)) break;
            if (currentIdentity.current === lease.identity) {
              setStreamError(messageOf(error));
            }
            invalidateMaterial();
            retry += 1;
            await abortableDelay(
              Math.min(400 * 2 ** Math.min(retry - 1, 4), 6_400),
              lease.controller.signal,
            );
          }
        }
      } finally {
        stream?.close();
        if (leases.current.get(lease.key) === lease) {
          leases.current.delete(lease.key);
        }
        invalidateMaterial();
      }
    },
    [connection, fold, invalidateMaterial],
  );

  const ensureLease = useCallback(
    (runId: string, segmentId: string, initial?: RunStream) => {
      const key = segmentKey(runId, segmentId);
      const existing = leases.current.get(key);
      if (existing !== undefined) {
        if (initial !== undefined) initial.close();
        return;
      }
      const lease: RunLease = {
        key,
        identity,
        runId,
        segmentId,
        controller: new AbortController(),
      };
      leases.current.set(key, lease);
      void runLease(lease, initial);
    },
    [identity, runLease],
  );

  const activeSegments = useMemo(
    () =>
      runs.flatMap((run) =>
        run.status === "running" && run.activeSegmentId
          ? [{ runId: run.id, segmentId: run.activeSegmentId }]
          : [],
      ),
    [runs],
  );
  useEffect(() => {
    const active = new Set(
      activeSegments.map(({ runId, segmentId }) => segmentKey(runId, segmentId)),
    );
    for (const segment of activeSegments) {
      ensureLease(segment.runId, segment.segmentId);
    }
    for (const [key, lease] of leases.current) {
      if (!active.has(key)) lease.controller.abort();
    }
  }, [activeSegments, ensureLease]);

  const send = useCallback(
    async (input: ContentBlock[], idempotencyKey: string) => {
      if (!sessionId || input.length === 0 || actionInFlight.current) return;
      const actionIdentity = identity;
      const controller = new AbortController();
      actionInFlight.current = true;
      pendingAction.current = controller;
      setActionPending(true);
      setActionError(undefined);
      try {
        let command = sendCommand.current;
        if (command?.key !== idempotencyKey) {
          if (activeRootRun?.status === "waiting") {
            throw new Error("This run is waiting for a response before it can continue.");
          }
          if (
            activeRootRun?.status === "running" &&
            !activeRootRun.activeSegmentId
          ) {
            throw new Error("The active run has no current segment.");
          }
          command =
            activeRootRun?.status === "running" &&
            activeRootRun.activeSegmentId
              ? {
                  key: idempotencyKey,
                  type: "steer",
                  runId: activeRootRun.id,
                  segmentId: activeRootRun.activeSegmentId,
                }
              : { key: idempotencyKey, type: "start" };
          sendCommand.current = command;
        }
        if (command.type === "steer") {
          await steerRun(
            connection,
            command.runId,
            command.segmentId,
            input,
            idempotencyKey,
            controller.signal,
          );
        } else {
          let stream: Awaited<ReturnType<typeof startRun>>;
          try {
            stream = await startRun(
              connection,
              sessionId,
              input,
              idempotencyKey,
              controller.signal,
            );
          } catch (error) {
            controller.abort();
            throw error;
          }
          const { runId, segmentId } = stream.acknowledgement;
          const key = segmentKey(runId, segmentId);
          const existing = leases.current.get(key);
          existing?.controller.abort();
          const lease: RunLease = {
            key,
            identity: actionIdentity,
            runId,
            segmentId,
            controller,
          };
          leases.current.set(key, lease);
          void runLease(lease, stream);
        }
        if (sendCommand.current?.key === idempotencyKey) {
          sendCommand.current = undefined;
        }
        invalidateMaterial();
      } catch (error) {
        if (currentIdentity.current === actionIdentity && !isAbort(error)) {
          setActionError(messageOf(error));
        }
        throw error;
      } finally {
        if (pendingAction.current === controller) {
          pendingAction.current = undefined;
          actionInFlight.current = false;
          setActionPending(false);
        }
      }
    },
    [
      activeRootRun,
      connection,
      identity,
      invalidateMaterial,
      runLease,
      sessionId,
    ],
  );

  const stop = useCallback(async () => {
    if (
      !activeRootRun ||
      activeRootRun.status !== "running" ||
      actionInFlight.current
    ) {
      return;
    }
    const actionIdentity = identity;
    const controller = new AbortController();
    actionInFlight.current = true;
    pendingAction.current = controller;
    setActionPending(true);
    setActionError(undefined);
    try {
      const result = await cancelRun(
        connection,
        activeRootRun.id,
        controller.signal,
      );
      if (currentIdentity.current !== actionIdentity) return;
      dispatch({ type: "run", run: result.run });
      if (result.rootRun !== undefined) {
        dispatch({ type: "run", run: result.rootRun });
      }
      invalidateMaterial();
    } catch (error) {
      if (currentIdentity.current === actionIdentity && !isAbort(error)) {
        setActionError(messageOf(error));
      }
      throw error;
    } finally {
      if (pendingAction.current === controller) {
        pendingAction.current = undefined;
        actionInFlight.current = false;
        setActionPending(false);
      }
    }
  }, [activeRootRun, connection, identity, invalidateMaterial]);

  return {
    items,
    runs,
    plan: visible.plan,
    activeRootRun,
    focusRootRun,
    progress:
      activeRootRun === undefined
        ? undefined
        : visible.progressByRunId[activeRootRun.id],
    contextTokens:
      (activeRootRun === undefined
        ? undefined
        : visible.progressByRunId[activeRootRun.id]?.contextTokens) ??
      focusRootRun?.contextTokens,
    actionPending,
    actionError,
    streamError,
    send,
    stop,
  };
}

function agentSessionReducer(
  state: AgentSessionState,
  action: AgentSessionAction,
): AgentSessionState {
  if (action.type === "hydrate") {
    return hydrateState(state, action.identity, action.snapshot);
  }
  if (action.type === "run") {
    return putRun(state, action.run);
  }
  return foldRunEvent(state, action.event);
}

function hydrateState(
  state: AgentSessionState,
  identity: string,
  snapshot: SessionSnapshot,
): AgentSessionState {
  const next: AgentSessionState = {
    identity,
    runsById: Object.fromEntries(snapshot.runs.map((run) => [run.id, run])),
    runOrder: snapshot.runs.map((run) => run.id),
    itemsById: Object.fromEntries(snapshot.items.map((item) => [item.id, item])),
    itemOrder: snapshot.items.map((item) => item.id),
    progressByRunId: {},
    ...(snapshot.plan === undefined ? {} : { plan: snapshot.plan }),
  };
  if (state.identity !== identity) return next;

  for (const itemId of state.itemOrder) {
    const item = state.itemsById[itemId];
    if (
      item === undefined ||
      item.status !== "running" ||
      (item.type !== "agentMessage" && item.type !== "reasoning") ||
      next.itemsById[item.id] !== undefined ||
      next.runsById[item.runId]?.status !== "running"
    ) {
      continue;
    }
    next.itemsById[item.id] = item;
    next.itemOrder.push(item.id);
  }
  for (const [runId, progress] of Object.entries(state.progressByRunId)) {
    if (next.runsById[runId]?.status === "running") {
      next.progressByRunId[runId] = progress;
    }
  }
  if (
    state.plan !== undefined &&
    (next.plan === undefined || state.plan.revision > next.plan.revision)
  ) {
    next.plan = state.plan;
  }
  return next;
}

function foldRunEvent(state: AgentSessionState, value: RunEvent): AgentSessionState {
  const event = value.event;
  switch (event.type) {
    case "segment.started":
      return event.run === undefined ? state : putRun(state, event.run);
    case "segment.progress":
      return event.progress === undefined
        ? state
        : {
            ...state,
            progressByRunId: {
              ...state.progressByRunId,
              [value.runId]: event.progress,
            },
          };
    case "segment.finished":
      return finishSegment(state, value);
    case "item.started":
      return event.item === undefined ? state : startItem(state, event.item);
    case "item.delta":
      return foldItemDelta(state, event.itemId, event.delta);
    case "item.completed":
      return event.item === undefined ? state : putItem(state, event.item);
    case "plan.updated":
      return event.plan === undefined ||
        (state.plan !== undefined && state.plan.revision > event.plan.revision)
        ? state
        : { ...state, plan: event.plan };
  }
}

function putRun(state: AgentSessionState, run: RunRef): AgentSessionState {
  return {
    ...state,
    runsById: { ...state.runsById, [run.id]: run },
    runOrder: state.runsById[run.id]
      ? state.runOrder
      : [...state.runOrder, run.id],
  };
}

function finishSegment(state: AgentSessionState, value: RunEvent) {
  const current = state.runsById[value.runId];
  if (current === undefined || value.event.outcome === undefined) return state;
  const outcome = value.event.outcome;
  const contextTokens =
    state.progressByRunId[value.runId]?.contextTokens ?? current.contextTokens;
  const next = putRun(state, {
    ...current,
    status:
      outcome.type === "interrupt" || outcome.type === "suspended"
        ? "waiting"
        : "finished",
    ...(outcome.type === "interrupt" || outcome.type === "suspended"
      ? {}
      : {
          outcome: {
            type: outcome.type,
            ...(outcome.error === undefined ? {} : { error: outcome.error }),
            ...(outcome.detail === undefined ? {} : { detail: outcome.detail }),
          },
          finishedAt: value.timestamp,
        }),
    activeSegmentId: undefined,
    metrics: value.event.metrics ?? current.metrics,
    ...(contextTokens === undefined ? {} : { contextTokens }),
  });
  const itemsById = { ...next.itemsById };
  const itemOrder = next.itemOrder.filter((itemId) => {
    const item = itemsById[itemId];
    const provisional =
      item?.runId === value.runId &&
      item.status === "running" &&
      (item.type === "agentMessage" || item.type === "reasoning");
    if (provisional) delete itemsById[itemId];
    return !provisional;
  });
  const progressByRunId = { ...next.progressByRunId };
  delete progressByRunId[value.runId];
  return { ...next, itemsById, itemOrder, progressByRunId };
}

function startItem(state: AgentSessionState, item: Item): AgentSessionState {
  const current = state.itemsById[item.id];
  if (current !== undefined && current.status !== "running") return state;
  return putItem(state, current ?? item);
}

function putItem(state: AgentSessionState, item: Item): AgentSessionState {
  return {
    ...state,
    itemsById: { ...state.itemsById, [item.id]: item },
    itemOrder: state.itemsById[item.id]
      ? state.itemOrder
      : [...state.itemOrder, item.id],
  };
}

function foldItemDelta(
  state: AgentSessionState,
  itemId: string | undefined,
  delta: RunEvent["event"]["delta"],
): AgentSessionState {
  if (itemId === undefined || delta === undefined) return state;
  const item = state.itemsById[itemId];
  if (item === undefined || item.status !== "running") return state;
  if (delta.type === "reasoning" && item.type === "reasoning") {
    return putItem(state, { ...item, text: (item.text ?? "") + (delta.text ?? "") });
  }
  if (
    delta.type !== "content" ||
    item.type !== "agentMessage" ||
    delta.index === undefined
  ) {
    return state;
  }
  const content = [...(item.content ?? [])];
  while (content.length <= delta.index) {
    content.push({ type: "text", text: "" });
  }
  if (content[delta.index]?.type !== "text") return state;
  content[delta.index] = {
    type: "text",
    text: (content[delta.index]?.text ?? "") + (delta.text ?? ""),
  };
  return putItem(state, { ...item, content });
}

function valueOf<Value>(values: Record<string, Value>, id: string): Value[] {
  const value = values[id];
  return value === undefined ? [] : [value];
}

function segmentKey(runId: string, segmentId: string) {
  return `${runId}:${segmentId}`;
}

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "Runtime stream interrupted.";
}

function isAbort(error: unknown) {
  return error instanceof DOMException && error.name === "AbortError";
}

function abortableDelay(milliseconds: number, signal: AbortSignal) {
  if (signal.aborted) return Promise.resolve();
  return new Promise<void>((resolve) => {
    const timer = window.setTimeout(finish, milliseconds);
    function finish() {
      window.clearTimeout(timer);
      signal.removeEventListener("abort", finish);
      resolve();
    }
    signal.addEventListener("abort", finish, { once: true });
  });
}
