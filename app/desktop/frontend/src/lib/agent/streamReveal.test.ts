// Unit tests for the `pickRate` selector — the rate-picking heart of
// useStreamReveal. Hook-level behavior (rAF loop, debt accounting, word
// segmentation, sentence pauses, typewriter char-reveal) stays untested
// here: it's a moving target tied to perf characteristics, and the rate
// function is the only piece that captures a clean contract worth pinning.

import { describe, expect, it } from "vitest";
import { pickRate } from "./streamReveal";

describe("pickRate — streaming mode (3-tier ladder)", () => {
  it("returns RATE_CRUISE (40 c/s) for small backlogs", () => {
    expect(pickRate(0, true)).toBe(40);
    expect(pickRate(5, true)).toBe(40);
    expect(pickRate(19, true)).toBe(40);
  });

  it("returns RATE_MODERATE (80 c/s) for mid-sized backlogs", () => {
    expect(pickRate(20, true)).toBe(80);
    expect(pickRate(40, true)).toBe(80);
    expect(pickRate(59, true)).toBe(80);
  });

  it("returns RATE_CATCHUP (160 c/s) for large backlogs", () => {
    expect(pickRate(60, true)).toBe(160);
    expect(pickRate(100, true)).toBe(160);
    expect(pickRate(10_000, true)).toBe(160);
  });

  it("rate strictly increases (or stays equal) with backlog", () => {
    // Property: monotone non-decreasing. Catches any future tier-shuffle
    // regression that would let a bigger backlog drain slower.
    let prev = 0;
    for (let backlog = 0; backlog < 200; backlog += 5) {
      const rate = pickRate(backlog, true);
      expect(rate).toBeGreaterThanOrEqual(prev);
      prev = rate;
    }
  });
});

describe("pickRate — drain mode (streaming=false)", () => {
  it("scales with backlog at DRAIN_RATE_PER_CHAR (=8)", () => {
    // 30 chars × 8 = 240 — within [MIN=80, MAX=280] so no clamp.
    expect(pickRate(30, false)).toBe(240);
  });

  it("clamps to DRAIN_RATE_MIN (=80) for tiny backlogs", () => {
    // 0 chars × 8 = 0 → clamped up to 80.
    expect(pickRate(0, false)).toBe(80);
    // 5 chars × 8 = 40 → also below MIN.
    expect(pickRate(5, false)).toBe(80);
  });

  it("clamps to DRAIN_RATE_MAX (=280) for huge backlogs", () => {
    // 100 chars × 8 = 800 → clamped down to 280.
    expect(pickRate(100, false)).toBe(280);
    expect(pickRate(10_000, false)).toBe(280);
  });

  it("drain rate stays ≥ streaming cruise rate at any backlog", () => {
    // Property: drain mode is never SLOWER than the streaming cruise
    // tier — once the stream stops we always want to catch up at least
    // as quickly as we were going.
    for (let backlog = 0; backlog < 200; backlog += 5) {
      expect(pickRate(backlog, false)).toBeGreaterThanOrEqual(40);
    }
  });
});
