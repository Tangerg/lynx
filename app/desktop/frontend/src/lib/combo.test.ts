import { describe, expect, it } from "vitest";
import { matchKeybindingPress, parseKeybinding } from "tinykeys";

import { dispatchBinding } from "./combo";

// A keydown as the browser reports it. `key` is what the active layout prints
// at that position; `code` is the position itself.
function keydown(key: string, code: string, modifiers: string[] = []): KeyboardEvent {
  return {
    key,
    code,
    getModifierState: (modifier: string) => modifiers.includes(modifier),
  } as unknown as KeyboardEvent;
}

function matches(combo: string, event: KeyboardEvent): boolean {
  const [press] = parseKeybinding(dispatchBinding(combo));
  return matchKeybindingPress(event, press!);
}

describe("dispatchBinding", () => {
  it("names the physical key for letters and digits", () => {
    expect(dispatchBinding("Mod+K")).toBe("$mod+KeyK");
    expect(dispatchBinding("Mod+Shift+L")).toBe("$mod+Shift+KeyL");
    expect(dispatchBinding("Alt+3")).toBe("Alt+Digit3");
  });

  it("passes named keys through for tinykeys to match case-insensitively", () => {
    expect(dispatchBinding("Escape")).toBe("Escape");
    expect(dispatchBinding("Mod+Enter")).toBe("$mod+Enter");
    expect(dispatchBinding("Mod+Shift+Backspace")).toBe("$mod+Shift+Backspace");
  });

  it("maps every modifier alias the registry accepts", () => {
    expect(dispatchBinding("cmd+k")).toBe("$mod+KeyK");
    expect(dispatchBinding("meta+k")).toBe("$mod+KeyK");
    expect(dispatchBinding("control+k")).toBe("Control+KeyK");
    expect(dispatchBinding("option+k")).toBe("Alt+KeyK");
  });

  it("keeps a space-separated sequence a sequence", () => {
    expect(dispatchBinding("Mod+K Mod+S")).toBe("$mod+KeyK $mod+KeyS");
    expect(parseKeybinding(dispatchBinding("Mod+K Mod+S"))).toHaveLength(2);
  });
});

describe("a letter shortcut under a non-US keyboard layout", () => {
  // The bug this replaced: ⌃K on a Cyrillic layout reports key "к", so a
  // dispatcher comparing against `KeyboardEvent.key` looked up "ctrl+к" and
  // found nothing. Every letter shortcut in the app was dead on that layout.
  const cyrillicK = keydown("к", "KeyK", ["Control"]);

  it("matches the binding we now emit", () => {
    expect(matches("Ctrl+K", cyrillicK)).toBe(true);
  });

  it("would not have matched one written against the printed character", () => {
    const [press] = parseKeybinding("Control+k");
    expect(matchKeybindingPress(cyrillicK, press!)).toBe(false);
  });

  it("still matches a US layout, where key and code agree", () => {
    expect(matches("Ctrl+K", keydown("k", "KeyK", ["Control"]))).toBe(true);
  });
});
