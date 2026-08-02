# HeroUI v3 — research notes (source of truth for this build)

Repo: https://github.com/heroui-inc/heroui — **default branch `v3`** (not `main`/`canary`).
Docs: https://heroui.com · v2 docs: https://v2.heroui.com · Storybook: https://storybook-v3.heroui.com
Figma kit (v3): https://www.figma.com/community/file/1546526812159103429/heroui-figma-kit-v3
llms.txt: https://heroui.com/llms.txt · Latest release read: v3.2.2 (Jul 7 2026), v3.2.3 in docs index.

## Repo layout (branch v3)
- `apps/docs` — Next.js docs site (MDX)
- `packages/` — component packages (`@heroui/react`, `@heroui/styles`, per-component pkgs)
- `packages/styles/themes/default/variables.css` — **all theme tokens** (READ, transcribed into tokens/)
- `packages/styles/themes/shared/theme.css` — `@theme inline` bridge: `--color-*`, `--radius-*`, `--ease-*` (NOT YET READ)
- `packages/styles/base/base.css` — resets (NOT YET READ)
- `packages/styles/components/<name>.css` — one BEM stylesheet per component (NOT YET READ)
- `prompts/`, `skills/`, `.claude/` — agent tooling
- Languages: MDX 47.8%, TypeScript 47.3%, CSS 3.4%

## Architecture facts (from docs/theming)
- Tailwind CSS v4 + React Aria Components. React 19. No `<Provider>` wrapper.
- Styling is **BEM classes + CSS variables**: `.button`, `.button--primary`, `.accordion__trigger`.
- Compound component API: `Card.Header`, `Card.Content`, `Select.Item`, `Tabs.ListContainer`…
- Theme applied via `<html class="light|dark" data-theme="light|dark">`; body gets `bg-background text-foreground`.
- Token naming: bare = background (`--accent`), `-foreground` = text on it (`--accent-foreground`).
- Three token tiers: Base (non-changing), Theme (light/dark), Calculated (`color-mix()` hover/soft/border levels).
- `--field-*` tokens style inputs/checkbox/radio/OTP independently of surfaces.
- Scrollbars: standard CSS props only, `data-scrollbar="thin|default|none"`. No ::-webkit overrides.
- `data-vibrant-palette="true"` swaps soft foregrounds to 92% color / 8% foreground.
- Border radius tokens use `min()` capping (added v3.0.4).
- Named preset themes exist in docs theme switcher: default, sky, lavender, mint, netflix, black, spotify, coinbase, airbnb, discord, rabbit.

## Component inventory (heroui.com/llms.txt, React/web — THE list to build)
Buttons: Button, ButtonGroup, CloseButton, ToggleButton, ToggleButtonGroup
Forms: Form, Fieldset, Label, Description, ErrorMessage, FieldError, Input, TextField, TextArea,
  InputGroup, InputOTP, NumberField, SearchField, Checkbox, CheckboxGroup, RadioGroup, Switch, Slider
Selection: Select, ComboBox, Autocomplete, ListBox, Dropdown, TagGroup
Date & time: Calendar, RangeCalendar, DateField, DatePicker, DateRangePicker, TimeField
Color: ColorPicker, ColorArea, ColorSlider, ColorField, ColorSwatch, ColorSwatchPicker
Feedback: Alert, Toast, Meter, ProgressBar, ProgressCircle, Skeleton, Spinner
Layout/containers: Card, Surface, Separator, Toolbar, ScrollShadow
Navigation: Accordion, Disclosure, DisclosureGroup, Breadcrumbs, Link, Pagination, Tabs
Overlays: Modal, AlertDialog, Drawer, Popover, Tooltip
Data: Table
Content: Avatar, Badge, Chip, Kbd, Typography
(75+ web components; 37 React Native components exist in heroui-native — out of scope here.)

Renames from v2 → v3 worth knowing: Divider→Separator, Listbox→ListBox, CircularProgress→ProgressCircle,
Progress→ProgressBar, DateInput→DateField, TimeInput→TimeField, NumberInput→NumberField, Text→Typography.
v2 had Navbar, Image, Snippet, Spacer, User, Code — v3 has no direct equivalents (see migration docs).

## Still to read
- themes/shared/theme.css (fonts, radius scale, easings) — **needed for typography.css + motion.css**
- base/base.css
- components/button.css, card.css, input.css (exact paddings/heights/radii)
- apps/docs — for the docs-site UI kit
- assets: logo (heroui-assets.nyc3.cdn.digitaloceanspaces.com/docs/heroui-og_2x.jpg is the OG image)

## Button spec (packages/styles/components/button.css, v3 — READ VERBATIM)
Base `.button`: relative isolate inline-flex, h-10 (md:h-9), w-fit, origin-center,
items-center justify-center, gap-2, **rounded-3xl (24px)**, px-4, text-sm, font-medium,
whitespace-nowrap, outline-none, select-none, transform-gpu.
Transition: `transform 250ms var(--ease-smooth), background-color 100ms var(--ease-out), box-shadow 100ms var(--ease-out)`; motion-reduce:transition-none AFTER.
Cursor: var(--cursor-interactive).
Local vars: --button-bg (transparent), --button-bg-hover (=bg), --button-bg-pressed (=bg-hover), --button-fg (currentColor).
Icons inside: `-mx-0.5 my-0.5 size-5 shrink-0 self-center sm:my-1 sm:size-4` (excludes spinner + link-icon slots).
Sizes: sm = h-9 px-3 (md:h-8), icon size-4, press scale(0.98) · md = default · lg = h-11 text-base (md:h-10), press scale(0.96). Base press = **scale(0.97)**.
Variants (token-only):
  primary   bg accent / hover accent-hover / fg accent-foreground
  secondary bg default / hover default-hover / fg accent-soft-foreground
  tertiary  bg default / hover default-hover / fg inherited
  ghost     transparent / hover default / fg default-foreground
  outline   ghost + `border border-border`, hover color-mix(in srgb, default 60%, transparent)
  danger    bg danger / hover danger-hover / fg danger-foreground
  danger-soft bg danger-soft / hover danger-soft-hover / fg danger-soft-foreground
Icon-only: w-10 (md:w-9); sm w-9 (md:w-8); lg w-11 (md:w-10); p-0. Full width: w-full.
State hooks: `.status-focused` (:focus-visible / data-focus-visible), `.status-disabled`
(:disabled / aria-disabled), `.status-pending` (data-pending). data-hovered / data-pressed
attributes come from React Aria.

## Type scale (heroui.com Typography specimen — READ)
h1 36px/600/1.11/tight  ← size INFERRED from Tailwind text-4xl, label was clipped; verify
h2 30px/600/1.17/tight · h3 24px/600/1.25/tight · h4 20px/600/1.33/tight
h5 18px/600/1.39/tight · h6 16px/600/1.50/tight
body 16px/400/1.75 · body-sm 14px/400/1.50 (UI default) · body-xs 12px/400/1.25 · code 14px mono
Classes: .typography, .typography-prose, .typography--h1…h6, --body/--body-sm/--body-xs, --code,
--align-{start,center,end,justify}, --color-{default,muted}, --truncate,
--weight-{normal,medium,semibold,bold}.
Props: type, align, color, weight, truncate, render, children. Compounds: Typography.Heading(level 1-6),
Typography.Paragraph(size base|sm|xs), Typography.Code, Typography.Prose.

## FONTS — no font files exist in the repo
@heroui/styles ships no @font-face and no --font-* tokens; it inherits Tailwind v4's default
system stack. heroui.com's own surface reads as Inter. We load **Inter** (Google Fonts) + JetBrains
Mono for code. FLAG THIS SUBSTITUTION to the user and ask for real font files if they have them.

## Radius / motion — confirmed from shared/theme.css (transcribed into tokens/)
Button = rounded-3xl = 24px. --radius base 0.5rem; fields = radius*1.5 = 12px.
No Framer Motion in v3 — native CSS transitions + keyframes only.

## Base variables (CONFIRMED, variables.css base section)
--white oklch(100% 0 0) · --black oklch(0% 0 0) · --snow oklch(0.9911 0 0) · --eclipse oklch(0.2103 0.0059 285.89)
--spacing 0.25rem · **--border-width 0px (no borders by default)** · --field-border-width var(--border-width)
--radius 0.5rem · --field-radius calc(radius*1.5)=12px · --ring-offset-width 2px
--disabled-opacity 0.5 · --cursor-interactive pointer · --cursor-disabled not-allowed

## Assets — BLOCKER-ish
No GitHub file tools available this run (connect flow not completed), and web_fetch cannot
download binaries. **No HeroUI logo file was copied.** Per instructions the wordmark is set in
type wherever a mark would go. Ask the user to drop `assets/logo.svg` in.
Known asset URLs (not fetched): repository-images.githubusercontent.com/360522116/... (repo OG),
heroui.com/og/react/components/<name>/image.png (per-page OG cards).
Docs theme thumbnails: heroui.com/_next/static/media/{default,sky,lavender,mint,netflix,black,spotify,coinbase,airbnb,discord,rabbit}.<hash>.png
