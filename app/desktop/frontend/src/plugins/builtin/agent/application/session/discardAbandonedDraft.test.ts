// An unused draft must not outlive the visit that created it: it is filtered out
// of the session list and there is no tab strip, so a draft the user navigated
// away from is unreachable and would sit on the runtime forever.

import { afterEach, describe, expect, it, vi } from "vitest";
import { configureAgentRuntimeGateway } from "../ports/runtimeGateway";
import { configureAgentSessionStatePort } from "../ports/sessionState";
import { configureAgentSessionViewPort } from "../ports/sessionView";
import type { AgentRuntimeGateway } from "../ports/runtimeGateway";
import type { AgentSessionStatePort } from "../ports/sessionState";
import type { AgentSessionViewPort } from "../ports/sessionView";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import type { Message } from "@/plugins/sdk/types/agentSessionView";
import { discardAbandonedDraft } from "./discardAbandonedDraft";

const disposers: Array<() => void> = [];

function wire({
  drafts,
  messages = {},
  deleteSession = vi.fn().mockResolvedValue(undefined),
}: {
  drafts: string[];
  messages?: Record<string, Message[]>;
  deleteSession?: AgentRuntimeGateway["deleteSession"];
}) {
  disposers.push(
    configureAgentSessionStatePort({
      isDraftSession: (id: string) => drafts.includes(id),
    } as AgentSessionStatePort),
    configureAgentSessionViewPort({
      getSession: (id: string) =>
        messages[id]
          ? { view: { ...EMPTY_AGENT_SESSION_VIEW, messages: messages[id] } }
          : undefined,
    } as unknown as AgentSessionViewPort),
    configureAgentRuntimeGateway({ deleteSession } as AgentRuntimeGateway),
  );
  return deleteSession;
}

afterEach(() => {
  while (disposers.length) disposers.pop()?.();
});

const message = (id: string): Message => ({
  id,
  role: "user",
  runId: null,
  blocks: [],
});

describe("discardAbandonedDraft", () => {
  it("deletes a draft the user never typed into", () => {
    const deleteSession = wire({ drafts: ["draft-1"] });

    discardAbandonedDraft("draft-1");

    expect(deleteSession).toHaveBeenCalledWith("draft-1");
  });

  it("keeps a draft that already holds a message", () => {
    const deleteSession = wire({ drafts: ["draft-1"], messages: { "draft-1": [message("m1")] } });

    discardAbandonedDraft("draft-1");

    expect(deleteSession).not.toHaveBeenCalled();
  });

  it("never touches an ordinary session, however empty it looks", () => {
    // A real session reads as message-less while its history loads.
    const deleteSession = wire({ drafts: [] });

    discardAbandonedDraft("session-1");
    discardAbandonedDraft("");

    expect(deleteSession).not.toHaveBeenCalled();
  });
});
