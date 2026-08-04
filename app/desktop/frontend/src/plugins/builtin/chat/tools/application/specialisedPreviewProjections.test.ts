import { describe, expect, it } from "vitest";
import {
  projectAskUserAnswer,
  projectConversationHits,
  projectFetchedPage,
  projectGlobPreview,
  projectGoalPreview,
  projectHttpPreview,
  projectRecalledMemories,
  projectSchedulePreviews,
  projectSkillPreview,
  projectToolSearchGroups,
  projectWebSearchPreview,
} from "./specialisedPreviewProjections";
import { parseJsonResult, resultLines } from "./toolResultParsing";

describe("tool result parsing", () => {
  it("returns trimmed result lines and only parses JSON objects", () => {
    expect(resultLines(" a\nb\n\n")).toEqual(["a", "b"]);
    expect(parseJsonResult('{"ok": true}')).toEqual({ ok: true });
    expect(parseJsonResult("[1,2]")).toBeUndefined();
    expect(parseJsonResult("plain")).toBeUndefined();
  });
});

describe("specialised preview projections", () => {
  it("parses skill catalog entries", () => {
    expect(
      projectSkillPreview(
        "<available_skills><skill><name>docs</name><description>Read docs</description></skill></available_skills>",
      ),
    ).toEqual([{ name: "docs", description: "Read docs" }]);
  });

  it("flattens ask_user answer shapes", () => {
    expect(projectAskUserAnswer("plain answer")).toBe("plain answer");
    expect(projectAskUserAnswer('{"answer":"yes"}')).toBe("yes");
    expect(projectAskUserAnswer('{"choices":["red","blue"],"note":"done"}')).toBe(
      "red, blue · done",
    );
  });

  // The runtime's search presentation folds every grep/glob output mode into one
  // `hits` envelope before it reaches the wire, so there is one key to read and no
  // priority left to get wrong.
  it("reads paths from the runtime's single hits envelope", () => {
    expect(projectGlobPreview('{"hits":[{"path":"src/a.ts"},{"path":"src/b.ts"}]}')).toEqual({
      paths: ["src/a.ts", "src/b.ts"],
    });
    expect(projectGlobPreview('{"files":["src/b.ts"]}')).toEqual({ paths: [] });
  });

  it("projects web search results without depending on UI types", () => {
    expect(
      projectWebSearchPreview(
        '{"results":[{"url":"https://www.example.com/a","title":"Example","snippet":"One"},{"url":""}]}',
      ),
    ).toEqual([
      {
        url: "https://www.example.com/a",
        domain: "example.com",
        title: "Example",
        snippet: "One",
      },
    ]);
  });
});

// The prose-answering tools. Each fixture is the runtime's ACTUAL output format —
// memorysearch/sessionsearch `results.String()`, discovery's catalog listing, and
// planpresentation.Render — because a parser tested against invented text is a
// parser tested against nothing.
describe("prose tool results", () => {
  it("keeps a wrapped memory whole and preserves the runtime's ranking", () => {
    expect(
      projectRecalledMemories(
        "1. Deploys go through the release branch.\n   Never from main.\n2. Staging resets nightly.",
      ),
    ).toEqual([
      "Deploys go through the release branch.\n   Never from main.",
      "Staging resets nightly.",
    ]);
    // The "nothing found" sentence is prose, not an entry — it must not render as one.
    expect(projectRecalledMemories("No relevant memories found for this project.")).toEqual([]);
  });

  it("splits a conversation hit into speaker, day and snippet", () => {
    expect(
      projectConversationHits(
        "1. [user · 2026-07-31] why did the retry loop change?\n2. [agent · 2026-08-01] because the backoff used real timers",
      ),
    ).toEqual([
      { speaker: "user", day: "2026-07-31", snippet: "why did the retry loop change?" },
      { speaker: "agent", day: "2026-08-01", snippet: "because the backoff used real timers" },
    ]);
    expect(projectConversationHits("No earlier conversation matched.")).toEqual([]);
  });

  it("groups loaded tool names by where they came from", () => {
    expect(
      projectToolSearchGroups(
        "Load additional built-in tools on demand.\n\nNot loaded:\n  [builtin] create_goal, get_goal\n  [mcp:sentry] list_issues",
      ),
    ).toEqual([
      { source: "builtin", names: ["create_goal", "get_goal"] },
      { source: "mcp:sentry", names: ["list_issues"] },
    ]);
  });
});

describe("structured tool results", () => {
  it("reads a goal's objective, status and both sides of every budget axis", () => {
    expect(
      projectGoalPreview(
        '{"goal":{"session_id":"ses_1","objective":"Ship the retry fix","status":"active","budget":{"max_runs":20,"max_cost_usd":5},"usage":{"runs":7,"cost_usd":1.25,"steps":31}},"message":"Goal started."}',
      ),
    ).toEqual({
      objective: "Ship the retry fix",
      status: "active",
      reason: "",
      runs: { used: 7, max: 20 },
      cost: { used: 1.25, max: 5 },
      // An omitted max is uncapped on that axis, which the preview renders as no bar
      // rather than as a full one.
      steps: { used: 31, max: 0 },
      message: "Goal started.",
    });
    expect(projectGoalPreview('{"message":"no goal"}')).toBeUndefined();
  });

  it("reads one created schedule and a whole list through the same reader", () => {
    const one = projectSchedulePreviews(
      '{"schedule":{"schedule_id":"sch_1","title":"Nightly audit","cron":"0 3 * * *","instructions":"audit deps","enabled":true,"next_run_at":"2026-08-05T03:00:00Z"}}',
    );
    expect(one).toEqual([
      {
        id: "sch_1",
        title: "Nightly audit",
        cron: "0 3 * * *",
        instructions: "audit deps",
        enabled: true,
        nextRunAt: "2026-08-05T03:00:00Z",
        lastRunAt: "",
      },
    ]);
    expect(
      projectSchedulePreviews(
        '{"schedules":[{"schedule_id":"a","cron":"@daily","enabled":false}]}',
      ),
    ).toMatchObject([{ id: "a", cron: "@daily", enabled: false }]);
  });

  it("reads an http response's status, timing and header count", () => {
    expect(
      projectHttpPreview(
        '{"status":503,"headers":{"content-type":"application/json","retry-after":"30"},"body":"{}","truncated":true,"duration":"412ms"}',
      ),
    ).toEqual({
      status: 503,
      duration: "412ms",
      truncated: true,
      headers: [
        ["content-type", "application/json"],
        ["retry-after", "30"],
      ],
      body: "{}",
    });
    // A plain-string answer is not a response envelope.
    expect(projectHttpPreview("blocked by allowlist")).toBeUndefined();
  });

  it("reads a fetched page and defaults its unnamed dialect to text", () => {
    expect(projectFetchedPage('{"content":"# Title","format":"markdown"}')).toEqual({
      content: "# Title",
      format: "markdown",
    });
    expect(projectFetchedPage('{"content":"raw"}')).toEqual({ content: "raw", format: "text" });
  });
});
