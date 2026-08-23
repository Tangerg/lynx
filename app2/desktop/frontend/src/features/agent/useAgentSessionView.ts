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
  InterruptResponse,
  Item,
  OpenRuntimeStream,
  PendingInterruptSet,
  Plan,
  RunEvent,
  RunProgress,
  RunRef,
  RuntimeConnection,
  SessionSnapshot,
} from "@lyra/runtime-contract";

import {
  cancelRun,
  resumeRun,
  runtimeQueryKeys,
  startRun,
  steerRun,
  subscribeRun,
} from "../../runtime/runtimeQueries";
import type { LiveToolOutput } from "./agentSessionTypes";

const seenEventLimit = 8_192;
const liveToolOutputCharacterLimit = 64 * 1_024;

interface AgentSessionState {
  identity: string;
  runsById: Record<string, RunRef>;
  runOrder: string[];
  itemsById: Record<string, Item>;
  itemOrder: string[];
  liveToolOutputsByItemId: Record<string, LiveToolOutput>;
  interruptsByRootRunId: Record<string, PendingInterruptSet>;
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

interface ResumeCommand {
  key: string;
  runId: string;
  responses: InterruptResponse[];
}

type RunStream = OpenRuntimeStream<unknown, RunEvent>;

export interface AgentSessionView {
  items: Item[];
  runs: RunRef[];
  liveToolOutputs: Record<string, LiveToolOutput>;
  interrupts: PendingInterruptSet[];
  plan?: Plan;
  activeRootRun?: RunRef;
  focusRootRun?: RunRef;
  progress?: RunProgress;
  contextTokens?: number;
  actionPending: boolean;
  actionError?: string;
  interruptError?: string;
  streamError?: string;
  cancelingRunId?: string;
  cancelError?: { runId: string; message: string };
  send(input: ContentBlock[], idempotencyKey: string): Promise<void>;
  resume(
    interruptSet: PendingInterruptSet,
    responses: InterruptResponse[],
    idempotencyKey: string,
  ): Promise<void>;
  cancel(runId: string): Promise<void>;
  stop(): Promise<void>;
}

const emptyState: AgentSessionState = {
  identity: "",
  runsById: {},
  runOrder: [],
  itemsById: {},
  itemOrder: [],
  liveToolOutputsByItemId: {},
  interruptsByRootRunId: {},
  progressByRunId: {},
};

export function useAgentSessionView(
  connection: RuntimeConnection,
  sessionId: string | undefined,
  snapshot: SessionSnapshot | undefined,
	selection: { provider: string; model: string } | undefined,
): AgentSessionView {
  const queryClient = useQueryClient();
  const identity = sessionId
    ? `${connection.instanceId}:${connection.generation}:${sessionId}`
    : "";
  const [state, dispatch] = useReducer(agentSessionReducer, emptyState);
  const [actionPending, setActionPending] = useState(false);
  const [actionError, setActionError] = useState<string>();
  const [interruptError, setInterruptError] = useState<string>();
  const [streamError, setStreamError] = useState<string>();
  const [cancelingRunId, setCancelingRunId] = useState<string>();
  const [cancelError, setCancelError] = useState<{
    runId: string;
    message: string;
  }>();
  const leases = useRef(new Map<string, RunLease>());
  const actionInFlight = useRef(false);
  const pendingAction = useRef<AbortController | undefined>(undefined);
  const sendCommand = useRef<SendCommand | undefined>(undefined);
  const resumeCommand = useRef<ResumeCommand | undefined>(undefined);
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
    setInterruptError(undefined);
    setStreamError(undefined);
    setCancelingRunId(undefined);
    setCancelError(undefined);
    seenEvents.current.clear();
    seenOrder.current = [];
    return () => {
      pendingAction.current?.abort();
      pendingAction.current = undefined;
      sendCommand.current = undefined;
      resumeCommand.current = undefined;
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
  const interrupts = useMemo(
    () =>
      Object.values(visible.interruptsByRootRunId).toSorted((left, right) =>
        left.createdAt.localeCompare(right.createdAt),
      ),
    [visible.interruptsByRootRunId],
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
              if (
                frame.event.runId === lease.runId &&
                frame.event.segmentId === lease.segmentId &&
                frame.event.event.type === "segment.finished"
              ) {
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
        run.parentRunId === undefined &&
        run.status === "running" &&
        run.activeSegmentId
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
					selection,
              controller.signal,
            );
          } catch (error) {
            controller.abort();
            throw error;
          }
          if (
            controller.signal.aborted ||
            currentIdentity.current !== actionIdentity
          ) {
            stream.close();
            return;
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
		selection?.model,
		selection?.provider,
      sessionId,
    ],
  );

  const cancel = useCallback(async (runId: string) => {
    const target = visible.runsById[runId];
    if (
      target === undefined ||
      (target.status !== "running" && target.status !== "waiting") ||
      actionInFlight.current
    ) {
      return;
    }
    const actionIdentity = identity;
    const controller = new AbortController();
    actionInFlight.current = true;
    pendingAction.current = controller;
    setActionPending(true);
    setCancelingRunId(runId);
    setCancelError(undefined);
    if (target.parentRunId === undefined) setActionError(undefined);
    try {
      const result = await cancelRun(
        connection,
        runId,
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
        const message = messageOf(error);
        setCancelError({ runId, message });
        if (target.parentRunId === undefined) setActionError(message);
      }
      throw error;
    } finally {
      if (pendingAction.current === controller) {
        pendingAction.current = undefined;
        actionInFlight.current = false;
        setActionPending(false);
        setCancelingRunId(undefined);
      }
    }
  }, [connection, identity, invalidateMaterial, visible.runsById]);

  const stop = useCallback(async () => {
    if (activeRootRun !== undefined) await cancel(activeRootRun.id);
  }, [activeRootRun, cancel]);

  const resume = useCallback(
    async (
      interruptSet: PendingInterruptSet,
      responses: InterruptResponse[],
      idempotencyKey: string,
    ) => {
      if (!sessionId) throw new Error("No session is mounted.");
      if (interruptSet.sessionId !== sessionId) {
        throw new Error("This interrupt belongs to a different session.");
      }
      if (responses.length !== interruptSet.interrupts.length) {
        throw new Error("Responses must cover the complete interrupt set.");
      }
      if (actionInFlight.current) {
        throw new Error("Another run action is already in progress.");
      }
      const actionIdentity = identity;
      const controller = new AbortController();
      actionInFlight.current = true;
      pendingAction.current = controller;
      setActionPending(true);
      setInterruptError(undefined);
      try {
        let command = resumeCommand.current;
        if (command?.key !== idempotencyKey) {
          command = {
            key: idempotencyKey,
            runId: interruptSet.rootRunId,
            responses: structuredClone(responses),
          };
          resumeCommand.current = command;
        }
        let stream: Awaited<ReturnType<typeof resumeRun>>;
        try {
          stream = await resumeRun(
            connection,
            command.runId,
            command.responses,
            idempotencyKey,
            controller.signal,
          );
        } catch (error) {
          controller.abort();
          throw error;
        }
        if (
          controller.signal.aborted ||
          currentIdentity.current !== actionIdentity
        ) {
          stream.close();
          return;
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
        if (resumeCommand.current?.key === idempotencyKey) {
          resumeCommand.current = undefined;
        }
        invalidateMaterial();
      } catch (error) {
        if (currentIdentity.current === actionIdentity && !isAbort(error)) {
          setInterruptError(messageOf(error));
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
    [connection, identity, invalidateMaterial, runLease, sessionId],
  );

  return {
    items,
    runs,
    liveToolOutputs: visible.liveToolOutputsByItemId,
    interrupts,
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
    interruptError,
    streamError,
    cancelingRunId,
    cancelError,
    send,
    resume,
    cancel,
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
    liveToolOutputsByItemId: {},
    interruptsByRootRunId: Object.fromEntries(
      snapshot.interrupts.map((set) => [set.rootRunId, set]),
    ),
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
  for (const [itemId, output] of Object.entries(
    state.liveToolOutputsByItemId,
  )) {
    const item = next.itemsById[itemId];
    if (
      item?.type === "toolCall" &&
      item.status === "running" &&
      next.runsById[item.runId]?.status === "running"
    ) {
      next.liveToolOutputsByItemId[itemId] = output;
    }
  }
  for (const [rootRunId, interruptSet] of Object.entries(
    state.interruptsByRootRunId,
  )) {
    if (
      next.interruptsByRootRunId[rootRunId] === undefined &&
      next.runsById[rootRunId]?.status === "waiting"
    ) {
      next.interruptsByRootRunId[rootRunId] = interruptSet;
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
      return event.run === undefined
        ? state
        : clearInterruptSet(putRun(state, event.run), rootRunIdentity(event.run));
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

function clearInterruptSet(state: AgentSessionState, rootRunId: string) {
  if (state.interruptsByRootRunId[rootRunId] === undefined) return state;
  const interruptsByRootRunId = { ...state.interruptsByRootRunId };
  delete interruptsByRootRunId[rootRunId];
  return { ...state, interruptsByRootRunId };
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
  const liveToolOutputsByItemId = { ...next.liveToolOutputsByItemId };
  for (const item of Object.values(itemsById)) {
    if (item.runId === value.runId) delete liveToolOutputsByItemId[item.id];
  }
  let settled = {
    ...next,
    itemsById,
    itemOrder,
    progressByRunId,
    liveToolOutputsByItemId,
  };
  const rootRunId = rootRunIdentity(current);
  if (outcome.type === "interrupt" && (outcome.interrupts?.length ?? 0) > 0) {
    settled = {
      ...settled,
      interruptsByRootRunId: {
        ...settled.interruptsByRootRunId,
        [rootRunId]: {
          rootRunId,
          sessionId: current.sessionId,
          interrupts: outcome.interrupts ?? [],
          createdAt: value.timestamp,
        },
      },
    };
  } else if (outcome.type !== "suspended") {
    settled = clearInterruptSet(settled, rootRunId);
  }
  return settled;
}

function startItem(state: AgentSessionState, item: Item): AgentSessionState {
  const current = state.itemsById[item.id];
  if (current !== undefined && current.status !== "running") return state;
  return putItem(state, current ?? item);
}

function putItem(state: AgentSessionState, item: Item): AgentSessionState {
  const liveToolOutputsByItemId = state.liveToolOutputsByItemId[item.id]
    && item.status !== "running"
    ? withoutKey(state.liveToolOutputsByItemId, item.id)
    : state.liveToolOutputsByItemId;
  return {
    ...state,
    itemsById: { ...state.itemsById, [item.id]: item },
    itemOrder: state.itemsById[item.id]
      ? state.itemOrder
      : [...state.itemOrder, item.id],
    liveToolOutputsByItemId,
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
  if (delta.type === "toolOutput" && item.type === "toolCall") {
    const text = delta.text ?? "";
    if (text === "") return state;
    return {
      ...state,
      liveToolOutputsByItemId: {
        ...state.liveToolOutputsByItemId,
        [itemId]: appendLiveToolOutput(
          state.liveToolOutputsByItemId[itemId],
          text,
        ),
      },
    };
  }
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

function appendLiveToolOutput(
  current: LiveToolOutput | undefined,
  delta: string,
): LiveToolOutput {
  const combined = (current?.text ?? "") + delta;
  if (combined.length <= liveToolOutputCharacterLimit) {
    return { text: combined, truncated: current?.truncated === true };
  }
  let text = combined.slice(-liveToolOutputCharacterLimit);
  if (isLowSurrogate(text.charCodeAt(0))) text = text.slice(1);
  return {
    text,
    truncated: true,
  };
}

function isLowSurrogate(value: number) {
  return value >= 0xdc00 && value <= 0xdfff;
}

function withoutKey<Value>(values: Record<string, Value>, key: string) {
  const next = { ...values };
  delete next[key];
  return next;
}

function segmentKey(runId: string, segmentId: string) {
  return `${runId}:${segmentId}`;
}

function rootRunIdentity(run: RunRef) {
  return run.parentRunId === undefined ? run.id : (run.rootRunId ?? run.id);
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
