Puts icons, prefixes or a small action inside the field box rather than beside it.

```jsx
<InputGroup startContent={<Icon name="search" />} endContent={<Kbd keys={['cmd','k']} />} placeholder="Search docs" />
```

The whole group takes the focus ring, so it still reads as one control.
