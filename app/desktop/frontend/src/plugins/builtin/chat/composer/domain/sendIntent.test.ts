import { describe, expect, it } from "vitest";
import { createComposerSendIntent, parseSlash } from "./sendIntent";

describe("parseSlash", () => {
  it("splits on the first whitespace", () => {
    expect(parseSlash("/lint src/foo.ts")).toEqual({ cmd: "/lint", args: "src/foo.ts" });
  });

  it("returns empty args when there is no whitespace", () => {
    expect(parseSlash("/diff")).toEqual({ cmd: "/diff", args: "" });
  });

  it("returns null for plain text without a leading slash", () => {
    expect(parseSlash("hello there")).toBeNull();
  });
});

describe("createComposerSendIntent", () => {
  it("rejects empty draft input", () => {
    expect(createComposerSendIntent({ value: "   ", images: [], pastes: [] })).toEqual({
      text: "",
      body: "",
      slash: null,
      shouldSend: false,
      historyText: null,
    });
  });

  it("allows image-only input", () => {
    expect(
      createComposerSendIntent({
        value: "   ",
        images: [{ mime: "image/png", data: "abc" }],
        pastes: [],
      }),
    ).toMatchObject({ body: "", shouldSend: true, historyText: null });
  });

  it("folds pasted text below the typed text", () => {
    expect(
      createComposerSendIntent({
        value: "look",
        images: [],
        pastes: [{ text: "PASTED" }],
      }),
    ).toMatchObject({
      text: "look",
      body: "look\n\nPASTED",
      shouldSend: true,
      historyText: "look",
    });
  });

  it("projects slash commands from typed text only", () => {
    expect(
      createComposerSendIntent({
        value: "/echo hi",
        images: [],
        pastes: [{ text: "/not-a-command" }],
      }),
    ).toMatchObject({
      body: "/echo hi\n\n/not-a-command",
      slash: { cmd: "/echo", args: "hi" },
    });
  });
});
