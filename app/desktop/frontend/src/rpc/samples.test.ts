import { describe, expect, it } from "vitest";

import { validateStatePayload, validateWire } from "./wire.validate.generated";
import { WIRE_ENUMS } from "./wire.generated";
import { WIRE_SAMPLES } from "./wire.samples.generated";
import requestMeta from "./samples/request.meta.json";

// The TypeScript half of contract §11.3's canonical-sample gate: every hand-written
// fixture is checked against the shape the binding says it is — the SAME binding the
// Go round-trip reads, projected into wire.samples.generated.ts.
//
// This replaced a parallel list of `wire<T>(sample)` pins, which said the same thing
// twice: 78 more lines naming which file was which shape, able to drift from the Go
// table and then silently check a fixture against the wrong type. The pins also only
// reached as far as structural typing does — they widened literals and stripped id
// brands, so a bad enum value, a missing required field, a variant carrying another
// variant's field and every cross-field rule went unchecked. Deriving both the TS
// types and the checks from one schema tree is what makes the weaker leg redundant:
// there is no path by which a shape is stated in the types and not in the checks.
//
// Sample loading is by directory, not by 78 import statements, so a fixture that
// nobody bound fails HERE as well as on the Go side.
const files = import.meta.glob<{ default: unknown }>("./samples/*.json", { eager: true });

function sample(file: string): unknown {
  const loaded = files[`./samples/${file}`];
  if (!loaded) throw new Error(`no such canonical sample: ${file}`);
  return loaded.default;
}

describe("the canonical wire samples", () => {
  it("covers every file in the samples directory", () => {
    const bound = new Set(WIRE_SAMPLES.map((entry) => entry.file));
    const present = Object.keys(files).map((path) => path.replace("./samples/", ""));
    expect(present.filter((file) => !bound.has(file))).toEqual([]);
    expect(WIRE_SAMPLES.length).toBe(present.length);
  });

  it.each(WIRE_SAMPLES)("$file satisfies $shape", ({ file, shape }) => {
    expect(validateWire(shape, sample(file))).toEqual([]);
  });

  // The state envelope is a map, so its own type says nothing about what a key
  // carries — the shape is declared per key and only checkable through that
  // declaration. Without this the canonical snapshot's todos could be anything.
  it("carries the declared shape under every state key", () => {
    const snapshot = sample("state.snapshot.json") as {
      event: { state?: Record<string, unknown> };
    };
    const state = snapshot.event.state ?? {};
    expect(Object.keys(state)).not.toEqual([]);
    for (const [key, value] of Object.entries(state)) {
      expect(validateStatePayload(key, value)).toEqual([]);
    }
  });

  // A client may suppress ephemeral previews. Naming an event the runtime does not
  // publish would be asking to opt out of nothing, and the runtime refuses the
  // request rather than ignoring the entry — so the sample has to be legal.
  it("excludes only published stream events", () => {
    const published = new Set<string>(WIRE_ENUMS.StreamEventType);
    for (const event of requestMeta.clientCapabilities.excludedEphemeralEvents ?? []) {
      expect(published.has(event)).toBe(true);
    }
  });
});
