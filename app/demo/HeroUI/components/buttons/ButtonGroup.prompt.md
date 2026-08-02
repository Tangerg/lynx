Joins related Buttons into a single segmented run (split actions, view switchers).

```jsx
<ButtonGroup>
  <Button variant="tertiary">Day</Button>
  <Button variant="tertiary">Week</Button>
  <Button variant="tertiary">Month</Button>
</ButtonGroup>
```

Inner corners square off automatically and the press-scale is suppressed so the run doesn't jitter. Pass `gap` to keep pills separate instead.
