Side or bottom sheet for secondary flows — inset from the edge with a 24px radius, so it floats rather than butting against the viewport.

```jsx
<Drawer isOpen={open} onClose={close}>
  <Drawer.Header><h2 className="modal__title">Filters</h2></Drawer.Header>
  <Drawer.Content>…</Drawer.Content>
</Drawer>
```
