Status and metadata label — fully rounded, 24px tall, soft semantic fills.

```jsx
<Chip color="success" dot>Live</Chip>
<Chip variant="outline" size="sm">beta</Chip>
<Chip color="accent" onRemove={() => …}>design</Chip>
```

Colors resolve to the `*-soft` pair, so text stays legible on light accent themes. Size and color classes combine (`.chip--sm.chip--success`).
