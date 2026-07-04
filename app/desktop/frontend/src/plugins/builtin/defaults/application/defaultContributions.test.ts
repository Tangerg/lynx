import { describe, expect, it } from "vitest";
import { DEFAULT_ACCENTS, defaultMessageRoles } from "./defaultContributions";

describe("DEFAULT_ACCENTS", () => {
  it("keeps accent ids stable and ordered for the appearance picker", () => {
    expect(DEFAULT_ACCENTS.map((accent) => accent.id)).toEqual(["blue", "green", "pink", "orange"]);
    expect(DEFAULT_ACCENTS.map((accent) => accent.order)).toEqual([0, 1, 2, 3]);
  });

  it("uses distinct light and dark values for every built-in accent", () => {
    expect(DEFAULT_ACCENTS.every((accent) => accent.light && accent.light !== accent.dark)).toBe(
      true,
    );
  });
});

describe("defaultMessageRoles", () => {
  it("projects translated display names into the three built-in message roles", () => {
    const roles = defaultMessageRoles((key) => `t:${key}`);

    expect(roles).toEqual([
      {
        id: "user",
        displayName: "t:role.user",
        icon: "user",
        avatarVariant: "msg-user",
      },
      {
        id: "assistant",
        displayName: "t:role.assistant",
        icon: "spark",
        avatarVariant: "msg-agent",
      },
      {
        id: "system",
        displayName: "t:role.system",
        icon: "shield",
        avatarVariant: "msg-agent",
      },
    ]);
  });
});
