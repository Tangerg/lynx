import { describe, expect, it } from "vitest";
import { LARGE_PASTE_CHARS } from "../domain/largePaste";
import {
  composerKeyBindingKey,
  composerPasteIntent,
  hasComposerImageTransferItems,
} from "./composerInputEvents";

const image = new File(["image"], "shot.png", { type: "image/png" });

describe("composerPasteIntent", () => {
  it("prefers pasted images over clipboard text", () => {
    expect(composerPasteIntent([image], "x".repeat(LARGE_PASTE_CHARS))).toEqual({
      kind: "images",
      files: [image],
    });
  });

  it("collapses large pasted text into a paste attachment", () => {
    const text = "x".repeat(LARGE_PASTE_CHARS);

    expect(composerPasteIntent([], text)).toEqual({ kind: "large-text", text });
  });

  it("leaves small text to the browser paste path", () => {
    expect(composerPasteIntent([], "small snippet")).toEqual({ kind: "native" });
  });
});

describe("hasComposerImageTransferItems", () => {
  it("detects image file transfer items", () => {
    expect(
      hasComposerImageTransferItems([
        { kind: "string", type: "text/plain" },
        { kind: "file", type: "image/png" },
      ]),
    ).toBe(true);
  });

  it("ignores non-image file transfer items", () => {
    expect(hasComposerImageTransferItems([{ kind: "file", type: "text/plain" }])).toBe(false);
    expect(hasComposerImageTransferItems(null)).toBe(false);
  });
});

describe("composerKeyBindingKey", () => {
  it("projects keyboard events to canonical composer binding keys", () => {
    expect(
      composerKeyBindingKey(new KeyboardEvent("keydown", { key: "Enter", metaKey: true })),
    ).toBe("mod+enter");
    expect(
      composerKeyBindingKey(
        new KeyboardEvent("keydown", {
          key: "Enter",
          ctrlKey: true,
          altKey: true,
          shiftKey: true,
        }),
      ),
    ).toBe("mod+alt+shift+enter");
    expect(composerKeyBindingKey(new KeyboardEvent("keydown", { key: "ArrowUp" }))).toBe("arrowup");
  });
});
