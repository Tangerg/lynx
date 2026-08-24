import { afterEach, describe, expect, it, vi } from "vitest";
import { queryClient } from "@/lib/queryClient";
import {
  AGENT_SESSIONS_KEY,
  subscribeAgentSessionProjection,
  type AgentSessionSummary,
} from "./sessionQueries";

const workspace = (path: string) => ({ path, availability: "available" as const });

function session(patch: Partial<AgentSessionSummary> = {}): AgentSessionSummary {
  return {
    id: "ses_1",
    revision: 1,
    title: "Session",
    status: "idle",
    provider: "provider",
    model: "model",
    workspace: workspace("/repo"),
    time: "2026-08-11T00:00:00Z",
    ...patch,
  };
}

afterEach(() => queryClient.clear());

describe("subscribeAgentSessionProjection", () => {
  it("ignores cache lifecycle churn and data changes outside the selected projection", async () => {
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session()]);
    const onChange = vi.fn();
    const unsubscribe = subscribeAgentSessionProjection(
      (sessions) =>
        JSON.stringify(sessions?.map(({ id, workspace: value }) => [id, value.path]) ?? null),
      onChange,
    );

    await queryClient.invalidateQueries({
      queryKey: [AGENT_SESSIONS_KEY],
      refetchType: "none",
    });
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session({ status: "running" })]);
    queryClient.setQueryData(["unrelated"], [session({ workspace: workspace("/elsewhere") })]);
    expect(onChange).not.toHaveBeenCalled();

    queryClient.setQueryData(
      [AGENT_SESSIONS_KEY],
      [session({ workspace: workspace("/elsewhere") })],
    );
    expect(onChange).toHaveBeenCalledOnce();
    expect(onChange).toHaveBeenCalledWith(JSON.stringify([["ses_1", "/elsewhere"]]));

    unsubscribe();
    queryClient.setQueryData([AGENT_SESSIONS_KEY], [session({ workspace: workspace("/third") })]);
    expect(onChange).toHaveBeenCalledOnce();
  });
});
