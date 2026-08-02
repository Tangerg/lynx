Scroll container that fades whichever edges still have content — the HeroUI way to signal overflow instead of a hard border.

```jsx
<ScrollShadow style={{maxHeight: 240}}>{rows}</ScrollShadow>
```

Sets `data-top-scroll` / `data-bottom-scroll` / `data-top-bottom-scroll` as you scroll.
