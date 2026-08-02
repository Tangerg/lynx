# HeroUI Design System

A design system built from **HeroUI v3** — the open-source React UI library previously known as
NextUI. This project turns the library's real token set, component inventory and documentation
surface into something a design agent can build against.

> **Sources used.** Everything here was derived from public HeroUI material. Nothing was invented
> where a source existed; where a source could **not** be read, it is flagged inline rather than
> filled in.
>
> - GitHub: <https://github.com/heroui-inc/heroui> — **default branch `v3`** (not `main`).
>   Explore it for anything this system doesn't cover; `packages/styles/` is the ground truth for
>   every value below.
> - Docs: <https://heroui.com> · v2 docs: <https://v2.heroui.com> · Storybook: <https://storybook-v3.heroui.com>
> - Machine-readable index: <https://heroui.com/llms.txt>
> - Figma kit (v3): <https://www.figma.com/community/file/1546526812159103429/heroui-figma-kit-v3>
> - React Native sibling: <https://github.com/heroui-inc/heroui-native>
>
> Files read verbatim: `packages/styles/themes/default/variables.css`,
> `packages/styles/themes/shared/theme.css`, `packages/styles/components/button.css`,
> plus the Typography, Theming, Styling and Migration doc pages. See `research-notes.md` for the
> raw transcription and the list of files **not** read.

---

## 1. What HeroUI is

HeroUI is a React component library — 75+ web components plus a 37-component React Native sibling —
rebuilt from scratch for v3 on **Tailwind CSS v4** and **React Aria Components**. It is a developer
product: the "brand" is the library's own visual language, and the only surface it ships is its
documentation site.

Four decisions define v3 and shape everything in this design system:

1. **Composition over configuration.** v2 components were black boxes; v3 exposes every internal
   piece as a real element — `Card.Header`, `Select.Item`, `Tabs.ListContainer`. More code, but
   nothing is hidden.
2. **Styles decoupled from behaviour.** `@heroui/styles` is standalone CSS you can use with any
   framework; `@heroui/react` handles behaviour. Drop the stylesheet and you have a headless library.
3. **BEM classes + CSS custom properties, no CSS-in-JS.** `.button`, `.button--primary`,
   `.accordion__trigger`. Every variant is reachable from your own stylesheet.
4. **Native CSS motion.** v2 used Framer Motion for every animation; v3 replaced it with CSS
   transitions and keyframes. No JS animation runtime.

There is **no `<Provider>` wrapper** in v3, and no Tailwind plugin — you `@import "@heroui/styles"`
and you're done.

### Products represented here
| Surface | Status |
| --- | --- |
| **Documentation site** (`heroui.com/docs`) | Recreated as a UI kit — see `ui_kits/docs/`. |
| **Component library** | Recreated as 67 components — see below. |
| HeroUI Pro (paid templates: Command Palette, Kanban, DataGrid, Dashboard) | **Not built.** No accessible source; inventing one would be fiction. |
| heroui-native (React Native) | **Not built.** Out of scope for a web design system. |

---

## 2. Content fundamentals

HeroUI writes like good API documentation: **flat, declarative, technically precise, and almost
never enthusiastic.** The register is a senior engineer explaining a decision, not a marketer
selling one.

**Voice and person.** Copy speaks as **"we"** for the library's decisions ("We maintain
functionality and accessibility") and **"you"** for the reader's ("You focus on your product").
First person singular never appears. Component descriptions drop the subject entirely and lead with
the noun: *"A semantic typography primitive for headings, body copy, and inline code built on
React Aria Components Text."*

**Casing.** Sentence case everywhere — headings, buttons, labels, nav items. Title Case appears
only in component names (`ButtonGroup`, `DateRangePicker`) and section headers in release notes.
Never ALL CAPS except for tiny 11px sidebar group labels.

**Sentence shape.** Short declaratives, often fragments. Parallel two- and three-part rhythms are
the house tic:
- *"Lighter bundles, GPU-accelerated, no JS animation runtime."*
- *"Beautiful by default, customizable by design."*
- *"You control the structure."*

**Honesty about tradeoffs.** The docs concede costs rather than hiding them — *"Yes, v3 requires
more code. But that code is explicit about what it renders."* Carry this over: when a design has a
downside, say so in the copy.

**Component descriptions** are one sentence, present tense, starting with what it *is*, then what
it's *built on*: *"A focusable list of tags with support for keyboard navigation, selection, and
removal."*

**Migration and release copy** is a before/after table or a diff, never prose. `v2: … v3: …`.

**Emoji.** The GitHub README opens with a single 🚀 in the repo tagline. That is the only emoji in
the entire surface — **do not use emoji anywhere in product UI, docs, or generated designs.**

**Unicode.** Used functionally, not decoratively: ⌘ ⇧ ⌥ ⌃ in keycaps, · as a separator in metadata
rows, → in "next page" links, — as the em-dash of choice.

**Numbers.** Always concrete and unrounded — "75+ components", "27.7k", "v3.2.2", "124,900 of
250,000 requests". Never "thousands of" when a figure exists.

**Words to avoid:** "seamless", "delightful", "revolutionary", "unlock", "supercharge",
"game-changing". None appear in the source. **Words that do appear:** *composable*, *explicit*,
*accessible*, *headless*, *token*, *slot*, *variant*, *primitive*.

---

## 3. Visual foundations

### Colour
The system is **OKLCH-native** and almost monochrome. A single saturated accent —
`oklch(0.6204 0.195 253.83)`, a clean cobalt blue — sits on a field of near-neutral greys with a
faint violet cast (`285–286°` hue on every neutral). Pure black and pure white exist as
`--black`/`--white` but are almost never used for text: the real ink is **eclipse**
`oklch(0.2103 0.0059 285.89)` and **snow** `oklch(0.9911 0 0)`.

Naming is strict and worth learning: **a bare name is a background, `-foreground` is the text on
it.** `--accent` / `--accent-foreground`, `--danger` / `--danger-foreground`.

Tokens come in **three tiers**:
1. **Base** — never change between themes (`--white`, `--snow`, `--eclipse`, `--spacing`, `--radius`).
2. **Theme** — swap with light/dark (`--background`, `--surface`, `--accent`, `--danger`).
3. **Calculated** — derived with `color-mix()` and never hand-authored. Every hover, soft pair,
   border level and separator level is computed from the base you declared:
   `--accent-hover: color-mix(in oklab, var(--accent) 90%, var(--accent-foreground) 10%)`.

This is why theming HeroUI means setting **four or five values**, not fifty. The docs site's theme
switcher (default, sky, lavender, mint, netflix, black, spotify, coinbase, airbnb, discord, rabbit)
is the proof.

Two more colour systems worth knowing:
- **`--field-*`** styles inputs, checkboxes, radios and OTP slots *independently of surfaces*, so
  you can recolour form controls without touching buttons and cards.
- **`data-vibrant-palette="true"`** swaps soft foregrounds from 70% to 92% colour — a lower-contrast,
  more saturated mode you opt into on the root element.

**Imagery colour vibe:** there is essentially none. HeroUI's surface is UI, code and type — no
photography, no illustration, no stock imagery in the docs. Where an image appears it's a
screenshot of the product itself.

### Typography
The library ships **no `@font-face` and no font tokens** — it inherits Tailwind v4's system stack.
The docs surface reads as **Inter**, which is what this system loads (see *Caveats*).

| Role | Size | Weight | Line height | Tracking |
| --- | --- | --- | --- | --- |
| h1 | 36px | 600 | 1.11 | -0.025em |
| h2 | 30px | 600 | 1.17 | -0.025em |
| h3 | 24px | 600 | 1.25 | -0.025em |
| h4 | 20px | 600 | 1.33 | -0.025em |
| h5 | 18px | 600 | 1.39 | -0.025em |
| h6 | 16px | 600 | 1.50 | -0.025em |
| body | 16px | 400 | 1.75 | — |
| body-sm | 14px | 400 | 1.50 | — |
| body-xs | 12px | 400 | 1.25 | — |
| code | 14px | 400 | — | mono |

**Headings are all 600 — never 700, never light.** `body-sm` (14px) is the working default for
interface chrome; 16px `body` is reserved for long-form prose. Only two text colours exist:
`--foreground` and `--muted`. There is no third grey.

### Spacing
Everything derives from `--spacing: 0.25rem`. Tailwind computes each step as
`calc(var(--spacing) * n)`, so changing one value rescales the whole system. Control heights are
the tell: **40px default, dropping to 36px at ≥768px** (the reverse of the usual mobile-first
instinct — HeroUI sizes *up* for touch and *down* for pointers). sm is 36→32, lg is 44→40.

### Borders
`--border-width: 0px`. **HeroUI has no borders by default.** Cards, inputs and popovers are
separated by fill contrast and shadow, not strokes. When a stroke is needed there are three graded
levels (`--border`, `--border-secondary`, `--border-tertiary`) and three separator levels, each
mixed toward the surface foreground rather than being a fixed grey.

### Corner radii
One base — `--radius: 0.5rem` — with eight derived steps: 2 / 4 / 6 / 8 / 12 / 16 / 24 / 32px.
The characteristic HeroUI shapes:
- **Buttons: `rounded-3xl` = 24px** on a 40px control — effectively a pill. This is the single most
  recognisable thing about the library.
- **Fields: 12px** (`--field-radius` = radius × 1.5).
- **Cards / surfaces: 16px.** Modals and drawers: **24px**.
- Chips, tabs, avatars, pagination items: fully rounded.

Since v3.0.4 every hardcoded radius is wrapped in `min()` so a large custom `--radius` can never
exceed the element's own dimensions.

### Shadows
**Three, and only three.** No `shadow-sm/md/lg/xl` ladder.
- `--surface-shadow` — cards, accordions, tabs. Three barely-there layers totalling ~4px blur.
- `--overlay-shadow` — modals, popovers, menus. Deeper, with a subtle upward layer.
- `--field-shadow` — inputs. Identical to surface.

In **dark mode the outer shadows vanish entirely** and are replaced by a 1px white inset hairline
(`0 0 1px 0 rgba(255,255,255,0.3) inset` on overlays). Depth is light-mode-only.

### Backgrounds
Flat colour. **No gradients, no textures, no patterns, no full-bleed imagery.** The page is
`--background` (a very slightly off-white `oklch(0.9702 0 0)`), and content sits on `--surface`
(pure white) — the inversion of the usual grey-card-on-white pattern. The only gradients in the
entire system are functional: the ScrollShadow mask and the ColorArea/ColorSlider ramps.

### Transparency and blur
Sparing and purposeful. `--backdrop` is `rgba(0,0,0,0.5)` (0.6 in dark) with a light blur behind
modals. The docs top bar uses `color-mix(... 82%, transparent)` plus `backdrop-filter: blur(12px)`
so content scrolls under it. Soft colour pairs are 15% tints (12% in dark). Nothing else is
translucent.

### Motion
All native CSS. The vocabulary is small and consistent:
- **Colour changes: 100ms `--ease-out`.** Background and box-shadow only.
- **Transforms: 250ms `--ease-smooth`** (plain `ease`).
- **Overlays: `--ease-out-fluid`** `cubic-bezier(0.32, 0.72, 0, 1)` — the "fast start, smooth stop"
  Apple curve. Modals rise 8px and scale from 0.98; drawers slide from the edge.
- A full easing library ships (`--ease-in/out-quad|cubic|quart|quint|expo|circ`) but components use
  the three above.
- Named animations: `spin-fast` (0.75s linear), `skeleton` (2s linear sweep), `caret-blink`
  (1.2s ease-out).
- Every transition is dropped under `prefers-reduced-motion`.

**No bounce, no spring, no stagger.** Nothing overshoots.

### Interaction states
- **Hover** = background only, mixed 4–10% toward the foreground. Text colour never changes on hover
  (except muted links resolving to `--foreground`).
- **Press** = **scale down**, not darken: 0.98 (sm) / 0.97 (md) / 0.96 (lg). The background stays at
  its hover value. This is HeroUI's signature interaction feel.
- **Focus** = 2px `--focus` (= accent) outline at 2px offset. Only on `:focus-visible`.
- **Disabled** = `opacity: 0.5` + `cursor: not-allowed`. No greyscale filter, no colour swap.
- **Pending** = the leading icon becomes a spinner; the control keeps its size.

### Layout rules
- Docs content maxes at **1280px** with a 230px left rail and a 190px right rail.
- Prose lines cap around **68ch**.
- The top bar is sticky and translucent; the sidebar scrolls independently with a thin scrollbar.
- Drawers are **inset 8px from the viewport edge with a 24px radius** — they float rather than butt
  against the screen.
- Scrollbars use standard CSS properties only (`scrollbar-width`, `scrollbar-color`), themed via
  `--scrollbar-*` and switchable with `data-scrollbar="thin|default|none"`. **Never `::-webkit-`
  overrides.**
- Overflow is signalled with **ScrollShadow's mask fade**, not a border or a hard cut.

---

## 4. Iconography

**HeroUI ships no icon package.** There is no icon font, no sprite sheet, and no `@heroui/icons` —
components that need a glyph (Select's chevron, Checkbox's tick, Accordion's indicator) draw a small
inline SVG, and everything else expects **you** to pass an icon node in.

Because the docs site's own glyph files could not be copied in this build, this system bundles a
**Lucide** subset as `components/content/Icon.jsx` — **this is a flagged substitution.** Lucide is
the closest match to what the docs render: 24×24 grid, 2px round-cap stroke, outline-only, no fills.

**Rules for icons in HeroUI designs:**
- **Stroke outline only.** No filled or duotone glyph sets.
- Inside a Button the stylesheet sizes them for you: **20px, dropping to 16px at ≥640px**, with
  `-2px` horizontal margin so the optical padding stays right. Don't override.
- Standalone UI icons are **16px** (18px for Alert leading glyphs).
- Icons inherit `currentColor` — never hard-code an icon colour.
- Spinner and link-icon SVGs are excluded from the button sizing rule via
  `[data-slot="spinner"]` / `[data-slot="link-icon"]`; keep those slots if you build custom controls.
- **Unicode, not icons, for keycaps:** ⌘ ⇧ ⌥ ⌃ ↵ ⇥ ⌫ ↑ ↓ ← →.
- **No emoji, ever.**

If you have the real HeroUI glyph set, drop the SVGs into `assets/icons/` and replace the `PATHS`
map in `Icon.jsx`.

---

## 5. Brand assets

**No logo file was available.** The sources reachable in this build contained no SVG or PNG mark,
and drawing one from memory would be a fabrication. Wherever a mark belongs, this system sets the
word **HeroUI** in the core face at **700 weight, -0.04em tracking** — see
`guidelines/brand-wordmark.card.html`.

**Please drop `assets/logo.svg` into this project** and I'll wire it into the thumbnail, the docs
kit header and the wordmark card.

Known asset URLs, not fetched (binaries can't be downloaded in this environment):
- Repo OG image: `repository-images.githubusercontent.com/360522116/…`
- Per-page OG cards: `heroui.com/og/react/components/<name>/image.png`
- Docs theme thumbnails: `heroui.com/_next/static/media/{default,sky,lavender,mint,netflix,black,spotify,coinbase,airbnb,discord,rabbit}.<hash>.png`

---

## 6. Index

### Root
| File | Purpose |
| --- | --- |
| `styles.css` | The single entry point consumers link. `@import` list only. |
| `readme.md` | This guide. |
| `SKILL.md` | Agent Skills manifest, for use in Claude Code. |
| `github.md` | Source-repo association and sync record. |
| `research-notes.md` | Verbatim transcriptions from the repo + what was **not** read. |
| `thumbnail.html` | Project tile. |

### `tokens/`
`primitives.css` (base values) · `colors.css` (light, `.dark`, vibrant palette) ·
`typography.css` (scale + `.typography` classes) · `spacing.css` · `radius.css` · `shadows.css` ·
`motion.css` · `base.css` (resets)

### `guidelines/` — 18 specimen cards
Colours (core, surfaces, semantic, soft pairs, field, borders, dark, primitives) ·
Type (headings, body, mono & weights) · Spacing (scale, radius, elevation, motion, states) ·
Brand (wordmark, theme swaps)

### `ui_kits/`
| Kit | What it is |
| --- | --- |
| `docs/` | Interactive recreation of the HeroUI documentation site — sidebar, component page, ⌘K palette, light/dark toggle. |

### Components — 67 exports across 11 groups

**buttons/** Button · ButtonGroup · CloseButton · ToggleButton · ToggleButtonGroup

**forms/** Form · Fieldset · Label · Description · FieldError · Input · TextField · TextArea ·
InputGroup · InputOTP · NumberField · SearchField · Checkbox · CheckboxGroup · RadioGroup ·
Switch · Slider

**selection/** Select · ComboBox · ListBox · Dropdown · TagGroup

**layout/** Card · Surface · Separator · Toolbar · ScrollShadow

**content/** Typography · Avatar · Badge · Chip · Kbd · Icon

**navigation/** Tabs · Accordion · Disclosure · Breadcrumbs · Link · Pagination

**feedback/** Alert · Toast · Spinner · ProgressBar · ProgressCircle · Meter · Skeleton

**overlays/** Modal · AlertDialog · Drawer · Popover · Tooltip

**data/** Table

**datetime/** Calendar · DateField · TimeField · DatePicker

**color/** ColorSwatch · ColorSwatchPicker · ColorArea · ColorSlider · ColorField · ColorPicker

Each directory holds `<Name>.jsx`, `<Name>.d.ts` (props contract), `<Name>.prompt.md` (when and how
to use it) and one `@dsCard` specimen HTML.

#### Intentional additions
- **`Icon`** — HeroUI defines no icon component, but every button, alert and menu in the source
  needs glyphs. Bundling a documented Lucide subset is better than each consumer inventing one.
  Flagged as a substitution above.

#### Deliberately not built
Families the docs list but which are variations this system covers with an existing component,
noted so you don't think they were missed: **Autocomplete** (= `ComboBox` with free text),
**RangeCalendar** (= `Calendar` with `rangeEnd`), **DateRangePicker** (= two `DatePicker`s),
**DisclosureGroup** (= `Accordion`), **ErrorMessage** (= `FieldError`).

---

## 7. Caveats

1. **Fonts are a substitution.** HeroUI ships no font files and no font tokens — it inherits
   Tailwind's system stack. We load **Inter** (+ JetBrains Mono for code) to match how heroui.com
   renders. If HeroUI has a licensed brand face, send the files and I'll swap them in.
2. **No logo.** See §5.
3. **Icons are Lucide, not HeroUI's own.** See §4.
4. **h1 size is inferred.** The Typography specimen's h1 label was clipped in the source we read;
   36px/1.11 is Tailwind's `text-4xl`, which the rest of the scale matches exactly. Worth
   confirming against `packages/styles/components/typography.css`.
5. **Component CSS beyond `button.css` was reconstructed**, not transcribed. The tokens, radii,
   heights and state behaviour all come from source; the exact padding of, say, `.card__header` is
   our best reading of the rendered docs. `button.css` is verbatim and is the reference for how a
   real HeroUI component CSS file is shaped.
6. **HeroUI Pro was not recreated** — no accessible source.
