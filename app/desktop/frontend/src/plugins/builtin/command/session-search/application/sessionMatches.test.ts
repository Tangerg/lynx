import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import { describe, expect, it } from "vitest";
import { matchSessions } from "./sessionMatches";

function session(title: string, time: string): AgentSessionSummary {
  return {
    id: `ses_${title}`,
    revision: 1,
    title,
    status: "idle",
    provider: "openai",
    model: "gpt-5.6",
    workspace: { path: "/repo", availability: "available" },
    time,
  };
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
