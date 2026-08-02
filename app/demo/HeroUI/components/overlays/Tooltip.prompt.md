Short hint on hover/focus — inverted (`--background-inverse`), 12px text, no wrapping.

```jsx
<Tooltip content="Duplicate" shortcut="⌘D">
  <Button variant="ghost" isIconOnly aria-label="Duplicate"><Icon name="copy" /></Button>
</Tooltip>
```

Never put interactive content in a Tooltip — that's a `Popover`.
