import { describe, expect, it } from "vitest";
import { draftMentions, removeMention } from "./draftContext";

describe("draftMentions", () => {
  it("finds a file the draft attached", () => {
    expect(draftMentions("look at @src/app.ts please")).toEqual([
      { path: "src/app.ts", start: 8, end: 19 },
    ]);
  });

  it("ignores an address, which is the reason the token must start a word", () => {
    expect(draftMentions("mail user@host.com")).toEqual([]);
  });

  it("keeps the same file twice apart, so closing one chip keeps the other", () => {
    const found = draftMentions("@a.ts and @a.ts");
    expect(found.map((m) => m.start)).toEqual([0, 10]);
  });

  it("finds one at the very start and the very end", () => {
    expect(draftMentions("@a.ts")).toHaveLength(1);
    expect(draftMentions("see @b.ts")).toHaveLength(1);
  });
});

describe("removeMention", () => {
  const only = (value: string) => draftMentions(value)[0]!;

  it("closes the gap it leaves rather than doubling the space", () => {
    const value = "look at @src/app.ts please";
    expect(removeMention(value, only(value))).toBe("look at please");
  });

  it("takes the whole thing when the draft was only a mention", () => {
    expect(removeMention("@a.ts", only("@a.ts"))).toBe("");
  });

  it("removes the occurrence it was given, not the first match of the same path", () => {
    const value = "@a.ts and @a.ts";
    const second = draftMentions(value)[1]!;
    expect(removeMention(value, second)).toBe("@a.ts and");
  });
});
