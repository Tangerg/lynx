Confirm before something irreversible. Narrow modal, danger-soft glyph badge, danger confirm button.

```jsx
<AlertDialog isOpen={confirming} title="Delete project?"
  description="This removes all deployments and cannot be undone."
  onCancel={() => setConfirming(false)} onConfirm={destroy} />
```
