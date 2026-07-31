import { afterEach, describe, expect, it } from "vitest";
import { installChatSearchHighlightStyles } from "./searchHighlights";

afterEach(() => {
  document.getElementById("lyra-chat-search-highlight-styles")?.remove();
});

describe("installChatSearchHighlightStyles", () => {
  it("owns one runtime stylesheet and removes the instance it created", () => {
    const dispose = installChatSearchHighlightStyles();

    const style = document.getElementById("lyra-chat-search-highlight-styles");
    expect(style?.textContent).toContain("::highlight(chat-search-active)");

    dispose();
    expect(document.getElementById("lyra-chat-search-highlight-styles")).toBeNull();
  });

  it("does not remove a stylesheet owned by an existing adapter instance", () => {
    const disposeOwner = installChatSearchHighlightStyles();
    const disposeGuest = installChatSearchHighlightStyles();

    disposeGuest();
    expect(document.getElementById("lyra-chat-search-highlight-styles")).not.toBeNull();

    disposeOwner();
  });
});
