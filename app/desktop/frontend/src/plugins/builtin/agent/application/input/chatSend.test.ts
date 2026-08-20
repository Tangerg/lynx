import { describe, expect, it } from "vitest";
import { canAcceptChatInput } from "./chatSend";

describe("chat input admission", () => {
  it("requires an explicitly selected project before accepting welcome input", () => {
    expect(canAcceptChatInput("", false, "idle")).toBe(false);
  });

  it("requires the existing Session lifecycle owner to be mounted", () => {
    expect(canAcceptChatInput("ses_1", false, "idle")).toBe(false);
    expect(canAcceptChatInput("ses_1", true, "idle")).toBe(true);
  });

  it("rejects a competing turn while the root is parked for HITL", () => {
    expect(canAcceptChatInput("ses_1", true, "waiting")).toBe(false);
  });
});
