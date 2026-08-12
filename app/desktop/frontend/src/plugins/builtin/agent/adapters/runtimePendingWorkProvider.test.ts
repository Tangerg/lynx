import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { createLyraClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess, waitForRequest } from "@/rpc/transports/memory.testkit";
import { createHost } from "@/plugins/sdk/host";
import { usePluginStore } from "@/plugins/sdk/registry";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import type { Disposable } from "@/plugins/sdk";
import { PENDING_WORK_KEY, type PendingWorkItem } from "../application/hitl/pendingWork";
import { contributeRuntimePendingWork } from "./runtimePendingWorkProvider";

let disposables: Disposable[] = [];

beforeEach(() => {
  usePluginStore.getState().resetForTest();
  disposables = [];
});

afterEach(() => {
  for (const disposable of disposables.reverse()) disposable.dispose();
  resetContainer();
});

describe("Agent-owned Runtime pending-work provider", () => {
  it("drains install-wide interrupts and publishes only the Agent read model", async () => {
    const transport = createMemoryTransport();
    setContainer({ client: () => createLyraClient(transport) });
    contributeRuntimePendingWork(createHost("agent-provider-test", disposables));

    const fetcher = lookupDataProvider<PendingWorkItem[]>(PENDING_WORK_KEY);
    expect(fetcher).toBeDefined();
    const pending = fetcher!();
    const request = await waitForRequest(transport, "interrupts.list");
    expect(request.params).toEqual({});
    respondSuccess(transport, request.id, {
      data: [
        {
          rootRunId: "run_root",
          sessionId: "ses_1",
          createdAt: "2026-08-12T08:00:00.000Z",
          interrupts: [
            {
              type: "approval",
              itemId: "item_approval",
              runId: "run_child",
              payload: {
                tool: { name: "shell", arguments: { command: "npm test" } },
              },
            },
            {
              type: "question",
              itemId: "item_question",
              runId: "run_child",
              payload: {
                question: {
                  fields: [{ type: "text", prompt: "Which release?", header: "Release" }],
                },
              },
            },
          ],
        },
      ],
    });

    await expect(pending).resolves.toEqual([
      {
        id: "ses_1:run_root",
        sessionId: "ses_1",
        rootRunId: "run_root",
        kind: "approval",
        subject: "shell",
        more: 1,
        waitingSince: "2026-08-12T08:00:00.000Z",
      },
    ]);
  });
});
