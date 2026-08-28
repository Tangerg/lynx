import { describe, expect, it } from "vitest";
import {
  EMPTY_AGENT_SESSION_VIEW,
  type AgentRunView,
  type AgentSessionView,
  type Message,
} from "@/plugins/sdk/types/agentSessionView";
import {
  selectCurrentRootAttention,
  selectDelegatedRunNarratives,
  selectRootNarrativeMessages,
  selectRunTree,
} from "./runTree";

function rootRun(id: string, createdAt: string, status: AgentRunView["status"] = "finished") {
  return run({
    id,
    rootRunId: id,
    createdAt,
    status,
    outcome: status === "finished" ? { type: "completed" } : null,
    activeSegmentId: status === "running" ? `segment-${id}` : null,
    finishedAt: status === "finished" ? createdAt : null,
  });
}

function childRun(
  id: string,
  parentRunId: string,
  rootRunId: string,
  spawnedByItemId: string,
  createdAt: string,
) {
  return run({ id, parentRunId, rootRunId, spawnedByItemId, createdAt });
}

function run(overrides: Partial<AgentRunView> & Pick<AgentRunView, "id">): AgentRunView {
  const { id, ...rest } = overrides;
  return {
    id,
    sessionId: "session-1",
    parentRunId: null,
    rootRunId: overrides.id,
    spawnedByItemId: null,
    status: "finished",
    activeSegmentId: null,
    outcome: { type: "completed" },
    metrics: {
      steps: 1,
      activeDurationMillis: 10,
      usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: null,
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: "2026-01-01T00:00:01.000Z",
    ...rest,
  };
}

function message(id: string, runId: string | null): Message {
  return {
    id,
    runId,
    role: "assistant",
    blocks: [{ kind: "text", text: id, status: "complete" }],
  };
}

function view(runs: AgentRunView[], messages: Message[] = []): AgentSessionView {
  return {
    ...EMPTY_AGENT_SESSION_VIEW,
    messages,
    runsById: Object.fromEntries(runs.map((item) => [item.id, item])),
  };
}

describe("root narrative", () => {
  it("keeps all root turns and local messages while excluding descendant material", () => {
    const rootA = rootRun("root-a", "2026-01-01T00:00:00.000Z");
    const rootB = rootRun("root-b", "2026-01-02T00:00:00.000Z");
    const child = childRun("child", "root-b", "root-b", "task-item", "2026-01-02T00:00:01.000Z");
    const projection = view(
      [rootA, rootB, child],
      [
        message("root-a-message", rootA.id),
        message("child-message", child.id),
        message("local-message", null),
        message("root-b-message", rootB.id),
      ],
    );

    expect(selectRootNarrativeMessages(projection).map((item) => item.id)).toEqual([
      "root-a-message",
      "local-message",
      "root-b-message",
    ]);
  });
});

describe("delegated narratives", () => {
  it("anchors siblings and nested work to exact parent Items in durable order", () => {
    const root = rootRun("root", "2026-01-01T00:00:00.000Z");
    const childB = childRun("child-b", root.id, root.id, "task-root", "2026-01-01T00:00:02.000Z");
    const childA = childRun("child-a", root.id, root.id, "task-root", "2026-01-01T00:00:01.000Z");
    const nested = childRun("nested", childA.id, root.id, "task-child", "2026-01-01T00:00:03.000Z");
    const projection = {
      ...view(
        [root, childB, nested, childA],
        [message("a", childA.id), message("b", childB.id), message("nested", nested.id)],
      ),
    };

    const narratives = selectDelegatedRunNarratives(projection);
    expect(narratives["task-root"]?.map((item) => item.run.id)).toEqual(["child-a", "child-b"]);
    expect(narratives["task-root"]?.[0]?.messages.map((message) => message.id)).toEqual(["a"]);
    expect(narratives["task-child"]?.map((item) => item.run.id)).toEqual(["nested"]);
  });
});

describe("Run tree", () => {
  it("derives root, sibling, and nested lineage without storing a second index", () => {
    const root = rootRun("root", "2026-01-01T00:00:00.000Z");
    const sibling = childRun("sibling", root.id, root.id, "task-root", "2026-01-01T00:00:02.000Z");
    const child = childRun("child", root.id, root.id, "task-root", "2026-01-01T00:00:01.000Z");
    const nested = childRun("nested", child.id, root.id, "task-child", "2026-01-01T00:00:03.000Z");

    const tree = selectRunTree(view([nested, sibling, root, child]));
    expect(tree.map((node) => node.run.id)).toEqual(["root"]);
    expect(tree[0]?.children.map((node) => node.run.id)).toEqual(["child", "sibling"]);
    expect(tree[0]?.children[0]?.children.map((node) => node.run.id)).toEqual(["nested"]);
  });

  it("keeps an unconnected child visible for audit", () => {
    const detached = childRun(
      "detached",
      "missing",
      "missing",
      "task-missing",
      "2026-01-01T00:00:00.000Z",
    );
    expect(selectRunTree(view([detached])).map((node) => node.run.id)).toEqual(["detached"]);
  });
});

describe("root attention", () => {
  it("distinguishes idle, running, waiting, and finished roots", () => {
    expect(selectCurrentRootAttention(view([]))).toEqual({ status: "idle", runId: null });
    expect(
      selectCurrentRootAttention(view([rootRun("running", "2026-01-01T00:00:00.000Z", "running")])),
    ).toEqual({ status: "running", runId: "running" });
    expect(
      selectCurrentRootAttention(
        view([
          rootRun("older-running", "2026-01-01T00:00:00.000Z", "running"),
          rootRun("newer-waiting", "2026-01-02T00:00:00.000Z", "waiting"),
        ]),
      ),
    ).toEqual({ status: "waiting", runId: "newer-waiting" });
    expect(
      selectCurrentRootAttention(
        view([rootRun("finished", "2026-01-01T00:00:00.000Z", "finished")]),
      ),
    ).toEqual({ status: "finished", runId: "finished" });
  });
});
