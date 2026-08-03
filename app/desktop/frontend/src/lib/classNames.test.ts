import { describe, expect, it } from "vitest";
import { cn } from "./classNames";

describe("cn", () => {
  it("keeps custom type size and text colour as independent properties", () => {
    expect(cn("text-cta-text", "text-ui-md")).toBe("text-cta-text text-ui-md");
    expect(cn("text-fg-soft", "text-display-lg")).toBe("text-fg-soft text-display-lg");
  });

  it("still resolves conflicts within each property", () => {
    expect(cn("text-ui-sm", "text-prose")).toBe("text-prose");
    expect(cn("text-fg-muted", "text-fg")).toBe("text-fg");
  });
});
