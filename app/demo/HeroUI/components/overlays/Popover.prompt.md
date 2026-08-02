Interactive floating panel — filters, pickers, mini-forms. Overlay fill, 16px radius, 16px padding.

```jsx
<Popover trigger={<Button variant="outline" size="sm">Filter</Button>}>
  <CheckboxGroup label="Status"><Checkbox>Open</Checkbox><Checkbox>Closed</Checkbox></CheckboxGroup>
</Popover>
```

Use `Tooltip` for a short non-interactive hint instead.
