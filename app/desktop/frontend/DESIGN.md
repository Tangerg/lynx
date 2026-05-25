---
version: 1.0
name: lyra-design-analysis
description: "Lyra is an agent client — a desktop chat shell that streams AG-UI events from a Go runtime. The design is dark-first, dense, keyboard-driven; built for engineers reading streaming markdown, inspecting tool calls, and approving actions. Visually a synthesis of Linear (canvas / surface ladder / hairline-defined regions / single accent) and Vercel (Geist typography / mono-as-eyebrow / stacked subtle elevation / sentence-case headlines). The system replaces the previous Spotify-inspired vocabulary (pill geometry / ALL-CAPS labels / 700+ display weight / heavy dark shadows) which read as a consumer media app and clashed with the engineering posture of an agent runtime."

colors:
  # ---- Brand / accent ----
  # Single chromatic accent — used scarcely. Allowed surfaces: active tab
  # indicator, primary CTA, focus ring, live indicator (streaming dot,
  # running pill). Forbidden as section background, card fill, or
  # decorative tint.
  primary: "#1ed760"            # Lyra signature green — kept from prior identity
  primary-hover: "#3fe57a"
  primary-pressed: "#169c46"
  on-primary: "#000000"

  # ---- Ink (text) ----
  ink: "#f7f8f8"                # Headlines + emphasized body
  ink-soft: "#d0d6e0"            # Body / paragraph
  ink-muted: "#8a8f98"           # Secondary / inactive nav / meta
  ink-faint: "#62666d"           # Tertiary / disabled / footnotes

  # ---- Surface ladder (Linear-derived) ----
  # Four steps above canvas. Depth comes from this ladder + hairlines —
  # never from drop shadows on dark.
  canvas: "#010102"              # Page background. NOT #000000 — faint blue tint
  surface-1: "#0f1011"           # Cards, sidebar, message bubble
  surface-2: "#141516"           # Hover / active row, raised surface
  surface-3: "#18191a"           # Sub-nav, dropdown, popover
  surface-4: "#191a1b"           # Deepest lifted surface

  # ---- Hairlines ----
  hairline: "#23252a"            # Default 1px border
  hairline-strong: "#34343a"     # Input focus, emphasized divider
  hairline-tertiary: "#3e3e44"   # Nested surface borders

  # ---- Semantic ----
  # Used ONLY for genuine errors / warnings / live confirmations. Not
  # decoration. RUN_ERROR banner / approval-card warnings / status dots.
  success: "#27a644"             # Confirmed action, run finished cleanly
  warning: "#f5a623"             # User attention required (approval pending)
  negative: "#ee0000"            # Errors (RUN_ERROR banner, tool failure)
  info: "#0070f3"                # Inline links, info badges

  # ---- Light theme (Vercel-derived) ----
  light-canvas: "#ffffff"
  light-surface-1: "#fafafa"
  light-surface-2: "#f5f5f5"
  light-surface-3: "#ececed"
  light-hairline: "#ebebeb"
  light-hairline-strong: "#a1a1a1"
  light-ink: "#171717"
  light-ink-soft: "#4d4d4d"
  light-ink-muted: "#888888"

typography:
  # ---- Font families ----
  # Sans: Geist — sharp geometric, the agent-tool consensus (Linear / Vercel
  # / Cursor all use it or Inter as fallback). Webfont loaded from Vercel CDN.
  # Mono: Geist Mono — every numeric / ID / timestamp / eyebrow / code snippet.
  # CJK fallback chain preserves Hiragino / Meiryo so mixed-script renders.
  #
  # Webfonts loaded in tokens.css via @import.

  # ---- Display ----
  # 600 is the display ceiling. Both Linear and Vercel forbid 700+.
  # Negative tracking on display, near-zero on body.
  display-xl:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 32px
    fontWeight: 600
    lineHeight: 1.10
    letterSpacing: -0.96px
  display-lg:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 24px
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: -0.6px
  display-md:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 20px
    fontWeight: 600
    lineHeight: 1.20
    letterSpacing: -0.4px
  display-sm:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 16px
    fontWeight: 600
    lineHeight: 1.25
    letterSpacing: -0.2px

  # ---- Body ----
  body-lg:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 15px
    fontWeight: 400
    lineHeight: 1.65
    letterSpacing: -0.1px
  body-md:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 14px
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: -0.05px
  body-sm:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 13px
    fontWeight: 400
    lineHeight: 1.50
    letterSpacing: 0
  body-xs:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.45
    letterSpacing: 0

  # ---- Button label ----
  button-md:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 13px
    fontWeight: 500
    lineHeight: 1.20
    letterSpacing: 0
  button-sm:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 12px
    fontWeight: 500
    lineHeight: 1.20
    letterSpacing: 0

  # ---- Caption / mono eyebrow ----
  # Replaces every ALL-CAPS + letter-spacing label from the previous system.
  # Mono signals "technical / observable / data" — used for reasoning headers,
  # tool-call signatures, file paths, durations, IDs, status-bar items.
  caption:
    fontFamily: Geist, Inter, system-ui, -apple-system, sans-serif
    fontSize: 12px
    fontWeight: 400
    lineHeight: 1.40
    letterSpacing: 0
  caption-mono:
    fontFamily: Geist Mono, JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, monospace
    fontSize: 11.5px
    fontWeight: 400
    lineHeight: 1.40
    letterSpacing: 0
    fontFeatureSettings: "\"tnum\""
  code:
    fontFamily: Geist Mono, JetBrains Mono, ui-monospace, SFMono-Regular, Menlo, monospace
    fontSize: 12.5px
    fontWeight: 400
    lineHeight: 1.55
    letterSpacing: 0
    fontFeatureSettings: "\"tnum\""

rounded:
  none: 0px
  xs: 4px        # Badges, status pills
  sm: 6px        # Inputs, small buttons, nav buttons (Vercel --geist-radius)
  md: 8px        # Default button, card chrome, dialog (Linear --geist-marketing-radius)
  lg: 12px       # Workspace cards, pricing-style summaries
  xl: 16px       # Hero / lightbox frame
  pill: 9999px   # Status badge / segmented toggle ONLY — NEVER for CTAs
  circle: 50%    # Avatar, dot indicator

spacing:
  # 4-base — every value a multiple of 4. Both Linear & Vercel agree.
  px: 1px
  xxs: 4px
  xs: 8px
  sm: 12px
  md: 16px
  lg: 24px
  xl: 32px
  2xl: 40px
  3xl: 48px
  4xl: 64px
  5xl: 96px

# ---- Lyra-specific layout constants ----
layout:
  chat-measure: 760px      # Max reading width for chat content — cap to keep lines ≤80 chars
  sidebar-rail: 56px       # Collapsed sidebar (default state)
  sidebar-expanded: 260px  # Expanded sidebar (on demand)
  topbar-height: 36px      # Chat tab strip — drag region on macOS
  statusbar-height: 24px   # Bottom status bar — dense data row
  app-divider: 1px         # Gap between flush panels — Linear hairline color shows through

motion:
  ease-out: cubic-bezier(0.3, 0, 0, 1)
  ease-emphasized: cubic-bezier(0.16, 1, 0.3, 1)
  ease-in-out: cubic-bezier(0.45, 0, 0.55, 1)
  dur-instant: 80ms
  dur-fast: 140ms
  dur-med: 220ms
  dur-slow: 360ms

components:
  # ---- Buttons ----
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.on-primary}"
    typography: "{typography.button-md}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
    description: Primary CTA. Lyra signature green. Reserved for explicit action ("Send", "Approve", "Run").
  button-secondary:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    borderColor: "{colors.hairline}"
    typography: "{typography.button-md}"
    rounded: "{rounded.md}"
    padding: "8px 14px"
    description: Charcoal button on hairline border. Most "Cancel" / "Dismiss" / inline actions.
  button-tertiary:
    backgroundColor: transparent
    textColor: "{colors.ink-muted}"
    typography: "{typography.button-sm}"
    rounded: "{rounded.sm}"
    padding: "6px 8px"
    description: Plain text button. Sidebar toggles, inline minor actions.
  icon-button:
    backgroundColor: transparent
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.sm}"
    minSize: 28px
    description: Square icon container. 28×28 standard, 32×32 emphasized. Hover surface-2 fill.

  # ---- Composer surface ----
  composer:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.lg}"
    padding: "12px 14px"
    maxWidth: "{layout.chat-measure}"
    description: Textarea + toolbar surface. Anchored bottom, centered to chat-measure. Focus ring uses primary at 14% alpha + hairline-strong.
  composer-chip:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.caption-mono}"
    rounded: "{rounded.xs}"
    padding: "2px 8px"
    description: Attachment / file ref pill. Mono caption — these are file paths and IDs.
  segmented-control:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.sm}"
    description: Composer mode picker (Agent / Ask / Plan). Active segment lifts to surface-3 + ink.

  # ---- Message stream ----
  message-bubble-user:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    textColor: "{colors.ink}"
    typography: "{typography.body-md}"
    rounded: "14px 14px 4px 14px"
    padding: "10px 14px"
    maxWidth: "580px"
    description: Right-aligned bubble. Compact, hairline-bordered.
  message-body-assistant:
    backgroundColor: transparent
    textColor: "{colors.ink-soft}"
    typography: "{typography.body-md}"
    maxWidth: "{layout.chat-measure}"
    description: Full-width prose. No bubble chrome. Avatar peeks left.
  reasoning-block:
    backgroundColor: transparent
    borderLeft: "2px solid {colors.hairline-strong}"
    paddingLeft: "12px"
    headerTypography: "{typography.caption-mono}"
    headerColor: "{colors.ink-muted}"
    bodyTypography: "{typography.body-sm}"
    bodyColor: "{colors.ink-muted}"
    bodyFontStyle: italic
    description: Collapsible thinking panel. Header shows "thinking · 12s" or "thought for 12s" in mono lowercase (NEVER "THINKING" all-caps).
  tool-call-card:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.md}"
    signatureTypography: "{typography.code}"
    metaTypography: "{typography.caption-mono}"
    description: |
      Renders like an RPC log entry, not a generic card. First line: function
      signature in mono (e.g. `read_file(path: "src/auth.ts")`). Second line:
      status glyph + duration + bytes/lines summary in caption-mono. Expandable
      for full result.

  # ---- Code & Mermaid ----
  shiki-code-block:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.md}"
    headTypography: "{typography.caption-mono}"
    bodyTypography: "{typography.code}"
    description: Shiki-highlighted code with mono header (lang lowercase, optional filename, copy button on hover). Long blocks auto-collapse > 24 lines.
  mermaid-block:
    backgroundColor: "{colors.surface-1}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.md}"
    description: Clickable diagram — click opens lightbox at native scale. Diagram colors derived from theme tokens at render time.

  # ---- Navigation ----
  chat-tab:
    backgroundColor: transparent
    activeBackgroundColor: "{colors.surface-1}"
    activeBorderColor: "{colors.hairline}"
    activeIndicatorColor: "{colors.primary}"
    typography: "{typography.body-sm}"
    rounded: "7px 7px 0 0"
    description: Topbar tab. Active = surface lift + 2px primary underline (the ONLY accent in the tab strip).
  view-tab:
    backgroundColor: "color-mix(in srgb, {colors.primary} 6%, transparent)"
    description: Same shape as chat-tab, faintly accent-washed to distinguish "this is a panel view, not a session".
  command-palette:
    backgroundColor: "{colors.surface-2}"
    borderColor: "{colors.hairline-strong}"
    rounded: "{rounded.lg}"
    backdropFilter: blur(10px)
    itemTypography: "{typography.body-sm}"
    description: ⌘K overlay. Surface-2 + hairline-strong + backdrop blur + Level 5 stacked shadow (the rare floating element where shadow is allowed).

  # ---- Status bar ----
  statusbar:
    backgroundColor: "{colors.canvas}"
    height: "{layout.statusbar-height}"
    typography: "{typography.caption-mono}"
    color: "{colors.ink-muted}"
    description: |
      Dense data row at app bottom. Slots, separated by hairline pipes:
      [● status] [branch] [run_8f3a2c] [tokens 12,847 / 200,000 (6.4%)]
      [$0.0234] [↑ 18.2 t/s]. Each value mono with tnum. The accent dot
      pulses while running.

  # ---- Overlays ----
  toast:
    backgroundColor: "{colors.surface-2}"
    borderColor: "{colors.hairline}"
    rounded: "{rounded.md}"
    typography: "{typography.body-sm}"
    description: Plugin toaster entry. Bottom-right stack. Auto-dismiss 4s.
  approval-card:
    backgroundColor: "color-mix(in srgb, {colors.warning} 8%, {colors.surface-1})"
    borderColor: "color-mix(in srgb, {colors.warning} 30%, transparent)"
    rounded: "{rounded.md}"
    titleTypography: "{typography.display-sm}"
    metaTypography: "{typography.caption-mono}"
    description: HITL approval prompt. Warning-tinted card with mono command preview, "Approve" primary + "Decline" secondary.
  run-error-banner:
    backgroundColor: "color-mix(in srgb, {colors.negative} 12%, transparent)"
    borderColor: "color-mix(in srgb, {colors.negative} 35%, transparent)"
    rounded: "{rounded.md}"
    titleTypography: "{typography.caption-mono}"
    titleColor: "{colors.negative}"
    bodyTypography: "{typography.body-sm}"
    description: AG-UI RUN_ERROR surface. Lives above MessageStream, dismissible. Cleared automatically on next RUN_STARTED.
---

## 1. Overview

Lyra is an agent client — a desktop application (Wails / React) that streams AG-UI protocol events from a Go runtime and renders them as a chat surface with inline tool calls, code, diagrams, and approval flows. The frontend is a **view onto a runtime**, not the runtime itself; the visual language reflects that posture: dense, observable, keyboard-driven.

The system is dark-first by deliberate choice — agent users spend long sessions in front of the surface, and dark reduces eye fatigue. Light mode is supported with full parity (no second-class treatment).

**Synthesis sources** — this system borrows specific elements from two reference systems analyzed at `frontend/linear.md` and `frontend/vercel.md`:

- **From Linear**: the color system (canvas `#010102`, four-step surface ladder, three-step hairline ladder, scarce single accent), the "depth via surface + hairlines, never shadow on dark" rule, and the strict accent-usage policy.
- **From Vercel**: the typography system (Geist + Geist Mono), the mono-as-eyebrow convention (replaces ALL-CAPS labels), sentence-case headlines, stacked-subtle elevation for the rare floating overlay, and the light-theme palette.

**Explicitly rejected** from the previous "Sonance" / Spotify-derived system:
- Pill-radius CTAs (500-9999px)
- ALL-CAPS labels with positive letter-spacing
- 700+ display weight
- Heavy drop shadows on dark surfaces
- Accent color used decoratively across 100+ surfaces

## 2. Color

### Philosophy

Color carries information, not decoration. The system uses **one chromatic accent**, **four greys for surfaces**, **three greys for hairlines**, and **four semantic colors used sparingly**. Decoration comes from the surface ladder, not from color variation.

### Surface ladder

| Token | Hex | Use |
|---|---|---|
| `canvas` | `#010102` | Page background. The faint blue tint is deliberate — pure `#000000` reads as void; this reads as "deep dark". |
| `surface-1` | `#0f1011` | Default lifted surface — cards, sidebar, message bubble, tool-call card. |
| `surface-2` | `#141516` | Hovered / active row, raised surface, command palette, **inset reading pane** (e.g. settings content area inside a workspace view — distinct from the outer chrome). |
| `surface-3` | `#18191a` | Sub-nav, dropdown, popover. |
| `surface-4` | `#191a1b` | Deepest lifted surface (rare). |

**Inset-pane rule** — when a workspace view has its own internal nav rail + content split (e.g. the settings page), keep the rail on `surface` (same as the outer sidebar — consistent chrome) and lift the content area to `surface-2`. This reverses the naïve "nav darker / content lighter" guess from web-app conventions; the chrome stays uniform, and the reading area gets the distinct surface so it reads as the focus zone.

### Hairlines

| Token | Hex | Use |
|---|---|---|
| `hairline` | `#23252a` | Default 1px border on cards, dividers, table rows. |
| `hairline-strong` | `#34343a` | Input focus border, emphasized divider. |
| `hairline-tertiary` | `#3e3e44` | Borders on nested surfaces. |

**Hairlines must use literal hex values, not `color-mix(text X%, transparent)`** — semi-transparent borders shift visually across different surface lifts and read as "approximate". Literal hex = precision = perceived craft.

### Accent policy

The single accent (`primary: #1ed760`) is reserved for **exactly four surfaces**:

1. Active tab indicator (2px underline on `chat-tab.active`)
2. Primary CTA fill (`button-primary`, Send button)
3. Focus ring (`:focus-visible` outline)
4. Live indicator (streaming dot, running pill, `tab-dot.running`)

Forbidden surfaces for accent: section background, card fill, avatar background, decorative borders, status icons that are not "live".

### Semantic palette

| Token | Hex | Use |
|---|---|---|
| `success` | `#27a644` | Run finished cleanly, action confirmed. Allowed in: run pill (idle/done), `tab-dot.idle` after success. |
| `warning` | `#f5a623` | User attention required. Allowed in: `approval-card`, `tab-dot.waiting`. |
| `negative` | `#ee0000` | Error. Allowed in: `run-error-banner`, tool-call `status: err`. |
| `info` | `#0070f3` | Information / link. Allowed in: inline links, info badges. |

## 3. Typography

### Font families

Two real webfonts, two fallback chains:

- **`Geist`** (sans) — primary UI face. Sharp geometric sans, the agent-tool consensus (Linear, Vercel, Cursor, Raycast all use it or Inter as fallback). Loaded from Vercel CDN. Weights 400 / 500 / 600.
- **`Geist Mono`** (mono) — every numeric / ID / timestamp / eyebrow / code snippet / file path. Weight 400 only.
- **Fallback sans**: Inter, system-ui, -apple-system, then CJK chain.
- **Fallback mono**: JetBrains Mono, ui-monospace, SFMono-Regular, Menlo.

The previous `--font-title` / `--font-ui` split is **removed** — display and body share one face, weight does the hierarchy work.

### Scale

The full scale is 11 tokens — narrower than the previous 13-step Spotify scale. Display sizes are smaller than typical marketing systems because Lyra is a product UI, not a hero page.

(See frontmatter `typography:` for canonical sizes / weights / tracking.)

### Principles

1. **Mono is the technical voice.** All eyebrows, IDs, durations, timestamps, file paths, tool signatures, status-bar items render in `caption-mono` or `code`. This replaces every ALL-CAPS + letter-spacing label from the prior system.

2. **600 is the display ceiling.** Never use 700, 800, 900. Hierarchy comes from size + weight contrast (400 vs 600), not from going heavier.

3. **Display gets negative tracking.** -0.96px at 32px display-xl, scaling proportionally. Body holds at -0.05 to 0.

4. **Sentence-case headlines.** Never ALL-CAPS. Welcome screen, settings sections, view headers — all sentence-case. Optional period termination is allowed (Vercel signature) but not required.

5. **Tabular numerals everywhere numeric.** `font-feature-settings: "tnum"` on caption-mono and code by default. Numbers don't jitter when counters update.

6. **CJK safety.** Letter-spacing > 0.02em should be scoped to `:lang(en)` — CJK characters are pulled visually apart by positive tracking.

## 4. Layout

### App shell

```
┌──────┬─────────────────────────────────────────────────────────┐
│      │ Topbar — chat-tab strip + slot for plugin actions       │ 36px
│ Rail │─────────────────────────────────────────────────────────│
│  56  │                                                          │
│   /  │             Message stream (max-width 760)              │ 1fr
│ Side │                                                          │
│ 260  │  ┌───────────────────────────────────────┐              │
│      │  │  Composer (max-width 760)             │              │
│      │  └───────────────────────────────────────┘              │
├──────┴─────────────────────────────────────────────────────────┤
│ Statusbar — dense data row, mono                                │ 24px
└─────────────────────────────────────────────────────────────────┘
```

### Sidebar

- **Default state: rail** (56px). The expanded mode (260px) is on-demand via `⌘B` or rail hover.
- This reverses the prior default (expanded) — agent users live in keyboard mode; the sidebar earns its width only when the user reaches for it.

### Chat measure

- Message stream + composer both cap at **`chat-measure: 760px`**, centered.
- Reading ergonomics: 45–75 characters per line at body-md (14px) translates to ≈ 720–820px. 760 is the comfortable middle.
- Long code blocks and tables can exceed 760 — they get horizontal scroll inside their own wrapper, the prose column stays at 760.

### Tabs

- **Chat session tabs are deprecated from the topbar.** Sessions live in the sidebar as a vertical list (already implemented). The topbar carries only **view tabs** (workspace views the user "opened" into the main area: Files, Diff, Plan, Terminal).
- When only one chat session is active and no view tabs are open, the topbar is empty (just the drag region).

### Spacing rhythm

Lyra is a **product UI**, not a marketing site. Spacing values from the frontmatter `spacing:` block apply, but:

- Section breaks inside a panel: `md` 16px to `lg` 24px (never `5xl` 96px — too marketing).
- Card interior padding: `md` 16px default, `lg` 24px for emphasized cards.
- Inline gaps: `xs` 8px to `sm` 12px.
- Marketing-band spacing (`5xl` / `section` 192px from Vercel) is **not used** in Lyra except in the welcome screen.

## 5. Elevation & Depth

### Dark mode

**Depth on dark is carried by the surface ladder + hairlines.** Drop shadows on dark backgrounds are nearly invisible and waste GPU on opacity blur. The rule:

| Level | Treatment | Use |
|---|---|---|
| 0 | No border, no shadow | Body text, message body |
| 1 | `surface-1` background + 1px `hairline` border | Default card, tool-call card, message bubble |
| 2 | `surface-2` background + 1px `hairline-strong` | Active row, hovered card, command palette base |
| 3 | `surface-3` background | Sub-nav, dropdown |
| 4 | Stacked subtle shadow (Vercel-style) + inset hairline | The few truly-floating overlays: command palette, mermaid lightbox, plugin toaster |

**Level 4 is the ONLY level on dark where shadow is allowed**, and it's a stacked-subtle (Vercel) shadow, not a single 24px-blur drop. Used for overlays that float above the entire app.

### Light mode

Light uses the full Vercel-style stacked-shadow ladder (5 levels). On light, shadows are visible and necessary.

## 6. Shapes

### Radius scale

| Token | Value | Use |
|---|---|---|
| `none` | 0px | Full-bleed bars (topbar, statusbar) |
| `xs` | 4px | Badges, status pills, file chips |
| `sm` | 6px | Inputs, small buttons, icon button square |
| `md` | 8px | **Default button**, card chrome, dialog corners |
| `lg` | 12px | Workspace cards, command palette frame |
| `xl` | 16px | Lightbox frame, hero panels |
| `pill` | 9999px | Status badges, segmented toggle ONLY |
| `circle` | 50% | Avatars, dot indicators |

### NEVER

- **No pill-radius CTAs.** The previous `--radius-pill: 500px` and `--radius-full: 9999px` are removed from button surfaces. Buttons are `md` 8px square.
- **No mixed scales on one screen.** Vercel allows pill at marketing scale + 6px at nav; Lyra picks one scale (8px) and stays there.

## 7. Motion

(See frontmatter `motion:` for tokens.)

- **Hover / press feedback**: `dur-fast` 140ms with `ease-out`. Every interactive element has a visible state change.
- **Layout enter/exit** (modal, toast, palette): `dur-med` 220ms with `ease-emphasized`.
- **Heavy transitions** (panel slide, accordion expand): `dur-slow` 360ms with `ease-emphasized`.
- **Active press scale**: every clickable surface gets `:active { transform: scale(0.92-0.96) }` per element size — gives tactile confirmation.
- **`prefers-reduced-motion`**: all transitions degrade to ≤80ms, all scale animations disabled.

## 8. Components

The frontmatter `components:` block carries the canonical spec for every Lyra-specific component. Highlights:

### Tool-call card — the "RPC log" rule

Tool calls render **like an RPC log entry, not a generic card**:

```
read_file(path: "src/auth.ts")            ✓ 12ms · 1,247 lines
```

- Line 1: function signature in `code` typography (mono 12.5px).
- Line 2: status glyph (`✓` / `✗` / pulsing dot) + duration + summary, all `caption-mono`.
- Expandable for full result body.
- Card chrome: `surface-1` + `hairline` + `md` radius.

This single change carries more "agent-tool" feel than any other component decision.

### Status bar — the data row

Bottom 24px row, mono caption throughout. Plugins contribute slots; default set:

```
[● running] [main] [run_8f3a2c] [12,847 / 200k · 6.4%] [$0.0234] [↑ 18.2 t/s]
```

Each value is mono with tnum. Separators are hairline pipes (` │ `). The leading `●` is the agent state dot, pulsing accent while running.

### Reasoning block — mono header, no caps

Header was `THOUGHT FOR 1S` ALL-CAPS — now `thought · 1.2s` in `caption-mono` lowercase. Body italic stays.

### Composer cheatsheet — auto-derived

The composer's hover-revealed cheatsheet **derives rows from `useCommands()`**. Any command with a `shortcut: string` field auto-appears. Static rows reserved for composer-local keys (`Enter`, `⇧↵`, `⌘↵`, `Esc`, `⌘1-9`).

## 9. Accent Usage Policy (strict)

Accent (`#1ed760`) appears in:

1. **Active tab indicator** — 2px underline on `chat-tab.active::after`
2. **Primary CTA fill** — `button-primary` background, send button
3. **Focus ring** — `:focus-visible { outline: 2px solid color-mix(primary 55%, transparent) }`
4. **Live indicator** — streaming `tab-dot.running`, status pill while `run.running === true`, the reasoning block's pulse dot

That's the entire list. Accent does **not** appear in:
- Avatar backgrounds (use `surface-3` + `ink-muted`)
- Section headers (use `ink`)
- Active-state list rows (use `surface-2` + `ink`)
- Iconography (icons are `ink-muted` → `ink` on hover)
- Tool-call success status (use `success` `#27a644`, not primary)

When in doubt: **does this surface convey "the agent is alive and live"?** If yes, accent. If no, grey.

## 10. Do's and Don'ts

### Do

- Render every eyebrow / label / ID / duration / file path in `caption-mono`.
- Cap chat content (message stream + composer) at `chat-measure: 760px`, centered.
- Use literal hex hairlines (`#23252a` / `#34343a` / `#3e3e44`) — not `color-mix(text X%, transparent)`.
- Set every interactive element with `:hover`, `:active`, `:focus-visible`.
- Use `font-feature-settings: "tnum"` on every numeric display.
- Default sidebar to rail (56px). Expand on demand.
- Render tool calls as RPC logs (mono signature + duration line).
- Pair display weight 600 with body weight 400. Hierarchy via size + weight contrast, never weight 700+.

### Don't

- **Don't use ALL-CAPS labels with letter-spacing.** Replace with `caption-mono` lowercase.
- **Don't use pill-radius CTAs** (`9999px`, `500px`, `100px` on a button). Buttons are `md` 8px.
- **Don't use weight 700+ for display.** 600 is the ceiling, Linear and Vercel both forbid this.
- **Don't drop a single heavy `box-shadow` on dark surfaces.** Use surface ladder + hairlines. Stacked-subtle shadow allowed only for Level 4 overlays.
- **Don't use `#000000` true black as canvas.** `#010102` with the faint blue tint is the right anchor.
- **Don't introduce a second chromatic accent.** Lyra has one accent + four semantic colors. No more.
- **Don't use accent decoratively.** Active tab / primary CTA / focus ring / live indicator — that's the entire allowed list.
- **Don't set body paragraphs in mono.** Mono is for the technical layer only.
- **Don't apply atmospheric gradients, mesh backdrops, or dot grids** (the latter was discussed and rejected — Linear explicitly forbids "atmospheric gradients or spotlight cards").
- **Don't add backdrop-filter / vibrancy / Mica effects.** Wails WebView is inconsistent across platforms; visual carries the load.

## 11. Light theme

Light mode targets full parity with dark. Surfaces invert: canvas `#ffffff`, surface-1 `#fafafa`, surface-2 `#f5f5f5`, surface-3 `#ececed`. Hairlines invert to `#ebebeb` / `#a1a1a1`. Ink inverts to `#171717` / `#4d4d4d` / `#888888`.

Light mode **does** use stacked shadows (Vercel-style level 1-5). This is the only place the dark/light symmetry breaks.

Accent stays the same green in light mode (no need to dim — `#1ed760` reads well on white).

## 12. References

- `frontend/linear.md` — source for canvas / surface ladder / hairline ladder / accent policy.
- `frontend/vercel.md` — source for typography (Geist), mono-as-eyebrow, stacked elevation, light palette.
- AG-UI protocol — `frontend/src/protocol/agui/` — drives the shape of the data this UI renders.

## 13. Iteration guide

1. When adding a new surface, reference its component spec in the frontmatter `components:` block. If none exists, propose one (commit + this doc together).
2. Default to the dark theme. Verify light parity before merging visual changes.
3. Run `npx tsc --noEmit && npx vitest run` after any token change.
4. Visually verify in `wails dev` — type/spacing changes especially.
5. Treat the accent as scarce: ask "is this live?" — if no, use grey.
