A focusable row of tags — filter sets, applied facets, token inputs.

```jsx
<TagGroup>
  <TagGroup.Tag isSelected>Design</TagGroup.Tag>
  <TagGroup.Tag onRemove={() => …}>Engineering</TagGroup.Tag>
</TagGroup>
```

Unlike `Chip` (display-only), tags are interactive.
