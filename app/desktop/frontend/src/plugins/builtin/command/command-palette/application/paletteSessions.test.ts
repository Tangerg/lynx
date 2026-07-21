import { describe, expect, it } from "vitest";
import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";
import { filterSessionsForPalette } from "./paletteSessions";

function session(id: string, title: string): AgentSessionSummary {
  return { id, revision: 1, title, status: "idle", model: "m", time: "" };
}

const sessions = [
  session("1", "Fix the auth bug"),
  session("2", "Refactor the parser"),
  session("3", "AUTH token rotation"),
  session("4", ""),
];

describe("filterSessionsForPalette", () => {
  it("returns nothing for an empty query (commands-only palette)", () => {
    expect(filterSessionsForPalette(sessions, "")).toEqual([]);
    expect(filterSessionsForPalette(sessions, "   ")).toEqual([]);
  });

  it("matches titles case-insensitively", () => {
    const ids = filterSessionsForPalette(sessions, "auth").map((s) => s.id);
    expect(ids).toEqual(["1", "3"]);
  });

  it("caps the result count", () => {
    const many = Array.from({ length: 50 }, (_, i) => session(String(i), `session ${i}`));
    expect(filterSessionsForPalette(many, "session", 5)).toHaveLength(5);
  });
});
