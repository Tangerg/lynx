Stroke icon for any HeroUI surface — Lucide glyphs at 24px grid / 2px round stroke, inheriting `currentColor`.

```jsx
<Button variant="primary"><Icon name="plus" />New project</Button>
<Icon name="chevron-down" size={20} />
```

Notes
- `iconNames` lists every bundled glyph.
- Inside `.button` the stylesheet already sizes SVGs (20px, 16px from sm breakpoint) — don't pass `size` there.
- Intentional addition: HeroUI ships no icon package, and its docs glyphs could not be copied. Lucide is the flagged substitute.
