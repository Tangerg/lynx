# Desktop UI Polish

This document captures two recurring quality bars that are easy to lose when a
React WebView app evolves quickly:

- surface depth should come from a deliberate shadow model, not gray borders or
  one large blurry shadow;
- ScopeApp should feel like a desktop app that happens to use web technology, not a
  website embedded in a window.

References:

- [高级感 UI 的边缘为什么看起来舒服：用 box-shadow hairline 解决卡片边缘发灰的问题](https://www.uweb.net.cn/zhishiku/wangzhanqianduankaifa/36454.html)
- [A Technical Deep Dive Into the New Raycast](https://www.raycast.com/blog/a-technical-deep-dive-into-the-new-raycast)

## 1. Shadow Model

The source article's useful idea is not "add stronger shadows". It is a layered
model:

1. **Edge**: a 1px optical boundary.
2. **Contact shadow**: a tiny near shadow that says the surface touches the
   background.
3. **Ambient shadow**: a wide, low-alpha falloff that gives elevation without a
   muddy border.

**Where the edge comes from (revised 2026-08).** A shadow ring stacked on a border
reads as a double edge, so each role owns exactly one mechanism — and, crucially,
**which mechanism a role uses is the active visual style's decision, not a call
site's**. The tool-window style now shipping answers:

- **region boundary** → NO line. Regions separate by value, and the seam is a short
  directional cast (`--app-card-edge` at the drawer, `--app-pane-split` at the
  dock). A hairline here draws the columns as pasted rectangles.
- **card** → fill only (`bg-card`). No border, no shadow: it is already a different
  material from the plane it sits on.
- **well** → fill only (`bg-sunken`).
- **floating overlay** (menus, popovers, tooltips) → one optical ring plus
  directional depth in `--shadow-popover`, and no second border. These are the only
  surfaces that genuinely leave the plane.
- **fixed control** (text fields, chips, the composer) → a real border. The composer
  takes the accent on focus, which is the same "live" colour the run indicator uses.
- **state** → background fill only (`bg-hover` / `bg-selected`), at a strength tied
  to the same ladder step the regions use.

Do not paste shadow values into components. Translate them through the rings:

- geometry, elevation and region roles → the **visual style** (`visualStyles/tokens.ts`);
- how dark a cast is for this palette → the **theme** (`--shadow-cast`);
- consume `var(--shadow-*)` / `var(--app-*)` from components.

## 2. Border Discipline

A border is cheap when it is decoration or when it compensates for weak
elevation. It is correct when it IS the edge of a control or a region.

**Revised 2026-08.** Two earlier models both failed, in opposite directions. The
first separated regions by background delta alone, at a delta too small to read —
`#f2f2f2` chrome against `#ffffff` — so the eye had no cue at all. The second gave
every boundary a hairline, which turned three columns into a wireframe. What works
is the third thing: a delta big enough to read as a different material, **plus** a
short directional cast at the seam, **and no line**.

Preferred:

- drawer ↔ plane: the plane's own inset cast (`--app-card-edge`), because the plane
  outranks the drawer on z-index and a panel underneath cannot cast onto it;
- dock ↔ conversation: the dock casts leftward (`--app-pane-split`);
- chrome-bar bottoms: nothing — a bar is the top of its column, not a third material;
- composer: a real field border, accent on focus;
- floating overlay edge: the shadow's own first layer at `--seam-line`, never a
  border as well;
- fixed control edge (text fields and chips): a real `border`;
- row state: fill delta;
- focus: the global keyboard-only ring — never a per-control one;
- danger/warning state: semantic border/fill token.

Avoid:

- a border AND a shadow edge ring on the same surface (double edge);
- gray borders that are only compensating for weak elevation;
- more than one line at a boundary — two 1px semi-transparent lines sharing a pixel
  double their alpha and read as a bright dot;
- decorative outlines that do not communicate structure, focus, or state.

## 3. Native WebView Feel

The Raycast article's key distinction is architectural and behavioral: a desktop
app may use web technology for UI, but it should not inherit website defaults.

Rules for ScopeApp:

- Do not use `cursor: pointer` as a default marker for controls. Desktop apps do
  not need the browser hand cursor everywhere.
- Do not add hover highlights just because an element is interactive. Hover is
  reserved for dense lists, sidebar rows, icon buttons, and controls where it
  improves scanability.
- Glass blur is material, not decoration. It belongs on exactly two things: the
  drawer surface (a translucent panel the content card floats over) and floating
  panels (menus, popovers, tooltips), where letting a hint of what they cover show
  through is what makes them read as above the surface. Everywhere else, tokenized
  surfaces and shadows.
- Prefer native-feeling immediacy: no flicker, no layout jumps, no delayed
  reveal that makes a persistent surface feel like a web page entering.
- Popovers, command palette, tooltips, and composer overlays should read as
  application chrome: compact, anchored, keyboard-safe, and using the same
  elevation tokens.

## 4. Practical Checklist

Before merging UI polish work, inspect these points:

- Does the role use exactly one edge mechanism: real border, optical ring, or
  region hairline?
- Is the shadow token semantic enough for reuse, or is it a one-off arbitrary
  class?
- Does the component still work in light and dark themes?
- Does the UI rely on blur, opacity, or low contrast to feel "premium"?
- Are primary labels clear enough on a neutral sidebar or panel surface?
- Are hover states restrained and purposeful?
- Are interactive controls keyboard/focus safe through Base UI or existing common
  primitives?
- Did the change preserve the plugin system boundary instead of reaching around
  registry/slots?

## 5. Anti-Patterns

These are regressions:

- adding a new hardcoded shadow value inside a feature component;
- using border and shadow together without a clear reason for each layer;
- drawing a line at a region boundary — that decision belongs to the visual style,
  and the one shipping does not draw one;
- inverting a surface with `bg-fg` to make it stand out: it flips with the scheme,
  so the thing you emphasised becomes the brightest object on a dark palette and
  the only inverted one in the app (the approval card's command block did exactly
  this);
- making the light sidebar nearly white against a white canvas;
- adding `backdrop-blur` to make an ordinary panel feel expensive;
- increasing rounded corners to create perceived softness;
- adding hover backgrounds to every button, text link, and list item;
- styling a WebView like a marketing website instead of app chrome.
