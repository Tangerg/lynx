# NATIVE_FEEL.md — 不要网页化

> Lyra is a Wails app: a React/TS UI rendered inside the system WebView
> (WKWebView on macOS, WebView2 on Windows), wrapped by a native Go shell.
> Architecturally this is the **same** trade-off Raycast 2.0 made — UI in web,
> control in native. The whole point of that trade-off collapses if the UI
> *reads* like a web page. This doc is the standing rule for keeping it from
> reading that way, plus a checklist of the specific tells to kill.
>
> Source of the playbook: Raycast, *A technical deep dive into the new Raycast*
> (`raycast.com/blog/a-technical-deep-dive-into-the-new-raycast`). We take the
> techniques, not the names; mapped onto our Wails stack below.

---

## The principle

The user should never be able to tell there is a browser under the window. A
WebView betrays itself through **behaviour**, not pixels — the link cursor, the
hover glow, the modal that dims the page, the text that selects when you drag,
the rubber-band overscroll, the flash on launch. Each of these is a default that
makes sense for a *document you can scroll and link to* and makes no sense for a
*desktop app*. Removing the web feel is mostly **subtraction**: turning those
defaults off, one by one, until what's left behaves like AppKit.

Treat "no flicker / no web tells" as a hard quality bar, the same way we treat
type errors — not a polish-later nicety.

---

## The Raycast playbook → our status

Two buckets: **(A)** avoid web conventions that signal "this is a website", and
**(B)** WebView/window engineering so it renders like a native surface. Status
is grounded in what's in the tree today (`src/styles/*`, `main.go`), not aspiration.

### A · Kill the web conventions

| Tell | Native behaviour | Lyra status |
|---|---|---|
| `cursor: pointer` on controls | macOS shows the **arrow** on buttons, not a link hand | ✅ Done — stripped all `cursor-pointer` utils from controls (47 files); buttons/rows/toggles fall back to the arrow. The hand survives only on genuine hyperlinks inside `.md`/`.msg-content` (native `<a href>` default, no util needed). Re-audit on new surfaces via the checklist. |
| Hover highlight on everything | Native rows highlight on **selection / keyboard focus**, hover is subdued or absent | ✅ Loud hover *pops* removed — no control grows (`hover:scale-*`) or lifts (`hover:-translate-y`) on hover (PillButton, composer send, Slider thumb, accent swatches, ToolCard, icon gallery). Quiet surface-tint hovers on rows stay; `active:scale-*` press feedback stays (tactile, not a web tell). Keep new surfaces quiet per the checklist. |
| Text selects when you drag the chrome | Chrome (tabs, sidebar, toolbar, status bar) is non-selectable; only content is | ✅ Done — `body { user-select: none }`, re-enabled on `.msg-content` subtree (`globals.css`). |
| OS-default blue selection bleeding through | A deliberate, on-brand selection tint | ✅ Done — `::selection` accent-tinted (`globals.css`). |
| Drag-on-body = scroll the page / "move the UI" | The document doesn't scroll; only inner panels do | ✅ Done — `html,body,#root { overflow: hidden }` with the WKWebView rationale spelled out (`layout.css`). |
| Settings / dialogs as page-dimming modals | Native apps open **separate windows** or inline panels, not overlays that gray out the app | ◻︎ Prefer inline panels / dedicated views over full-screen dimming modals. Where a Radix Dialog is used, keep it small and app-like, not a page takeover. |
| Tooltips / popovers as DOM nodes | Raycast renders them as **native windows** so they escape the WebView and never clip | ⚠️ **Divergence (accepted).** We use Radix tooltips/popovers (DOM, portaled). Wails has no first-class native-tooltip API, so native windows aren't a cheap win here. Mitigation: portal to body, allow overflow, never let a popover get clipped by a `panel-scroll` container. Revisit only if clipping/throttling becomes visible. |
| Browser scrollbars | Thin, themed, overlay-style scrollbars | ✅ Done — `::-webkit-scrollbar` themed per surface (`layout.css`, `markdown.css`, `globals.css`). |
| Rubber-band / overscroll glow | No bounce at the edges of inner scrollers | ✅ Done — `overscroll-behavior: contain` on `.panel-scroll` (`layout.css`), which is THE scroller everywhere (panels + the chat's inner stick-to-bottom div via `scrollClassName`). |
| Tap highlight, callout, drag-ghost on long-press | Nothing | ✅ Done — `-webkit-tap-highlight-color: transparent` + `-webkit-touch-callout: none` on `html` (inherited tree-wide), `-webkit-user-drag: none` on `img, svg` (`globals.css`). |
| Right-click = browser context menu | App-defined context menu, or nothing | ✅ Done — `native-shell` builtin plugin suppresses the default `contextmenu` (`plugins/builtin/shell/native-shell`). Exempts real text fields (system edit menu) and leaves Radix context menus intact (they open at the trigger before the document listener). |
| Native font edges | Crisp, OS-matched text | ✅ Done — `-webkit-font-smoothing: antialiased`; mono carries `tabular-nums` (`globals.css`). |
| Fonts fetched from a CDN | A desktop app ships its fonts; it doesn't phone Google on launch | ◻︎ **Owed.** We load Geist / Geist Mono / JetBrains Mono from `fonts.googleapis.com` (a render-blocking `<link>` ×4 in `index.html` **and** an `@import` in `globals.css`). That blocks first paint, breaks fonts offline, and leaks the launch to Google — a loud web tell. Fix: self-host (e.g. `@fontsource-variable/geist` + `…/geist-mono`), drop both the links and the `@import`. Mind the family name (`Geist Variable` vs `Geist`) when wiring — verify against the package before flipping `--font-ui` / `--font-mono`. |

### B · Make the WebView render like a native surface

These are the harder, shell-side moves. Several are Raycast's WebKit workarounds;
on Wails some are the Go shell's job and some we simply don't need yet.

| Concern | Raycast technique | Lyra status / where it lives |
|---|---|---|
| Window chrome | Native shell owns titlebar; content goes edge-to-edge | ✅ macOS `TitleBarHiddenInset()` + traffic-lights inset over our content (`main.go`); content padded down by the `.sidebar` titlebar shim (`layout.css`). ◻︎ Windows/WebView2 equivalent not yet configured. |
| Window dragging | Native drag region, not "drag anywhere" | ✅ Dedicated absolutely-positioned drag strips in `SidebarExpanded`/`SidebarRail` — *not* the whole column (would make every row drag the window). Documented in `layout.css`. |
| Transparency / vibrancy | Transparent WebView blended into a vibrant window material; on macOS Tahoe they adopt Liquid Glass | ◻︎ **Gap / opportunity.** We ship an **opaque** window today (`BackgroundColour {18,18,18,A:1}`, no `WebviewIsTransparent`/`WindowIsTranslucent`). A transparent webview over a vibrancy/`NSVisualEffectView` background is the single biggest "this is a real Mac app" upgrade available — but it's a deliberate design call (DESIGN.md is dark-first, surface-ladder depth, *not* glass). **Do not flip this unilaterally — propose to design first.** |
| Launch flicker | `_doAfterNextPresentationUpdate` — don't show the window until the WebView has drawn | ✅ White-flash killed at the source — the first-paint script in `index.html` paints `<html>` the canvas colour (dark `#121212` / light `#fff` by persisted scheme) BEFORE the CSS bundle parses, so a cold boot opens straight into the surface. Matches the native window `BackgroundColour` (`main.go`). |
| Resize jank | Keep the WebView rendering during window resize (implicit Core Animation; frame held at expanded size) | N/A at our layer — Wails owns the resize loop. Watch for content reflow stutter on slow resize; our `overflow:hidden` + grid shell already avoids document reflow. |
| Background throttling | Order window to front but keep it `alphaValue = 0`, disable occlusion detection, so WebKit doesn't throttle a hidden window | N/A unless we add hidden/prewarmed windows. Note for the day we add a quick-launcher-style panel. |
| Emoji / first-glyph stall | Prewarm the emoji font on startup to avoid font-fallback lag | ◻︎ If the emoji picker or first emoji render ever stalls, prewarm the face on startup. Not observed yet — don't pre-optimise. |

---

## Rules for contributors (the short version)

1. **No `cursor: pointer` on app controls.** The hand cursor means "hyperlink".
   Buttons, tabs, rows, toggles → default arrow. Hyperlinks inside rendered
   markdown → hand, fine.
2. **Chrome is not selectable; content is.** Don't undo the `user-select` scope.
   New chrome inherits `none`; if you build something inside `.msg-content` that
   shouldn't be copyable, opt it out locally.
3. **Don't introduce a new document-level scroller.** All scrolling happens
   inside `panel-scroll` / `msg-scroll-frame`. Keep `overflow: hidden` on the
   shell. Add `overscroll-behavior: contain` to new inner scrollers.
4. **Prefer inline panels / dedicated views over page-dimming modals.** A modal
   that grays the whole app is a web pattern.
5. **No flicker on appear/transition.** If a view flashes empty before content,
   that's a bug, not a frame to accept. Gate on data or reserve layout.
6. **Window-shell behaviour (titlebar, transparency, drag regions, frameless on
   Windows) is a `main.go` / Wails-options concern** — coordinate there, don't
   fake native chrome in CSS.
7. **Transparency/vibrancy is a design decision, not a CSS toggle.** See the gap
   row above — propose before flipping.

## New-surface checklist

Before merging a new panel / dialog / control, confirm:

- [ ] No `cursor-pointer` on non-link controls.
- [ ] Chrome regions non-selectable; only real content text is selectable.
- [ ] Any new scroll container sets `overscroll-behavior: contain` and themes
      its scrollbar (or reuses `panel-scroll`).
- [ ] Tooltips/popovers portal correctly and never clip inside a scroller.
- [ ] No empty-frame flash on first mount (data-gated or layout-reserved).
- [ ] Right-click does nothing or shows an app menu — never the browser menu.
- [ ] Images/icons can't be drag-ghosted out (`-webkit-user-drag: none`).

---

## Why this lives here

Lyra deliberately took the WebView path for the same reasons Raycast did —
one UI codebase, fast iteration, rich text/markdown rendering — and pays the
same tax: the UI inherits a browser's instincts. This file is where we record
which instincts we've suppressed (✅), which we still owe (◻︎), and which we've
*chosen* to diverge on (⚠️), so the "feels native" bar stays a tracked
requirement instead of folklore. Pair it with `DESIGN.md` (what it should look
like) and `ARCHITECTURE.md` (how it's wired).
