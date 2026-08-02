Label, control and helper/error text wired together with matching ids.

```jsx
<TextField label="Work email" description="Used for billing receipts." placeholder="you@company.com" />
<TextField label="Password" type="password" errorMessage="Must be 8+ characters" />
```

Pass `children` to swap in a `TextArea`, `InputGroup` or `Select` and keep the same label/error frame.
