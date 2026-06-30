# Desktop UI Polish

This document captures two recurring quality bars that are easy to lose when a
React WebView app evolves quickly:

- surface depth should come from a deliberate shadow model, not gray borders or
  one large blurry shadow;
- Lyra should feel like a desktop app that happens to use web technology, not a
  website embedded in a window.

References:

- [高级感 UI 的边缘为什么看起来舒服：用 box-shadow hairline 解决卡片边缘发灰的问题](https://www.uweb.net.cn/zhishiku/wangzhanqianduankaifa/36454.html)
- [A Technical Deep Dive Into the New Raycast](https://www.raycast.com/blog/a-technical-deep-dive-into-the-new-raycast)

## 1. Shadow Model

The source article's useful idea is not "add stronger shadows". It is a layered
model:

1. **Edge ring**: a 1px optical boundary implemented as the first shadow layer.
2. **Contact shadow**: a tiny near shadow that says the surface touches the
   background.
3. **Ambient shadow**: a wide, low-alpha falloff that gives elevation without a
   muddy border.

The key CSS shape from the article's image examples is:

```css
box-shadow:
  0 0 0 1px rgb(15 23 42 / 0.08),
  0 1px 2px rgb(15 23 42 / 0.06),
  0 8px 24px rgb(15 23 42 / 0.1);
```

For a stronger lifted surface:

```css
box-shadow:
  0 0 0 1px rgb(15 23 42 / 0.08),
  0 1px 2px rgb(15 23 42 / 0.06),
  0 18px 48px rgb(15 23 42 / 0.14);
```

Do not paste these values directly into components. Translate them through the
theme system:

- define semantic tokens in the theme kit;
- override per theme only when a palette needs different alpha or hue;
- consume `var(--shadow-*)` from components.

## 2. Border Discipline

Hard borders are often what makes a light UI look gray, foggy, or cheap. Use a
border only when it is a real structural rule or input affordance.

Preferred:

- floating surface edge: shadow ring;
- sidebar/main separation: a restrained hairline on the shell boundary;
- row state: surface fill delta;
- focus: focus token;
- danger/warning state: semantic border/fill token.

Avoid:

- `border border-line` plus a large shadow on every card;
- gray borders that are only compensating for weak elevation;
- one huge shadow without an edge ring;
- decorative outlines that do not communicate structure, focus, or state.

## 3. Native WebView Feel

The Raycast article's key distinction is architectural and behavioral: a desktop
app may use web technology for UI, but it should not inherit website defaults.

Rules for Lyra:

- Do not use `cursor: pointer` as a default marker for controls. Desktop apps do
  not need the browser hand cursor everywhere.
- Do not add hover highlights just because an element is interactive. Hover is
  reserved for dense lists, sidebar rows, icon buttons, and controls where it
  improves scanability.
- Do not use glass blur as a cheap premium effect. It should appear only when
  the shell/native environment calls for material; otherwise use tokenized
  surfaces and shadows.
- Prefer native-feeling immediacy: no flicker, no layout jumps, no delayed
  reveal that makes a persistent surface feel like a web page entering.
- Popovers, command palette, tooltips, and composer overlays should read as
  application chrome: compact, anchored, keyboard-safe, and using the same
  elevation tokens.

## 4. Practical Checklist

Before merging UI polish work, inspect these points:

- Does the surface boundary come from `shadow ring + contact + ambient`, or from
  a gray border?
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
- making the light sidebar nearly white against a white canvas;
- adding `backdrop-blur` to make an ordinary panel feel expensive;
- increasing rounded corners to create perceived softness;
- adding hover backgrounds to every button, text link, and list item;
- styling a WebView like a marketing website instead of app chrome.
