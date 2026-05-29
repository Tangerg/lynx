// First cutover slice — the `sessions` data provider now rides the
// JSON-RPC stack (methods.sessions.list) instead of REST GET /sessions.
// This locks the full wiring: provider → container.methods() → client →
// transport, plus the protocol Session → SidebarSession mapping.

import type { SidebarSession } from "@/lib/data/queries";
import { afterEach, describe, expect, it } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { lookupDataProvider } from "@/plugins/sdk/selectors";
import { createMethods, createRpcClient } from "@/rpc";
import { createMemoryTransport } from "@/rpc/transports/memory";
import { respondSuccess, waitForRequest } from "@/rpc/transports/memory.testkit";
import { defaultData } from "./index";

afterEach(resetContainer);

describe("defaultData — sessions over JSON-RPC", () => {
  it("maps a sessions.list Page<Session> into SidebarSession rows", async () => {
    const t = createMemoryTransport();
    const methods = createMethods(createRpcClient(t));
    setContainer({ methods: () => methods });
    await loadPlugin(defaultData);

    const fetcher = lookupDataProvider<SidebarSession[]>("sessions");
    expect(fetcher).toBeDefined();

    const pending = fetcher!();
    const req = await waitForRequest(t, "sessions.list");
    respondSuccess(t, req.id, {
      items: [
        {
          id: "s1",
          title: "Refactor auth",
          status: "running",
          model: "Sonnet 4.5",
          createdAt: "2026-05-29T00:00:00Z",
          updatedAt: "2026-05-29T01:00:00Z",
          metadata: {},
        },
      ],
      hasMore: false,
    });

    const rows = await pending;
    expect(rows).toEqual([
      {
        id: "s1",
        title: "Refactor auth",
        status: "running",
        model: "Sonnet 4.5",
        time: "2026-05-29T01:00:00Z", // updatedAt wins over createdAt
      },
    ]);
  });
});
