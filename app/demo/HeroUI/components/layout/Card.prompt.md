The workhorse container — white surface, 16px radius, soft three-layer shadow, no border.

```jsx
<Card>
  <Card.Header>
    <Card.Title>Usage this month</Card.Title>
    <Card.Description>Resets on the 1st.</Card.Description>
  </Card.Header>
  <Card.Content>…</Card.Content>
  <Card.Footer><Button size="sm">Upgrade</Button></Card.Footer>
</Card>
```

Variants `transparent | default | secondary | tertiary` step the fill through the surface ramp. For a clickable card wrap the content in a real `<button>` rather than putting `onClick` on the Card.
