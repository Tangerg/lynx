import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { selectAgentSession } from "@/plugins/builtin/agent/public/session";
import { MESSAGE_CONTENT_SELECTOR } from "@/plugins/builtin/chat/message/public/rendering";
import { openChatSearch } from "../application/openChatSearch";
import { ChatSearchOverlay } from "./ChatSearchOverlay";

const nativeScrollIntoView = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  "scrollIntoView",
);

beforeEach(() => {
  Object.defineProperty(HTMLElement.prototype, "scrollIntoView", {
    configurable: true,
    value: vi.fn(),
  });
});

afterEach(() => {
  if (nativeScrollIntoView) {
    Object.defineProperty(HTMLElement.prototype, "scrollIntoView", nativeScrollIntoView);
  } else {
    Reflect.deleteProperty(HTMLElement.prototype, "scrollIntoView");
  }
});

function renderSearch(): void {
  render(
    <>
      <article className={MESSAGE_CONTENT_SELECTOR.slice(1)}>Alpha beta ALPHA</article>
      <ChatSearchOverlay />
    </>,
  );
}

function openSearch(): HTMLInputElement {
  act(() => openChatSearch());
  return screen.getByRole("textbox") as HTMLInputElement;
}

describe("ChatSearchOverlay", () => {
  it("updates and navigates matches directly from user events", () => {
    renderSearch();
    const input = openSearch();

    fireEvent.change(input, { target: { value: "alpha" } });
    expect(screen.getByText("1 / 2")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Next match" }));
    expect(screen.getByText("2 / 2")).toBeTruthy();

    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    expect(screen.getByText("1 / 2")).toBeTruthy();
  });

  it("owns reset semantics at close and session boundaries", () => {
    selectAgentSession("session-a");
    renderSearch();
    const input = openSearch();
    fireEvent.change(input, { target: { value: "alpha" } });

    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(openSearch().value).toBe("");

    fireEvent.change(screen.getByRole("textbox"), { target: { value: "beta" } });
    act(() => selectAgentSession("session-b"));
    expect(screen.queryByRole("textbox")).toBeNull();
    expect(openSearch().value).toBe("");
  });
});
