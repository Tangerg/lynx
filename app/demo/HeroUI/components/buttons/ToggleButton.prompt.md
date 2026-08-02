A Button that latches on — formatting toggles, filter pills, view flags.

```jsx
<ToggleButton isSelected={bold} onClick={() => setBold(!bold)} isIconOnly aria-label="Bold">
  <Icon name="code" />
</ToggleButton>
```

Unselected it reads as `ghost`; selected it inverts to `--foreground` on `--background`. Inside a `ToggleButtonGroup` the selected chip becomes a raised `--segment` instead.
