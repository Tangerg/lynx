import { describe, expect, it } from "vitest";

import type { RunEvent } from "./shapes";

import custom from "./samples/custom.json";
import itemCompleted from "./samples/item.completed.json";
import itemDelta from "./samples/item.delta.json";
import itemStarted from "./samples/item.started.json";
import runFinished from "./samples/run.finished.json";
import runProgress from "./samples/run.progress.json";
import runStarted from "./samples/run.started.json";
import stateDelta from "./samples/state.delta.json";
import stateSnapshot from "./samples/state.snapshot.json";

// Strip the phantom id brands. RunId / SessionId / ItemId are `string & {brand}`
// (ids.ts) — a compile-time-only nominal tag that does NOT exist on the wire —
// so an imported JSON sample (plain strings) can be structurally checked against
// the branded wire interface without asserting the brand.
type Unbrand<T> = T extends string
  ? string
  : T extends readonly (infer E)[]
    ? Unbrand<E>[]
    : T extends object
      ? { [K in keyof T]: Unbrand<T[K]> }
      : T;

// The TS half of the API.md §14 drift gate: each canonical wire sample must
// structurally satisfy the hand-written wire type. Renaming / removing /
// retyping a shapes.ts field away from the sample fails `tsc` (the frontend
// `check` gate). The Go side (protocol/wire_golden_test.go) round-trips the SAME
// files against the SSOT structs, so the two pin one contract — replacing the
// old "keep in sync by review" with a machine check.
const samples = [
  runStarted,
  runProgress,
  runFinished,
  itemStarted,
  itemDelta,
  itemCompleted,
  stateSnapshot,
  stateDelta,
  custom,
] satisfies Unbrand<RunEvent>[];

describe("wire golden samples", () => {
  it("carry the RunEvent envelope + a StreamEvent discriminator", () => {
    for (const ev of samples) {
      expect(ev.runId).toBeTruthy();
      expect(ev.eventId).toBeTruthy();
      expect(ev.event.type).toBeTruthy();
    }
  });
});
