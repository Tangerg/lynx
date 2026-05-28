// Run + step lifecycle handlers — RUN_STARTED / RUN_FINISHED / RUN_ERROR
// / STEP_STARTED / STEP_FINISHED.

import type {
  RunErrorEvent,
  RunStartedEvent,
  StepFinishedEvent,
  StepStartedEvent,
} from "@ag-ui/core";
import type { AgentViewState } from "@/protocol/agui/viewState";
import { appendTimeline } from "../helpers";

export const onRunStarted = (state: AgentViewState, ev: RunStartedEvent): AgentViewState => {
  const next: AgentViewState = {
    ...state,
    // Clear the previous run's error banner on a fresh start — once the
    // agent is moving again, the stale message would just confuse.
    error: null,
    run: { ...state.run, running: true, threadId: ev.threadId, runId: ev.runId },
  };
  return appendTimeline(next, { kind: "run-start", runId: ev.runId, refId: ev.runId });
};

export const onRunError = (state: AgentViewState, ev: RunErrorEvent): AgentViewState => {
  const next: AgentViewState = {
    ...state,
    error: { message: ev.message, code: ev.code },
    run: { ...state.run, running: false, activity: "" },
  };
  return appendTimeline(next, { kind: "run-error", summary: ev.message, status: "err" });
};

export const onRunFinished = (state: AgentViewState): AgentViewState => {
  const next: AgentViewState = { ...state, run: { ...state.run, running: false } };
  return appendTimeline(next, { kind: "run-end", status: "ok" });
};

export const onStepStarted = (state: AgentViewState, ev: StepStartedEvent): AgentViewState => {
  const next: AgentViewState = { ...state, run: { ...state.run, activity: ev.stepName } };
  return appendTimeline(next, { kind: "step-start", summary: ev.stepName });
};

// STEP_FINISHED clears `activity` (the topbar "what is the agent doing"
// pill) and bumps the step counter — keeping the old step name visible
// after it finishes is misleading.
export const onStepFinished = (state: AgentViewState, ev: StepFinishedEvent): AgentViewState => {
  const next: AgentViewState = {
    ...state,
    run: {
      ...state.run,
      step: state.run.step + 1,
      // Only clear if it matches — defensive against out-of-order events.
      activity: state.run.activity === ev.stepName ? "" : state.run.activity,
    },
  };
  return appendTimeline(next, { kind: "step-end", summary: ev.stepName });
};
