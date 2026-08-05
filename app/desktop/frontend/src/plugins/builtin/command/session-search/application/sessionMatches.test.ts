import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import { describe, expect, it } from "vitest";
import { clampHighlight, matchSessions, moveHighlight } from "./sessionMatches";

function session(title: string, time: string): AgentSessionSummary {
  return { id: `ses_${title}`, revision: 1, title, status: "idle", model: "gpt-5.6", time };
}

const SESSIONS = [
  session("Fix the retry loop", "2026-08-01T10:00:00.000Z"),
  session("Retry budget review", "2026-08-03T10:00:00.000Z"),
  session("Rename the dock", "2026-08-02T10:00:00.000Z"),
];

describe("which sessions to offer", () => {
  // The palette answered nothing on an empty query, which was right for a palette:
  // it had commands to show instead. A surface whose whole job is going somewhere
  // has to answer with somewhere.
  it("answers an empty query with the most recent, newest first", () => {
    expect(matchSessions(SESSIONS, "").map((s) => s.title)).toEqual([
      "Retry budget review",
      "Rename the dock",
      "Fix the retry loop",
    ]);
  });

  it("matches any part of the title, case-insensitively, still newest first", () => {
    expect(matchSessions(SESSIONS, "retry").map((s) => s.title)).toEqual([
      "Retry budget review",
      "Fix the retry loop",
    ]);
  });

  it("caps the list so a long history cannot flood it", () => {
    const many = Array.from({ length: 50 }, (_, i) =>
      session(`Session ${i}`, `2026-08-01T10:00:${String(i).padStart(2, "0")}.000Z`),
    );
    expect(matchSessions(many, "")).toHaveLength(20);
    expect(matchSessions(many, "", 3)).toHaveLength(3);
  });
});

// The highlight is held in state while the list is derived from a query, so typing
// one more character can leave the index past the end — which renders as no
// highlight at all and an Enter that opens nothing.
describe("where the highlight lands", () => {
  it("pulls an index past the end back onto the last row", () => {
    expect(clampHighlight(7, 3)).toBe(2);
    expect(clampHighlight(-1, 3)).toBe(0);
    expect(clampHighlight(2, 0)).toBe(0);
  });

  it("wraps at both ends, so the bottom is one Up away", () => {
    expect(moveHighlight(0, 3, -1)).toBe(2);
    expect(moveHighlight(2, 3, 1)).toBe(0);
    expect(moveHighlight(0, 0, 1)).toBe(0);
  });
});
