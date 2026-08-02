Action menu on a trigger. Destructive rows take `danger`; shortcuts sit right-aligned and muted.

```jsx
<Dropdown align="end" trigger={<Button variant="ghost" isIconOnly aria-label="More"><Icon name="menu" /></Button>}>
  <Dropdown.Item shortcut="⌘D">Duplicate</Dropdown.Item>
  <Dropdown.Separator />
  <Dropdown.Item danger>Delete</Dropdown.Item>
</Dropdown>
```
