// Deleting a session must never rest on a guess: the list only narrows the
// candidates, and the runtime is asked about each one before it goes.

import { afterEach, describe, expect, it, vi } from "vitest";
import { configureAgentRuntimeGateway } from "../ports/runtimeGateway";
import type { AgentRuntimeGateway } from "../ports/runtimeGateway";
import type { AgentSessionSummary } from "./sessionQueries";
import { pruneUnusedSessions } from "./pruneUnusedSessions";

vi.mock("@/plugins/sdk", () => ({ notifyInfo: vi.fn() }));
vi.mock("./sessionQueries", () => ({
  invalidateAgentSessions: vi.fn().mockResolvedValue(undefined),
}));

const disposers: Array<() => void> = [];

afterEach(() => {
  while (disposers.length) disposers.pop()?.();
});

function wire(holdsNothing: (id: string) => boolean) {
  const deleteSession = vi.fn().mockResolvedValue(undefined);
  disposers.push(
    configureAgentRuntimeGateway({
      sessionHoldsNothing: (id: string) => Promise.resolve(holdsNothing(id)),
      deleteSession,
    } as unknown as AgentRuntimeGateway),
  );
  return deleteSession;
}

const session = (over: Partial<AgentSessionSummary>): AgentSessionSummary => ({
  id: "s1",
  revision: 1,
  title: "",
  status: "idle",
  model: "gpt-5",
  time: "",
  ...over,
});

describe("pruneUnusedSessions", () => {
  it("removes an untitled session the runtime confirms is empty", async () => {
    const deleteSession = wire(() => true);

    expect(await pruneUnusedSessions([session({ id: "empty" })], [])).toBe(1);
    expect(deleteSession).toHaveBeenCalledWith("empty");
  });

  it("keeps an untitled session that turns out to hold something", async () => {
    // The title is only a narrowing signal — a run that never finished leaves it
    // empty, and that session still has a transcript.
    const deleteSession = wire(() => false);

    expect(await pruneUnusedSessions([session({ id: "busy" })], [])).toBe(0);
    expect(deleteSession).not.toHaveBeenCalled();
  });

  it("never asks about a titled, favourite, or open session", async () => {
    const holdsNothing = vi.fn().mockReturnValue(true);
    const deleteSession = wire(holdsNothing);

    const removed = await pruneUnusedSessions(
      [
        session({ id: "titled", title: "Fix the parser" }),
        session({ id: "pinned", favorite: true }),
        session({ id: "restored" }),
      ],
      ["restored"],
    );

    expect(removed).toBe(0);
    expect(holdsNothing).not.toHaveBeenCalled();
    expect(deleteSession).not.toHaveBeenCalled();
  });
});
