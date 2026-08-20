import { describe, expect, it } from "vitest";
import { ICON_NAMES, type IconName } from "@/ui/icons";
import { TOOL_ICON_BY_NAME } from "@/lib/toolFamilies";
import { defaultToolIconContributions, defaultToolIconFor } from "./toolIconContributions";

const entries = (items: { key: string; icon: string }[]) =>
  Object.fromEntries(items.map((item) => [item.key, item.icon]));

describe("tool icon contributions", () => {
  // The glyph is the only part of a folded row a reader takes in without reading
  // it, so a shared one spends that on nothing: `list` used to stand for reading
  // shell output, three plan-mode calls and a deferred result, and `search` for
  // grep, two recall families and the tool catalog. Assert the WHOLE table is
  // injective rather than spot-checking pairs — the pairs were what let the reuse
  // build up unnoticed.
  it("gives every built-in tool a glyph of its own", () => {
    const byGlyph = new Map<string, string[]>();
    for (const [tool, glyph] of Object.entries(TOOL_ICON_BY_NAME)) {
      byGlyph.set(glyph, [...(byGlyph.get(glyph) ?? []), tool]);
    }
    const shared = [...byGlyph].filter(([, tools]) => tools.length > 1);

    expect(shared).toEqual([]);
    expect(byGlyph.size).toBe(Object.keys(TOOL_ICON_BY_NAME).length);
    expect(Object.keys(TOOL_ICON_BY_NAME)).toHaveLength(30);
    expect(TOOL_ICON_BY_NAME).not.toHaveProperty("edit");
    expect(TOOL_ICON_BY_NAME).not.toHaveProperty("write");
  });

  // A glyph the vocabulary does not have renders as nothing at all, and the table
  // is a plain Record of strings — so this is the only thing standing between a
  // typo here and an invisible icon in the transcript.
  it("names glyphs the icon vocabulary actually has", () => {
    const unknown = Object.values(TOOL_ICON_BY_NAME).filter(
      (glyph) => !ICON_NAMES.has(glyph as IconName),
    );
    expect(unknown).toEqual([]);
  });

  it("turns the default icon table into registry contributions", () => {
    expect(entries(defaultToolIconContributions())).toEqual(TOOL_ICON_BY_NAME);
  });

  it("falls back to the generic tool glyph for a name it does not know", () => {
    expect(defaultToolIconFor("lsp")).toBe("code");
    expect(defaultToolIconFor("acme_do_thing")).toBe("tool");
  });
});
