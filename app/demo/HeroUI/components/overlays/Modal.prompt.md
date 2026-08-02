Focused task on a blurred scrim — 24px radius, overlay shadow, rises 8px on open.

```jsx
<Modal isOpen={open} onClose={close}>
  <Modal.Header title="Invite teammates" description="They'll get an email invite." onClose={close} />
  <Modal.Content><TextField label="Email" /></Modal.Content>
  <Modal.Footer><Button variant="ghost" onClick={close}>Cancel</Button><Button variant="primary">Send</Button></Modal.Footer>
</Modal>
```
