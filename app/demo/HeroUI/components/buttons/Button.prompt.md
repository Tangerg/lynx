The action control for everything clickable — pill-shaped, 40px tall, seven token-driven variants.

```jsx
<Button variant="primary" startIcon={<Icon name="plus" />}>New project</Button>
<Button variant="outline" size="sm">Cancel</Button>
<Button variant="danger-soft" isPending>Deleting</Button>
```

Variants: `primary` (accent) · `secondary` (default bg, accent-tinted text) · `tertiary` · `ghost` · `outline` · `danger` · `danger-soft`.
Sizes `sm | md | lg` — heights 36/40/44px, each shrinking one step at ≥768px. Press scales 0.98/0.97/0.96.
Pair `isIconOnly` with an `aria-label`.

Source note: the shipped `button.css` drives every variant through four component-local custom
properties — `--button-bg`, `--button-bg-hover`, `--button-bg-pressed`, `--button-fg` — set by the
variant class and read by `.button`. Adding a custom variant upstream is three lines:

```css
.button--brand { --button-bg: var(--brand); --button-bg-hover: var(--brand-hover); --button-fg: var(--brand-foreground); }
```

This system writes the resolved values directly instead, so the token index stays clean; the
colours, states and geometry are identical.
