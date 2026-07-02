import { describe, expect, it } from "vitest";
import type { ContextDockDestinationSpec } from "@/plugins/sdk";
import { groupContextDockDestinations } from "./contextDockDestinationGroups";

const destination = (
  patch: Partial<ContextDockDestinationSpec> & Pick<ContextDockDestinationSpec, "id" | "scope">,
): ContextDockDestinationSpec => ({
  ...patch,
  id: patch.id,
  scope: patch.scope,
  title: `title.${patch.id}`,
  placement: "context-dock",
});

describe("groupContextDockDestinations", () => {
  it("groups destinations by workspace mental-model scope", () => {
    const groups = groupContextDockDestinations([
      destination({ id: "timeline", scope: "session", order: 10 }),
      destination({ id: "plan", scope: "run", order: 10 }),
      destination({ id: "files", scope: "workspace", order: 20 }),
      destination({ id: "search", scope: "workspace", order: 10 }),
    ]);

    expect(groups.map((group) => group.id)).toEqual(["workspace", "run", "session"]);
    expect(groups[0]?.destinations.map((item) => item.id)).toEqual(["search", "files"]);
    expect(groups[1]?.destinations.map((item) => item.id)).toEqual(["plan"]);
    expect(groups[2]?.destinations.map((item) => item.id)).toEqual(["timeline"]);
  });
});
